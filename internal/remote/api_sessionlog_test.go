package remote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"vulpineos/internal/config"
)

func TestAgentsGetSessionLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	logPath := filepath.Join(config.OpenClawProfileDir(), "agents", "main", "sessions", "vulpine-agent-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("{\"type\":\"message\"}\n"), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	api := &PanelAPI{}
	payload, err := api.HandleMessage("agents.getSessionLog", json.RawMessage(`{"agentId":"agent-1"}`))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Path != logPath {
		t.Fatalf("path = %q, want %q", result.Path, logPath)
	}
	if result.Content != "{\"type\":\"message\"}\n" {
		t.Fatalf("content = %q", result.Content)
	}
}
