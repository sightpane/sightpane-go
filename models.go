// Package sightpane provides the official Go SDK for Sightpane error tracking,
// performance monitoring, and analytics.
package sightpane

import (
	"encoding/json"
	"time"
)

// Level indicates the severity of a message or breadcrumb.
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
	LevelFatal Level = "fatal"
)

// Envelope is the top-level payload sent to the Sightpane Ingest API.
type Envelope struct {
	SDK     SDKInfo           `json:"sdk"`
	Session SessionInfo       `json:"session"`
	Items   []json.RawMessage `json:"items"`
}

// SDKInfo contains metadata identifying the SDK.
type SDKInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// SessionInfo carries session metadata and context.
type SessionInfo struct {
	ID        string          `json:"id"`
	StartedAt string          `json:"started_at"`
	User      json.RawMessage `json:"user,omitempty"`
	Device    json.RawMessage `json:"device,omitempty"`
	Props     json.RawMessage `json:"props,omitempty"`
}

// User represents the user associated with an event or session.
type User struct {
	ID       string         `json:"id,omitempty"`
	Email    string         `json:"email,omitempty"`
	Username string         `json:"username,omitempty"`
	Name     string         `json:"name,omitempty"`
	Role     string         `json:"role,omitempty"`
	IP       string         `json:"ip_address,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// Device contains system and runtime environment metadata.
type Device struct {
	Platform         string `json:"platform"`
	PlatformCategory string `json:"platform_category"`
	AppType          string `json:"app_type"`
	OS               string `json:"os"`
	OSVersion        string `json:"os_version,omitempty"`
	Arch             string `json:"arch"`
	Hostname         string `json:"hostname,omitempty"`
	GoVersion        string `json:"go_version"`
	Environment      string `json:"environment,omitempty"`
	Release          string `json:"release,omitempty"`
}

// Frame represents a single stack frame.
type Frame struct {
	Function string `json:"function"`
	Filename string `json:"filename"`
	Lineno   int    `json:"lineno"`
	Colno    int    `json:"colno,omitempty"`
}

// ErrorItem represents an error or panic event.
type ErrorItem struct {
	Type        string            `json:"type"`
	TS          string            `json:"ts"`
	Name        string            `json:"name,omitempty"`
	Message     string            `json:"message"`
	Exception   string            `json:"exception,omitempty"`
	Stack       string            `json:"stack,omitempty"`
	Frames      []Frame           `json:"frames,omitempty"`
	Handled     bool              `json:"handled"`
	Category    string            `json:"category,omitempty"`
	Route       string            `json:"route,omitempty"`
	User        *User             `json:"user,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	Extra       map[string]any    `json:"extra,omitempty"`
	Breadcrumbs []Breadcrumb      `json:"breadcrumbs,omitempty"`
}

// EventItem represents an analytics or product event.
type EventItem struct {
	Type  string         `json:"type"`
	TS    string         `json:"ts"`
	Name  string         `json:"name"`
	Props map[string]any `json:"props,omitempty"`
	User  *User          `json:"user,omitempty"`
}

// Breadcrumb represents a trail of events leading up to an error.
type Breadcrumb struct {
	Type     string         `json:"type"`
	TS       string         `json:"ts"`
	Category string         `json:"category"`
	Message  string         `json:"message"`
	Level    Level          `json:"level,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// SpanItem represents a unit of work in distributed tracing.
type SpanItem struct {
	Type         string         `json:"type"`
	TS           string         `json:"ts"`
	Op           string         `json:"op"`
	Name         string         `json:"name"`
	DurationMs   float64        `json:"duration_ms"`
	Status       string         `json:"status,omitempty"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	SpanID       string         `json:"span_id"`
	TraceID      string         `json:"trace_id"`
	Tags         map[string]any `json:"tags,omitempty"`
	Spans        []SpanChild    `json:"spans,omitempty"`
}

// SpanChild represents a nested child span within a transaction.
type SpanChild struct {
	Op           string         `json:"op"`
	Name         string         `json:"name"`
	TS           string         `json:"ts"`
	DurationMs   float64        `json:"duration_ms"`
	Status       string         `json:"status,omitempty"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	SpanID       string         `json:"span_id"`
	Tags         map[string]any `json:"tags,omitempty"`
}

// HeartbeatItem keeps session alive and updates route without writing an event row.
type HeartbeatItem struct {
	Type  string `json:"type"`
	TS    string `json:"ts"`
	Route string `json:"route,omitempty"`
}

// SessionEndItem explicitly terminates a session.
type SessionEndItem struct {
	Type string `json:"type"`
	TS   string `json:"ts"`
}

func nowISO8601() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
