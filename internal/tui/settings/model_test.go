package settings

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConstrainedHeightShowsFocusedSection(t *testing.T) {
	m := New()
	m.SetActive(true)
	m.SetSize(36, 8)

	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("unexpected message after first tab: %#v", msg)
		}
	}
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("unexpected message after second tab: %#v", msg)
		}
	}

	view := m.View()
	if !strings.Contains(view, "Skills") {
		t.Fatalf("constrained settings view did not show focused section:\n%s", view)
	}
	if strings.Contains(view, "General") && strings.Index(view, "General") < strings.Index(view, "Skills") {
		t.Fatalf("constrained settings view kept earlier sections above focused section:\n%s", view)
	}
}
