package agentlist

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"vulpineos/internal/tui/shared"
	"vulpineos/internal/vault"
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

func TestViewBudgetsUnreadBadgeWidth(t *testing.T) {
	m := New()
	m.SetWidth(14)
	m.SetAgents([]vault.Agent{{
		ID:     "agent-1",
		Name:   "very-long-agent-name",
		Status: "active",
	}})
	m.agents[0].Unread = 12

	lines := strings.Split(m.View(), "\n")
	if len(lines) < 2 {
		t.Fatalf("view lines = %d, want agent row", len(lines))
	}
	if got := lipgloss.Width(lines[1]); got > m.width {
		t.Fatalf("agent row width = %d, want <= %d: %q", got, m.width, lines[1])
	}
}

func TestStatusEventWithoutTokensPreservesExistingCount(t *testing.T) {
	m := New()
	m.SetAgents([]vault.Agent{{
		ID:          "agent-1",
		Name:        "Agent",
		Status:      "active",
		TotalTokens: 42,
	}})

	updated, _ := m.Update(shared.AgentStatusMsg{AgentID: "agent-1", Status: "paused", Tokens: 0})

	if got := updated.agents[0].Tokens; got != 42 {
		t.Fatalf("tokens = %d, want preserved 42", got)
	}
}
