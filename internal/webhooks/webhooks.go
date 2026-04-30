// Package webhooks fires HTTP webhooks for VulpineOS events.
package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// EventType identifies the kind of webhook event.
type EventType string

const (
	AgentCompleted    EventType = "agent.completed"
	AgentFailed       EventType = "agent.failed"
	AgentPaused       EventType = "agent.paused"
	AgentInterrupted  EventType = "agent.interrupted"
	RateLimitDetected EventType = "rate_limit.detected"
	InjectionDetected EventType = "injection.detected"
	BudgetAlert       EventType = "budget.alert"
	BudgetExceeded    EventType = "budget.exceeded"
)

var supportedEvents = map[EventType]struct{}{
	AgentCompleted:    {},
	AgentFailed:       {},
	AgentPaused:       {},
	AgentInterrupted:  {},
	RateLimitDetected: {},
	InjectionDetected: {},
	BudgetAlert:       {},
	BudgetExceeded:    {},
}

var (
	webhookLogQuerySecretPattern = regexp.MustCompile(`(?i)([?&](?:api[_-]?key|apikey|token|access[_-]?token|access[_-]?key|secret|password|credential|authorization|cookie|session)=)[^&#\s"]+`)
	webhookLogUserinfoPattern    = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s"@]+:[^/\s"@]+@`)
	webhookLogKeyValuePattern    = regexp.MustCompile(`(?i)(^|[^?&A-Za-z0-9_])((?:api[_-]?key|apikey|token|access[_-]?token|access[_-]?key|secret|password|credential|authorization|cookie|session)\s*=\s*)[^\s,;"]+`)
)

// SupportedEvent reports whether event is a built-in webhook event type.
func SupportedEvent(event EventType) bool {
	_, ok := supportedEvents[event]
	return ok
}

// Webhook is a registered webhook endpoint.
type Webhook struct {
	ID     string      `json:"id"`
	URL    string      `json:"url"`
	Events []EventType `json:"events"` // empty = all events
	Secret string      `json:"secret,omitempty"`
	Active bool        `json:"active"`
}

// Payload is the body sent to webhook endpoints.
type Payload struct {
	Event     EventType              `json:"event"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// Manager handles webhook registration and delivery.
type Manager struct {
	mu       sync.RWMutex
	webhooks []Webhook
	client   *http.Client
	counter  int
}

// New creates a new webhook manager.
func New() *Manager {
	return &Manager{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Register adds a webhook endpoint.
func (m *Manager) Register(url string, events []EventType, secret string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	id := fmt.Sprintf("wh-%d", m.counter)
	m.webhooks = append(m.webhooks, Webhook{
		ID:     id,
		URL:    url,
		Events: events,
		Secret: secret,
		Active: true,
	})
	return id
}

// Unregister removes a webhook.
func (m *Manager) Unregister(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, w := range m.webhooks {
		if w.ID == id {
			m.webhooks = append(m.webhooks[:i], m.webhooks[i+1:]...)
			return
		}
	}
}

// List returns all registered webhooks.
func (m *Manager) List() []Webhook {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Webhook, len(m.webhooks))
	copy(result, m.webhooks)
	return result
}

// Fire sends an event to all matching webhooks.
func (m *Manager) Fire(event EventType, data map[string]interface{}) {
	m.mu.RLock()
	var targets []Webhook
	for _, w := range m.webhooks {
		if !w.Active {
			continue
		}
		if len(w.Events) == 0 || containsEvent(w.Events, event) {
			targets = append(targets, w)
		}
	}
	m.mu.RUnlock()

	if len(targets) == 0 {
		return
	}

	payload := Payload{
		Event:     event,
		Timestamp: time.Now(),
		Data:      data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("webhooks: marshal error: %v", err)
		return
	}

	for _, wh := range targets {
		go m.deliver(wh, event, body)
	}
}

func (m *Manager) deliver(wh Webhook, event EventType, body []byte) {
	safeURL := redactWebhookURLForLog(wh.URL)
	req, err := http.NewRequest("POST", wh.URL, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhooks: request error for %s: %s", safeURL, redactWebhookLogText(err.Error()))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-VulpineOS-Event", string(event))
	if wh.Secret != "" {
		req.Header.Set("X-VulpineOS-Secret", wh.Secret)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		log.Printf("webhooks: delivery failed to %s: %s", safeURL, redactWebhookLogText(err.Error()))
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("webhooks: %s returned %d", safeURL, resp.StatusCode)
	}
}

func redactWebhookURLForLog(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return redactWebhookLogText(raw)
	}
	if parsed.User != nil {
		parsed.User = url.UserPassword("redacted", "redacted")
	}
	query := parsed.Query()
	redacted := false
	for key := range query {
		if sensitiveWebhookURLKey(key) {
			query.Set(key, "[redacted]")
			redacted = true
		}
	}
	if redacted {
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func redactWebhookLogText(value string) string {
	value = webhookLogUserinfoPattern.ReplaceAllString(value, "${1}redacted:redacted@")
	value = webhookLogQuerySecretPattern.ReplaceAllString(value, "${1}[redacted]")
	value = webhookLogKeyValuePattern.ReplaceAllString(value, "${1}${2}[redacted]")
	return value
}

func sensitiveWebhookURLKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	for _, marker := range []string{"api_key", "apikey", "token", "secret", "password", "credential", "authorization", "cookie", "session"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func containsEvent(events []EventType, target EventType) bool {
	for _, e := range events {
		if e == target {
			return true
		}
	}
	return false
}
