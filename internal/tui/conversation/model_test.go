package conversation

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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

func TestSetSizeRewrapsRenderedEntries(t *testing.T) {
	m := New()
	m.SetSize(80, 20)
	m.SetAgentID("agent-1")
	m.AddEntry("assistant", strings.Repeat("wrapped words ", 12))

	wideLineCount := len(m.entries[0].renderedLines)
	m.SetSize(24, 20)

	narrowLineCount := len(m.entries[0].renderedLines)
	if narrowLineCount <= wideLineCount {
		t.Fatalf("narrow line count = %d, want more than wide count %d", narrowLineCount, wideLineCount)
	}
	for _, line := range m.entries[0].renderedLines {
		if got := ansiVisualWidth(line); got > 16 {
			t.Fatalf("line width = %d, want <= 16 after resize: %q", got, line)
		}
	}
}

func TestSetSizeAllowsVeryNarrowContentWidth(t *testing.T) {
	m := New()
	m.SetSize(6, 10)
	m.SetAgentID("agent-1")
	m.AddEntry("assistant", "abcdef")

	if m.textInput.Width > 2 {
		t.Fatalf("text input width = %d, want <= 2", m.textInput.Width)
	}
	for _, line := range m.entries[0].renderedLines {
		if got := ansiVisualWidth(line); got > 1 {
			t.Fatalf("line width = %d, want <= 1 in narrow view: %q", got, line)
		}
	}
}

func TestRenderMarkdownDoesNotSplitStyledANSI(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	lines := renderMarkdown("**"+strings.Repeat("bold ", 8)+"**", 12)
	for _, line := range lines {
		if hasUnclosedSGR(line) {
			t.Fatalf("line has unclosed ANSI style: %q\nall lines: %#v", line, lines)
		}
	}
}

func hasUnclosedSGR(line string) bool {
	active := false
	for i := 0; i < len(line); i++ {
		if line[i] != '\x1b' || i+1 >= len(line) || line[i+1] != '[' {
			continue
		}
		end := i + 2
		for end < len(line) && (line[end] < 0x40 || line[end] > 0x7e) {
			end++
		}
		if end >= len(line) {
			return true
		}
		seq := line[i : end+1]
		if strings.HasSuffix(seq, "m") {
			active = seq != "\x1b[0m" && seq != "\x1b[m"
		}
		i = end
	}
	return active
}
