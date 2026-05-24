package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/lohtbrok/deviceos/internal/config"
)

func cmdStatus(cfgPath string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to load config: %v\n", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("http://%s:%d/healthz", cfg.Server.Host, cfg.Server.Port)
	resp, err := http.Get(addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: server not reachable at %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Server: %s\n", addr)
	fmt.Printf("Status: %s\n", resp.Status)
	for k, v := range result {
		fmt.Printf("  %s: %v\n", k, v)
	}
}
