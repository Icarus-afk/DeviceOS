package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Stats struct {
	registered atomic.Int64
	sentTele   atomic.Int64
	errors     atomic.Int64
	start      time.Time
}

func main() {
	server := flag.String("server", "http://localhost:8080", "DeviceOS server URL")
	devices := flag.Int("devices", 10, "number of devices to register")
	concurrency := flag.Int("concurrency", 5, "concurrent workers")
	apiKey := flag.String("api-key", "", "admin API key (default: DEVICEOS_ADMIN_TOKEN env)")
	withTelemetry := flag.Bool("telemetry", true, "send telemetry from each device")
	flag.Parse()

	key := *apiKey
	if key == "" {
		key = os.Getenv("DEVICEOS_ADMIN_TOKEN")
	}
	if key == "" {
		key = "dos_dev_admin_token_0001"
	}

	fmt.Printf("=== DeviceOS Load Simulator ===\n\n")
	fmt.Printf("Server:      %s\n", *server)
	fmt.Printf("Devices:     %d\n", *devices)
	fmt.Printf("Concurrency: %d\n", *concurrency)
	fmt.Printf("Telemetry:   %v\n", *withTelemetry)
	fmt.Println()

	// Login
	token, err := login(*server, key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Authenticated (token: %s...)\n", token[:min(len(token), 16)])

	var stats Stats
	stats.start = time.Now()

	// Register devices with worker pool
	deviceIDs := make([]string, *devices)
	type job struct{ idx int }
	type result struct {
		idx      int
		deviceID string
		err      error
	}

	jobs := make(chan job, *devices)
	results := make(chan result, *devices)

	var wg sync.WaitGroup
	for w := 0; w < *concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 30 * time.Second}
			for j := range jobs {
				id, err := registerDevice(client, *server, token, j.idx)
				results <- result{j.idx, id, err}
			}
		}()
	}

	for i := 0; i < *devices; i++ {
		jobs <- job{i}
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	done := make(chan struct{}, 1)
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n := stats.registered.Load()
				pct := float64(n) / float64(*devices) * 100
				elapsed := time.Since(stats.start)
				rate := float64(n) / elapsed.Seconds()
				fmt.Printf("  Progress: %d/%d (%.1f%%) — %.0f devices/s\n", n, *devices, pct, rate)
			case <-done:
				return
			}
		}
	}()

	for r := range results {
		if r.err != nil {
			stats.errors.Add(1)
		} else {
			stats.registered.Add(1)
			deviceIDs[r.idx] = r.deviceID
		}
	}
	done <- struct{}{}

	regTime := time.Since(stats.start)
	fmt.Printf("\nRegistration: %d devices in %v (%.0f devices/s)\n",
		stats.registered.Load(), roundDur(regTime), float64(stats.registered.Load())/regTime.Seconds())

	if stats.errors.Load() > 0 {
		fmt.Printf("Errors:      %d\n", stats.errors.Load())
	}

	// Send telemetry
	if *withTelemetry && stats.registered.Load() > 0 {
		teleStart := time.Now()
		teleResults := make(chan error, stats.registered.Load())
		teleWg := sync.WaitGroup{}
		teleSem := make(chan struct{}, *concurrency)

		for _, id := range deviceIDs {
			if id == "" {
				continue
			}
			teleWg.Add(1)
			go func(did string) {
				defer teleWg.Done()
				teleSem <- struct{}{}
				defer func() { <-teleSem }()
				client := &http.Client{Timeout: 30 * time.Second}
				if err := sendTelemetry(client, *server, token, did); err != nil {
					stats.errors.Add(1)
					teleResults <- fmt.Errorf("%s: %w", did, err)
					return
				}
				stats.sentTele.Add(1)
				teleResults <- nil
			}(id)
		}

		// Wait for all
		done := make(chan struct{})
		go func() {
			teleWg.Wait()
			close(done)
			close(teleResults)
		}()
		<-done

		teleTime := time.Since(teleStart)
		fmt.Printf("Telemetry:   %d datapoints in %v (%.0f points/s)\n",
			stats.sentTele.Load(), roundDur(teleTime), float64(stats.sentTele.Load())/teleTime.Seconds())
	}

	fmt.Printf("\nTotal:       %d registered, %d telemetry, %d errors in %v\n",
		stats.registered.Load(), stats.sentTele.Load(), stats.errors.Load(), roundDur(time.Since(stats.start)))

	if stats.errors.Load() > 0 {
		os.Exit(1)
	}
}

func login(server, apiKey string) (string, error) {
	body, _ := json.Marshal(map[string]string{"api_key": apiKey})
	resp, err := http.Post(server+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Token string `json:"token"`
	}
	json.Unmarshal(raw, &result)
	if result.Token == "" {
		return "", fmt.Errorf("no token in response")
	}
	return result.Token, nil
}

func registerDevice(client *http.Client, server, token string, idx int) (string, error) {
	payload, _ := json.Marshal(map[string]any{
		"name": fmt.Sprintf("sim-device-%05d", idx),
		"type": "simulator",
		"tags": []string{"simulation"},
	})

	req, _ := http.NewRequest("POST", server+"/api/v1/devices", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 201 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Device struct {
			ID string `json:"id"`
		} `json:"device"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("parse: %w (%s)", err, string(raw))
	}
	if result.Device.ID == "" {
		return "", fmt.Errorf("empty device id")
	}

	return result.Device.ID, nil
}

func sendTelemetry(client *http.Client, server, token, deviceID string) error {
	payload, _ := json.Marshal(map[string]any{
		"device_id": deviceID,
		"metrics": map[string]float64{
			"temperature": 20.0 + float64(time.Now().UnixNano()%100)/10.0,
			"humidity":    40.0 + float64(time.Now().UnixNano()%500)/10.0,
		},
	})

	req, _ := http.NewRequest("POST", server+"/api/v1/telemetry", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

func roundDur(d time.Duration) time.Duration {
	return d.Round(time.Millisecond)
}
