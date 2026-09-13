---
name: sightpane-go
description: Use when integrating, configuring, testing, or troubleshooting the official sightpane Go SDK (github.com/sightpane/sightpane-go) for error tracking, panic recovery, distributed tracing, runtime performance metrics, or HTTP middleware in Go backend services and CLI applications.
---

# sightpane for Go (`sightpane-go`)

## Overview

`github.com/sightpane/sightpane-go` is the official, zero-external-dependency Go client SDK for Sightpane. It provides Sentry-style error tracking, goroutine panic recovery, distributed tracing (W3C `traceparent`), APM performance latency percentiles (p50/p95/p99), and toggleable Go runtime metrics (memory allocations, goroutines, GC activity), batching and streaming compressed envelopes to Sightpane via `POST /api/v1/envelope`.

## When to Use

- Initializing error tracking and structured logging in Go microservices, HTTP APIs, or CLI tools.
- Protecting goroutines from crashing the application using `defer sightpane.Recover()`.
- Capturing accurate call stacks (`runtime.CallersFrames`) for caught Go errors (`sightpane.CaptureException(err)`).
- Adding HTTP middleware (`sightpanehttp.Middleware`) to measure request latencies, status codes, and trace context.
- Instrumenting outbound HTTP requests (`sightpanehttp.WrapClient`) with W3C `traceparent` headers and breadcrumbs.
- Measuring transaction execution times and child spans (`StartTransaction`, `StartChild`).
- Monitoring Go runtime memory, heap allocations, goroutines, and GC pause metrics (`EnableRuntimeMetrics: true`).
- Testing Go code that uses Sightpane with mock HTTP servers or fake transports without timer/goroutine leaks.

### When NOT to Use

- For Flutter / Dart applications (use `package:sightpane` via the `sightpane-flutter` skill).
- For React, React Native, or browser applications (use `@sightpane/react` or `@sightpane/browser`).

---

## Critical Rules & Traps (STOP & Check)

### 1. The Exit Flush Trap (`defer sightpane.Flush(...)`)
`sightpane-go` queues envelopes asynchronously in an in-memory ring buffer to avoid blocking application paths.
```go
// ❌ WRONG: Calling Init and exiting without Flush causes queued events to be lost
func main() {
    sightpane.Init(sightpane.Options{...})
    sightpane.CaptureException(errors.New("fatal boot error"))
    // Process exits immediately -> HTTP batch request never sends!
}

// ✅ CORRECT: Always defer Flush or Close in main()
func main() {
    if err := sightpane.Init(sightpane.Options{...}); err != nil {
        log.Fatalf("sightpane init: %v", err)
    }
    defer sightpane.Flush(2 * time.Second) // or defer sightpane.Close()
    
    // Application code...
}
```

### 2. The Goroutine Panic Recovery Trap
In Go, an unhandled panic in a spawned goroutine terminates the entire operating process, bypassing outer recovery blocks in the parent goroutine.
```go
// ❌ WRONG: A panic here crashes the entire server before Sightpane can report it
go func() {
    processBackgroundJob() // if this panics, process dies
}()

// ✅ CORRECT: Place defer sightpane.Recover() inside EACH goroutine body
go func() {
    defer sightpane.Recover()
    processBackgroundJob()
}()

// ✅ WITH CONTEXT & CUSTOM SCOPE:
go func(ctx context.Context) {
    defer sightpane.RecoverWithContext(ctx,
        sightpane.WithRethrow(false), // don't re-panic if you want the worker to stay alive
    )
    processBackgroundJobWithContext(ctx)
}(ctx)
```

### 3. Distributed Tracing Context Propagation
To preserve trace waterfall trees across service boundaries, you must pass `context.Context` through HTTP requests and client calls.
```go
// Inbound HTTP:
// sightpanehttp.Middleware extracts incoming "traceparent" header and embeds a root transaction into r.Context().

// Outbound HTTP:
// Use sightpanehttp.WrapClient and pass the request context!
req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.internal/v1/resource", nil)
resp, err := client.Do(req) // sightpanehttp.WrapClient automatically injects traceparent header
```

### 4. Test Teardown Requirement (Goroutine Leak Prevention)
`sightpane-go` runs background workers for queue batching and optional runtime metrics polling.
- Always call `defer client.Close()` or `defer sightpane.Close()` in unit and integration tests.
- Failure to call `Close()` causes `go test -race ./...` to detect leaked running goroutines across test cases.

### 5. DSN vs Endpoint & ProjectKey
You can configure the SDK using either a full DSN or individual fields:
- DSN format: `https://<PROJECT_KEY>@<HOST>/api/v1/envelope` (or set environment variable `SIGHTPANE_ENDPOINT` and `SIGHTPANE_KEY`).
- Ingestion endpoint: `POST /api/v1/envelope` with header `X-Sightpane-Key: <PROJECT_KEY>`.

