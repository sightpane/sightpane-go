package sightpane

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type spanContextKey struct{}

// Span represents an active performance tracing unit (a transaction or child span).
type Span struct {
	mu           sync.Mutex
	client       *Client
	parent       *Span
	op           string
	name         string
	traceID      string
	spanID       string
	parentSpanID string
	startTime    time.Time
	durationMs   float64
	status       string
	tags         map[string]any
	children     []*Span
	sampled      bool
	finished     bool
}

// SpanOption configures span properties on creation.
type SpanOption func(*Span)

// WithSpanTags attaches initial tags to the span.
func WithSpanTags(tags map[string]any) SpanOption {
	return func(s *Span) {
		for k, v := range tags {
			s.tags[k] = v
		}
	}
}

// WithParentTraceparent seeds trace IDs from an incoming W3C traceparent header.
func WithParentTraceparent(traceparent string) SpanOption {
	return func(s *Span) {
		traceID, parentID, sampled := ParseTraceparent(traceparent)
		if traceID != "" {
			s.traceID = traceID
			s.parentSpanID = parentID
			s.sampled = sampled
		}
	}
}

// StartTransaction begins a root span (transaction) and embeds it into the context.
func (c *Client) StartTransaction(ctx context.Context, name, op string, opts ...SpanOption) (*Span, context.Context) {
	s := &Span{
		client:    c,
		op:        op,
		name:      name,
		traceID:   generateID(16), // 32 hex characters
		spanID:    generateID(8),  // 16 hex characters
		startTime: time.Now(),
		status:    "ok",
		tags:      make(map[string]any),
		sampled:   true,
	}

	for _, opt := range opts {
		opt(s)
	}

	newCtx := context.WithValue(ctx, spanContextKey{}, s)
	return s, newCtx
}

// StartChild creates a child span linked to this span.
func (s *Span) StartChild(op, name string, opts ...SpanOption) *Span {
	s.mu.Lock()
	defer s.mu.Unlock()

	child := &Span{
		client:       s.client,
		parent:       s,
		op:           op,
		name:         name,
		traceID:      s.traceID,
		spanID:       generateID(8),
		parentSpanID: s.spanID,
		startTime:    time.Now(),
		status:       "ok",
		tags:         make(map[string]any),
		sampled:      s.sampled,
	}

	for _, opt := range opts {
		opt(child)
	}

	s.children = append(s.children, child)
	return child
}

// SetTag adds a tag to the span.
func (s *Span) SetTag(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tags[key] = value
}

// SetStatus updates the completion status of the span (e.g. "ok", "error", "cancelled").
func (s *Span) SetStatus(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

// Traceparent formats the W3C traceparent header string for distributed tracing propagation.
func (s *Span) Traceparent() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	sampledFlag := "00"
	if s.sampled {
		sampledFlag = "01"
	}
	return fmt.Sprintf("00-%s-%s-%s", s.traceID, s.spanID, sampledFlag)
}

// Finish records duration and, if this is a root transaction, dispatches it to the ingest queue.
func (s *Span) Finish() {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	s.durationMs = float64(time.Since(s.startTime).Microseconds()) / 1000.0
	isRoot := s.parent == nil
	s.mu.Unlock()

	if isRoot && s.client != nil && s.sampled {
		s.client.enqueueSpan(s)
	}
}

func (s *Span) toSpanItem() SpanItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := SpanItem{
		Type:         "span",
		TS:           s.startTime.UTC().Format(time.RFC3339Nano),
		Op:           s.op,
		Name:         s.name,
		DurationMs:   s.durationMs,
		Status:       s.status,
		ParentSpanID: s.parentSpanID,
		SpanID:       s.spanID,
		TraceID:      s.traceID,
		Tags:         s.tags,
	}

	for _, c := range s.children {
		c.mu.Lock()
		item.Spans = append(item.Spans, SpanChild{
			Op:           c.op,
			Name:         c.name,
			TS:           c.startTime.UTC().Format(time.RFC3339Nano),
			DurationMs:   c.durationMs,
			Status:       c.status,
			ParentSpanID: c.parentSpanID,
			SpanID:       c.spanID,
			Tags:         c.tags,
		})
		c.mu.Unlock()
	}

	return item
}

// SpanFromContext retrieves the currently active span in context, or nil if none.
func SpanFromContext(ctx context.Context) *Span {
	if ctx == nil {
		return nil
	}
	if s, ok := ctx.Value(spanContextKey{}).(*Span); ok {
		return s
	}
	return nil
}

// ParseTraceparent extracts trace ID, parent span ID, and sampled flag from a W3C traceparent header.
func ParseTraceparent(header string) (traceID, parentSpanID string, sampled bool) {
	header = strings.TrimSpace(header)
	parts := strings.Split(header, "-")
	if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 {
		return "", "", false
	}
	return parts[1], parts[2], parts[3] == "01"
}
