package foxbridge

import (
	"encoding/json"
	"fmt"
	"sync"

	"vulpineos/internal/juggler"

	"github.com/PopcornDev1/foxbridge/pkg/backend"
)

type jugglerBackend interface {
	Call(sessionID, method string, params interface{}) (json.RawMessage, error)
	Subscribe(event string, handler juggler.EventHandler)
}

type scopedBackend struct {
	client           jugglerBackend
	browserContextID string

	mu              sync.RWMutex
	allowedSessions map[string]struct{}
	allowedTargets  map[string]struct{}
}

var _ backend.Backend = (*scopedBackend)(nil)

func newScopedBackend(client jugglerBackend, browserContextID string) *scopedBackend {
	return &scopedBackend{
		client:           client,
		browserContextID: browserContextID,
		allowedSessions:  make(map[string]struct{}),
		allowedTargets:   make(map[string]struct{}),
	}
}

func (b *scopedBackend) Call(sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	switch method {
	case "Browser.createBrowserContext":
		return json.Marshal(map[string]string{"browserContextId": b.browserContextID})
	case "Browser.removeBrowserContext":
		return json.RawMessage(`{}`), nil
	case "Browser.newPage":
		withContext, err := b.injectBrowserContext(params)
		if err != nil {
			return nil, err
		}
		return b.client.Call(sessionID, method, withContext)
	default:
		return b.client.Call(sessionID, method, params)
	}
}

func (b *scopedBackend) Subscribe(event string, handler backend.EventHandler) {
	b.client.Subscribe(event, func(sessionID string, params json.RawMessage) {
		if !b.shouldForward(event, sessionID, params) {
			return
		}
		handler(sessionID, params)
	})
}

func (b *scopedBackend) Close() error {
	return nil
}

func (b *scopedBackend) injectBrowserContext(params json.RawMessage) (json.RawMessage, error) {
	payload := map[string]interface{}{}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &payload); err != nil {
			return nil, fmt.Errorf("parse Browser.newPage params: %w", err)
		}
	}
	if raw, ok := payload["browserContextId"]; ok {
		if raw != nil && raw != b.browserContextID {
			return nil, fmt.Errorf("browser context %v is outside scoped backend", raw)
		}
	}
	payload["browserContextId"] = b.browserContextID
	return json.Marshal(payload)
}

func (b *scopedBackend) shouldForward(event, sessionID string, params json.RawMessage) bool {
	switch event {
	case "Browser.attachedToTarget":
		var ev struct {
			SessionID  string `json:"sessionId"`
			TargetInfo struct {
				TargetID         string `json:"targetId"`
				BrowserContextID string `json:"browserContextId"`
			} `json:"targetInfo"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			return false
		}
		if ev.TargetInfo.BrowserContextID != b.browserContextID {
			return false
		}
		b.mu.Lock()
		if ev.SessionID != "" {
			b.allowedSessions[ev.SessionID] = struct{}{}
		}
		if ev.TargetInfo.TargetID != "" {
			b.allowedTargets[ev.TargetInfo.TargetID] = struct{}{}
		}
		b.mu.Unlock()
		return true

	case "Browser.detachedFromTarget":
		var ev struct {
			SessionID string `json:"sessionId"`
			TargetID  string `json:"targetId"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			return false
		}
		b.mu.Lock()
		_, sessionAllowed := b.allowedSessions[ev.SessionID]
		_, targetAllowed := b.allowedTargets[ev.TargetID]
		if sessionAllowed {
			delete(b.allowedSessions, ev.SessionID)
		}
		if targetAllowed {
			delete(b.allowedTargets, ev.TargetID)
		}
		b.mu.Unlock()
		return sessionAllowed || targetAllowed
	}

	if sessionID == "" {
		return false
	}

	b.mu.RLock()
	_, ok := b.allowedSessions[sessionID]
	b.mu.RUnlock()
	return ok
}
