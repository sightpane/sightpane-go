package sightpane

import (
	"context"
	"sync"
	"time"
)

var (
	globalMu     sync.RWMutex
	globalClient *Client
)

// Init initializes the global Sightpane client.
func Init(opts Options) error {
	client, err := NewClient(opts)
	if err != nil {
		return err
	}

	globalMu.Lock()
	if globalClient != nil {
		_ = globalClient.Close()
	}
	globalClient = client
	globalMu.Unlock()

	return nil
}

// CurrentClient returns the globally initialized client, or nil.
func CurrentClient() *Client {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalClient
}

// StartRuntimeMetrics enables periodic background collection of Go runtime metrics on the global client.
func StartRuntimeMetrics(interval ...time.Duration) {
	if c := CurrentClient(); c != nil {
		c.StartRuntimeMetrics(interval...)
	}
}

// StopRuntimeMetrics pauses or disables periodic collection of runtime metrics on the global client.
func StopRuntimeMetrics() {
	if c := CurrentClient(); c != nil {
		c.StopRuntimeMetrics()
	}
}

// IsRuntimeMetricsEnabled reports whether the runtime metrics worker is active on the global client.
func IsRuntimeMetricsEnabled() bool {
	if c := CurrentClient(); c != nil {
		return c.IsRuntimeMetricsEnabled()
	}
	return false
}

// CaptureRuntimeMetrics reads current runtime metrics and enqueues them on the global client.
func CaptureRuntimeMetrics() (RuntimeMetrics, bool) {
	if c := CurrentClient(); c != nil {
		return c.CaptureRuntimeMetrics(), true
	}
	return RuntimeMetrics{}, false
}

// CaptureException reports an error using the global client.
func CaptureException(err error) string {
	c := CurrentClient()
	if c == nil {
		return ""
	}
	return c.CaptureException(err, nil)
}

// CaptureExceptionWithContext reports an error using the context's scope.
func CaptureExceptionWithContext(ctx context.Context, err error) string {
	c := CurrentClient()
	if c == nil {
		return ""
	}
	return c.CaptureException(err, ScopeFromContext(ctx))
}

// CaptureMessage logs a message using the global client.
func CaptureMessage(message string, level Level) string {
	c := CurrentClient()
	if c == nil {
		return ""
	}
	return c.CaptureMessage(message, level, nil)
}

// CaptureEvent sends an analytics event using the global client.
func CaptureEvent(name string, props map[string]any) {
	c := CurrentClient()
	if c == nil {
		return
	}
	c.CaptureEvent(name, props, nil)
}

// AddBreadcrumb records a breadcrumb to the global scope.
func AddBreadcrumb(b Breadcrumb) {
	c := CurrentClient()
	if c == nil {
		return
	}
	c.AddBreadcrumb(b, nil)
}

// SetUser sets user details on the global scope.
func SetUser(u User) {
	c := CurrentClient()
	if c == nil {
		return
	}
	c.Scope().SetUser(u)
}

// SetTag attaches a key-value tag to the global scope.
func SetTag(key, value string) {
	c := CurrentClient()
	if c == nil {
		return
	}
	c.Scope().SetTag(key, value)
}

// SetExtra attaches extra metadata to the global scope.
func SetExtra(key string, val any) {
	c := CurrentClient()
	if c == nil {
		return
	}
	c.Scope().SetExtra(key, val)
}

// StartTransaction starts a distributed tracing root span using the global client.
func StartTransaction(ctx context.Context, name, op string, opts ...SpanOption) (*Span, context.Context) {
	c := CurrentClient()
	if c == nil {
		return nil, ctx
	}
	return c.StartTransaction(ctx, name, op, opts...)
}

// Flush dispatches all queued items on the global client.
func Flush(timeout time.Duration) bool {
	c := CurrentClient()
	if c == nil {
		return true
	}
	return c.Flush(timeout)
}

// Close flushes and stops the global client.
func Close() error {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalClient != nil {
		err := globalClient.Close()
		globalClient = nil
		return err
	}
	return nil
}
