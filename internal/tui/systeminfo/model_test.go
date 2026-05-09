package systeminfo

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"vulpineos/internal/tui/shared"
	"vulpineos/internal/vault"
)

func TestKernelStatusViewShowsModeAndRoute(t *testing.T) {
	model := New()
	model.SetHeight(20)

	updated, _ := model.Update(shared.KernelStatusMsg{
		Running:       true,
		PID:           1234,
		Uptime:        2 * time.Minute,
		Headless:      false,
		BrowserRoute:  "CAMOUFOX",
		BrowserWindow: "HIDDEN",
	})

	view := updated.View()
	if !strings.Contains(view, "Mode GUI") {
		t.Fatalf("expected GUI mode in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Route CAMOUFOX") {
		t.Fatalf("expected CAMOUFOX route in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Win HIDDEN") {
		t.Fatalf("expected window state in view, got:\n%s", view)
	}
}

func TestViewFitsRuntimeEventsToWidth(t *testing.T) {
	model := New()
	model.SetWidth(18)
	model.SetHeight(20)

	updated, _ := model.Update(shared.RuntimeEventMsg{Event: vault.RuntimeEvent{
		Component: "gateway",
		Event:     "very-long-runtime-event-name-that-would-wrap",
		Timestamp: time.Date(2026, 5, 10, 12, 34, 0, 0, time.UTC),
	}})

	view := updated.View()
	for i, line := range strings.Split(view, "\n") {
		if width := lipgloss.Width(line); width > 18 {
			t.Fatalf("line %d width = %d, want <= 18:\n%s", i+1, width, view)
		}
	}
}
