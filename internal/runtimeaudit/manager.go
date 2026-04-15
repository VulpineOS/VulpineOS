package runtimeaudit

import (
	"sync"

	"vulpineos/internal/vault"
)

// Manager persists and broadcasts recent runtime lifecycle events.
type Manager struct {
	vault *vault.DB

	mu    sync.RWMutex
	subs  map[chan vault.RuntimeEvent]struct{}
	close bool
}

// New creates a runtime audit manager.
func New(v *vault.DB) *Manager {
	return &Manager{
		vault: v,
		subs:  make(map[chan vault.RuntimeEvent]struct{}),
	}
}

// Log persists a runtime event and broadcasts it to subscribers.
func (m *Manager) Log(component, level, event, message string, metadata map[string]string) (*vault.RuntimeEvent, error) {
	if m == nil || m.vault == nil {
		return nil, nil
	}
	entry, err := m.vault.AppendRuntimeEvent(component, level, event, message, metadata)
	if err != nil || entry == nil {
		return entry, err
	}

	m.mu.RLock()
	for ch := range m.subs {
		select {
		case ch <- *entry:
		default:
		}
	}
	m.mu.RUnlock()
	return entry, nil
}

// List returns the most recent persisted runtime events.
func (m *Manager) List(limit int) ([]vault.RuntimeEvent, error) {
	if m == nil || m.vault == nil {
		return nil, nil
	}
	return m.vault.ListRuntimeEvents(limit)
}

// Subscribe returns a live event channel.
func (m *Manager) Subscribe() <-chan vault.RuntimeEvent {
	ch := make(chan vault.RuntimeEvent, 32)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.close {
		close(ch)
		return ch
	}
	m.subs[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a live event subscriber.
func (m *Manager) Unsubscribe(ch chan vault.RuntimeEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.subs[ch]; exists {
		delete(m.subs, ch)
		close(ch)
	}
}

// Close closes all subscribers.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.close {
		return
	}
	m.close = true
	for ch := range m.subs {
		close(ch)
		delete(m.subs, ch)
	}
}
