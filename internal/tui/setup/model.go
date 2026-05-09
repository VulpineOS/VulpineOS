package setup

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"vulpineos/internal/config"
)

type step int

const (
	stepProvider step = iota
	stepAPIKey
	stepDone
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED"))
	activeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED")).Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Bold(true)
	boxStyle     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(1, 2).
			Width(76)
)

// Model is the setup wizard Bubbletea model.
type Model struct {
	step          step
	width, height int
	providers     []config.Provider
	providerIdx   int
	apiKeyInput   textinput.Model
	modelIdx      int
	cfg           *config.Config
	done          bool
}

// New creates a new setup wizard.
func New() Model {
	return NewWithConfig(nil)
}

// NewWithConfig creates a setup wizard seeded from an existing config.
func NewWithConfig(existing *config.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "Paste your API key here..."
	ti.CharLimit = 200
	ti.Width = 50

	m := Model{
		step:        stepProvider,
		providers:   config.Providers,
		apiKeyInput: ti,
		cfg:         &config.Config{},
	}
	if existing == nil {
		return m
	}

	m.cfg.Provider = existing.Provider
	m.cfg.APIKey = existing.APIKey
	m.cfg.Model = existing.Model
	m.cfg.BinaryPath = existing.BinaryPath
	m.cfg.ResizePanelsWithArrows = existing.ResizePanelsWithArrows
	m.cfg.GlobalSkills = append([]config.SkillEntry(nil), existing.GlobalSkills...)
	if len(existing.AgentSkills) > 0 {
		m.cfg.AgentSkills = make(map[string][]config.SkillEntry, len(existing.AgentSkills))
		for agentID, skills := range existing.AgentSkills {
			m.cfg.AgentSkills[agentID] = append([]config.SkillEntry(nil), skills...)
		}
	}
	for i, p := range m.providers {
		if p.ID != existing.Provider {
			continue
		}
		m.providerIdx = i
		for idx, model := range p.Models {
			if model == existing.Model {
				m.modelIdx = idx
				break
			}
		}
		break
	}
	return m
}

// Config returns the completed config.
func (m Model) Config() *config.Config {
	return m.cfg
}

// Done returns true if setup is complete and user pressed Enter on the final screen.
func (m Model) Done() bool {
	return m.done
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.apiKeyInput.Width = m.inputWidth()

	case tea.KeyMsg:
		switch m.step {
		case stepProvider:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "j", "down":
				if m.providerIdx < len(m.providers)-1 {
					m.providerIdx++
				}
			case "k", "up":
				if m.providerIdx > 0 {
					m.providerIdx--
				}
			case "enter":
				p := m.providers[m.providerIdx]
				previousProvider := m.cfg.Provider
				m.cfg.Provider = p.ID
				m.cfg.Model = p.DefaultModel
				m.modelIdx = 0
				if previousProvider != "" && previousProvider != p.ID {
					m.cfg.APIKey = ""
				}
				if !p.NeedsKey {
					// Ollama — skip API key step
					m.cfg.SetupComplete = true
					m.step = stepDone
				} else {
					if strings.TrimSpace(m.cfg.APIKey) != "" {
						m.apiKeyInput.Placeholder = fmt.Sprintf("%s already stored; leave blank to keep it", p.EnvVar)
					} else {
						m.apiKeyInput.Placeholder = fmt.Sprintf("Enter your %s...", p.EnvVar)
					}
					m.apiKeyInput.SetValue("")
					m.apiKeyInput.Focus()
					m.step = stepAPIKey
				}
			}

		case stepAPIKey:
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.step = stepProvider
				m.apiKeyInput.Blur()
			case "tab":
				// Cycle model
				p := m.providers[m.providerIdx]
				m.modelIdx = (m.modelIdx + 1) % len(p.Models)
				m.cfg.Model = p.Models[m.modelIdx]
			case "enter":
				key := strings.TrimSpace(m.apiKeyInput.Value())
				if key == "" {
					if strings.TrimSpace(m.cfg.APIKey) == "" {
						break
					}
				} else {
					m.cfg.APIKey = key
				}
				m.cfg.SetupComplete = true
				m.step = stepDone
			default:
				var cmd tea.Cmd
				m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
				return m, cmd
			}

		case stepDone:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "enter":
				m.done = true
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	var content string

	switch m.step {
	case stepProvider:
		content = m.viewProvider()
	case stepAPIKey:
		content = m.viewAPIKey()
	case stepDone:
		content = m.viewDone()
	}

	if m.width > 0 && m.width < 8 {
		return fitSetupBlock(content, m.width, m.height)
	}

	box := boxStyle.Width(m.boxWidth()).Render(content)

	// Center vertically
	pad := (m.height - lipgloss.Height(box)) / 2
	if pad < 0 {
		pad = 0
	}

	return fitSetupBlock(strings.Repeat("\n", pad)+lipgloss.PlaceHorizontal(m.width, lipgloss.Center, box), m.width, m.height)
}

