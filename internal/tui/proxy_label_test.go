package tui

import (
	"strings"
	"testing"
)

func TestSafeProxyLabelRedactsImportedProxyCredentials(t *testing.T) {
	got := safeProxyLabel("http://user:pass@example.com:8080")
	if strings.Contains(got, "user") || strings.Contains(got, "pass") {
		t.Fatalf("proxy label leaked credentials: %q", got)
	}
	if got != "http example.com:8080" {
		t.Fatalf("proxy label = %q, want display form", got)
	}
}

func TestSafeProxyLabelKeepsCustomLabels(t *testing.T) {
	got := safeProxyLabel("Sydney residential")
	if got != "Sydney residential" {
		t.Fatalf("proxy label = %q, want custom label", got)
	}
}
