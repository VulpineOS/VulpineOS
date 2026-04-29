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