func (m Model) boxWidth() int {
	if m.width <= 0 {
		return 76
	}
	width := m.width - 2
	if width > 76 {
		width = 76
	}
	if width < 12 {
		width = max(1, m.width-2)
	}
	return width
}

func (m Model) inputWidth() int {
	width := m.boxWidth() - 8
	if width > 50 {
		width = 50
	}
	if width < 10 {
		width = 10
	}
	return width
}

func (m Model) contentWidth() int {
	width := m.boxWidth() - 4
	if width < 1 {
		return 1
	}
	return width
}

func (m Model) viewProvider() string {
	if m.height > 0 && m.height < 14 {
		return m.viewProviderCompact()
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("VulpineOS — First Time Setup"))
	b.WriteString("\n\n")
	b.WriteString("Select your AI provider:\n\n")

	// Show a scrollable window of providers (max 12 visible)
	maxVisible := m.maxVisibleProviders(7)
	start := 0
	if m.providerIdx >= maxVisible {
		start = m.providerIdx - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(m.providers) {
		end = len(m.providers)
	}

	for i := start; i < end; i++ {
		p := m.providers[i]
		envText := p.EnvVar
		if !p.NeedsKey {
			envText = "No key needed"
		}
		envHint := mutedStyle.Render(envText)
		rowWidth := m.contentWidth()
		nameWidth := rowWidth - 4 - lipgloss.Width(envText)
		if nameWidth < 8 {
			nameWidth = 8
		}
		name := padSetupText(fitSetupText(p.Name, nameWidth), nameWidth)
		var line string
		if i == m.providerIdx {
			line = activeStyle.Render("▸ ") + lipgloss.NewStyle().Bold(true).Render(name) + "  " + envHint
		} else {
			line = "  " + mutedStyle.Render(name) + "  " + envHint
		}
		b.WriteString(fitSetupLine(line, rowWidth) + "\n")
	}

	// Scroll indicators
	scrollInfo := ""
	if start > 0 && end < len(m.providers) {
		scrollInfo = fmt.Sprintf("  ↑ %d above · ↓ %d below", start, len(m.providers)-end)
	} else if start > 0 {
		scrollInfo = fmt.Sprintf("  ↑ %d more above", start)
	} else if end < len(m.providers) {
		scrollInfo = fmt.Sprintf("  ↓ %d more below", len(m.providers)-end)
	}
	if scrollInfo != "" {
		b.WriteString(mutedStyle.Render(scrollInfo) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(fitSetupLine(mutedStyle.Render(fmt.Sprintf("[j/k] navigate  [Enter] select  [q] quit  (%d providers)", len(m.providers))), m.contentWidth()))
	return b.String()
}

func (m Model) viewProviderCompact() string {
	var b strings.Builder
	p := m.providers[m.providerIdx]
	b.WriteString(titleStyle.Render("VulpineOS Setup"))
	b.WriteString("\n")
	b.WriteString("Provider:\n")
	row := fmt.Sprintf("%d/%d  %s", m.providerIdx+1, len(m.providers), p.Name)
	b.WriteString(activeStyle.Render(fitSetupText(row, m.contentWidth())))
	b.WriteString("\n")
	hint := "[j/k] choose  [Enter] select  [q] quit"
	b.WriteString(mutedStyle.Render(fitSetupText(hint, m.contentWidth())))
	return b.String()
}

func (m Model) maxVisibleProviders(fixedContentLines int) int {
	maxVisible := 12
	if m.height > 0 {
		contentBudget := m.height - 4
		if contentBudget < 1 {
			contentBudget = 1
		}
		if byHeight := contentBudget - fixedContentLines; byHeight < maxVisible {
			maxVisible = byHeight
		}
	}
	if maxVisible < 1 {
		return 1
	}
	return maxVisible
}

func (m Model) viewAPIKey() string {
	p := m.providers[m.providerIdx]
	if m.height > 0 && m.height < 14 {
		return m.viewAPIKeyCompact(p)
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("VulpineOS — %s Setup", p.Name)))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("API Key (%s):\n", p.EnvVar))
	b.WriteString(m.apiKeyInput.View())
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Model: %s\n", activeStyle.Render(m.cfg.Model)))
	b.WriteString(mutedStyle.Render("(Press Tab to change model)"))
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("[Enter] save  [Tab] model  [Esc] back"))
	return b.String()
}