---

## Quick Reference

| Feature | Primary API | Description |
|---|---|---|
| **Initialize** | `sightpane.Init(Options)` | Initializes the global Sightpane client |
| **New Client** | `sightpane.NewClient(Options)` | Creates an isolated client instance |
| **Capture Error** | `sightpane.CaptureException(err)` | Captures error with call stack (`runtime.CallersFrames`) |
| **Context Error** | `sightpane.CaptureExceptionWithContext(ctx, err)` | Captures error including context-bound tags and scope |
| **Capture Message** | `sightpane.CaptureMessage(msg, level)` | Logs an info, warning, or error message |
| **Panic Recovery** | `defer sightpane.Recover()` | Catches goroutine panics and dispatches report immediately |
| **Custom Event** | `sightpane.CaptureEvent("order_completed", props)` | Tracks product analytics and conversion event |
| **Breadcrumbs** | `sightpane.AddBreadcrumb(Breadcrumb{...})` | Appends event to the circular breadcrumb buffer |
| **Set User** | `sightpane.SetUser(User{ID:, Email:, Username:})` | Identifies user for subsequent events |
| **Set Tags / Extra** | `sightpane.SetTag(k, v)`, `sightpane.SetExtra(k, v)` | Attaches metadata to current global scope |
| **Transaction** | `sightpane.StartTransaction(ctx, name, op)` | Starts distributed tracing root span |
| **Child Span** | `span.StartChild(op, name)` | Tracks sub-operation (e.g. `db.query`, `cache.get`) |
| **Trace Header** | `span.Traceparent()` | Generates W3C `traceparent` (e.g. `00-{trace}-{span}-01`) |
| **Runtime Metrics**| `sightpane.StartRuntimeMetrics(interval)` | Starts periodic poller for Go memory, goroutines, GC |
| **Stop Metrics** | `sightpane.StopRuntimeMetrics()` | Pauses runtime metrics background poller |
| **Check Metrics** | `sightpane.IsRuntimeMetricsEnabled()` | Returns boolean status of runtime metrics collector |
| **One-Shot Metrics**| `sightpane.CaptureRuntimeMetrics()` | Captures and dispatches immediate runtime metrics snapshot |
| **HTTP Middleware** | `sightpanehttp.Middleware(handler, opts...)` | Measures HTTP server latency, records status, catches panics |
| **HTTP Client** | `sightpanehttp.WrapClient(client)` | Injects `traceparent`, measures outbound HTTP durations |
| **Flush Queue** | `sightpane.Flush(timeout)` | Synchronously sends all pending items |
| **Close** | `sightpane.Close()` | Stops workers and cleanly shuts down client |

---

## Standard Backend Service Integration

Here is the standard idiomatic setup for a production Go HTTP service:

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sightpane/sightpane-go"
	"github.com/sightpane/sightpane-go/middleware/sightpanehttp"
)

func main() {
	// 1. Initialize SDK
	err := sightpane.Init(sightpane.Options{
		DSN:                    os.Getenv("SIGHTPANE_DSN"),
		Environment:            os.Getenv("ENV"),
		Release:                os.Getenv("RELEASE_VERSION"),
		SampleRate:             1.0,
		TracesSampleRate:       1.0,
		EnableRuntimeMetrics:   true,                 // Track Goroutines & Memory
		RuntimeMetricsInterval: 30 * time.Second,     // Collect every 30 seconds
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize sightpane: %v\n", err)
		os.Exit(1)
	}
	defer sightpane.Flush(2 * time.Second)

	// 2. Setup HTTP Router
	mux := http.NewServeMux()
	mux.HandleFunc("/api/hello", handleHello)
	mux.HandleFunc("/api/risky", handleRisky)

	// 3. Wrap router with Sightpane Middleware
	handler := sightpanehttp.Middleware(mux)

	server := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	// 4. Graceful Shutdown Handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Println("Server running on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			sightpane.CaptureException(err)
		}
	}()

	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	_ = sightpane.Close()
}

func handleHello(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello from Sightpane Go!"))
}

func handleRisky(w http.ResponseWriter, r *http.Request) {
	// Any panic here is captured automatically by sightpanehttp.Middleware
	// with full call stack, HTTP status 500, and transaction error status.
	var ptr *int
	*ptr = 100
}
```

---

## Implementation Recipes

### 1. Goroutine Panic Recovery & Worker Threads
```go
func StartWorker(ctx context.Context, jobID string) {
	go func() {
		// Recover with custom context and tags
		defer sightpane.RecoverWithContext(ctx,
			sightpane.WithRethrow(false),
		)

		sightpane.AddBreadcrumb(sightpane.Breadcrumb{
			Category: "worker",
			Message:  fmt.Sprintf("Started processing job %s", jobID),
			Level:    sightpane.LevelInfo,
		})

		processJob(jobID)
	}()
}
```

### 2. Outbound HTTP Client with Distributed Tracing
```go
// Wrap your shared HTTP client once
var httpClient = sightpanehttp.WrapClient(&http.Client{
	Timeout: 10 * time.Second,
})

