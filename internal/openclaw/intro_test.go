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
	if !strings.Contains(got, "Your task for this session is: Track GPU prices.") {
		t.Fatalf("intro prompt missing task: %q", got)
	}
	if strings.Contains(got, "Introduce yourself briefly") {
		t.Fatalf("intro prompt should not force an introduction: %q", got)
	}
	if !strings.Contains(got, "If the task asks for a specific reply or exact wording, return that output exactly.") {
		t.Fatalf("intro prompt missing exact-output guard: %q", got)
	}
}
