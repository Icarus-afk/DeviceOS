package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Stats struct {
	registered atomic.Int64
	sentTele   atomic.Int64
	errors     atomic.Int64
	start      time.Time
}

type latencyTracker struct {
	mu    sync.Mutex
	times []time.Duration
}

func (lt *latencyTracker) Record(d time.Duration) {
	lt.mu.Lock()
	lt.times = append(lt.times, d)
	lt.mu.Unlock()
}

func (lt *latencyTracker) Report() (p50, p95, p99 time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	if len(lt.times) == 0 {
		return 0, 0, 0
	}
	sorted := make([]time.Duration, len(lt.times))
	copy(sorted, lt.times)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	p50 = sorted[int(math.Ceil(float64(len(sorted))*0.50))-1]
	p95 = sorted[int(math.Ceil(float64(len(sorted))*0.95))-1]
	p99 = sorted[int(math.Ceil(float64(len(sorted))*0.99))-1]
	return
}

func main() {
	server := flag.String("server", "http://localhost:8080", "DeviceOS server URL")
	devices := flag.Int("devices", 10, "number of devices to register")
	concurrency := flag.Int("concurrency", 5, "concurrent workers")
	apiKey := flag.String("api-key", "", "admin API key (default: DEVICEOS_ADMIN_TOKEN env)")
	withTelemetry := flag.Bool("telemetry", true, "send telemetry from each device")
	rate := flag.Int("rate", 0, "telemetry messages per second (0 = one-shot)")
	duration := flag.Duration("duration", 30*time.Second, "how long to run sustained telemetry")
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
	if *rate > 0 {
		fmt.Printf("Rate:        %d msg/s\n", *rate)
		fmt.Printf("Duration:    %s\n", *duration)
	}
	fmt.Println()

	// Login
	token, err := login(*server, key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Authenticated (token: %s...)\n", token[:min(len(token), 16)])

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n\nShutting down gracefully...")
		cancel()
	}()

	var stats Stats
	stats.start = time.Now()

	var regLatency latencyTracker

	// Register devices with worker pool
	deviceIDs := make([]string, *devices)
	type job struct{ idx int }
	type result struct {
		idx      int
		deviceID string
		err      error
		latency  time.Duration
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
				start := time.Now()
				id, err := registerDevice(client, *server, token, j.idx)
				results <- result{j.idx, id, err, time.Since(start)}
			}
		}()
	}

	for i := 0; i < *devices; i++ {
		jobs <- job{i}
	}
	close(jobs)

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
			regLatency.Record(r.latency)
		}
	}
	done <- struct{}{}

	regTime := time.Since(stats.start)
	regP50, regP95, regP99 := regLatency.Report()
	fmt.Printf("\nRegistration: %d devices in %v (%.0f devices/s)\n",
		stats.registered.Load(), roundDur(regTime), float64(stats.registered.Load())/regTime.Seconds())
	fmt.Printf("  Latency:    p50=%v  p95=%v  p99=%v\n", roundDur(regP50), roundDur(regP95), roundDur(regP99))

	if stats.errors.Load() > 0 {
		fmt.Printf("Errors:      %d\n", stats.errors.Load())
	}

	// Send telemetry
	if *withTelemetry && stats.registered.Load() > 0 {
		if *rate > 0 {
			runSustainedTelemetry(ctx, *server, token, deviceIDs, *rate, *duration, &stats)
		} else {
			runOneShotTelemetry(*server, token, deviceIDs, *concurrency, &stats)
		}
	}

	fmt.Printf("\nTotal:       %d registered, %d telemetry, %d errors in %v\n",
		stats.registered.Load(), stats.sentTele.Load(), stats.errors.Load(), roundDur(time.Since(stats.start)))

	if stats.errors.Load() > 0 {
		os.Exit(1)
	}
}

func runOneShotTelemetry(server, token string, deviceIDs []string, concurrency int, stats *Stats) {
	teleStart := time.Now()
	teleResults := make(chan error, stats.registered.Load())
	teleWg := sync.WaitGroup{}
	teleSem := make(chan struct{}, concurrency)

	var teleLatency latencyTracker

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
			start := time.Now()
			if err := sendTelemetry(client, server, token, did); err != nil {
				stats.errors.Add(1)
				teleResults <- fmt.Errorf("%s: %w", did, err)
				return
			}
			teleLatency.Record(time.Since(start))
			stats.sentTele.Add(1)
			teleResults <- nil
		}(id)
	}

	done := make(chan struct{})
	go func() {
		teleWg.Wait()
		close(done)
		close(teleResults)
	}()
	<-done

	teleTime := time.Since(teleStart)
	teleP50, teleP95, teleP99 := teleLatency.Report()
	fmt.Printf("Telemetry:   %d datapoints in %v (%.0f points/s)\n",
		stats.sentTele.Load(), roundDur(teleTime), float64(stats.sentTele.Load())/teleTime.Seconds())
	fmt.Printf("  Latency:    p50=%v  p95=%v  p99=%v\n", roundDur(teleP50), roundDur(teleP95), roundDur(teleP99))
}

func runSustainedTelemetry(ctx context.Context, server, token string, deviceIDs []string, rate int, duration time.Duration, stats *Stats) {
	var teleLatency latencyTracker
	interval := time.Second / time.Duration(rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	start := time.Now()
	end := start.Add(duration)
	reportTicker := time.NewTicker(5 * time.Second)
	defer reportTicker.Stop()

	fmt.Printf("\nSustained telemetry: %d msg/s for %s\n\n", rate, roundDur(duration))
	fmt.Printf("  %-8s %-8s %-8s  %s\n", "Time", "Sent", "Errors", "Latency(p50/p95/p99)")
	fmt.Printf("  %-8s %-8s %-8s  %s\n", "--------", "--------", "--------", "--------------------")

	client := &http.Client{Timeout: 30 * time.Second}
	devCount := len(deviceIDs)
	idx := 0

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Now().After(end) {
					return
				}
				did := deviceIDs[idx%devCount]
				if did == "" {
					idx++
					continue
				}
				idx++

				tStart := time.Now()
				if err := sendTelemetry(client, server, token, did); err != nil {
					stats.errors.Add(1)
				} else {
					teleLatency.Record(time.Since(tStart))
					stats.sentTele.Add(1)
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			<-done
			goto printStats
		case <-done:
			goto printStats
		case <-reportTicker.C:
			elapsed := time.Since(start).Round(time.Second)
			sent := stats.sentTele.Load()
			errs := stats.errors.Load()
			p50, p95, p99 := teleLatency.Report()
			fmt.Printf("  %-8s %-8d %-8d  %v/%v/%v\n",
				elapsed.String(), sent, errs,
				roundDur(p50), roundDur(p95), roundDur(p99))
		}
	}

printStats:
	totalTime := time.Since(start)
	p50, p95, p99 := teleLatency.Report()
	fmt.Printf("\nSustained telemetry results:\n")
	fmt.Printf("  Total:      %d datapoints in %v (%.0f points/s)\n",
		stats.sentTele.Load(), roundDur(totalTime), float64(stats.sentTele.Load())/totalTime.Seconds())
	fmt.Printf("  Latency:    p50=%v  p95=%v  p99=%v\n", roundDur(p50), roundDur(p95), roundDur(p99))
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
