package remote

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContextsListRedactsSensitiveURLParts(t *testing.T) {
	reg := NewContextRegistry()
	reg.Attached("session-1", "context-1", "https://user:pass@example.com/page?token=url-token&view=ok")
	api := &PanelAPI{Contexts: reg}

	payload, err := api.HandleMessage("contexts.list", nil)
	if err != nil {
		t.Fatalf("contexts.list: %v", err)
	}

	var result struct {
		Contexts []ContextInfo `json:"contexts"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal contexts: %v", err)
	}
	if len(result.Contexts) != 1 {
		t.Fatalf("contexts len = %d, want 1", len(result.Contexts))
	}
	url := result.Contexts[0].LastURL
	for _, leaked := range []string{"user", "pass", "url-token"} {
		if strings.Contains(url, leaked) {
			t.Fatalf("context URL leaked %q: %q", leaked, url)
		}
	}
	if !strings.Contains(url, "token=%5Bredacted%5D") || !strings.Contains(url, "view=ok") {
		t.Fatalf("context URL was not redacted as expected: %q", url)
	}
}

func TestContextRegistryUpdatesURLOnNavigation(t *testing.T) {
	reg := NewContextRegistry()
	reg.Attached("session-1", "context-1", "about:blank")
	reg.FrameAttached("session-1", "frame-main", "")
	reg.Navigated("session-1", "frame-child", "https://example.test/child")
	reg.Navigated("session-1", "frame-main", "https://example.test/main")

	contexts := reg.List()
	if len(contexts) != 1 {
		t.Fatalf("contexts len = %d, want 1", len(contexts))
	}
	if contexts[0].LastURL != "https://example.test/main" {
		t.Fatalf("LastURL = %q, want main navigation URL", contexts[0].LastURL)
	}
}

func TestPanelAPIRejectsUnknownContextID(t *testing.T) {
	api := &PanelAPI{Contexts: NewContextRegistry()}
	if err := api.requireKnownContext("missing-context"); err == nil {
		t.Fatal("expected unknown context to be rejected")
	}
	api.Contexts.Created("context-1")
	if err := api.requireKnownContext("context-1"); err != nil {
		t.Fatalf("known context rejected: %v", err)
	}
}
