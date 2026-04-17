package systeminfo

import (
	"strings"
	"testing"
	"time"

	"vulpineos/internal/tui/shared"
)

func TestKernelStatusViewShowsModeAndRoute(t *testing.T) {
	model := New()
	model.SetHeight(20)

	updated, _ := model.Update(shared.KernelStatusMsg{
		Running:      true,
		PID:          1234,
		Uptime:       2 * time.Minute,
		Headless:     false,
		BrowserRoute: "CAMOUFOX",
	})

	view := updated.View()
	if !strings.Contains(view, "Mode GUI") {
		t.Fatalf("expected GUI mode in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Route CAMOUFOX") {
		t.Fatalf("expected CAMOUFOX route in view, got:\n%s", view)
	}
}
