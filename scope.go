package sightpane

import (
	"context"
	"sync"
)

type scopeKey struct{}

// Scope stores contextual data attached to events and errors.
type Scope struct {
	mu          sync.RWMutex
	user        *User
	tags        map[string]string
	extra       map[string]any
	breadcrumbs []Breadcrumb
	maxBread    int
}

// NewScope creates a fresh, thread-safe Scope.
func NewScope() *Scope {
	return &Scope{
		tags:     make(map[string]string),
		extra:    make(map[string]any),
		maxBread: defaultMaxBreadcrumb,
	}
}

// Clone creates a deep copy of the scope for request/goroutine isolation.
func (s *Scope) Clone() *Scope {
	if s == nil {
		return NewScope()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	c := &Scope{
		maxBread: s.maxBread,
		tags:     make(map[string]string, len(s.tags)),
		extra:    make(map[string]any, len(s.extra)),
	}
	for k, v := range s.tags {
		c.tags[k] = v
	}
	for k, v := range s.extra {
		c.extra[k] = v
	}
	if s.user != nil {
		u := *s.user
		c.user = &u
	}
	c.breadcrumbs = make([]Breadcrumb, len(s.breadcrumbs))
	copy(c.breadcrumbs, s.breadcrumbs)
	return c
}

// SetUser assigns user context to the scope.
func (s *Scope) SetUser(u User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.user = &u
}

// SetTag attaches a key-value tag.
func (s *Scope) SetTag(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tags[key] = value
}

// SetTags assigns multiple key-value tags.
func (s *Scope) SetTags(tags map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range tags {
		s.tags[k] = v
	}
}

// SetExtra attaches arbitrary extra metadata.
func (s *Scope) SetExtra(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.extra[key] = val
}

// AddBreadcrumb records a breadcrumb to the ring buffer.
func (s *Scope) AddBreadcrumb(b Breadcrumb) {
	if b.TS == "" {
		b.TS = nowISO8601()
	}
	if b.Type == "" {
		b.Type = "breadcrumb"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	limit := s.maxBread
	if limit <= 0 {
		limit = defaultMaxBreadcrumb
	}

	if len(s.breadcrumbs) >= limit {
		s.breadcrumbs = s.breadcrumbs[1:]
	}
	s.breadcrumbs = append(s.breadcrumbs, b)
}

// ClearBreadcrumbs resets all recorded breadcrumbs.
func (s *Scope) ClearBreadcrumbs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.breadcrumbs = s.breadcrumbs[:0]
}

// ApplyToError copies scope metadata into an ErrorItem.
func (s *Scope) ApplyToError(errItem *ErrorItem) {
	if s == nil || errItem == nil {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if errItem.User == nil && s.user != nil {
		u := *s.user
		errItem.User = &u
	}

	if len(s.tags) > 0 {
		if errItem.Tags == nil {
			errItem.Tags = make(map[string]string, len(s.tags))
		}
		for k, v := range s.tags {
			if _, exists := errItem.Tags[k]; !exists {
				errItem.Tags[k] = v
			}
		}
	}

	if len(s.extra) > 0 {
		if errItem.Extra == nil {
			errItem.Extra = make(map[string]any, len(s.extra))
		}
		for k, v := range s.extra {
			if _, exists := errItem.Extra[k]; !exists {
				errItem.Extra[k] = v
			}
		}
	}

	if len(s.breadcrumbs) > 0 && len(errItem.Breadcrumbs) == 0 {
		errItem.Breadcrumbs = make([]Breadcrumb, len(s.breadcrumbs))
		copy(errItem.Breadcrumbs, s.breadcrumbs)
	}
}

// ContextWithScope embeds a scope into a context.
func ContextWithScope(ctx context.Context, scope *Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// ScopeFromContext retrieves or creates an isolated scope from context.
func ScopeFromContext(ctx context.Context) *Scope {
	if ctx == nil {
		return NewScope()
	}
	if sc, ok := ctx.Value(scopeKey{}).(*Scope); ok && sc != nil {
		return sc
	}
	return NewScope()
}
