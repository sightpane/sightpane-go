package sightpanehttp

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

func TestHTTPMiddlewareAndRoundTripper(t *testing.T) {
	var mu sync.Mutex
	var receivedEnvelopes []sightpane.Envelope

	ingestServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var env sightpane.Envelope
		_ = json.Unmarshal(body, &env)

		mu.Lock()
		receivedEnvelopes = append(receivedEnvelopes, env)
		mu.Unlock()

		w.WriteHeader(http.StatusAccepted)
	}))
	defer ingestServer.Close()

	// Initialize Sightpane
	err := sightpane.Init(sightpane.Options{
		Endpoint:   ingestServer.URL,
		ProjectKey: "dev",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sightpane.Close()

	// 1. Target downstream server (for outbound HTTP call)
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tp := r.Header.Get("traceparent")
		if tp == "" {
			t.Error("expected traceparent header on downstream request")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer downstream.Close()

	// 2. Application HTTP handler under test
	appHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Outbound call via traced client
		client := WrapClient(&http.Client{Timeout: 5 * time.Second})
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, downstream.URL+"/data", nil)
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer resp.Body.Close()

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello from Sightpane app"))
	})

	// Wrap with Middleware
	wrappedHandler := Middleware(appHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/hello", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	sightpane.Flush(2 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedEnvelopes) == 0 {
		t.Fatal("expected at least 1 envelope received from middleware + roundtripper")
	}
}

func TestHTTPMiddlewarePanicRecovery(t *testing.T) {
	ingestServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ingestServer.Close()

	_ = sightpane.Init(sightpane.Options{
		Endpoint:   ingestServer.URL,
		ProjectKey: "dev",
	})
	defer sightpane.Close()

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("fatal unexpected nil pointer")
	})

	// WithRethrow(false) so the handler returns 500 without crashing the test runner
	wrapped := Middleware(panicHandler, WithRethrow(false))

	req := httptest.NewRequest(http.MethodGet, "/crash", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", rec.Code)
	}
}
