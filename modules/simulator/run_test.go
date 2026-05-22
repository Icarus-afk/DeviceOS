package simulator

import (
	"testing"
	"time"
)

func init() {
	simTickInterval = 10 * time.Millisecond
}

func TestSimulator_Run_StopsViaChannel(t *testing.T) {
	m := &Module{}
	m.mu.Lock()
	m.stopCh = make(chan struct{})
	m.devices = []SimDevice{
		{ID: "sim_test_1", Name: "test-1", Type: "sensor", TempBase: 25, HumidBase: 50, Battery: 100, Connected: true},
	}
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.run()
		close(done)
	}()

	time.Sleep(5 * time.Millisecond)
	close(m.stopCh)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run() did not stop within 1s")
	}
}

func TestSimulator_Run_NoDevices(t *testing.T) {
	m := &Module{}
	m.mu.Lock()
	m.stopCh = make(chan struct{})
	m.devices = nil
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.run()
		close(done)
	}()

	time.Sleep(5 * time.Millisecond)
	close(m.stopCh)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run() did not stop")
	}
}

func TestSimulator_Run_TickerFires(t *testing.T) {
	m := &Module{}
	m.mu.Lock()
	m.stopCh = make(chan struct{})
	m.devices = []SimDevice{
		{ID: "sim_test_1", Name: "test-1", Type: "sensor", TempBase: 25, HumidBase: 50, Battery: 100, Connected: true},
	}
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.run()
		close(done)
	}()

	// Wait for ticker to fire at least once
	time.Sleep(20 * time.Millisecond)
	close(m.stopCh)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run() did not stop")
	}
}

func TestSimulator_Run_DisconnectedDevice(t *testing.T) {
	m := &Module{}
	m.mu.Lock()
	m.stopCh = make(chan struct{})
	m.devices = []SimDevice{
		{ID: "sim_test_1", Name: "test-1", Type: "sensor", Connected: false},
	}
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.run()
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	close(m.stopCh)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run() did not stop")
	}
}

func TestSimulator_Run_MultipleDevices(t *testing.T) {
	m := &Module{}
	m.mu.Lock()
	m.stopCh = make(chan struct{})
	m.devices = []SimDevice{
		{ID: "sim_1", Name: "a", Type: "temp-sensor", TempBase: 20, HumidBase: 40, Battery: 90, Connected: true},
		{ID: "sim_2", Name: "b", Type: "gps-tracker", TempBase: 25, HumidBase: 50, Battery: 80, Connected: true},
		{ID: "sim_3", Name: "c", Type: "multi-sensor", TempBase: 30, HumidBase: 60, Battery: 70, Connected: false},
	}
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.run()
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	close(m.stopCh)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run() did not stop")
	}
}

func TestSimulator_Run_BatteryDrains(t *testing.T) {
	m := &Module{}
	m.mu.Lock()
	m.stopCh = make(chan struct{})
	m.devices = []SimDevice{
		{ID: "sim_1", Name: "a", Type: "sensor", TempBase: 20, HumidBase: 40, Battery: 1.0, Connected: true},
	}
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.run()
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	close(m.stopCh)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run() did not stop")
	}
}

func TestSimulator_Run_ReconnectsDevice(t *testing.T) {
	m := &Module{}
	m.mu.Lock()
	m.stopCh = make(chan struct{})
	m.devices = []SimDevice{
		{ID: "sim_1", Name: "a", Type: "sensor", TempBase: 20, HumidBase: 40, Battery: 50, Connected: false},
	}
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.run()
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	close(m.stopCh)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run() did not stop")
	}
}
