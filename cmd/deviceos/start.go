package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lohtbrok/deviceos/internal/config"
	"github.com/lohtbrok/deviceos/internal/db"
	"github.com/lohtbrok/deviceos/internal/registry"
	"github.com/lohtbrok/deviceos/internal/server"
	"github.com/lohtbrok/deviceos/internal/version"
	"github.com/lohtbrok/deviceos/modules/alerts"
	"github.com/lohtbrok/deviceos/modules/audit"
	"github.com/lohtbrok/deviceos/modules/auth"
	"github.com/lohtbrok/deviceos/modules/commands"
	"github.com/lohtbrok/deviceos/modules/dashboard"
	"github.com/lohtbrok/deviceos/modules/devices"
	"github.com/lohtbrok/deviceos/modules/events"
	"github.com/lohtbrok/deviceos/modules/fleet"
	"github.com/lohtbrok/deviceos/modules/mqtt"
	"github.com/lohtbrok/deviceos/modules/ota"
	"github.com/lohtbrok/deviceos/modules/simulator"
	"github.com/lohtbrok/deviceos/modules/telemetry"
	"github.com/lohtbrok/deviceos/modules/tenant"
	"github.com/lohtbrok/deviceos/modules/webhooks"
)

func cmdStart(cfgPath string) {
	fmt.Println(version.Banner())
	bi := version.ReadBuildInfo()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	setLogLevel(cfg.Server.LogLevel)

	slog.Info("starting deviceos", "version", bi.Version, "commit", bi.Commit)

	if err := os.MkdirAll("data", 0755); err != nil {
		slog.Error("failed to create data directory", "error", err)
		os.Exit(1)
	}

	database, err := db.Open(db.Config{
		Path:         cfg.Storage.Path,
		MaxOpenConns: cfg.Storage.MaxOpenConns,
		MaxIdleConns: cfg.Storage.MaxIdleConns,
	})
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}

	r := registry.New()
	r.Register(devices.New(database))

	telemetryTTL, _ := time.ParseDuration(cfg.Modules.TelemetryTTL)
	telemetryPruneInterval, _ := time.ParseDuration(cfg.Modules.TelemetryPruneInterval)
	telemetryMod := telemetry.New(database, telemetryTTL, telemetryPruneInterval)
	r.Register(telemetryMod)

	mqttMod := mqtt.New(database, telemetryMod, mqtt.Config{
		Port: cfg.Modules.MQTT.Port,
	})
	r.Register(mqttMod)

	eventsMod := events.New()
	r.Register(eventsMod)

	alertsMod := alerts.New(database)
	r.Register(alertsMod)

	eventsHub := eventsMod.Hub()
	telemetryMod.AddTelemetryHook(alertsMod.OnTelemetry)
	telemetryMod.AddTelemetryHook(func(deviceID string, metrics, metadata json.RawMessage) {
		eventsHub.Publish(events.Event{
			Type: "telemetry",
			Data: map[string]any{
				"device_id": deviceID,
				"metrics":   metrics,
				"metadata":  metadata,
			},
		})
	})

	authMod := auth.New(database, cfg.Modules.JWTSecret, cfg.Modules.AdminAPIKey)
	r.Register(authMod)
	r.Register(commands.New(database))
	r.Register(ota.New(database))
	r.Register(webhooks.New(database))
	r.Register(fleet.New(database))
	r.Register(tenant.New(database))
	r.Register(audit.New(database))
	r.Register(simulator.New())
	r.Register(dashboard.New())

	srv := server.New(server.Config{
		Host:           cfg.Server.Host,
		Port:           cfg.Server.Port,
		TLSKey:         cfg.Server.TLSKey,
		TLSCert:        cfg.Server.TLSCert,
		AllowedOrigins: cfg.Server.AllowedOrigins,
		RateLimitRPM:   cfg.Server.RateLimitRPM,
		AuthMiddleware: authMod.Middleware,
		ModuleStats: func() map[string]string {
			mods := make(map[string]string)
			for _, name := range r.Names() {
				mods[name] = "ok"
			}
			return mods
		},
	})

	if err := r.InitAll(cfg); err != nil {
		slog.Error("module init failed", "error", err)
		os.Exit(1)
	}

	if err := r.RegisterAllRoutes(srv.Mux()); err != nil {
		slog.Error("route registration failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			slog.Error("server error", "error", err)
			cancel()
		}
	}()

	slog.Info("deviceos running", "version", bi.Version, "addr", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port))

	select {
	case <-sigCh:
		slog.Info("shutting down...")
	case <-ctx.Done():
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	r.StopAll()
	database.Close()
	srv.Stop(shutdownCtx)
}

func setLogLevel(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "info":
		l = slog.LevelInfo
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}
