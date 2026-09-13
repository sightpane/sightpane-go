# Sightpane Go SDK (`sightpane-go`)

Official Go client SDK for [Sightpane](https://sightpane.cloud) — real-time error tracking, distributed tracing, and observability.

[![Go Reference](https://pkg.go.dev/badge/github.com/sightpane/sightpane-go.svg)](https://pkg.go.dev/github.com/sightpane/sightpane-go)
[![CI](https://github.com/sightpane/sightpane-go/actions/workflows/ci.yml/badge.svg)](https://github.com/sightpane/sightpane-go/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

## Highlights

- **Zero External Dependencies**: Implemented entirely with the Go standard library (`net/http`, `sync`, `runtime`, `crypto/rand`).
- **High Performance & Asynchronous**: Lock-free/buffered worker queues batch and flush envelopes in the background without blocking request paths.
- **Accurate Stack Traces**: Uses `runtime.CallersFrames` to resolve exact function names, file paths, line numbers, and in-app indicators.
- **Panic Recovery**: Idiomatic `defer sightpane.Recover()` captures unhandled panics with full stack traces and contextual tags.
- **Distributed Tracing & APM**: Native W3C Trace Context (`traceparent`) support, transactions, child spans, and performance metrics (p50, p95, p99 latencies).
- **Runtime Performance Metrics**: Built-in, toggleable background collector for Go runtime memory (`Alloc`, `Sys`, `HeapInuse`), goroutine counts, and GC activity.
- **Context & Scopes**: Thread-safe Scope management with user identification, custom tags, extras, and breadcrumb ring buffers.
- **HTTP Middleware**: Ready-to-use `net/http` middleware and `http.RoundTripper` for automatic request timing and distributed trace propagation.

---

## Installation

```bash
go get github.com/sightpane/sightpane-go
```

---

## Quickstart

### 1. Initialize the SDK

Call `sightpane.Init` in your application's entrypoint (e.g., `main.go`). Ensure you call `defer sightpane.Flush(...)` or `defer sightpane.Close()` to flush queued events before exit.

```go
package main

import (
	"context"
	"errors"
	"time"

	"github.com/sightpane/sightpane-go"
)

func main() {
	err := sightpane.Init(sightpane.Options{
		DSN:                  "https://YOUR_API_KEY@sightpane.cloud/api/v1/envelope",
		Environment:          "production",
		Release:              "v1.0.0",
		SampleRate:           1.0,
		EnableRuntimeMetrics: true,                 // Turn on Go runtime metrics
		RuntimeMetricsInterval: 30 * time.Second, // Collect every 30s
	})
	if err != nil {
		panic(err)
	}
	defer sightpane.Flush(2 * time.Second)

	// Capture a caught error
	if err := doSomething(); err != nil {
		sightpane.CaptureException(err)
	}
}

func doSomething() error {
	return errors.New("something went wrong")
}
```

---

## Go Runtime Performance Metrics

Sightpane Go includes an integrated, toggleable background poller that monitors Go runtime performance:
- **Goroutines**: Number of active goroutines (`runtime.NumGoroutine()`)
- **Memory Allocations**: `alloc_bytes`, `total_alloc_bytes`, `sys_bytes`, `heap_inuse_bytes`, `stack_inuse_bytes`
- **Garbage Collector**: GC cycle count (`num_gc`), cumulative pause time (`pause_total_ns`), and CPU fraction (`gc_cpu_fraction`)

### Dynamic Start & Stop

You can enable, disable, or manually sample runtime metrics at any time during execution:

```go
// Start or adjust polling interval dynamically
sightpane.StartRuntimeMetrics(15 * time.Second)

// Check if poller is currently running
if sightpane.IsRuntimeMetricsEnabled() {
	println("Runtime metrics poller is active")
}

// Manually capture a one-off runtime snapshot
metrics, ok := sightpane.CaptureRuntimeMetrics()

// Stop / pause the background collector
sightpane.StopRuntimeMetrics()
```

---

## Panic Recovery

Recover from unhandled goroutine panics while ensuring errors are captured in Sightpane:

```go
go func() {
	defer sightpane.Recover()

	// Risky code here
	var ptr *int
	*ptr = 42
}()
```

With context and custom scope options:

```go
func handleJob(ctx context.Context) {
	defer sightpane.RecoverWithContext(ctx,
		sightpane.WithRethrow(false), // don't re-panic
	)

	// Work logic
}
```

---

## Breadcrumbs, User & Tags

Enrich error reports with breadcrumbs and user context:

```go
// Add breadcrumbs
sightpane.AddBreadcrumb(sightpane.Breadcrumb{
	Category: "auth",
	Message:  "User attempted login",
	Level:    sightpane.LevelInfo,
	Data: map[string]any{
		"provider": "oauth2",
	},
})

// Set user info
sightpane.SetUser(sightpane.User{
	ID:       "usr_12345",
	Email:    "user@example.com",
	Username: "alice",
})

// Set tags
sightpane.SetTag("tier", "enterprise")
sightpane.SetExtra("job_id", 9876)
```

---

## Distributed Tracing & APM

Sightpane Go supports transactions, child spans, and standard W3C `traceparent` headers.

```go
// Start a root transaction
tx, ctx := sightpane.StartTransaction(context.Background(), "ProcessOrder", "task")
defer tx.Finish()

// Create child spans
span := tx.StartChild("db.query", "SELECT * FROM orders")
// run query...
span.Finish()

// Get W3C traceparent header for outbound calls:
// e.g. "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
tpHeader := tx.Traceparent()
```

---

## HTTP Integration (`sightpanehttp`)

### Inbound HTTP Middleware

Automatically records transactions, spans, HTTP status codes, extracts incoming `traceparent` headers, and recovers from panics:

```go
import (
	"net/http"
	"github.com/sightpane/sightpane-go/middleware/sightpanehttp"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, World!"))
	})

	// Wrap mux with Sightpane middleware
	handler := sightpanehttp.Middleware(mux)
	http.ListenAndServe(":8080", handler)
}
```

### Outbound HTTP Client (`RoundTripper`)

Automatically injects `traceparent` headers, measures request duration, logs HTTP breadcrumbs, and links child spans:

```go
client := sightpanehttp.WrapClient(&http.Client{
	Timeout: 10 * time.Second,
})

// Outbound request automatically receives W3C traceparent and creates child spans
req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.service.local/data", nil)
resp, err := client.Do(req)
```

---

## Configuration Options

| Option | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `DSN` | `string` | `""` | Sightpane ingest DSN (e.g. `https://<key>@sightpane.cloud/api/v1/envelope` or `key`) |
| `Endpoint` | `string` | `https://sightpane.cloud/api/v1/envelope` | Custom ingest API endpoint |
| `ProjectKey` | `string` | `""` | Sightpane API project key |
| `Environment`| `string` | `"production"` | Deployment environment (`production`, `staging`, `dev`) |
| `Release` | `string` | `""` | Application version or commit hash |
| `SampleRate` | `float64`| `1.0` | Sampling rate for errors (0.0 to 1.0) |
| `TracesSampleRate` | `float64`| `1.0` | Sampling rate for distributed traces (0.0 to 1.0) |
| `EnableRuntimeMetrics` | `bool` | `false` | Enables periodic Go runtime metrics collection (goroutines, memory, GC) |
| `RuntimeMetricsInterval` | `time.Duration` | `30s` | Polling frequency for runtime metrics (minimum 500ms) |
| `MaxQueueSize` | `int` | `1000` | Maximum items in the asynchronous queue |
| `MaxBatchSize` | `int` | `50` | Maximum items per envelope payload |
| `FlushInterval` | `time.Duration` | `2s` | Flush interval for queued items |
| `BeforeSend` | `func(any) any` | `nil` | Callback to mutate or drop items prior to sending |

---

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
