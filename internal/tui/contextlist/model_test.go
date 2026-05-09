package contextlist

import (
	"strings"
	"testing"

	"vulpineos/internal/tui/shared"
)

func TestSafeDisplayURLRedactsSensitiveParts(t *testing.T) {
	got := SafeDisplayURL("https://user:pass@example.com/page?token=url-token&view=ok")
	for _, leaked := range []string{"user", "pass", "url-token"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("display URL leaked %q: %q", leaked, got)
		}
	}
	if !strings.Contains(got, "token=%5Bredacted%5D") || !strings.Contains(got, "view=ok") {
		t.Fatalf("display URL was not redacted as expected: %q", got)
	}
}

func TestViewRedactsContextURL(t *testing.T) {
	m := New()
	m.SetWidth(120)
	m, _ = m.Update(shared.TargetAttachedMsg{
		SessionID: "session-1",
		TargetID:  "target-1",
		ContextID: "context-1",
		URL:       "https://user:pass@example.com/page?token=url-token&view=ok",
	})

	view := m.View()
	for _, leaked := range []string{"user", "pass", "url-token"} {
		if strings.Contains(view, leaked) {
			t.Fatalf("context view leaked %q: %q", leaked, view)
		}
	}
	if !strings.Contains(view, "token=%5Bredacted%5D") {
		t.Fatalf("context view missing redacted URL: %q", view)
	}
}

func TestContextListReplayedTargetAttachedUpsertsExistingTarget(t *testing.T) {
	m := New()

	msg := shared.TargetAttachedMsg{
		SessionID: "session-1",
		TargetID:  "target-1",
		ContextID: "context-1",
		URL:       "https://first.example",
	}
	m, _ = m.Update(msg)
	msg.URL = "https://second.example"
	m, _ = m.Update(msg)

	if len(m.items) != 1 {
		t.Fatalf("items = %d, want replayed target to update existing row", len(m.items))
	}
	if m.items[0].URL != "https://second.example" {
		t.Fatalf("url = %q, want replayed URL", m.items[0].URL)
	}
}
