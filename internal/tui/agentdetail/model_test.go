package agentdetail

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestViewConstrainsLongRowsToWidth(t *testing.T) {
	m := New()
	m.SetSize(18, 8)
	m.SetAgent(
		"agent-1",
		"agent-with-a-very-long-display-name",
		"task with a very long description that should not overflow",
		"active",
		123456789,
		"profile-summary-that-is-too-wide",
		"socks5://very-long-proxy-hostname.example.internal:1080",
		time.Now(),
	)
	m.SetBrowserContext("pinned context-with-a-very-long-identifier")

	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		if width := lipgloss.Width(line); width > m.width {
			t.Fatalf("line %d width = %d, want <= %d:\n%s", i+1, width, m.width, view)
		}
	}
}

func TestStatusIndicatorNamesPausedAndInterrupted(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   string
	}{
		{status: "paused", want: "paused"},
		{status: "interrupted", want: "interrupted"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			if got := statusIndicator(tc.status); !strings.Contains(got, tc.want) {
				t.Fatalf("statusIndicator(%q) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}
