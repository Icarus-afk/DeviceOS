package sparkdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type ServerConfig struct {
	BinPath        string         `yaml:"bin_path"`
	Host           string         `yaml:"host"`
	Port           int            `yaml:"port"`
	DataDir        string         `yaml:"data_dir"`
	WALMode        *bool          `yaml:"wal_mode"`
	Auth           bool           `yaml:"auth"`
	MaxConnections *int           `yaml:"max_connections"`
	ExtraConfig    map[string]any `yaml:"extra_config"`
}

type Server struct {
	cmd  *exec.Cmd
	cfg  ServerConfig
	Port int
}

func NewServer(cfg ServerConfig) *Server {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 9600
	}
	port := cfg.Port
	return &Server{cfg: cfg, Port: port}
}

func findSparkDBBin(hint string) (string, error) {
	checkPath := func(p string) (string, bool) {
		if p == "" {
			return "", false
		}
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
		return "", false
	}

	// 1. Check hint from config (BinPath)
	if p, ok := checkPath(hint); ok {
		return p, nil
	}

	// 2. Check next to the deviceos binary
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		if p, ok := checkPath(filepath.Join(exeDir, "sparkdb")); ok {
			return p, nil
		}
	}

	// 3. Check common locations relative to CWD
	for _, loc := range []string{
		"./sparkdb",
		"sparkdb",
		"./bin/sparkdb",
	} {
		if p, ok := checkPath(loc); ok {
			return p, nil
		}
	}

	// 4. Check PATH
	if bin, err := exec.LookPath("sparkdb"); err == nil {
		return bin, nil
	}

	return "", fmt.Errorf("sparkdb binary not found — place sparkdb next to deviceos binary, in the current directory, set bin_path in deviceos.yaml, or add sparkdb to PATH")
}

func (s *Server) Start(ctx context.Context) error {
	bin, err := findSparkDBBin(s.cfg.BinPath)
	if err != nil {
		return fmt.Errorf("sparkdb: %w", err)
	}

	dir := s.cfg.DataDir
	if dir == "" {
		dir = filepath.Join(".", "data", "sparkdb")
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0755); err != nil {
		return fmt.Errorf("sparkdb: mkdir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "backups"), 0755); err != nil {
		return fmt.Errorf("sparkdb: mkdir backups: %w", err)
	}

	walMode := true
	if s.cfg.WALMode != nil {
		walMode = *s.cfg.WALMode
	}
	maxConns := 100
	if s.cfg.MaxConnections != nil {
		maxConns = *s.cfg.MaxConnections
	}

	sparkCfg := map[string]any{
		"server": map[string]any{
			"host": s.cfg.Host,
			"port": s.Port,
		},
		"database": map[string]any{
			"data_dir":        filepath.Join(dir, "data"),
			"wal_mode":        walMode,
			"max_connections": maxConns,
		},
		"auth": map[string]any{
			"enabled": s.cfg.Auth,
		},
		"backup": map[string]any{
			"dir":        filepath.Join(dir, "backups"),
			"schedule":   "",
			"keep_count": 10,
		},
		"tls": map[string]any{
			"enabled": false,
		},
		"encryption": map[string]any{
			"enabled": false,
		},
		"replication": map[string]any{
			"role": "standalone",
		},
	}

	sparkCfg = mergeConfigs(sparkCfg, s.cfg.ExtraConfig)

	cfgPath := filepath.Join(dir, "sparkdb.json")
	cfgData, _ := json.MarshalIndent(sparkCfg, "", "  ")
	if err := os.WriteFile(cfgPath, cfgData, 0644); err != nil {
		return fmt.Errorf("sparkdb: write config: %w", err)
	}

	ctx2, cancel := context.WithCancel(ctx)
	s.cmd = exec.CommandContext(ctx2, bin, "start", "--config", cfgPath)
	s.cmd.Stdout = io.Discard
	s.cmd.Stderr = io.Discard

	if err := s.cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("sparkdb: start: %w", err)
	}

	baseURL := fmt.Sprintf("http://%s:%d", s.cfg.Host, s.Port)
	ready := false
	for i := 0; i < 30; i++ {
		resp, err := http.Get(baseURL + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			ready = true
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !ready {
		cancel()
		s.cmd.Wait()
		return fmt.Errorf("sparkdb: did not become ready after 15s")
	}

	slog.Info("sparkdb started", "addr", baseURL)
	return nil
}

func (s *Server) Stop() {
	if s.cmd == nil {
		return
	}
	slog.Info("stopping sparkdb")
	s.cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() {
		done <- s.cmd.Wait()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		slog.Warn("sparkdb did not stop gracefully, killing")
		s.cmd.Process.Kill()
		<-done
	}
	slog.Info("sparkdb stopped")
}

func mergeConfigs(base, extra map[string]any) map[string]any {
	if extra == nil {
		return base
	}
	for k, v := range extra {
		baseMap, baseIsMap := base[k].(map[string]any)
		extraMap, extraIsMap := v.(map[string]any)
		if baseIsMap && extraIsMap {
			base[k] = mergeConfigs(baseMap, extraMap)
		} else {
			base[k] = v
		}
	}
	return base
}
