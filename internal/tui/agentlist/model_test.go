package agentlist

import (
	"strings"
	"testing"
)

func TestStatusIconDistinguishesPausedAndInterrupted(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   string
	}{
		{status: "paused", want: "Ⅱ"},
		{status: "interrupted", want: "×"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			if got := statusIcon(tc.status); !strings.Contains(got, tc.want) {
				t.Fatalf("statusIcon(%q) = %q, want marker %q", tc.status, got, tc.want)
			}
		})
	}
}
