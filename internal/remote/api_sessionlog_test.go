package remote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	if err := os.WriteFile(logPath, []byte("{\"type\":\"message\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"thinking\",\"thinking\":\"secret chain\",\"thinkingSignature\":\"reasoning_content\"},{\"type\":\"text\",\"text\":\"Done\"}]}}\n"), 0644); err != nil {
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
	if !strings.HasSuffix(result.Content, "\n") {
		t.Fatalf("content should preserve trailing newline: %q", result.Content)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Content)), &entry); err != nil {
		t.Fatalf("unmarshal sanitized content: %v", err)
	}

	message, ok := entry["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("message = %#v", entry["message"])
	}
	content, ok := message["content"].([]interface{})
	if !ok || len(content) != 2 {
		t.Fatalf("content = %#v", message["content"])
	}
	thinking, ok := content[0].(map[string]interface{})
	if !ok {
		t.Fatalf("thinking entry = %#v", content[0])
	}
	if got := thinking["thinking"]; got != redactedReasoningText {
		t.Fatalf("thinking = %v, want %q", got, redactedReasoningText)
	}
	if _, ok := thinking["thinkingSignature"]; ok {
		t.Fatalf("thinkingSignature should be removed: %#v", thinking)
	}
	textItem, ok := content[1].(map[string]interface{})
	if !ok {
		t.Fatalf("text entry = %#v", content[1])
	}
	if got := textItem["text"]; got != "Done" {
		t.Fatalf("text = %v, want Done", got)
	}
}