func CallDownstreamService(ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://users-api.internal/v1/profile", nil)
	if err != nil {
		return nil, err
	}

	// Automatically:
	// 1. Injects W3C 'traceparent' header (e.g. 00-{trace}-{span}-01)
	// 2. Starts child span under current transaction
	// 3. Measures duration and logs HTTP breadcrumb
	return httpClient.Do(req)
}
```

### 3. Custom Performance Transactions & Child Spans
```go
func ProcessOrder(ctx context.Context, orderID string) error {
	tx, ctx := sightpane.StartTransaction(ctx, "process_order", "task")
	defer tx.Finish()

	tx.SetTag("order_id", orderID)

	// Step 1: Database query span
	dbSpan := tx.StartChild("db.query", "SELECT * FROM orders WHERE id = $1")
	order, err := queryOrder(ctx, orderID)
	if err != nil {
		dbSpan.SetStatus("error")
		dbSpan.Finish()
		sightpane.CaptureExceptionWithContext(ctx, err)
		return err
	}
	dbSpan.Finish()

	// Step 2: Payment span
	paySpan := tx.StartChild("payment.charge", "stripe.charges.create")
	if err := chargePayment(ctx, order); err != nil {
		paySpan.SetStatus("error")
		paySpan.Finish()
		sightpane.CaptureExceptionWithContext(ctx, err)
		return err
	}
	paySpan.Finish()

	return nil
}
```

### 4. Dynamic Go Runtime Performance Metrics
```go
// Enable on startup:
sightpane.Init(sightpane.Options{
    EnableRuntimeMetrics:   true,
    RuntimeMetricsInterval: 30 * time.Second,
})

// Or toggle at runtime:
sightpane.StartRuntimeMetrics(10 * time.Second) // poll faster during high load
if sightpane.IsRuntimeMetricsEnabled() {
    log.Println("Runtime metrics active")
}

// Manually capture an immediate snapshot:
metrics, ok := sightpane.CaptureRuntimeMetrics()
if ok {
    log.Printf("Current goroutines: %d, heap inuse: %d bytes", metrics.Goroutines, metrics.HeapInuseBytes)
}

// Disable when entering low-power / idle mode:
sightpane.StopRuntimeMetrics()
```

### 5. Unit and Integration Testing Pattern
Use `httptest.NewServer` to test Sightpane events without sending network requests to the real server:

```go
package myapp_test

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

func TestErrorTrackingIntegration(t *testing.T) {
	var mu sync.Mutex
	var receivedEnvelopes []sightpane.Envelope

	// Mock Sightpane server
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

	// Initialize client pointing to mock server
	client, err := sightpane.NewClient(sightpane.Options{
		Endpoint:      srv.URL,
		ProjectKey:    "test_key",
		FlushInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close() // REQUIRED: stops timers and background workers

	// Capture exception
	client.CaptureException(errors.New("test failure"), nil)
	client.Flush(1 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedEnvelopes) == 0 {
		t.Fatal("expected envelope to be delivered to server")
	}
}
```

---

## Troubleshooting Guide

| Issue / Symptom | Root Cause | Solution |
|---|---|---|
| **No errors appearing in dashboard** | Process exited before background queue flushed. | Add `defer sightpane.Flush(2 * time.Second)` or `defer sightpane.Close()` to `main()`. |
| **401 Unauthorized response from server** | Missing or incorrect API key. | Verify `ProjectKey` (or DSN user `https://<KEY>@host/api/v1/envelope`). The server checks header `X-Sightpane-Key`. |
| **Goroutine panics crash the process** | Goroutine lacked local recovery. | In Go, panics across goroutine boundaries crash the runtime. Add `defer sightpane.Recover()` inside the spawned goroutine. |
| **Trace waterfall is disconnected** | Outbound HTTP requests didn't pass context or wrap client. | Use `sightpanehttp.WrapClient(httpClient)` and execute requests with `req.WithContext(ctx)`. |
| **`go test -race` detects leaked goroutines** | Client or global singleton was not closed. | Always call `defer client.Close()` or `defer sightpane.Close()` at the end of tests. |
| **Runtime metrics not reporting** | `EnableRuntimeMetrics` is `false` by default. | Set `EnableRuntimeMetrics: true` in `Options` or call `sightpane.StartRuntimeMetrics()`. |
