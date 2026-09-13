package sightpane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestTracingAndW3CTraceparent(t *testing.T) {
	var mu sync.Mutex
	var receivedSpans []SpanItem

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env Envelope
		_ = json.NewDecoder(r.Body).Decode(&env)

		mu.Lock()
		defer mu.Unlock()
		for _, raw := range env.Items {
			var sp SpanItem
			if err := json.Unmarshal(raw, &sp); err == nil && sp.Type == "span" {
				receivedSpans = append(receivedSpans, sp)
			}
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	client, err := NewClient(Options{
		Endpoint:   ts.URL,
		ProjectKey: "dev",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()

	// Incoming W3C header test
	incomingTP := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	traceID, parentID, sampled := ParseTraceparent(incomingTP)
	if traceID != "4bf92f3577b34da6a3ce929d0e0e4736" || parentID != "00f067aa0ba902b7" || !sampled {
		t.Fatalf("unexpected parsed values: traceID=%s, parentID=%s, sampled=%v", traceID, parentID, sampled)
	}

	tx, ctx := client.StartTransaction(ctx, "HTTP GET /users", "http.server", WithParentTraceparent(incomingTP))
	if tx.traceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("expected traceID 4bf92f3577b34da6a3ce929d0e0e4736, got %s", tx.traceID)
	}

	// Verify Context retrieval
	activeSpan := SpanFromContext(ctx)
	if activeSpan != tx {
		t.Fatal("expected SpanFromContext to return active transaction")
	}

	// Child span
	child := tx.StartChild("db.query", "SELECT * FROM users")
	time.Sleep(10 * time.Millisecond)
	child.SetTag("db.rows", 42)
	child.Finish()

	time.Sleep(5 * time.Millisecond)
	tx.SetStatus("ok")
	tx.Finish()

	if ok := client.Flush(2 * time.Second); !ok {
		t.Fatal("flush failed")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(receivedSpans) == 0 {
		t.Fatal("expected at least 1 span received")
	}

	sp := receivedSpans[0]
	if sp.Name != "HTTP GET /users" || sp.Op != "http.server" {
		t.Fatalf("unexpected root span: %+v", sp)
	}
	if sp.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("unexpected trace id: %s", sp.TraceID)
	}
	if len(sp.Spans) != 1 {
		t.Fatalf("expected 1 child span, got %d", len(sp.Spans))
	}
	ch := sp.Spans[0]
	if ch.Name != "SELECT * FROM users" || ch.Op != "db.query" {
		t.Fatalf("unexpected child span: %+v", ch)
	}
	if ch.Tags["db.rows"] != float64(42) {
		t.Fatalf("expected db.rows=42, got %v", ch.Tags["db.rows"])
	}
	if ch.DurationMs <= 0 {
		t.Fatalf("expected positive duration, got %f", ch.DurationMs)
	}
}
