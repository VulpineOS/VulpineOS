package conversation

import (
	"strings"
	"testing"
)

func TestTraceOnlyFiltersToSystemMessages(t *testing.T) {
	m := New()
	m.SetSize(80, 20)
	m.SetAgentID("agent-1")
	m.SetAwake(true)
	m.AddEntry("user", "hello")
	m.AddEntry("assistant", "working on it")
	m.AddEntry("system", "Running browser action: open https://example.com")
	m.AddEntry("system", "Tool failed: browser — gateway token mismatch")
	m.SetTraceOnly(true)

	view := m.View()
	if !strings.Contains(view, "ACTION TRACE") {
		t.Fatalf("expected action trace title, got:\n%s", view)
	}
	if strings.Contains(view, "hello") {
		t.Fatalf("trace view should not include user messages, got:\n%s", view)
	}
	if strings.Contains(view, "working on it") {
		t.Fatalf("trace view should not include assistant messages, got:\n%s", view)
	}
	if !strings.Contains(view, "Running browser action: open https://example.com") {
		t.Fatalf("trace view missing tool start, got:\n%s", view)
	}
	if !strings.Contains(view, "Tool failed: browser") {
		t.Fatalf("trace view missing tool result, got:\n%s", view)
	}
}

func TestTraceOnlyShowsPlaceholderWhenEmpty(t *testing.T) {
	m := New()
	m.SetSize(80, 20)
	m.SetAgentID("agent-1")
	m.SetAwake(true)
	m.AddEntry("assistant", "ready")
	m.SetTraceOnly(true)

	view := m.View()
	if !strings.Contains(view, "No action trace yet.") {
		t.Fatalf("expected empty trace placeholder, got:\n%s", view)
	}
}