func (m Model) viewAPIKeyCompact(p config.Provider) string {
	var b strings.Builder
	width := m.contentWidth()
	b.WriteString(titleStyle.Render("API Key"))
	b.WriteString("\n")
	b.WriteString(fitSetupText(p.EnvVar, width))
	b.WriteString("\n")
	b.WriteString(fitSetupLine(m.apiKeyInput.View(), width))
	b.WriteString("\n")
	b.WriteString(activeStyle.Render(fitSetupText(m.cfg.Model, width)))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(fitSetupText("[Enter] save  [Esc] back", width)))
	return b.String()
}

func fitSetupText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	var b strings.Builder
	for _, r := range text {
		next := b.String() + string(r)
		if lipgloss.Width(next) > width-1 {
			break
		}
		b.WriteRune(r)
	}
	b.WriteString("…")
	return b.String()
}

func padSetupText(text string, width int) string {
	for lipgloss.Width(text) < width {
		text += " "
	}
	return text
}

func fitSetupLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(line) <= width {
		return line
	}
	fitted := lipgloss.NewStyle().MaxWidth(width).Render(line)
	lines := strings.Split(fitted, "\n")
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func fitSetupBlock(block string, width, height int) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		lines[i] = fitSetupLine(line, width)
	}
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewDone() string {
	p := config.GetProvider(m.cfg.Provider)
	name := m.cfg.Provider
	if p != nil {
		name = p.Name
	}
	if m.height > 0 && m.height < 14 {
		return m.viewDoneCompact(name)
	}

	var b strings.Builder
	b.WriteString(successStyle.Render("VulpineOS — Setup Complete ✓"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Provider:  %s\n", name))
	b.WriteString(fmt.Sprintf("Model:     %s\n", m.cfg.Model))
	b.WriteString(fmt.Sprintf("Config:    %s\n", config.Path()))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Press Enter to launch dashboard..."))
	return b.String()
}

func (m Model) viewDoneCompact(providerName string) string {
	width := m.contentWidth()
	var b strings.Builder
	b.WriteString(successStyle.Render("Setup Complete"))
	b.WriteString("\n")
	b.WriteString(fitSetupText(providerName, width))
	b.WriteString("\n")
	b.WriteString(fitSetupText(m.cfg.Model, width))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(fitSetupText("Enter to launch", width)))
	return b.String()
}
