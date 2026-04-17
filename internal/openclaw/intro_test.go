package openclaw

import (
	"strings"
	"testing"
)

func TestIntroMessagePinsAssignedName(t *testing.T) {
	got := IntroMessage("Scraper", "Track GPU prices")
	if !strings.Contains(got, "Your assigned runtime name for this session is exactly 'Scraper'") {
		t.Fatalf("intro prompt missing assigned-name guard: %q", got)
	}
	if !strings.Contains(got, "Your purpose: Track GPU prices.") {
		t.Fatalf("intro prompt missing task: %q", got)
	}
}
