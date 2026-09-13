package sightpane

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestOptionsNormalize(t *testing.T) {
	// Missing both
	opts := Options{}
	if err := opts.normalize(); err == nil {
		t.Fatal("expected error on empty options")
	}

	// DSN parsing
	opts = Options{DSN: "http://mykey@localhost:8790"}
	if err := opts.normalize(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.ProjectKey != "mykey" {
		t.Fatalf("expected project key 'mykey', got %q", opts.ProjectKey)
	}
	if opts.Endpoint != "http://localhost:8790" {
		t.Fatalf("expected endpoint 'http://localhost:8790', got %q", opts.Endpoint)
	}
	if opts.AppType != "server" {
		t.Fatalf("expected default app_type 'server', got %q", opts.AppType)
	}
	if opts.MaxBatchSize != defaultMaxBatchSize {
		t.Fatalf("expected max batch size %d, got %d", defaultMaxBatchSize, opts.MaxBatchSize)
	}
}

func TestCaptureExceptionAndEnvelopeDelivery(t *testing.T) {
	var mu sync.Mutex
	var receivedEnvelopes []Envelope
	var receivedKeys []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/envelope" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()

		receivedKeys = append(receivedKeys, r.Header.Get("X-Sightpane-Key"))

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, err.Error(), 500)
			return
		}

		var env Envelope
		if err := json.Unmarshal(body, &env); err != nil {
			t.Errorf("unmarshal envelope: %v", err)
			http.Error(w, err.Error(), 400)
			return
		}

		receivedEnvelopes = append(receivedEnvelopes, env)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]int{"accepted": len(env.Items), "rejected": 0})
	}))
	defer ts.Close()

	client, err := NewClient(Options{
		Endpoint:      ts.URL,
		ProjectKey:    "test-secret-key",
		Environment:   "testing",
		Release:       "v1.0.0",
		FlushInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	// 1. Capture Error with Scope
	scope := NewScope()
	scope.SetUser(User{ID: "usr-42", Email: "alice@example.com"})
	scope.SetTag("version", "1.0.0")
	scope.SetExtra("account_type", "premium")
	scope.AddBreadcrumb(Breadcrumb{
		Category: "auth",
		Message:  "User signed in",
		Level:    LevelInfo,
	})

	testErr := errors.New("database connection timeout")
	eventID := client.CaptureException(testErr, scope)
	if eventID == "" {
		t.Fatal("expected non-empty eventID")
	}

	// 2. Capture Custom Event
	client.CaptureEvent("checkout.completed", map[string]any{
		"amount": 99.50,
		"items":  3,
	}, scope)

	// 3. Flush
	if ok := client.Flush(3 * time.Second); !ok {
		t.Fatal("flush timed out")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(receivedEnvelopes) == 0 {
		t.Fatal("expected at least 1 envelope received")
	}

	if receivedKeys[0] != "test-secret-key" {
		t.Fatalf("expected X-Sightpane-Key 'test-secret-key', got %q", receivedKeys[0])
	}

	env := receivedEnvelopes[0]
	if env.SDK.Name != SDKName || env.SDK.Version != SDKVersion {
		t.Fatalf("unexpected SDK metadata: %+v", env.SDK)
	}
	if env.Session.ID == "" {
		t.Fatal("expected non-empty session ID")
	}

	// Verify items
	if len(env.Items) < 2 {
		t.Fatalf("expected at least 2 items, got %d", len(env.Items))
	}

	// Decode first item (ErrorItem)
	var errItem ErrorItem
	if err := json.Unmarshal(env.Items[0], &errItem); err != nil {
		t.Fatalf("unmarshal error item: %v", err)
	}
	if errItem.Type != "error" {
		t.Fatalf("expected type 'error', got %q", errItem.Type)
	}
	if errItem.Message != "database connection timeout" {
		t.Fatalf("expected message 'database connection timeout', got %q", errItem.Message)
	}
	if len(errItem.Frames) == 0 {
		t.Fatal("expected stack frames to be populated")
	}
	if errItem.User == nil || errItem.User.ID != "usr-42" {
		t.Fatalf("expected user usr-42, got %+v", errItem.User)
	}
	if errItem.Tags["version"] != "1.0.0" {
		t.Fatalf("expected tag version=1.0.0, got %v", errItem.Tags)
	}
	if len(errItem.Breadcrumbs) != 1 || errItem.Breadcrumbs[0].Message != "User signed in" {
		t.Fatalf("unexpected breadcrumbs: %+v", errItem.Breadcrumbs)
	}

	// Decode second item (EventItem)
	var evtItem EventItem
	if err := json.Unmarshal(env.Items[1], &evtItem); err != nil {
		t.Fatalf("unmarshal event item: %v", err)
	}
	if evtItem.Type != "event" || evtItem.Name != "checkout.completed" {
		t.Fatalf("unexpected event item: %+v", evtItem)
	}
	if evtItem.Props["items"] != float64(3) {
		t.Fatalf("expected items=3, got %v", evtItem.Props["items"])
	}
}

func TestBeforeSendFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	client, err := NewClient(Options{
		Endpoint:   ts.URL,
		ProjectKey: "test",
		BeforeSend: func(item any) any {
			if errItem, ok := item.(*ErrorItem); ok {
				if errItem.Message == "ignore me" {
					return nil // drop
				}
				errItem.Message = "sanitized: " + errItem.Message
				return errItem
			}
			return item
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Ignored
	client.CaptureException(errors.New("ignore me"), nil)
	// Modified
	client.CaptureException(errors.New("real error"), nil)
}

func TestBreadcrumbRingBuffer(t *testing.T) {
	scope := NewScope()
	scope.maxBread = 3

	for i := 1; i <= 5; i++ {
		scope.AddBreadcrumb(Breadcrumb{Message: string(rune('0' + i))})
	}

	if len(scope.breadcrumbs) != 3 {
		t.Fatalf("expected 3 breadcrumbs, got %d", len(scope.breadcrumbs))
	}
	if scope.breadcrumbs[0].Message != "3" || scope.breadcrumbs[2].Message != "5" {
		t.Fatalf("expected [3, 4, 5], got %+v", scope.breadcrumbs)
	}
}

func TestGlobalSingleton(t *testing.T) {
	err := Init(Options{
		Endpoint:   "http://localhost:8790",
		ProjectKey: "dev",
	})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer Close()

	c := CurrentClient()
	if c == nil {
		t.Fatal("expected global client to be set")
	}

	SetUser(User{ID: "global-user"})
	SetTag("env", "prod")
	SetExtra("key", "val")
	AddBreadcrumb(Breadcrumb{Message: "global breadcrumb"})

	CaptureMessage("test warning message", LevelWarn)
	CaptureEvent("global.event", map[string]any{"ok": true})
}
