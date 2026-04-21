package remote

import (
	"slices"
	"sync"
)

// ContextInfo is the server-side view of a browser context.
type ContextInfo struct {
	ID      string `json:"id"`
	Pages   int    `json:"pages"`
	LastURL string `json:"url,omitempty"`
}

// ContextRegistry tracks created contexts and attached targets so the panel can
// render context state without depending on client-side event history.
type ContextRegistry struct {
	mu               sync.RWMutex
	contexts         map[string]*ContextInfo
	sessionToContext map[string]string
}

// NewContextRegistry creates an empty context registry.
func NewContextRegistry() *ContextRegistry {
	return &ContextRegistry{
		contexts:         make(map[string]*ContextInfo),
		sessionToContext: make(map[string]string),
	}
}

// Created records a newly created browser context.
func (r *ContextRegistry) Created(contextID string) {
	if contextID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.contexts[contextID]; !ok {
		r.contexts[contextID] = &ContextInfo{ID: contextID}
	}
}

// Removed removes a browser context from the registry.
func (r *ContextRegistry) Removed(contextID string) {
	if contextID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.contexts, contextID)
	for sessionID, mapped := range r.sessionToContext {
		if mapped == contextID {
			delete(r.sessionToContext, sessionID)
		}
	}
}

// Attached records a target attaching to a browser context.
func (r *ContextRegistry) Attached(sessionID, contextID, url string) {
	if sessionID == "" || contextID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	ctx, ok := r.contexts[contextID]
	if !ok {
		ctx = &ContextInfo{ID: contextID}
		r.contexts[contextID] = ctx
	}

	if prevContextID, ok := r.sessionToContext[sessionID]; ok {
		if prevContextID != contextID {
			if prev := r.contexts[prevContextID]; prev != nil && prev.Pages > 0 {
				prev.Pages--
			}
		}
	} else {
		ctx.Pages++
	}

	if url != "" {
		ctx.LastURL = url
	}
	r.sessionToContext[sessionID] = contextID
}

// Detached removes an attached session from its context page count.
func (r *ContextRegistry) Detached(sessionID string) {
	if sessionID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	contextID, ok := r.sessionToContext[sessionID]
	if !ok {
		return
	}
	if ctx := r.contexts[contextID]; ctx != nil && ctx.Pages > 0 {
		ctx.Pages--
	}
	delete(r.sessionToContext, sessionID)
}

// List returns a stable snapshot of tracked contexts.
func (r *ContextRegistry) List() []ContextInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]ContextInfo, 0, len(r.contexts))
	for _, ctx := range r.contexts {
		out = append(out, *ctx)
	}
	slices.SortFunc(out, func(a, b ContextInfo) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return out
}

// SessionForContext returns one attached session ID for the given browser context.
func (r *ContextRegistry) SessionForContext(contextID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for sessionID, mapped := range r.sessionToContext {
		if mapped == contextID {
			return sessionID
		}
	}
	return ""
}
