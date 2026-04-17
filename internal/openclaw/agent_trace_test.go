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

func TestSummarizeToolCallExec(t *testing.T) {
	got := summarizeToolCall("exec", map[string]interface{}{
		"command": "vulpineos-browser navigate \"https://example.com\"",
	})
	want := "Running tool: exec vulpineos-browser navigate \"https://example.com\""
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

func TestSummarizeToolResultBrowserOpenSuccess(t *testing.T) {
	content := []struct {
		Type      string                 `json:"type"`
		Text      string                 `json:"text"`
		Thinking  string                 `json:"thinking"`
		ID        string                 `json:"id"`
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}{
		{Type: "text", Text: `{"status":"ok","targetId":"abc123","url":"https://pbtech.co.nz"}`},
	}
	got := summarizeToolResult("browser", content, nil)
	want := "Tool completed: browser — opened https://pbtech.co.nz"
	if got != want {
		t.Fatalf("summarizeToolResult = %q, want %q", got, want)
	}
}

func TestHandleSessionLogLine_EmitsAssistantText(t *testing.T) {
	agent := newAgent("agent-1", "ctx-1", make(chan AgentStatus, 1))

	line := `{"type":"message","message":{"role":"assistant","content":[{"type":"thinking","thinking":"internal"},{"type":"text","text":"STATUS:clicked"}]}}`
	agent.handleSessionLogLine(line)

	select {
	case msg := <-agent.conversationCh:
		if msg.Role != "assistant" {
			t.Fatalf("role = %q, want assistant", msg.Role)
		}
		if msg.Content != "STATUS:clicked" {
			t.Fatalf("content = %q, want STATUS:clicked", msg.Content)
		}
	default:
		t.Fatal("expected assistant text to be emitted from session log")
	}
}

func TestHandleSessionLogLine_WarnsAfterFailedToolThenAssistantReply(t *testing.T) {
	agent := newAgent("agent-1", "ctx-1", make(chan AgentStatus, 1))

	toolResult := `{"type":"message","message":{"role":"toolResult","toolName":"browser","content":[{"type":"text","text":"{\"status\":\"error\",\"error\":\"gateway token mismatch\"}"}],"details":{"status":"error","error":"gateway token mismatch"}}}`
	assistant := `{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"STATUS:clicked"}]}}`

	agent.handleSessionLogLine(toolResult)
	agent.handleSessionLogLine(assistant)

	msg1 := <-agent.conversationCh
	if msg1.Role != "system" || msg1.Content != "Tool failed: browser — gateway token mismatch" {
		t.Fatalf("first message = %#v", msg1)
	}

	msg2 := <-agent.conversationCh
	if msg2.Role != "system" || msg2.Content != "Warning: assistant replied after failed browser action — gateway token mismatch" {
		t.Fatalf("second message = %#v", msg2)
	}

	msg3 := <-agent.conversationCh
	if msg3.Role != "assistant" || msg3.Content != "STATUS:clicked" {
		t.Fatalf("third message = %#v", msg3)
	}
}

func TestHandleSessionLogLine_SuccessfulToolClearsFailureWarning(t *testing.T) {
	agent := newAgent("agent-1", "ctx-1", make(chan AgentStatus, 1))

	failed := `{"type":"message","message":{"role":"toolResult","toolName":"browser","content":[{"type":"text","text":"{\"status\":\"error\",\"error\":\"gateway token mismatch\"}"}],"details":{"status":"error","error":"gateway token mismatch"}}}`
	success := `{"type":"message","message":{"role":"toolResult","toolName":"browser","content":[{"type":"text","text":"{\"status\":\"ok\",\"url\":\"https://example.com\"}"}],"details":{"status":"ok","url":"https://example.com"}}}`
	assistant := `{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"Done"}]}}`

	agent.handleSessionLogLine(failed)
	agent.handleSessionLogLine(success)
	agent.handleSessionLogLine(assistant)

	msg1 := <-agent.conversationCh
	if msg1.Content != "Tool failed: browser — gateway token mismatch" {
		t.Fatalf("first message = %#v", msg1)
	}

	msg2 := <-agent.conversationCh
	if msg2.Content != "Tool completed: browser — at https://example.com" {
		t.Fatalf("second message = %#v", msg2)
	}

	msg3 := <-agent.conversationCh
	if msg3.Role != "assistant" || msg3.Content != "Done" {
		t.Fatalf("third message = %#v", msg3)
	}

	select {
	case extra := <-agent.conversationCh:
		t.Fatalf("unexpected extra warning after successful tool: %#v", extra)
	default:
	}
}
