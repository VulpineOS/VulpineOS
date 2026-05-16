package setup

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"vulpineos/internal/config"
)

func TestNewWithConfigSeedsExistingValues(t *testing.T) {
	cfg := &config.Config{
		Provider: "anthropic",
		APIKey:   "sk-test",
		Model:    "anthropic/claude-sonnet-4-6",
	}

	m := NewWithConfig(cfg)

	if m.cfg.Provider != cfg.Provider {
		t.Fatalf("provider = %q, want %q", m.cfg.Provider, cfg.Provider)
	}
	if m.cfg.Model != cfg.Model {
		t.Fatalf("model = %q, want %q", m.cfg.Model, cfg.Model)
	}
	if m.cfg.APIKey != cfg.APIKey {
		t.Fatalf("cfg APIKey = %q, want existing key preserved", m.cfg.APIKey)
	}
	if m.apiKeyInput.Value() != "" {
		t.Fatalf("apiKeyInput = %q, want empty", m.apiKeyInput.Value())
	}
	if got := m.viewAPIKey(); strings.Contains(got, cfg.APIKey) {
		t.Fatalf("viewAPIKey leaked stored key: %q", got)
	}
	if m.providerIdx < 0 || m.providerIdx >= len(m.providers) || m.providers[m.providerIdx].ID != cfg.Provider {
		t.Fatalf("providerIdx does not point at %q", cfg.Provider)
	}
}

func TestExistingAPIKeyCanBeKeptBlank(t *testing.T) {
	cfg := &config.Config{
		Provider: "anthropic",
		APIKey:   "sk-test",
		Model:    "anthropic/claude-sonnet-4-6",
	}

	m := NewWithConfig(cfg)
	m.step = stepAPIKey
	m.apiKeyInput.SetValue("")

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := model.(*Model)

	if updated.step != stepDone {
		t.Fatalf("step = %v, want stepDone", updated.step)
	}
	if updated.cfg.APIKey != cfg.APIKey {
		t.Fatalf("cfg APIKey = %q, want existing key preserved", updated.cfg.APIKey)
	}
}

func TestSetupViewFitsNarrowTerminal(t *testing.T) {
	m := New()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 50, Height: 20})
	mm := model.(*Model)

	assertSetupViewFits(t, mm)
}

func TestSetupProviderViewFitsShortTerminal(t *testing.T) {
	m := New()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 50, Height: 10})
	mm := model.(*Model)

	assertSetupViewFits(t, mm)
}

func TestSetupViewFitsUltraTinyTerminal(t *testing.T) {
	m := New()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 4, Height: 4})
	mm := model.(*Model)

	assertSetupViewFits(t, mm)
}

func TestSetupViewFitsVeryNarrowAPIKeyAndDoneSteps(t *testing.T) {
	cfg := &config.Config{
		Provider: "anthropic",
		APIKey:   "sk-test",
		Model:    "anthropic/claude-sonnet-4-6-with-a-long-name",
	}
	m := NewWithConfig(cfg)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	mm := model.(*Model)

	mm.step = stepAPIKey
	assertSetupViewFits(t, mm)

	mm.step = stepDone
	assertSetupViewFits(t, mm)
}

func assertSetupViewFits(t *testing.T, m *Model) {
	t.Helper()
	view := m.View()
	lines := strings.Split(view, "\n")
	if m.height > 0 && len(lines) > m.height {
		t.Fatalf("line count = %d, want <= %d:\n%s", len(lines), m.height, view)
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width > m.width {
			t.Fatalf("line %d width = %d, want <= %d:\n%s", i+1, width, m.width, view)
		}
	}
}
