package sightpane_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sightpane/sightpane-go"
)

func TestReadRuntimeMetrics(t *testing.T) {
	m := sightpane.ReadRuntimeMetrics()

	if m.Goroutines < 1 {
		t.Errorf("expected at least 1 goroutine, got %d", m.Goroutines)
	}
	if m.NumCPU < 1 {
		t.Errorf("expected at least 1 CPU, got %d", m.NumCPU)
	}
	if m.AllocBytes == 0 {
		t.Errorf("expected non-zero AllocBytes")
	}

	props := m.ToProps()
	if props["goroutines"] != m.Goroutines {
		t.Errorf("props goroutines mismatch: %v vs %d", props["goroutines"], m.Goroutines)
	}
	if props["alloc_bytes"] != m.AllocBytes {
		t.Errorf("props alloc_bytes mismatch: %v vs %d", props["alloc_bytes"], m.AllocBytes)
	}
}

func TestRuntimeMetricsPollerLifecycle(t *testing.T) {
	client, err := sightpane.NewClient(sightpane.Options{
		Endpoint:             "http://localhost:9999",
		ProjectKey:           "test_key",
		EnableRuntimeMetrics: false,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	if client.IsRuntimeMetricsEnabled() {
		t.Error("expected runtime metrics poller to be initially disabled")
	}

	// Turn it on dynamically
	client.StartRuntimeMetrics(500 * time.Millisecond)
	if !client.IsRuntimeMetricsEnabled() {
		t.Error("expected runtime metrics poller to be active after start")
	}

	// Turn it off dynamically
	client.StopRuntimeMetrics()
	if client.IsRuntimeMetricsEnabled() {
		t.Error("expected runtime metrics poller to be disabled after stop")
	}
}

func TestRuntimeMetricsIngestion(t *testing.T) {
	var mu sync.Mutex
	var receivedEnvelopes []sightpane.Envelope

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var env sightpane.Envelope
		_ = json.Unmarshal(body, &env)

		mu.Lock()
		receivedEnvelopes = append(receivedEnvelopes, env)
		mu.Unlock()

		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"accepted": 1}`))
	}))
	defer srv.Close()

	client, err := sightpane.NewClient(sightpane.Options{
		Endpoint:               srv.URL,
		ProjectKey:             "test_key",
		EnableRuntimeMetrics:   true,
		RuntimeMetricsInterval: 500 * time.Millisecond,
		FlushInterval:          200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Wait enough for the poller to tick at least once
	time.Sleep(1100 * time.Millisecond)

	// Trigger a manual capture as well
	client.CaptureRuntimeMetrics()

	if !client.Flush(2 * time.Second) {
		t.Fatal("Flush failed")
	}
	_ = client.Close()

	mu.Lock()
	defer mu.Unlock()

	foundMetricsEvent := false
	for _, env := range receivedEnvelopes {
		for _, raw := range env.Items {
			var event sightpane.EventItem
			if err := json.Unmarshal(raw, &event); err == nil && event.Type == "event" && event.Name == "runtime_metrics" {
				foundMetricsEvent = true
				if event.Props == nil {
					t.Error("expected props in runtime_metrics event")
				}
				if _, ok := event.Props["alloc_bytes"]; !ok {
					t.Error("expected alloc_bytes in runtime_metrics event props")
				}
				if _, ok := event.Props["goroutines"]; !ok {
					t.Error("expected goroutines in runtime_metrics event props")
				}
			}
		}
	}

	if !foundMetricsEvent {
		t.Error("expected to find at least one runtime_metrics event delivered in envelope")
	}
}

func TestGlobalRuntimeMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	err := sightpane.Init(sightpane.Options{
		Endpoint:   srv.URL,
		ProjectKey: "test_key",
	})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer sightpane.Close()

	if sightpane.IsRuntimeMetricsEnabled() {
		t.Error("expected runtime metrics to be disabled initially")
	}

	sightpane.StartRuntimeMetrics(500 * time.Millisecond)
	if !sightpane.IsRuntimeMetricsEnabled() {
		t.Error("expected runtime metrics to be active after StartRuntimeMetrics")
	}

	m, ok := sightpane.CaptureRuntimeMetrics()
	if !ok || m.Goroutines == 0 {
		t.Errorf("CaptureRuntimeMetrics returned unexpected: %v, %t", m, ok)
	}

	sightpane.StopRuntimeMetrics()
	if sightpane.IsRuntimeMetricsEnabled() {
		t.Error("expected runtime metrics to be disabled after StopRuntimeMetrics")
	}
}
