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
	if cfg.SparkDB.Host != "127.0.0.1" {
		t.Fatalf("expected sparkdb host 127.0.0.1, got %s", cfg.SparkDB.Host)
	}
	if cfg.SparkDB.Port != 9600 {
		t.Fatalf("expected sparkdb port 9600, got %d", cfg.SparkDB.Port)
	}
	if cfg.SparkDB.Database != "deviceos" {
		t.Fatalf("expected database deviceos, got %s", cfg.SparkDB.Database)
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
sparkdb:
  host: "127.0.0.1"
  port: 9600
  database: "deviceos"
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
	if cfg.SparkDB.Port != 9600 {
		t.Fatalf("expected sparkdb port 9600, got %d", cfg.SparkDB.Port)
	}
	if cfg.SparkDB.Database != "deviceos" {
		t.Fatalf("expected deviceos, got %s", cfg.SparkDB.Database)
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

func TestLoadPartiallyInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.yaml")
	yaml := `
server:
  host: "0.0.0.0"
  port: "not-a-number"
`
	os.WriteFile(path, []byte(yaml), 0644)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for port type mismatch")
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

func TestEnvOverrideSparkDBBin(t *testing.T) {
	os.Setenv("DEVICEOS_SPARKDB_BIN_PATH", "/custom/path/sparkdb")
	defer os.Unsetenv("DEVICEOS_SPARKDB_BIN_PATH")

	dir := t.TempDir()
	path := filepath.Join(dir, "env.yaml")
	os.WriteFile(path, []byte(""), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SparkDB.BinPath != "/custom/path/sparkdb" {
		t.Fatalf("expected /custom/path/sparkdb, got %s", cfg.SparkDB.BinPath)
	}
}

func TestEnvOverrideSPARKDB_BIN_BackwardCompat(t *testing.T) {
	os.Setenv("SPARKDB_BIN", "/old/path/sparkdb")
	defer os.Unsetenv("SPARKDB_BIN")

	dir := t.TempDir()
	path := filepath.Join(dir, "env.yaml")
	os.WriteFile(path, []byte(""), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SparkDB.BinPath != "/old/path/sparkdb" {
		t.Fatalf("expected /old/path/sparkdb, got %s", cfg.SparkDB.BinPath)
	}
}

func TestEnvOverrideTakesPrecedenceOverYAML(t *testing.T) {
	os.Setenv("DEVICEOS_SERVER_PORT", "7070")
	defer os.Unsetenv("DEVICEOS_SERVER_PORT")

	dir := t.TempDir()
	path := filepath.Join(dir, "override.yaml")
	yaml := `server:
  port: 8080
`
	os.WriteFile(path, []byte(yaml), 0644)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 7070 {
		t.Fatalf("expected 7070 from env override, got %d", cfg.Server.Port)
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
