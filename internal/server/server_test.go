package server_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/lohtbrok/deviceos/internal/server"
)

func TestNewServer(t *testing.T) {
	s := server.New(server.Config{Host: "127.0.0.1", Port: 0})
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestMux(t *testing.T) {
	s := server.New(server.Config{Host: "127.0.0.1", Port: 0})
	mux := s.Mux()
	if mux == nil {
		t.Fatal("expected non-nil mux")
	}
}

func TestStartAndStop(t *testing.T) {
	s := server.New(server.Config{Host: "127.0.0.1", Port: 9999})
	mux := s.Mux()
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://127.0.0.1:9999/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestMiddlewareHeaders(t *testing.T) {
	s := server.New(server.Config{Host: "127.0.0.1", Port: 9998})
	mux := s.Mux()
	mux.HandleFunc("GET /headers", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	go s.Start()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://127.0.0.1:9998/headers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}
	if v := resp.Header.Get("X-DeviceOS-Version"); v != "0.1.0" {
		t.Fatalf("expected 0.1.0, got %s", v)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.Stop(ctx)
}

func TestStopWithoutStart(t *testing.T) {
	s := server.New(server.Config{Host: "127.0.0.1", Port: 9997})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err == nil {
		t.Log("stopping without start may or may not error")
	}
}
