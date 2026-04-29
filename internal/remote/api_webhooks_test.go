package remote

import (
	"encoding/json"
	"strings"
	"testing"

	"vulpineos/internal/webhooks"
)

func TestWebhooksAddNormalizesInput(t *testing.T) {
	api := &PanelAPI{Webhooks: webhooks.New()}

	payload, err := api.HandleMessage("webhooks.add", json.RawMessage(`{
		"url":"  https://example.com/hook  ",
		"events":[" agent.completed ",""," agent.interrupted "],
		"secret":"  secret-token  "
	}`))
	if err != nil {
		t.Fatalf("webhooks.add: %v", err)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ID == "" {
		t.Fatal("expected webhook id")
	}

	hooks := api.Webhooks.List()
	if len(hooks) != 1 {
		t.Fatalf("hooks len = %d, want 1", len(hooks))
	}
	if hooks[0].URL != "https://example.com/hook" {
		t.Fatalf("url = %q, want trimmed URL", hooks[0].URL)
	}
	if hooks[0].Secret != "secret-token" {
		t.Fatalf("secret = %q, want trimmed secret", hooks[0].Secret)
	}
	wantEvents := []webhooks.EventType{webhooks.AgentCompleted, webhooks.AgentInterrupted}
	if len(hooks[0].Events) != len(wantEvents) {
		t.Fatalf("events = %#v, want %#v", hooks[0].Events, wantEvents)
	}
	for i, want := range wantEvents {
		if hooks[0].Events[i] != want {
			t.Fatalf("event[%d] = %q, want %q", i, hooks[0].Events[i], want)
		}
	}
}

func TestWebhooksAddRejectsInvalidURL(t *testing.T) {
	api := &PanelAPI{Webhooks: webhooks.New()}

	if _, err := api.HandleMessage("webhooks.add", json.RawMessage(`{"url":"ftp://example.com/hook"}`)); err == nil {
		t.Fatal("expected invalid URL error")
	}
	if len(api.Webhooks.List()) != 0 {
		t.Fatal("invalid webhook should not be registered")
	}
}

func TestWebhooksAddRejectsUnsupportedEvents(t *testing.T) {
	api := &PanelAPI{Webhooks: webhooks.New()}

	if _, err := api.HandleMessage("webhooks.add", json.RawMessage(`{
		"url":"https://example.com/hook",
		"events":["agent.completed","agent.unknown"]
	}`)); err == nil {
		t.Fatal("expected unsupported event error")
	}
	if len(api.Webhooks.List()) != 0 {
		t.Fatal("invalid webhook should not be registered")
	}
}

func TestWebhooksListDoesNotExposeSecrets(t *testing.T) {
	api := &PanelAPI{Webhooks: webhooks.New()}
	api.Webhooks.Register("https://user:pass@example.com/hook?token=url-token&view=events", nil, "secret-token")

	payload, err := api.HandleMessage("webhooks.list", nil)
	if err != nil {
		t.Fatalf("webhooks.list: %v", err)
	}

	var result struct {
		Webhooks []struct {
			URL       string `json:"url"`
			Secret    string `json:"secret"`
			HasSecret bool   `json:"hasSecret"`
		} `json:"webhooks"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Webhooks) != 1 {
		t.Fatalf("webhooks len = %d, want 1", len(result.Webhooks))
	}
	if result.Webhooks[0].Secret != "" {
		t.Fatalf("secret leaked in list response: %q", result.Webhooks[0].Secret)
	}
	if strings.Contains(result.Webhooks[0].URL, "user") || strings.Contains(result.Webhooks[0].URL, "pass") || strings.Contains(result.Webhooks[0].URL, "url-token") {
		t.Fatalf("webhook URL leaked credentials: %q", result.Webhooks[0].URL)
	}
	if !strings.Contains(result.Webhooks[0].URL, "token=%5Bredacted%5D") || !strings.Contains(result.Webhooks[0].URL, "view=events") {
		t.Fatalf("webhook URL was not redacted as expected: %q", result.Webhooks[0].URL)
	}
	if !result.Webhooks[0].HasSecret {
		t.Fatal("hasSecret = false, want true")
	}
}

func TestWebhooksRemoveRejectsBlankID(t *testing.T) {
	api := &PanelAPI{Webhooks: webhooks.New()}
	api.Webhooks.Register("https://example.com/hook", nil, "")

	if _, err := api.HandleMessage("webhooks.remove", json.RawMessage(`{"id":"   "}`)); err == nil {
		t.Fatal("expected blank webhook id error")
	}
	if len(api.Webhooks.List()) != 1 {
		t.Fatal("blank remove should not unregister existing hooks")
	}
}
