package sightpane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Transport defines the interface for delivering Envelopes to Sightpane.
type Transport interface {
	Send(ctx context.Context, env *Envelope) error
}

type httpTransport struct {
	endpoint   string
	projectKey string
	client     *http.Client
	debug      bool
}

// newHTTPTransport creates an HTTP transport for Sightpane.
func newHTTPTransport(opts Options) *httpTransport {
	return &httpTransport{
		endpoint:   opts.Endpoint,
		projectKey: opts.ProjectKey,
		client:     opts.HTTPClient,
		debug:      opts.Debug,
	}
}

func (t *httpTransport) Send(ctx context.Context, env *Envelope) error {
	if env == nil || len(env.Items) == 0 {
		return nil
	}

	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("sightpane transport marshal: %w", err)
	}

	url := t.endpoint + "/api/v1/envelope"

	// Retry loop for transient network/server failures
	const maxRetries = 2
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt*100) * time.Millisecond):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("sightpane transport request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Sightpane-Key", t.projectKey)
		req.Header.Set("User-Agent", fmt.Sprintf("%s/%s", SDKName, SDKVersion))

		resp, err := t.client.Do(req)
		if err != nil {
			lastErr = err
			if t.debug {
				log.Printf("sightpane: transport request error (attempt %d): %v", attempt+1, err)
			}
			continue
		}

		// Drain and close body
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusOK {
			if t.debug {
				log.Printf("sightpane: delivered %d items (status %d)", len(env.Items), resp.StatusCode)
			}
			return nil
		}

		lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		// Don't retry client 4xx errors (e.g. 401 Unauthorized, 403 Forbidden, 400 Bad Request)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return lastErr
		}
	}

	return lastErr
}
