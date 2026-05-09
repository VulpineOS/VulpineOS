package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type lockedLogBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *lockedLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *lockedLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

func TestRegisterAndList(t *testing.T) {
	m := New()
	id := m.Register("https://example.com/hook", []EventType{AgentCompleted}, "secret")
	if id == "" {
		t.Fatal("Register returned empty ID")
	}
	list := m.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(list))
	}
	if list[0].URL != "https://example.com/hook" {
		t.Error("wrong URL")
	}
	if list[0].Secret != "secret" {
		t.Error("wrong secret")
	}
}

func TestUnregister(t *testing.T) {
	m := New()
	id := m.Register("https://example.com/hook", nil, "")
	m.Unregister(id)
	if len(m.List()) != 0 {
		t.Error("should have 0 webhooks after unregister")
	}
}

func TestFireDelivers(t *testing.T) {
	var received Payload
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	m := New()
	m.Register(srv.URL, nil, "mysecret") // nil events = all events

	m.Fire(AgentCompleted, map[string]interface{}{
		"agentId": "agent-1",
		"result":  "success",
	})

	// Wait for async delivery
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if received.Event != AgentCompleted {
		t.Errorf("event = %s, want agent.completed", received.Event)
	}
	if received.Data["agentId"] != "agent-1" {
		t.Error("missing agentId in data")
	}
}

func TestDeliveryLogsDoNotExposeURLSecrets(t *testing.T) {
	var logs lockedLogBuffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	}()

	m := New()
	m.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf(`Post "https://user:pass@example.com/hook?token=url-token": token=err-token`)
		}),
	}
	m.Register("https://user:pass@example.com/hook?token=url-token&view=events", nil, "")
	m.Fire(AgentCompleted, map[string]interface{}{"agentId": "agent-1"})

	time.Sleep(100 * time.Millisecond)
	out := logs.String()
	for _, leaked := range []string{"user:pass", "url-token", "err-token"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("webhook delivery logs leaked %q: %s", leaked, out)
		}
	}
	if !strings.Contains(out, "redacted:redacted@example.com") || !strings.Contains(out, "token=[redacted]") {
		t.Fatalf("webhook delivery log was not redacted as expected: %s", out)
	}
}

func TestEventFiltering(t *testing.T) {
	var called bool
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		called = true
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	m := New()
	m.Register(srv.URL, []EventType{RateLimitDetected}, "") // only rate limit events

	// Fire a different event — should NOT be delivered
	m.Fire(AgentCompleted, map[string]interface{}{})
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	if called {
		mu.Unlock()
		t.Fatal("webhook should not fire for non-matching event")
	}
	mu.Unlock()

	// Fire matching event — should be delivered
	m.Fire(RateLimitDetected, map[string]interface{}{"url": "example.com"})
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	if !called {
		mu.Unlock()
		t.Fatal("webhook should fire for matching event")
	}
	mu.Unlock()
}

func TestSecretHeader(t *testing.T) {
	var gotSecret string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotSecret = r.Header.Get("X-VulpineOS-Secret")
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	m := New()
	m.Register(srv.URL, nil, "my-secret-key")
	m.Fire(AgentCompleted, nil)

	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	if gotSecret != "my-secret-key" {
		t.Errorf("secret = %q, want my-secret-key", gotSecret)
	}
	mu.Unlock()
}

func TestEventHeaderMatchesPayloadEvent(t *testing.T) {
	var gotEvent string
	var received Payload
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotEvent = r.Header.Get("X-VulpineOS-Event")
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	m := New()
	m.Register(srv.URL, []EventType{AgentInterrupted}, "")
	m.Fire(AgentInterrupted, map[string]interface{}{"agentId": "agent-1"})

	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if gotEvent != string(AgentInterrupted) {
		t.Fatalf("event header = %q, want %q", gotEvent, AgentInterrupted)
	}
	if received.Event != AgentInterrupted {
		t.Fatalf("payload event = %q, want %q", received.Event, AgentInterrupted)
	}
}

func TestInactiveWebhook(t *testing.T) {
	m := New()
	id := m.Register("https://example.com/hook", nil, "")

	// Deactivate
	m.mu.Lock()
	for i := range m.webhooks {
		if m.webhooks[i].ID == id {
			m.webhooks[i].Active = false
		}
	}
	m.mu.Unlock()

	// Fire — should not panic or attempt delivery
	m.Fire(AgentCompleted, nil)
}
