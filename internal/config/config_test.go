package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lohtbrok/deviceos/internal/config"
)

func TestDefault(t *testing.T) {
	cfg := config.Default()
	if cfg.Server.Port != 8080 {
		t.Fatalf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Fatalf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Storage.Path != "./data/deviceos.db" {
		t.Fatalf("expected ./data/deviceos.db, got %s", cfg.Storage.Path)
	}
	if cfg.Server.LogLevel != "info" {
		t.Fatalf("expected log level info, got %s", cfg.Server.LogLevel)
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8080 {
		t.Fatal("expected default port")
	}
}

func TestLoadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deviceos.yaml")
	yaml := `
server:
  host: "127.0.0.1"
  port: 9090
  log_level: "debug"
storage:
  path: "./data/custom.db"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Fatalf("expected 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("expected 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.LogLevel != "debug" {
		t.Fatalf("expected debug, got %s", cfg.Server.LogLevel)
	}
	if cfg.Storage.Path != "./data/custom.db" {
		t.Fatalf("expected ./data/custom.db, got %s", cfg.Storage.Path)
	}
}

func TestValidationBadPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_port.yaml")
	yaml := `server:
  port: 99999
storage:
  path: "./data/db.db"
`
	os.WriteFile(path, []byte(yaml), 0644)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected validation error for bad port")
	}
}

func TestValidationTLSMissingKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_tls.yaml")
	yaml := `server:
  tls_cert: "/path/to/cert.pem"
storage:
  path: "./data/db.db"
`
	os.WriteFile(path, []byte(yaml), 0644)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected validation error for missing tls_key")
	}
}

func TestValidationBadLogLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_loglevel.yaml")
	yaml := `server:
  log_level: "trace"
storage:
  path: "./data/db.db"
`
	os.WriteFile(path, []byte(yaml), 0644)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected validation error for bad log level")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	os.WriteFile(path, []byte(": : invalid yaml {{"), 0644)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}

func TestLoadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	os.WriteFile(path, []byte(""), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8080 {
		t.Fatal("expected default port")
	}
}

func TestEnvOverrideServerPort(t *testing.T) {
	os.Setenv("DEVICEOS_SERVER_PORT", "9090")
	defer os.Unsetenv("DEVICEOS_SERVER_PORT")

	dir := t.TempDir()
	path := filepath.Join(dir, "env.yaml")
	os.WriteFile(path, []byte(""), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("expected 9090 from env, got %d", cfg.Server.Port)
	}
}

func TestEnvOverrideStoragePath(t *testing.T) {
	os.Setenv("DEVICEOS_STORAGE_PATH", "/custom/path/data.db")
	defer os.Unsetenv("DEVICEOS_STORAGE_PATH")

	dir := t.TempDir()
	path := filepath.Join(dir, "env.yaml")
	os.WriteFile(path, []byte(""), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.Path != "/custom/path/data.db" {
		t.Fatalf("expected /custom/path/data.db, got %s", cfg.Storage.Path)
	}
}

func TestEnvOverrideLogLevel(t *testing.T) {
	os.Setenv("DEVICEOS_LOG_LEVEL", "debug")
	defer os.Unsetenv("DEVICEOS_LOG_LEVEL")

	dir := t.TempDir()
	path := filepath.Join(dir, "env.yaml")
	os.WriteFile(path, []byte(""), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.LogLevel != "debug" {
		t.Fatalf("expected debug, got %s", cfg.Server.LogLevel)
	}
}

func TestEnvOverrideRateLimit(t *testing.T) {
	os.Setenv("DEVICEOS_RATE_LIMIT_RPM", "100")
	defer os.Unsetenv("DEVICEOS_RATE_LIMIT_RPM")

	dir := t.TempDir()
	path := filepath.Join(dir, "env.yaml")
	os.WriteFile(path, []byte(""), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.RateLimitRPM != 100 {
		t.Fatalf("expected 100, got %d", cfg.Server.RateLimitRPM)
	}
}

func TestEnvOverrideJWTSecret(t *testing.T) {
	os.Setenv("DEVICEOS_JWT_SECRET", "my-super-secret-key")
	defer os.Unsetenv("DEVICEOS_JWT_SECRET")

	dir := t.TempDir()
	path := filepath.Join(dir, "env.yaml")
	os.WriteFile(path, []byte(""), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Modules.JWTSecret != "my-super-secret-key" {
		t.Fatalf("expected my-super-secret-key, got %s", cfg.Modules.JWTSecret)
	}
}

func TestEnvOverrideAdminToken(t *testing.T) {
	os.Setenv("DEVICEOS_ADMIN_TOKEN", "dos_mytoken_1234")
	defer os.Unsetenv("DEVICEOS_ADMIN_TOKEN")

	dir := t.TempDir()
	path := filepath.Join(dir, "env.yaml")
	os.WriteFile(path, []byte(""), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Modules.AdminAPIKey != "dos_mytoken_1234" {
		t.Fatalf("expected dos_mytoken_1234, got %s", cfg.Modules.AdminAPIKey)
	}
}

func TestEnvOverrideTelemetryTTL(t *testing.T) {
	os.Setenv("DEVICEOS_TELEMETRY_TTL", "168h")
	defer os.Unsetenv("DEVICEOS_TELEMETRY_TTL")

	dir := t.TempDir()
	path := filepath.Join(dir, "env.yaml")
	os.WriteFile(path, []byte(""), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Modules.TelemetryTTL != "168h" {
		t.Fatalf("expected 168h, got %s", cfg.Modules.TelemetryTTL)
	}
}
