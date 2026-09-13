package sightpane

import (
	"context"
)

// RecoverOption configures panic recovery behavior.
type RecoverOption func(*recoverConfig)

type recoverConfig struct {
	scope    *Scope
	callback func(p any)
	rethrow  bool
}

// WithRecoverScope attaches a specific scope to the captured panic.
func WithRecoverScope(s *Scope) RecoverOption {
	return func(cfg *recoverConfig) {
		cfg.scope = s
	}
}

// WithPanicCallback registers a function invoked when a panic is recovered.
func WithPanicCallback(fn func(p any)) RecoverOption {
	return func(cfg *recoverConfig) {
		cfg.callback = fn
	}
}

// WithRethrow causes the panic to be re-panicked after being captured.
func WithRethrow(rethrow bool) RecoverOption {
	return func(cfg *recoverConfig) {
		cfg.rethrow = rethrow
	}
}

// Recover handles panics in deferred calls using the default client.
//
// Usage:
//
//	defer sightpane.Recover()
func Recover(opts ...RecoverOption) {
	if r := recover(); r != nil {
		handleRecover(nil, r, opts...)
	}
}

// RecoverWithContext handles panics with request context scope.
//
// Usage:
//
//	defer sightpane.RecoverWithContext(ctx)
func RecoverWithContext(ctx context.Context, opts ...RecoverOption) {
	if r := recover(); r != nil {
		handleRecover(ctx, r, opts...)
	}
}

func handleRecover(ctx context.Context, p any, opts ...RecoverOption) {
	cfg := &recoverConfig{
		rethrow: true,
	}

	if ctx != nil {
		cfg.scope = ScopeFromContext(ctx)
	}

	for _, opt := range opts {
		opt(cfg)
	}

	client := CurrentClient()
	if client != nil {
		client.capturePanic(p, cfg.scope)
	}

	if cfg.callback != nil {
		cfg.callback(p)
	}

	if cfg.rethrow {
		panic(p)
	}
}
