package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	SparkDB  SparkDBConfig  `yaml:"sparkdb"`
	Modules  ModulesConfig  `yaml:"modules"`
}

type ServerConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	TLSKey  string `yaml:"tls_key"`
	TLSCert string `yaml:"tls_cert"`
}

type SparkDBConfig struct {
	BinPath        string         `yaml:"bin_path"`
	Host           string         `yaml:"host"`
	Port           int            `yaml:"port"`
	Database       string         `yaml:"database"`
	DataDir        string         `yaml:"data_dir"`
	APIKey         string         `yaml:"api_key"`
	Auth           bool           `yaml:"auth"`
	WALMode        *bool          `yaml:"wal_mode"`
	MaxConnections *int           `yaml:"max_connections"`
	ExtraConfig    map[string]any `yaml:"extra_config"`
}

type ModulesConfig struct {
	JWTSecret   string `yaml:"jwt_secret"`
	AdminAPIKey string `yaml:"admin_api_key"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		SparkDB: SparkDBConfig{
			Host:     "127.0.0.1",
			Port:     9600,
			Database: "deviceos",
		},
		Modules: ModulesConfig{
			JWTSecret:   "dev-change-me-in-production",
			AdminAPIKey: "",
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

	return cfg, nil
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
	if v, ok := lookupEnv("DEVICEOS_SPARKDB_BIN_PATH"); ok {
		cfg.SparkDB.BinPath = v
	}
	if v, ok := lookupEnv("SPARKDB_BIN"); ok {
		cfg.SparkDB.BinPath = v
	}
	if v, ok := lookupEnv("DEVICEOS_SPARKDB_HOST"); ok {
		cfg.SparkDB.Host = v
	}
	if v, ok := lookupEnv("DEVICEOS_SPARKDB_PORT"); ok {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.SparkDB.Port = p
		}
	}
	if v, ok := lookupEnv("DEVICEOS_SPARKDB_DATABASE"); ok {
		cfg.SparkDB.Database = v
	}
	if v, ok := lookupEnv("DEVICEOS_SPARKDB_DATA_DIR"); ok {
		cfg.SparkDB.DataDir = v
	}
	if v, ok := lookupEnv("DEVICEOS_SPARKDB_API_KEY"); ok {
		cfg.SparkDB.APIKey = v
	}
	if v, ok := lookupEnv("DEVICEOS_SPARKDB_AUTH"); ok {
		cfg.SparkDB.Auth = v == "true" || v == "1" || v == "yes"
	}
	if v, ok := lookupEnv("DEVICEOS_SPARKDB_WAL_MODE"); ok {
		b := v == "true" || v == "1" || v == "yes"
		cfg.SparkDB.WALMode = &b
	}
	if v, ok := lookupEnv("DEVICEOS_SPARKDB_MAX_CONNECTIONS"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.SparkDB.MaxConnections = &n
		}
	}
	if v, ok := lookupEnv("DEVICEOS_JWT_SECRET"); ok {
		cfg.Modules.JWTSecret = v
	}
	if v, ok := lookupEnv("DEVICEOS_ADMIN_TOKEN"); ok {
		cfg.Modules.AdminAPIKey = v
	}
}

func lookupEnv(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	return strings.TrimSpace(v), ok
}
