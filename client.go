package sightpane

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"sync"
	"time"
)

// Client is the primary interface to the Sightpane observability service.
type Client struct {
	options   Options
	transport Transport
	queue     *eventQueue
	scope     *Scope

	metricsMu     sync.Mutex
	metricsCancel context.CancelFunc
	metricsActive bool
}

// NewClient initializes a new Sightpane client with the specified options.
func NewClient(opts Options) (*Client, error) {
	if err := opts.normalize(); err != nil {
		return nil, err
	}

	transport := newHTTPTransport(opts)
	queue := newEventQueue(transport, opts)

	client := &Client{
		options:   opts,
		transport: transport,
		queue:     queue,
		scope:     NewScope(),
	}

	if opts.EnableRuntimeMetrics {
		client.StartRuntimeMetrics(opts.RuntimeMetricsInterval)
	}

	return client, nil
}

// Options returns the active client options.
func (c *Client) Options() Options {
	return c.options
}

// Scope returns the client's global scope.
func (c *Client) Scope() *Scope {
	return c.scope
}

// StartRuntimeMetrics enables periodic background collection and ingestion of Go runtime metrics.
// If interval is provided, it sets the ticker duration (minimum 500ms).
func (c *Client) StartRuntimeMetrics(interval ...time.Duration) {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if c.metricsActive && c.metricsCancel != nil {
		c.metricsCancel()
		c.metricsCancel = nil
		c.metricsActive = false
	}

	d := c.options.RuntimeMetricsInterval
	if len(interval) > 0 && interval[0] > 0 {
		d = interval[0]
	}
	if d < 500*time.Millisecond {
		d = 500 * time.Millisecond
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.metricsCancel = cancel
	c.metricsActive = true

	go c.runRuntimeMetricsWorker(ctx, d)
}

// StopRuntimeMetrics pauses or disables the periodic collection of runtime metrics.
func (c *Client) StopRuntimeMetrics() {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	if !c.metricsActive {
		return
	}
	if c.metricsCancel != nil {
		c.metricsCancel()
		c.metricsCancel = nil
	}
	c.metricsActive = false
}

// IsRuntimeMetricsEnabled reports whether the background runtime metrics poller is currently active.
func (c *Client) IsRuntimeMetricsEnabled() bool {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()
	return c.metricsActive
}

// CaptureRuntimeMetrics reads the current runtime metrics and immediately enqueues them as an event.
func (c *Client) CaptureRuntimeMetrics() RuntimeMetrics {
	rm := ReadRuntimeMetrics()
	c.CaptureEvent("runtime_metrics", rm.ToProps(), nil)
	return rm
}

func (c *Client) runRuntimeMetricsWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.CaptureRuntimeMetrics()
		}
	}
}

// CaptureException logs an error with stack trace and scope metadata.
func (c *Client) CaptureException(err error, scope *Scope) string {
	if err == nil || !c.shouldSample(c.options.SampleRate) {
		return ""
	}

	frames, stackStr := captureStack(2)

	errType := reflect.TypeOf(err).String()
	eventID := generateID(16)

	item := &ErrorItem{
		Type:      "error",
		TS:        nowISO8601(),
		Name:      errType,
		Message:   err.Error(),
		Exception: errType,
		Stack:     stackStr,
		Frames:    frames,
		Handled:   true,
	}

	// Apply scope
	effectiveScope := c.scope.Clone()
	if scope != nil {
		scope.ApplyToError(item)
	}
	effectiveScope.ApplyToError(item)

	c.enqueueItem(item)
	return eventID
}

// CaptureMessage logs a simple informational or warning message.
func (c *Client) CaptureMessage(message string, level Level, scope *Scope) string {
	if message == "" || !c.shouldSample(c.options.SampleRate) {
		return ""
	}

	eventID := generateID(16)
	frames, stackStr := captureStack(2)

	item := &ErrorItem{
		Type:     "error",
		TS:       nowISO8601(),
		Name:     string(level),
		Message:  message,
		Category: "message",
		Stack:    stackStr,
		Frames:   frames,
		Handled:  true,
	}

	effectiveScope := c.scope.Clone()
	if scope != nil {
		scope.ApplyToError(item)
	}
	effectiveScope.ApplyToError(item)

	c.enqueueItem(item)
	return eventID
}

// CaptureEvent sends an analytics or tracking event.
func (c *Client) CaptureEvent(name string, props map[string]any, scope *Scope) {
	if name == "" {
		return
	}

	item := &EventItem{
		Type:  "event",
		TS:    nowISO8601(),
		Name:  name,
		Props: props,
	}

	if scope != nil && scope.user != nil {
		item.User = scope.user
	} else if c.scope.user != nil {
		item.User = c.scope.user
	}

	c.enqueueItem(item)
}

// AddBreadcrumb records a breadcrumb to the global or provided scope.
func (c *Client) AddBreadcrumb(b Breadcrumb, scope *Scope) {
	if scope != nil {
		scope.AddBreadcrumb(b)
	} else {
		c.scope.AddBreadcrumb(b)
	}
}

// Flush dispatches all queued items, waiting up to the given timeout.
func (c *Client) Flush(timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := c.queue.Flush(ctx)
	if err != nil && c.options.Debug {
		log.Printf("sightpane: flush returned error: %v", err)
	}
	return err == nil
}

// Close stops runtime metric collection, flushes pending items, and shuts down the background worker.
func (c *Client) Close() error {
	c.StopRuntimeMetrics()
	return c.queue.Close()
}

func (c *Client) enqueueSpan(s *Span) {
	if s == nil || !c.shouldSample(c.options.TracesSampleRate) {
		return
	}
	item := s.toSpanItem()
	c.enqueueItem(item)
}

func (c *Client) enqueueItem(raw any) {
	if c.options.BeforeSend != nil {
		raw = c.options.BeforeSend(raw)
		if raw == nil {
			return
		}
	}

	data, err := json.Marshal(raw)
	if err != nil {
		if c.options.Debug {
			log.Printf("sightpane: failed to marshal item: %v", err)
		}
		return
	}

	c.queue.Enqueue(data)
}

func (c *Client) capturePanic(p any, scope *Scope) string {
	frames, stackStr := captureStack(3)

	var msg string
	if err, ok := p.(error); ok {
		msg = err.Error()
	} else {
		msg = fmt.Sprint(p)
	}

	item := &ErrorItem{
		Type:      "error",
		TS:        nowISO8601(),
		Name:      "panic",
		Message:   msg,
		Exception: "panic",
		Stack:     stackStr,
		Frames:    frames,
		Handled:   false,
	}

	effectiveScope := c.scope.Clone()
	if scope != nil {
		scope.ApplyToError(item)
	}
	effectiveScope.ApplyToError(item)

	c.enqueueItem(item)

	// Immediately flush on unhandled panic
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.queue.Flush(ctx)

	return generateID(16)
}

func (c *Client) shouldSample(rate float64) bool {
	if rate >= 1.0 {
		return true
	}
	if rate <= 0.0 {
		return false
	}
	return rand.Float64() < rate
}
