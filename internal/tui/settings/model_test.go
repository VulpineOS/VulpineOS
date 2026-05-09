package settings

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"vulpineos/internal/tui/shared"
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

func TestProxyTestedMsgUpdatesLatencyAndCountry(t *testing.T) {
	m := New()
	m.SetActive(true)
	m.SetProxies([]ProxyItem{{
		ID:      "proxy-1",
		Label:   "edge",
		Type:    "socks5",
		Host:    "127.0.0.1",
		Port:    1080,
		Country: "",
		Latency: "untested",
	}})

	updated, cmd := m.Update(shared.ProxyTestedMsg{
		ProxyID: "proxy-1",
		Latency: "42ms",
		Country: "United Kingdom",
	})
	if cmd != nil {
		t.Fatalf("unexpected command: %#v", cmd())
	}

	got := updated.proxies[0]
	if got.Latency != "42ms" {
		t.Fatalf("latency = %q, want 42ms", got.Latency)
	}
	if got.Country != "United Kingdom" {
		t.Fatalf("country = %q, want United Kingdom", got.Country)
	}
}
