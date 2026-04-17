package openclaw

import "testing"

func TestSummarizeToolCall(t *testing.T) {
	got := summarizeToolCall("browser", map[string]interface{}{
		"action": "open",
		"url":    "https://example.com",
	})
	want := "Running browser action: open https://example.com"
	if got != want {
		t.Fatalf("summarizeToolCall = %q, want %q", got, want)
	}
}

func TestSummarizeToolResultError(t *testing.T) {
	got := summarizeToolResult("browser", nil, map[string]interface{}{
		"status": "error",
		"error":  "gateway token mismatch",
	})
	want := "Tool failed: browser — gateway token mismatch"
	if got != want {
		t.Fatalf("summarizeToolResult = %q, want %q", got, want)
	}
}

func TestSummarizeToolResultSuccessFromJSONText(t *testing.T) {
	content := []struct {
		Type      string                 `json:"type"`
		Text      string                 `json:"text"`
		Thinking  string                 `json:"thinking"`
		ID        string                 `json:"id"`
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}{
		{Type: "text", Text: `{"status":"completed"}`},
	}
	got := summarizeToolResult("web_fetch", content, nil)
	want := "Tool completed: web_fetch"
	if got != want {
		t.Fatalf("summarizeToolResult = %q, want %q", got, want)
	}
}
