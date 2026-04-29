package remote

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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
	logData := []byte("{\"type\":\"message\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"thinking\",\"thinking\":\"secret chain\",\"thinkingSignature\":\"reasoning_content\"},{\"type\":\"text\",\"text\":\"Done\"}]}}\n")
	if err := os.WriteFile(logPath, logData, 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	api := &PanelAPI{}
	payload, err := api.HandleMessage("agents.getSessionLog", json.RawMessage(`{"agentId":"agent-1"}`))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result struct {
		Path       string `json:"path"`
		Content    string `json:"content"`
		Truncated  bool   `json:"truncated"`
		Bytes      int64  `json:"bytes"`
		TotalBytes int64  `json:"totalBytes"`
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
	if result.Truncated {
		t.Fatalf("truncated = true, want false")
	}
	if result.Bytes != int64(len(logData)) {
		t.Fatalf("bytes = %d, want raw file length %d", result.Bytes, len(logData))
	}
	if result.TotalBytes != int64(len(logData)) {
		t.Fatalf("totalBytes = %d, want raw file length %d", result.TotalBytes, len(logData))
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

func TestAgentsGetSessionLogTailsLargeLogsAtLineBoundary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	logPath := filepath.Join(config.OpenClawProfileDir(), "agents", "main", "sessions", "vulpine-agent-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	secretLine := []byte(`{"type":"message","message":{"role":"assistant","content":[{"type":"thinking","thinking":"` + strings.Repeat("secret-", int(maxPanelSessionLogBytes/7)+1000) + `","thinkingSignature":"sig"}]}}`)
	tailLine := []byte(`{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"tail visible"}]}}`)
	data := bytes.Join([][]byte{secretLine, tailLine}, []byte("\n"))
	data = append(data, '\n')
	if int64(len(data)) <= maxPanelSessionLogBytes {
		t.Fatalf("test log size %d must exceed tail limit %d", len(data), maxPanelSessionLogBytes)
	}
	if err := os.WriteFile(logPath, data, 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	api := &PanelAPI{}
	payload, err := api.HandleMessage("agents.getSessionLog", json.RawMessage(`{"agentId":"agent-1"}`))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result struct {
		Content    string `json:"content"`
		Truncated  bool   `json:"truncated"`
		Bytes      int64  `json:"bytes"`
		TotalBytes int64  `json:"totalBytes"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.Truncated {
		t.Fatalf("truncated = false, want true")
	}
	if result.TotalBytes != int64(len(data)) {
		t.Fatalf("totalBytes = %d, want %d", result.TotalBytes, len(data))
	}
	if result.Bytes >= result.TotalBytes {
		t.Fatalf("bytes = %d should be less than totalBytes %d", result.Bytes, result.TotalBytes)
	}
	if strings.Contains(result.Content, "secret-") || strings.Contains(result.Content, "thinkingSignature") {
		t.Fatalf("partial hidden reasoning line leaked through tail content: %.120q", result.Content)
	}
	if !strings.Contains(result.Content, "tail visible") {
		t.Fatalf("tail content missing expected line: %.120q", result.Content)
	}
}

func TestAgentsGetSessionLogRejectsPathTraversalAgentID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	escapePath := filepath.Join(config.OpenClawProfileDir(), "agents", "main", "escape.jsonl")
	if err := os.MkdirAll(filepath.Dir(escapePath), 0755); err != nil {
		t.Fatalf("mkdir escape dir: %v", err)
	}
	if err := os.WriteFile(escapePath, []byte("leaked"), 0600); err != nil {
		t.Fatalf("write escape file: %v", err)
	}

	api := &PanelAPI{}
	for _, agentID := range []string{"../escape", `..\escape`, "nested/escape"} {
		t.Run(agentID, func(t *testing.T) {
			_, err := api.HandleMessage("agents.getSessionLog", json.RawMessage(`{"agentId":`+strconv.Quote(agentID)+`}`))
			if err == nil || !strings.Contains(err.Error(), "invalid agentId") {
				t.Fatalf("error = %v, want invalid agentId", err)
			}
		})
	}
}
