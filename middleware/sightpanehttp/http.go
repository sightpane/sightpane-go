// Package sightpanehttp provides HTTP middleware and RoundTripper integration for Sightpane.
package sightpanehttp

import (
	"fmt"
	"net/http"
	"time"

	"github.com/sightpane/sightpane-go"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// HandlerOption configures the HTTP middleware.
type HandlerOption func(*handlerConfig)

type handlerConfig struct {
	rethrow bool
}

// WithRethrow controls whether panics are re-thrown after capture (default true).
func WithRethrow(rethrow bool) HandlerOption {
	return func(cfg *handlerConfig) {
		cfg.rethrow = rethrow
	}
}

// Middleware wraps an http.Handler with automatic tracing, panic recovery, and scope enrichment.
func Middleware(next http.Handler, opts ...HandlerOption) http.Handler {
	cfg := &handlerConfig{rethrow: true}
	for _, opt := range opts {
		opt(cfg)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := sightpane.CurrentClient()
		if client == nil {
			next.ServeHTTP(w, r)
			return
		}

		scope := sightpane.NewScope()
		scope.SetTag("http.method", r.Method)
		scope.SetTag("http.url", r.URL.String())
		scope.SetTag("http.host", r.Host)
		if ua := r.UserAgent(); ua != "" {
			scope.SetTag("http.user_agent", ua)
		}

		ctx := sightpane.ContextWithScope(r.Context(), scope)

		var spanOpts []sightpane.SpanOption
		if tp := r.Header.Get("traceparent"); tp != "" {
			spanOpts = append(spanOpts, sightpane.WithParentTraceparent(tp))
		}

		txName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		span, ctx := client.StartTransaction(ctx, txName, "http.server", spanOpts...)
		defer func() {
			if span != nil {
				span.Finish()
			}
		}()

		rw := &responseWriter{ResponseWriter: w}

		defer func() {
			if rec := recover(); rec != nil {
				if span != nil {
					span.SetStatus("error")
					span.SetTag("error", true)
				}
				sightpane.RecoverWithContext(ctx,
					sightpane.WithRecoverScope(scope),
					sightpane.WithRethrow(cfg.rethrow),
				)
				if !cfg.rethrow && rw.statusCode == 0 {
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}
		}()

		next.ServeHTTP(rw, r.WithContext(ctx))

		if rw.statusCode == 0 {
			rw.statusCode = http.StatusOK
		}

		if span != nil {
			span.SetTag("http.status_code", rw.statusCode)
			if rw.statusCode >= 500 {
				span.SetStatus("error")
			} else {
				span.SetStatus("ok")
			}
		}
	})
}

type roundTripper struct {
	base http.RoundTripper
}

// NewRoundTripper returns an http.RoundTripper that attaches W3C traceparent headers,
// logs breadcrumbs, and records child spans for outgoing HTTP requests.
func NewRoundTripper(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &roundTripper{base: base}
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	client := sightpane.CurrentClient()
	if client == nil {
		return rt.base.RoundTrip(req)
	}

	ctx := req.Context()
	activeSpan := sightpane.SpanFromContext(ctx)

	var span *sightpane.Span
	spanName := fmt.Sprintf("%s %s", req.Method, req.URL.String())

	if activeSpan != nil {
		span = activeSpan.StartChild("http.client", spanName)
	} else {
		span, ctx = client.StartTransaction(ctx, spanName, "http.client")
	}

	// Propagate W3C traceparent
	clonedReq := req.Clone(ctx)
	if span != nil {
		clonedReq.Header.Set("traceparent", span.Traceparent())
	}

	start := time.Now()
	resp, err := rt.base.RoundTrip(clonedReq)
	duration := time.Since(start)

	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}

	// Breadcrumb
	client.AddBreadcrumb(sightpane.Breadcrumb{
		Category: "http",
		Message:  fmt.Sprintf("%s %s [%d]", req.Method, req.URL.Path, statusCode),
		Level:    sightpane.LevelInfo,
		Data: map[string]any{
			"url":         req.URL.String(),
			"method":      req.Method,
			"status_code": statusCode,
			"duration_ms": float64(duration.Microseconds()) / 1000.0,
		},
	}, sightpane.ScopeFromContext(ctx))

	if span != nil {
		span.SetTag("http.status_code", statusCode)
		if err != nil || statusCode >= 400 {
			span.SetStatus("error")
		} else {
			span.SetStatus("ok")
		}
		span.Finish()
	}

	return resp, err
}

// WrapClient wraps an http.Client's Transport with Sightpane tracing.
func WrapClient(c *http.Client) *http.Client {
	if c == nil {
		c = &http.Client{}
	}
	c.Transport = NewRoundTripper(c.Transport)
	return c
}
