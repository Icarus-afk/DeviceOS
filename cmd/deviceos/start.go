package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/lohtbrok/deviceos/internal/config"
	"github.com/lohtbrok/deviceos/internal/registry"
	"github.com/lohtbrok/deviceos/internal/server"
	"github.com/lohtbrok/deviceos/internal/sparkdb"
	"github.com/lohtbrok/deviceos/internal/version"
	"github.com/lohtbrok/deviceos/modules/alerts"
	"github.com/lohtbrok/deviceos/modules/audit"
	"github.com/lohtbrok/deviceos/modules/auth"
	"github.com/lohtbrok/deviceos/modules/commands"
	"github.com/lohtbrok/deviceos/modules/dashboard"
	"github.com/lohtbrok/deviceos/modules/devices"
	"github.com/lohtbrok/deviceos/modules/fleet"
	"github.com/lohtbrok/deviceos/modules/ota"
	"github.com/lohtbrok/deviceos/modules/telemetry"
	"github.com/lohtbrok/deviceos/modules/simulator"
	"github.com/lohtbrok/deviceos/modules/tenant"
	"github.com/lohtbrok/deviceos/modules/webhooks"
)

func cmdStart(cfgPath string) {
	fmt.Println(version.Banner())
	bi := version.ReadBuildInfo()
	slog.Info("starting deviceos", "version", bi.Version, "commit", bi.Commit)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	sparkSrv := sparkdb.NewServer(sparkdb.ServerConfig{
		BinPath:        cfg.SparkDB.BinPath,
		Host:           cfg.SparkDB.Host,
		Port:           cfg.SparkDB.Port,
		DataDir:        cfg.SparkDB.DataDir,
		Auth:           cfg.SparkDB.Auth,
		WALMode:        cfg.SparkDB.WALMode,
		MaxConnections: cfg.SparkDB.MaxConnections,
		ExtraConfig:    cfg.SparkDB.ExtraConfig,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.Info("starting SparkDB")
	if err := sparkSrv.Start(ctx); err != nil {
		slog.Error("failed to start SparkDB", "error", err)
		os.Exit(1)
	}

	db, err := sparkdb.Open(sparkdb.Config{
		Host:     cfg.SparkDB.Host,
		Port:     sparkSrv.Port,
		Database: cfg.SparkDB.Database,
		APIKey:   cfg.SparkDB.APIKey,
	})
	if err != nil {
		sparkSrv.Stop()
		slog.Error("failed to open SparkDB", "error", err)
		os.Exit(1)
	}

	r := registry.New()
	r.Register(devices.New(db))

	telemetryMod := telemetry.New(db)
	r.Register(telemetryMod)

	alertsMod := alerts.New(db)
	r.Register(alertsMod)

	telemetryMod.SetTelemetryHook(alertsMod.OnTelemetry)

	r.Register(auth.New(db, cfg.Modules.JWTSecret, cfg.Modules.AdminAPIKey))
	r.Register(commands.New(db))
	r.Register(ota.New(db))
	r.Register(webhooks.New(db))
	r.Register(fleet.New(db))
	r.Register(tenant.New(db))
	r.Register(audit.New(db))
	r.Register(simulator.New())
	r.Register(dashboard.New())

	srv := server.New(server.Config{
		Host: cfg.Server.Host,
		Port: cfg.Server.Port,
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

	sigCtx, sigStop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer sigStop()

	go func() {
		if err := srv.Start(); err != nil {
			slog.Error("server error", "error", err)
			cancel()
		}
	}()

	slog.Info("deviceos running", "version", bi.Version)
	<-sigCtx.Done()
	slog.Info("shutting down...")
	r.StopAll()
	db.Close()
	sparkSrv.Stop()
}
