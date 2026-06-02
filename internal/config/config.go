package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Storage StorageConfig `yaml:"storage"`
	Modules ModulesConfig `yaml:"modules"`
}

type ServerConfig struct {
	Host           string   `yaml:"host"`
	Port           int      `yaml:"port"`
	TLSKey         string   `yaml:"tls_key"`
	TLSCert        string   `yaml:"tls_cert"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	RateLimitRPM   int      `yaml:"rate_limit_rpm"`
	LogLevel       string   `yaml:"log_level"`
}

type StorageConfig struct {
	Path         string `yaml:"path"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
}

type MQTTConfig struct {
	Port int `yaml:"port"`
}

type ModulesConfig struct {
	JWTSecret              string     `yaml:"jwt_secret"`
	AdminAPIKey            string     `yaml:"admin_api_key"`
	TelemetryTTL           string     `yaml:"telemetry_ttl"`
	TelemetryPruneInterval string     `yaml:"telemetry_prune_interval"`
	MQTT                   MQTTConfig `yaml:"mqtt"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:     "0.0.0.0",
			Port:     8080,
			LogLevel: "info",
		},
		Storage: StorageConfig{
			Path: "./data/deviceos.db",
		},
		Modules: ModulesConfig{
			JWTSecret:              "dev-change-me-in-production",
			AdminAPIKey:            "",
			TelemetryTTL:           "720h",
			TelemetryPruneInterval: "1h",
			MQTT: MQTTConfig{
				Port: 1883,
			},
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}

	applyEnvOverrides(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func (cfg *Config) Validate() error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if cfg.Server.TLSCert != "" && cfg.Server.TLSKey == "" {
		return fmt.Errorf("server.tls_key is required when server.tls_cert is set")
	}
	if cfg.Server.TLSKey != "" && cfg.Server.TLSCert == "" {
		return fmt.Errorf("server.tls_cert is required when server.tls_key is set")
	}
	if cfg.Storage.Path == "" {
		return fmt.Errorf("storage.path must not be empty")
	}
	if cfg.Server.RateLimitRPM < 0 {
		return fmt.Errorf("server.rate_limit_rpm must not be negative")
	}
	if cfg.Modules.JWTSecret == "" || cfg.Modules.JWTSecret == "dev-change-me-in-production" {
		if os.Getenv("DEVICEOS_JWT_SECRET") == "" {
			// Soft warning, not error — allows dev defaults
		}
	}
	switch cfg.Server.LogLevel {
	case "debug", "info", "warn", "error", "":
	default:
		return fmt.Errorf("server.log_level must be one of: debug, info, warn, error")
	}
	return nil
}

func applyEnvOverrides(cfg *Config) {
	if v, ok := lookupEnv("DEVICEOS_SERVER_HOST"); ok {
		cfg.Server.Host = v
	}
	if v, ok := lookupEnv("DEVICEOS_SERVER_PORT"); ok {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v, ok := lookupEnv("DEVICEOS_TLS_CERT"); ok {
		cfg.Server.TLSCert = v
	}
	if v, ok := lookupEnv("DEVICEOS_TLS_KEY"); ok {
		cfg.Server.TLSKey = v
	}
	if v, ok := lookupEnv("DEVICEOS_ALLOWED_ORIGINS"); ok {
		cfg.Server.AllowedOrigins = strings.Split(v, ",")
	}
	if v, ok := lookupEnv("DEVICEOS_RATE_LIMIT_RPM"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Server.RateLimitRPM = n
		}
	}
	if v, ok := lookupEnv("DEVICEOS_LOG_LEVEL"); ok {
		cfg.Server.LogLevel = v
	}
	if v, ok := lookupEnv("DEVICEOS_STORAGE_PATH"); ok {
		cfg.Storage.Path = v
	}
	if v, ok := lookupEnv("DEVICEOS_STORAGE_MAX_OPEN_CONNS"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Storage.MaxOpenConns = n
		}
	}
	if v, ok := lookupEnv("DEVICEOS_STORAGE_MAX_IDLE_CONNS"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Storage.MaxIdleConns = n
		}
	}
	if v, ok := lookupEnv("DEVICEOS_JWT_SECRET"); ok {
		cfg.Modules.JWTSecret = v
	}
	if v, ok := lookupEnv("DEVICEOS_ADMIN_TOKEN"); ok {
		cfg.Modules.AdminAPIKey = v
	}
	if v, ok := lookupEnv("DEVICEOS_TELEMETRY_TTL"); ok {
		cfg.Modules.TelemetryTTL = v
	}
	if v, ok := lookupEnv("DEVICEOS_TELEMETRY_PRUNE_INTERVAL"); ok {
		cfg.Modules.TelemetryPruneInterval = v
	}
}

func lookupEnv(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	return strings.TrimSpace(v), ok
}
