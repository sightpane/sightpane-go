package sightpane

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	SDKName    = "sightpane-go"
	SDKVersion = "0.1.0"

	defaultFlushInterval = 2 * time.Second
	defaultMaxBatchSize  = 50
	defaultMaxQueueSize  = 1000
	defaultMaxBreadcrumb = 50
	defaultSampleRate    = 1.0
)

// Options holds configuration for initializing the Sightpane client.
type Options struct {
	// DSN is an optional URL combining the endpoint and project key, e.g.
	// "http://dev@localhost:8790" or "https://<key>@sightpane.cloud"
	DSN string

	// Endpoint is the Sightpane server URL, e.g. "http://localhost:8790"
	Endpoint string

	// ProjectKey is the ingestion key configured for your project in Sightpane (header X-Sightpane-Key).
	ProjectKey string

	// Environment (e.g. "production", "staging", "development").
	Environment string

	// Release / version string (e.g. "v1.2.0" or git commit hash).
	Release string

	// AppType identifies the role of this binary (e.g. "server", "worker", "cli"). Default is "server".
	AppType string

	// ServerName overrides the detected host name.
	ServerName string

	// Debug enables verbose debug logging to stdout.
	Debug bool

	// SampleRate specifies the error and event sampling rate (0.0 to 1.0). Default is 1.0 (100%).
	SampleRate float64

	// TracesSampleRate specifies the performance span sampling rate (0.0 to 1.0). Default is 1.0.
	TracesSampleRate float64

	// MaxBreadcrumbs limits how many breadcrumbs are retained per scope. Default is 50.
	MaxBreadcrumbs int

	// MaxQueueSize is the buffer limit of in-flight items before oldest are dropped. Default is 1000.
	MaxQueueSize int

	// MaxBatchSize is the maximum items bundled into a single envelope. Default is 50.
	MaxBatchSize int

	// FlushInterval is how often pending items are dispatched if batch size is not reached. Default is 2s.
	FlushInterval time.Duration

	// HTTPClient allows providing a custom *http.Client for transport.
	HTTPClient *http.Client

	// BeforeSend is a callback to mutate or filter items before enqueueing.
	// Returning nil drops the item.
	BeforeSend func(item any) any
}

func (o *Options) normalize() error {
	if o.DSN != "" {
		u, err := url.Parse(o.DSN)
		if err != nil {
			return err
		}
		if u.User != nil {
			o.ProjectKey = u.User.Username()
		}
		o.Endpoint = u.Scheme + "://" + u.Host
	}

	if o.Endpoint == "" {
		o.Endpoint = os.Getenv("SIGHTPANE_ENDPOINT")
	}
	if o.ProjectKey == "" {
		o.ProjectKey = os.Getenv("SIGHTPANE_KEY")
	}
	if o.Environment == "" {
		o.Environment = os.Getenv("SIGHTPANE_ENVIRONMENT")
		if o.Environment == "" {
			o.Environment = os.Getenv("ENV")
		}
		if o.Environment == "" {
			o.Environment = "development"
		}
	}
	if o.Release == "" {
		o.Release = os.Getenv("SIGHTPANE_RELEASE")
	}
	if o.AppType == "" {
		o.AppType = "server"
	}
	if o.ServerName == "" {
		h, err := os.Hostname()
		if err == nil {
			o.ServerName = h
		}
	}
	if o.SampleRate <= 0 {
		o.SampleRate = defaultSampleRate
	}
	if o.TracesSampleRate <= 0 {
		o.TracesSampleRate = defaultSampleRate
	}
	if o.MaxBreadcrumbs <= 0 {
		o.MaxBreadcrumbs = defaultMaxBreadcrumb
	}
	if o.MaxQueueSize <= 0 {
		o.MaxQueueSize = defaultMaxQueueSize
	}
	if o.MaxBatchSize <= 0 {
		o.MaxBatchSize = defaultMaxBatchSize
	}
	if o.FlushInterval <= 0 {
		o.FlushInterval = defaultFlushInterval
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{
			Timeout: 10 * time.Second,
		}
	}

	o.Endpoint = strings.TrimRight(o.Endpoint, "/")
	if o.Endpoint == "" {
		return errors.New("sightpane: Endpoint or DSN is required")
	}
	if o.ProjectKey == "" {
		return errors.New("sightpane: ProjectKey or DSN user is required")
	}
	return nil
}
