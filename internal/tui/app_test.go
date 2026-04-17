package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"vulpineos/internal/config"
	"vulpineos/internal/tui/shared"
	"vulpineos/internal/vault"
)

func openTestVault(t *testing.T) *vault.DB {
	t.Helper()
	db, err := vault.OpenPath(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestNewAppReconcilesNonTerminalAgentsToPaused(t *testing.T) {
	db := openTestVault(t)

	active, err := db.CreateAgent("active-agent", "task", "{}")
	if err != nil {
		t.Fatalf("create active agent: %v", err)
	}
	if err := db.UpdateAgentStatus(active.ID, "active"); err != nil {
		t.Fatalf("set active status: %v", err)
	}

	running, err := db.CreateAgent("running-agent", "task", "{}")
	if err != nil {
		t.Fatalf("create running agent: %v", err)
	}
	if err := db.UpdateAgentStatus(running.ID, "running"); err != nil {
		t.Fatalf("set running status: %v", err)
	}

	completed, err := db.CreateAgent("completed-agent", "task", "{}")
	if err != nil {
		t.Fatalf("create completed agent: %v", err)
	}
	if err := db.UpdateAgentStatus(completed.ID, "completed"); err != nil {
		t.Fatalf("set completed status: %v", err)
	}

	app := NewApp(nil, nil, nil, db, nil, nil)

	if got := app.agentList.SelectedAgentID(); got == "" {
		t.Fatal("expected initial selection after loading agents")
	}

	activeAgent, err := db.GetAgent(active.ID)
	if err != nil {
		t.Fatalf("get active agent: %v", err)
	}
	if activeAgent.Status != "paused" {
		t.Fatalf("active agent status = %q, want paused", activeAgent.Status)
	}

	runningAgent, err := db.GetAgent(running.ID)
	if err != nil {
		t.Fatalf("get running agent: %v", err)
	}
	if runningAgent.Status != "paused" {
		t.Fatalf("running agent status = %q, want paused", runningAgent.Status)
	}

	completedAgent, err := db.GetAgent(completed.ID)
	if err != nil {
		t.Fatalf("get completed agent: %v", err)
	}
	if completedAgent.Status != "completed" {
		t.Fatalf("completed agent status = %q, want completed", completedAgent.Status)
	}
}

func TestNewAppLoadsPersistedConversationAndSelectionSwitch(t *testing.T) {
	db := openTestVault(t)

	older, err := db.CreateAgent("older-agent", "first task", "{}")
	if err != nil {
		t.Fatalf("create first agent: %v", err)
	}
	if err := db.AppendMessage(older.ID, "assistant", "older persisted reply", 5); err != nil {
		t.Fatalf("append first message: %v", err)
	}

	newer, err := db.CreateAgent("newer-agent", "second task", "{}")
	if err != nil {
		t.Fatalf("create second agent: %v", err)
	}
	if err := db.AppendMessage(newer.ID, "assistant", "newer persisted reply", 6); err != nil {
		t.Fatalf("append second message: %v", err)
	}
	if _, err := db.Conn().Exec(`UPDATE agents SET last_active = ? WHERE id = ?`, time.Now().Unix()+10, newer.ID); err != nil {
		t.Fatalf("set newer last_active: %v", err)
	}
	if _, err := db.Conn().Exec(`UPDATE agents SET last_active = ? WHERE id = ?`, time.Now().Unix()-10, older.ID); err != nil {
		t.Fatalf("set older last_active: %v", err)
	}

	app := NewApp(nil, nil, nil, db, nil, nil)
	app.conversation.SetSize(80, 20)

	if app.selectedAgentID != newer.ID {
		t.Fatalf("selected agent = %q, want newest %q", app.selectedAgentID, newer.ID)
	}
	if !strings.Contains(app.conversation.View(), "newer persisted reply") {
		t.Fatalf("expected initial conversation to include newest persisted message, got:\n%s", app.conversation.View())
	}

	app.agentList.MoveDown()
	if cmd := app.selectCurrentAgent(); cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("selectCurrentAgent returned unexpected msg: %#v", msg)
		}
	}

	if app.selectedAgentID != older.ID {
		t.Fatalf("selected agent after move = %q, want %q", app.selectedAgentID, older.ID)
	}
	if !strings.Contains(app.conversation.View(), "older persisted reply") {
		t.Fatalf("expected conversation to reload older persisted message, got:\n%s", app.conversation.View())
	}
}

func TestConversationUnreadCountsTrackNonSelectedAgents(t *testing.T) {
	db := openTestVault(t)

	first, err := db.CreateAgent("first-agent", "first task", "{}")
	if err != nil {
		t.Fatalf("create first agent: %v", err)
	}
	second, err := db.CreateAgent("second-agent", "second task", "{}")
	if err != nil {
		t.Fatalf("create second agent: %v", err)
	}

	app := NewApp(nil, nil, nil, db, nil, nil)
	app.selectedAgentID = first.ID
	app.conversation.SetAgentID(first.ID)

	model, _ := app.Update(shared.ConversationEntryMsg{
		AgentID: second.ID,
		Role:    "assistant",
		Content: "background reply",
	})
	app = model.(App)

	if got := app.agentList.UnreadCount(second.ID); got != 1 {
		t.Fatalf("second unread = %d, want 1", got)
	}

	app.agentList.MoveDown()
	if cmd := app.selectCurrentAgent(); cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("selectCurrentAgent returned unexpected msg: %#v", msg)
		}
	}

	if got := app.agentList.UnreadCount(second.ID); got != 0 {
		t.Fatalf("second unread after selection = %d, want 0", got)
	}
	if !strings.Contains(app.conversation.View(), "background reply") {
		t.Fatalf("expected conversation to include unread message after selection, got:\n%s", app.conversation.View())
	}
}

func TestDeletingSelectedAgentLoadsNextPersistedConversation(t *testing.T) {
	db := openTestVault(t)

	first, err := db.CreateAgent("first-agent", "first task", "{}")
	if err != nil {
		t.Fatalf("create first agent: %v", err)
	}
	if err := db.AppendMessage(first.ID, "assistant", "first persisted reply", 1); err != nil {
		t.Fatalf("append first message: %v", err)
	}

	second, err := db.CreateAgent("second-agent", "second task", "{}")
	if err != nil {
		t.Fatalf("create second agent: %v", err)
	}
	if err := db.AppendMessage(second.ID, "assistant", "second persisted reply", 2); err != nil {
		t.Fatalf("append second message: %v", err)
	}

	app := NewApp(nil, nil, nil, db, nil, nil)
	app.conversation.SetSize(80, 20)
	app.selectedAgentID = first.ID
	app.conversation.SetAgentID(first.ID)
	app.conversation.LoadMessages([]vault.AgentMessage{
		{Role: "assistant", Content: "first persisted reply"},
	})

	model, _ := app.Update(shared.AgentDeletedMsg{AgentID: first.ID})
	app = model.(App)

	if app.selectedAgentID != second.ID {
		t.Fatalf("selected agent = %q, want %q", app.selectedAgentID, second.ID)
	}
	if !strings.Contains(app.conversation.View(), "second persisted reply") {
		t.Fatalf("expected conversation to reload second agent messages, got:\n%s", app.conversation.View())
	}
	if app.notice != "Agent deleted" {
		t.Fatalf("notice = %q, want %q", app.notice, "Agent deleted")
	}
}

func TestDeletingOnlySelectedAgentClearsWorkbenchState(t *testing.T) {
	db := openTestVault(t)

	agent, err := db.CreateAgent("solo-agent", "solo task", "{}")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.AppendMessage(agent.ID, "assistant", "solo persisted reply", 1); err != nil {
		t.Fatalf("append message: %v", err)
	}

	app := NewApp(nil, nil, nil, db, nil, nil)
	app.conversation.SetSize(80, 20)
	app.agentDetail.SetSize(40, 8)
	app.selectedAgentID = agent.ID
	app.conversation.SetAgentID(agent.ID)
	app.conversation.LoadMessages([]vault.AgentMessage{
		{Role: "assistant", Content: "solo persisted reply"},
	})
	app.agentDetail.SetAgent(agent.ID, "solo-agent", "solo task", "paused", 0, "", "", time.Now())

	model, _ := app.Update(shared.AgentDeletedMsg{AgentID: agent.ID})
	app = model.(App)

	if app.selectedAgentID != "" {
		t.Fatalf("selected agent = %q, want empty", app.selectedAgentID)
	}
	if app.conversation.AgentID() != "" {
		t.Fatalf("conversation agent id = %q, want empty", app.conversation.AgentID())
	}
	if app.agentDetail.HasAgent() {
		t.Fatal("expected agent detail to clear after deleting final agent")
	}
	conversationView := app.conversation.View()
	if !strings.Contains(conversationView, "to create a new agent") {
		t.Fatalf("expected empty conversation prompt, got:\n%s", conversationView)
	}
	detailView := app.agentDetail.View()
	if !strings.Contains(detailView, "to create a new agent") {
		t.Fatalf("expected empty detail prompt, got:\n%s", detailView)
	}
}

func TestAgentCreatedFocusesUnlockedChat(t *testing.T) {
	db := openTestVault(t)
	cfg := &config.Config{}
	app := NewApp(nil, nil, nil, db, cfg, nil)
	app.conversation.SetSize(80, 20)

	agent, err := db.CreateAgent("Scraper", "Scrape prices", "{}")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agent.Status = "active"

	model, _ := app.Update(shared.AgentCreatedMsg{Agent: *agent})
	app = model.(App)

	if app.focus != FocusConversation {
		t.Fatalf("focus = %d, want conversation", app.focus)
	}
	if app.inputMode != "chat" {
		t.Fatalf("inputMode = %q, want chat", app.inputMode)
	}
	if !app.conversation.IsAwake() {
		t.Fatal("conversation should be awake after agent creation")
	}
}

func TestArrowKeysNavigateAgentsWhenResizeModeDisabled(t *testing.T) {
	db := openTestVault(t)
	first, err := db.CreateAgent("first-agent", "first task", "{}")
	if err != nil {
		t.Fatalf("create first agent: %v", err)
	}
	second, err := db.CreateAgent("second-agent", "second task", "{}")
	if err != nil {
		t.Fatalf("create second agent: %v", err)
	}
	if _, err := db.Conn().Exec(`UPDATE agents SET last_active = ? WHERE id = ?`, time.Now().Unix()+10, first.ID); err != nil {
		t.Fatalf("set first last_active: %v", err)
	}
	if _, err := db.Conn().Exec(`UPDATE agents SET last_active = ? WHERE id = ?`, time.Now().Unix(), second.ID); err != nil {
		t.Fatalf("set second last_active: %v", err)
	}

	cfg := &config.Config{ResizePanelsWithArrows: false}
	app := NewApp(nil, nil, nil, db, cfg, nil)
	app.focus = FocusAgentList

	originalSplit := app.leftSplit
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = model.(App)

	if app.leftSplit != originalSplit {
		t.Fatalf("leftSplit changed from %d to %d with resize mode disabled", originalSplit, app.leftSplit)
	}
	if app.selectedAgentID != second.ID {
		t.Fatalf("selected agent = %q, want %q after down arrow", app.selectedAgentID, second.ID)
	}
	if !strings.Contains(app.renderStatusBar(), "mode:navigate") {
		t.Fatalf("status bar missing navigate mode: %s", app.renderStatusBar())
	}
}

func TestStatusBarShowsResizeModeWhenEnabled(t *testing.T) {
	db := openTestVault(t)
	cfg := &config.Config{ResizePanelsWithArrows: true}
	app := NewApp(nil, nil, nil, db, cfg, nil)
	app.width = 180

	if !strings.Contains(app.renderStatusBar(), "mode:resize") {
		t.Fatalf("status bar missing resize mode: %s", app.renderStatusBar())
	}
}

func TestModeHotkeyTogglesResizeMode(t *testing.T) {
	db := openTestVault(t)
	cfg := &config.Config{ResizePanelsWithArrows: false}
	app := NewApp(nil, nil, nil, db, cfg, nil)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	app = model.(App)
	if !app.resizeModeEnabled() {
		t.Fatal("resize mode should be enabled after pressing m")
	}
	if app.notice == "" || !strings.Contains(app.notice, "Resize mode enabled") {
		t.Fatalf("unexpected notice after enabling resize mode: %q", app.notice)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	app = model.(App)
	if app.resizeModeEnabled() {
		t.Fatal("resize mode should be disabled after pressing m again")
	}
	if app.notice == "" || !strings.Contains(app.notice, "Resize mode disabled") {
		t.Fatalf("unexpected notice after disabling resize mode: %q", app.notice)
	}
}

func TestModeHotkeyDoesNotPersistResizePreference(t *testing.T) {
	db := openTestVault(t)
	cfg := &config.Config{ResizePanelsWithArrows: false}
	app := NewApp(nil, nil, nil, db, cfg, nil)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	app = model.(App)

	if !app.resizeModeEnabled() {
		t.Fatal("resize mode should be enabled after pressing m")
	}
	if cfg.ResizePanelsWithArrows {
		t.Fatal("mode hotkey should not persist the resize preference")
	}
}

func TestSelectedAssistantReplyRefocusesChatInput(t *testing.T) {
	db := openTestVault(t)
	cfg := &config.Config{}
	app := NewApp(nil, nil, nil, db, cfg, nil)
	app.conversation.SetSize(80, 20)

	agent, err := db.CreateAgent("Scraper", "Scrape prices", "{}")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	app.selectedAgentID = agent.ID
	app.focus = FocusConversation
	app.inputMode = "chat"
	app.conversation.SetAgentID(agent.ID)
	app.conversation.SetAgentName(agent.Name)
	app.conversation.SetAwake(true)
	app.conversation.Blur()

	model, cmd := app.Update(shared.ConversationEntryMsg{
		AgentID: agent.ID,
		Role:    "assistant",
		Content: "Ready",
	})
	app = model.(App)

	if cmd == nil {
		t.Fatal("expected selected assistant reply to return a focus command")
	}
	if !app.conversation.IsAwake() {
		t.Fatal("conversation should stay awake after assistant reply")
	}
}

func TestPendingStartupReplyRefocusesChatInput(t *testing.T) {
	db := openTestVault(t)
	cfg := &config.Config{}
	app := NewApp(nil, nil, nil, db, cfg, nil)
	app.conversation.SetSize(80, 20)

	agent, err := db.CreateAgent("Scraper", "Scrape prices", "{}")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	app.selectedAgentID = agent.ID
	app.focus = FocusAgentDetail
	app.inputMode = ""
	app.pendingChatFocusAgentID = agent.ID
	app.conversation.SetAgentID(agent.ID)
	app.conversation.SetAgentName(agent.Name)
	app.conversation.Blur()

	model, cmd := app.Update(shared.ConversationEntryMsg{
		AgentID: agent.ID,
		Role:    "assistant",
		Content: "Ready to work.",
	})
	app = model.(App)

	if cmd == nil {
		t.Fatal("expected first assistant reply to return a focus command")
	}
	if app.focus != FocusConversation {
		t.Fatalf("focus = %d, want conversation", app.focus)
	}
	if app.inputMode != "chat" {
		t.Fatalf("inputMode = %q, want chat", app.inputMode)
	}
	if app.pendingChatFocusAgentID != "" {
		t.Fatalf("pendingChatFocusAgentID = %q, want cleared", app.pendingChatFocusAgentID)
	}
	if !app.conversation.IsAwake() {
		t.Fatal("conversation should be awake after first assistant reply")
	}
}

func TestPendingStartupTerminalStatusRefocusesChatInput(t *testing.T) {
	db := openTestVault(t)
	cfg := &config.Config{}
	app := NewApp(nil, nil, nil, db, cfg, nil)
	app.conversation.SetSize(80, 20)

	agent, err := db.CreateAgent("Scraper", "Scrape prices", "{}")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	app.selectedAgentID = agent.ID
	app.focus = FocusAgentDetail
	app.inputMode = ""
	app.pendingChatFocusAgentID = agent.ID
	app.conversation.SetAgentID(agent.ID)
	app.conversation.SetAgentName(agent.Name)
	app.conversation.Blur()

	model, cmd := app.Update(shared.AgentStatusMsg{
		AgentID: agent.ID,
		Status:  "completed",
	})
	app = model.(App)

	if cmd == nil {
		t.Fatal("expected terminal status to return a focus command")
	}
	if app.focus != FocusConversation {
		t.Fatalf("focus = %d, want conversation", app.focus)
	}
	if app.inputMode != "chat" {
		t.Fatalf("inputMode = %q, want chat", app.inputMode)
	}
	if app.pendingChatFocusAgentID != "" {
		t.Fatalf("pendingChatFocusAgentID = %q, want cleared", app.pendingChatFocusAgentID)
	}
}

func TestChatInputKeystrokeRefocusesAndCapturesFirstRune(t *testing.T) {
	db := openTestVault(t)
	cfg := &config.Config{}
	app := NewApp(nil, nil, nil, db, cfg, nil)
	app.conversation.SetSize(80, 20)

	agent, err := db.CreateAgent("Scraper", "Scrape prices", "{}")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	app.selectedAgentID = agent.ID
	app.focus = FocusConversation
	app.inputMode = "chat"
	app.conversation.SetAgentID(agent.ID)
	app.conversation.SetAgentName(agent.Name)
	app.conversation.SetAwake(true)
	app.conversation.Blur()

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	app = model.(App)

	if !app.conversation.Focused() {
		t.Fatal("conversation input should refocus on first chat keystroke")
	}
	if got := app.conversation.TextInput().Value(); got != "h" {
		t.Fatalf("conversation input = %q, want %q", got, "h")
	}
}

func TestUnfocusedChatAllowsViewShortcut(t *testing.T) {
	db := openTestVault(t)
	cfg := &config.Config{}
	app := NewApp(nil, nil, nil, db, cfg, nil)
	app.conversation.SetSize(80, 20)

	agent, err := db.CreateAgent("Scraper", "Scrape prices", "{}")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	app.selectedAgentID = agent.ID
	app.focus = FocusConversation
	app.inputMode = "chat"
	app.conversation.SetAgentID(agent.ID)
	app.conversation.SetAgentName(agent.Name)
	app.conversation.SetAwake(true)
	app.conversation.Blur()

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	app = model.(App)

	if got := app.conversation.TextInput().Value(); got != "" {
		t.Fatalf("conversation input = %q, want empty when view shortcut is used", got)
	}
}

func TestFocusedChatAllowsCtrlViewShortcut(t *testing.T) {
	db := openTestVault(t)
	cfg := &config.Config{}
	app := NewApp(nil, nil, nil, db, cfg, nil)
	app.conversation.SetSize(80, 20)

	agent, err := db.CreateAgent("Scraper", "Scrape prices", "{}")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	app.selectedAgentID = agent.ID
	app.focus = FocusConversation
	app.inputMode = "chat"
	app.conversation.SetAgentID(agent.ID)
	app.conversation.SetAgentName(agent.Name)
	app.conversation.SetAwake(true)
	app.conversation.Focus()

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	app = model.(App)

	if got := app.conversation.TextInput().Value(); got != "" {
		t.Fatalf("conversation input = %q, want empty when ctrl+v shortcut is used", got)
	}
}

func TestUnfocusedChatAllowsTraceShortcut(t *testing.T) {
	db := openTestVault(t)
	cfg := &config.Config{}
	app := NewApp(nil, nil, nil, db, cfg, nil)
	app.conversation.SetSize(80, 20)

	agent, err := db.CreateAgent("Scraper", "Scrape prices", "{}")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	app.selectedAgentID = agent.ID
	app.focus = FocusConversation
	app.inputMode = "chat"
	app.conversation.SetAgentID(agent.ID)
	app.conversation.SetAgentName(agent.Name)
	app.conversation.SetAwake(true)
	app.conversation.Blur()

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	app = model.(App)

	if !app.conversation.TraceOnly() {
		t.Fatal("trace shortcut should enable trace mode from unfocused chat state")
	}
	if got := app.conversation.TextInput().Value(); got != "" {
		t.Fatalf("conversation input = %q, want empty when trace shortcut is used", got)
	}
}

func TestReconfigureShortcutQueuesWizardWithoutClearingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &config.Config{
		Provider:      "anthropic",
		APIKey:        "sk-test",
		Model:         "anthropic/claude-sonnet-4-6",
		SetupComplete: true,
	}
	app := NewApp(nil, nil, nil, nil, cfg, nil)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	updated := model.(App)

	if !cfg.SetupComplete {
		t.Fatal("reconfigure shortcut should not clear setupComplete on the active config")
	}
	if !config.ReconfigureRequested() {
		t.Fatal("reconfigure shortcut should queue the setup wizard for next launch")
	}
	if updated.notice != "" {
		t.Fatalf("unexpected notice after queuing reconfigure: %q", updated.notice)
	}
}

func TestUnfocusedChatAllowsOpenLogShortcut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &config.Config{}
	db := openTestVault(t)
	agent, err := db.CreateAgent("agent", "task", "{}")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	logPath := agentSessionLogPath(agent.ID)
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("{}\n"), 0644); err != nil {
		t.Fatalf("write session log: %v", err)
	}

	original := startExternalCommand
	defer func() { startExternalCommand = original }()
	var opened []string
	startExternalCommand = func(name string, args ...string) error {
		opened = append([]string{name}, args...)
		return nil
	}

	app := NewApp(nil, nil, nil, db, cfg, nil)
	app.conversation.SetSize(80, 20)
	app.selectedAgentID = agent.ID
	app.conversation.SetAgentID(agent.ID)
	app.conversation.SetAgentName(agent.Name)
	app.conversation.SetAwake(true)
	app.conversation.Blur()
	app.focus = FocusConversation
	app.inputMode = "chat"

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	updated := model.(App)

	if got := updated.conversation.TextInput().Value(); got != "" {
		t.Fatalf("conversation input = %q, want empty when open-log shortcut is used", got)
	}
	if len(opened) != 2 || opened[0] != "open" || opened[1] != logPath {
		t.Fatalf("unexpected open command: %#v", opened)
	}
	if updated.notice != "Opened session log" {
		t.Fatalf("notice = %q, want %q", updated.notice, "Opened session log")
	}
}

func TestFocusedChatAllowsCtrlOpenLogShortcut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &config.Config{}
	db := openTestVault(t)
	agent, err := db.CreateAgent("agent", "task", "{}")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	logPath := agentSessionLogPath(agent.ID)
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("{}\n"), 0644); err != nil {
		t.Fatalf("write session log: %v", err)
	}

	original := startExternalCommand
	defer func() { startExternalCommand = original }()
	var opened []string
	startExternalCommand = func(name string, args ...string) error {
		opened = append([]string{name}, args...)
		return nil
	}

	app := NewApp(nil, nil, nil, db, cfg, nil)
	app.conversation.SetSize(80, 20)
	app.selectedAgentID = agent.ID
	app.conversation.SetAgentID(agent.ID)
	app.conversation.SetAgentName(agent.Name)
	app.conversation.SetAwake(true)
	app.focus = FocusConversation
	app.inputMode = "chat"
	app.conversation.Focus()

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	updated := model.(App)

	if got := updated.conversation.TextInput().Value(); got != "" {
		t.Fatalf("conversation input = %q, want empty when ctrl+o shortcut is used", got)
	}
	if len(opened) != 2 || opened[0] != "open" || opened[1] != logPath {
		t.Fatalf("unexpected open command: %#v", opened)
	}
}

func TestTraceModeHotkeyTogglesConversationTrace(t *testing.T) {
	db := openTestVault(t)
	cfg := &config.Config{}
	app := NewApp(nil, nil, nil, db, cfg, nil)

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	app = model.(App)
	if !app.conversation.TraceOnly() {
		t.Fatal("trace mode should be enabled after pressing t")
	}
	if !strings.Contains(app.notice, "Trace mode enabled") {
		t.Fatalf("unexpected trace enable notice: %q", app.notice)
	}

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	app = model.(App)
	if app.conversation.TraceOnly() {
		t.Fatal("trace mode should be disabled after pressing t again")
	}
	if !strings.Contains(app.notice, "Trace mode disabled") {
		t.Fatalf("unexpected trace disable notice: %q", app.notice)
	}
}

func TestPauseResumeKeybindings(t *testing.T) {
	db := openTestVault(t)

	paused, err := db.CreateAgent("paused-agent", "task", "{}")
	if err != nil {
		t.Fatalf("create paused agent: %v", err)
	}
	if err := db.UpdateAgentStatus(paused.ID, "paused"); err != nil {
		t.Fatalf("set paused status: %v", err)
	}

	active, err := db.CreateAgent("active-agent", "task", "{}")
	if err != nil {
		t.Fatalf("create active agent: %v", err)
	}
	if err := db.UpdateAgentStatus(active.ID, "active"); err != nil {
		t.Fatalf("set active status: %v", err)
	}

	app := NewApp(nil, nil, nil, db, nil, nil)
	if err := db.UpdateAgentStatus(active.ID, "active"); err != nil {
		t.Fatalf("restore active status after startup reconciliation: %v", err)
	}

	app.selectedAgentID = active.ID
	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd == nil {
		t.Fatal("expected pause key to return a command")
	}
	pauseMsg, ok := cmd().(statusNotice)
	if !ok {
		t.Fatalf("pause command returned %T, want statusNotice", cmd())
	}
	if pauseMsg.text != "No orchestrator" {
		t.Fatalf("pause notice = %q, want %q", pauseMsg.text, "No orchestrator")
	}
	app = model.(App)

	app.selectedAgentID = paused.ID
	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("expected resume key to return a command")
	}
	resumeMsg, ok := cmd().(statusNotice)
	if !ok {
		t.Fatalf("resume command returned %T, want statusNotice", cmd())
	}
	if resumeMsg.text != "No orchestrator" {
		t.Fatalf("resume notice = %q, want %q", resumeMsg.text, "No orchestrator")
	}
}

func TestPauseResumeKeybindingsShortCircuitOnAgentState(t *testing.T) {
	db := openTestVault(t)

	paused, err := db.CreateAgent("paused-agent", "task", "{}")
	if err != nil {
		t.Fatalf("create paused agent: %v", err)
	}
	if err := db.UpdateAgentStatus(paused.ID, "paused"); err != nil {
		t.Fatalf("set paused status: %v", err)
	}

	active, err := db.CreateAgent("active-agent", "task", "{}")
	if err != nil {
		t.Fatalf("create active agent: %v", err)
	}
	if err := db.UpdateAgentStatus(active.ID, "active"); err != nil {
		t.Fatalf("set active status: %v", err)
	}

	app := NewApp(nil, nil, nil, db, nil, nil)
	if err := db.UpdateAgentStatus(active.ID, "active"); err != nil {
		t.Fatalf("restore active status after startup reconciliation: %v", err)
	}

	app.selectedAgentID = paused.ID
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd == nil {
		t.Fatal("expected pause key to return a command")
	}
	msg := cmd()
	notice, ok := msg.(statusNotice)
	if !ok {
		t.Fatalf("pause command returned %T, want statusNotice", msg)
	}
	if notice.text != "Agent already paused" {
		t.Fatalf("pause notice = %q, want %q", notice.text, "Agent already paused")
	}

	app.selectedAgentID = active.ID
	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("expected resume key to return a command")
	}
	msg = cmd()
	notice, ok = msg.(statusNotice)
	if !ok {
		t.Fatalf("resume command returned %T, want statusNotice", msg)
	}
	if notice.text != "Agent already active" {
		t.Fatalf("resume notice = %q, want %q", notice.text, "Agent already active")
	}
}

func TestBulkKillKeybindingWithoutOrchestrator(t *testing.T) {
	db := openTestVault(t)

	app := NewApp(nil, nil, nil, db, nil, nil)

	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if cmd != nil {
		t.Fatal("did not expect first X press to return a command")
	}
	app = model.(App)
	if !app.confirmKillAll {
		t.Fatal("expected bulk kill confirmation to be armed")
	}

	model, cmd = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if cmd == nil {
		t.Fatal("expected second X press to return a command")
	}
	app = model.(App)
	if app.confirmKillAll {
		t.Fatal("expected bulk kill confirmation to clear after execution")
	}
	msg := cmd()
	notice, ok := msg.(statusNotice)
	if !ok {
		t.Fatalf("bulk kill command returned %T, want statusNotice", msg)
	}
	if notice.text != "Kill all unavailable" {
		t.Fatalf("bulk kill notice = %q, want %q", notice.text, "Kill all unavailable")
	}
}

func TestBulkAgentStatusMsgMarksAgentsInterrupted(t *testing.T) {
	db := openTestVault(t)

	first, err := db.CreateAgent("first-agent", "task", "{}")
	if err != nil {
		t.Fatalf("create first agent: %v", err)
	}
	second, err := db.CreateAgent("second-agent", "task", "{}")
	if err != nil {
		t.Fatalf("create second agent: %v", err)
	}
	if err := db.UpdateAgentStatus(first.ID, "active"); err != nil {
		t.Fatalf("set first status: %v", err)
	}
	if err := db.UpdateAgentStatus(second.ID, "active"); err != nil {
		t.Fatalf("set second status: %v", err)
	}

	app := NewApp(nil, nil, nil, db, nil, nil)
	app.selectedAgentID = first.ID
	app.conversation.SetAgentID(first.ID)
	app.conversation.SetThinking(true)

	model, _ := app.Update(shared.BulkAgentStatusMsg{
		AgentIDs: []string{first.ID, second.ID},
		Status:   "interrupted",
		Notice:   "Killed 2 agents",
	})
	app = model.(App)

	if got := app.agentList.Status(first.ID); got != "interrupted" {
		t.Fatalf("first status = %q, want interrupted", got)
	}
	if got := app.agentList.Status(second.ID); got != "interrupted" {
		t.Fatalf("second status = %q, want interrupted", got)
	}
	if app.notice != "Killed 2 agents" {
		t.Fatalf("notice = %q, want %q", app.notice, "Killed 2 agents")
	}
	if app.conversation.IsThinking() {
		t.Fatal("expected selected conversation thinking state to clear")
	}
	stored, err := db.GetAgent(second.ID)
	if err != nil {
		t.Fatalf("get second agent: %v", err)
	}
	if stored.Status != "interrupted" {
		t.Fatalf("stored second status = %q, want interrupted", stored.Status)
	}
}

func TestBulkAgentStatusMsgMarksAgentsPaused(t *testing.T) {
	db := openTestVault(t)

	first, err := db.CreateAgent("first-agent", "task", "{}")
	if err != nil {
		t.Fatalf("create first agent: %v", err)
	}
	second, err := db.CreateAgent("second-agent", "task", "{}")
	if err != nil {
		t.Fatalf("create second agent: %v", err)
	}
	if err := db.UpdateAgentStatus(first.ID, "active"); err != nil {
		t.Fatalf("set first status: %v", err)
	}
	if err := db.UpdateAgentStatus(second.ID, "active"); err != nil {
		t.Fatalf("set second status: %v", err)
	}

	app := NewApp(nil, nil, nil, db, nil, nil)

	model, _ := app.Update(shared.BulkAgentStatusMsg{
		AgentIDs: []string{first.ID, second.ID},
		Status:   "paused",
		Notice:   "Paused 2 agents",
	})
	app = model.(App)

	if got := app.agentList.Status(first.ID); got != "paused" {
		t.Fatalf("first status = %q, want paused", got)
	}
	if got := app.agentList.Status(second.ID); got != "paused" {
		t.Fatalf("second status = %q, want paused", got)
	}
	if app.notice != "Paused 2 agents" {
		t.Fatalf("notice = %q, want %q", app.notice, "Paused 2 agents")
	}
	stored, err := db.GetAgent(first.ID)
	if err != nil {
		t.Fatalf("get first agent: %v", err)
	}
	if stored.Status != "paused" {
		t.Fatalf("stored first status = %q, want paused", stored.Status)
	}
}

func TestGracefulShutdownPausesActiveAgents(t *testing.T) {
	db := openTestVault(t)

	agent, err := db.CreateAgent("active-agent", "task", "{}")
	if err != nil {
		t.Fatalf("create active agent: %v", err)
	}
	if err := db.UpdateAgentStatus(agent.ID, "active"); err != nil {
		t.Fatalf("set active status: %v", err)
	}

	app := NewApp(nil, nil, nil, db, nil, nil)
	if err := db.UpdateAgentStatus(agent.ID, "active"); err != nil {
		t.Fatalf("restore active status after startup reconciliation: %v", err)
	}

	app.gracefulShutdown()

	stored, err := db.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("get agent after shutdown: %v", err)
	}
	if stored.Status != "active" {
		t.Fatalf("agent status = %q, want active without orchestrator", stored.Status)
	}
	if !shouldPauseOnShutdown("active") {
		t.Fatal("expected active agents to be paused during shutdown")
	}
	if shouldPauseOnShutdown("paused") {
		t.Fatal("did not expect paused agents to be re-paused during shutdown")
	}
}

func TestBrowserEventsDoNotPolluteSelectedConversation(t *testing.T) {
	db := openTestVault(t)

	agent, err := db.CreateAgent("agent", "task", "{}")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.AppendMessage(agent.ID, "assistant", "persisted reply", 0); err != nil {
		t.Fatalf("append message: %v", err)
	}

	app := NewApp(nil, nil, nil, db, nil, nil)
	app.conversation.SetSize(80, 20)
	app.selectedAgentID = agent.ID
	app.conversation.SetAgentID(agent.ID)
	app.conversation.LoadMessages([]vault.AgentMessage{
		{Role: "assistant", Content: "persisted reply"},
	})
	before := app.conversation.View()

	model, _ := app.Update(shared.NavigationMsg{
		SessionID: "sess-1",
		FrameID:   "frame-1",
		URL:       "https://example.com/other-agent",
	})
	app = model.(App)
	model, _ = app.Update(shared.PageLoadMsg{
		SessionID: "sess-1",
		FrameID:   "frame-1",
		Name:      "load",
	})
	app = model.(App)

	after := app.conversation.View()
	if after != before {
		t.Fatalf("conversation changed after unrelated browser events\nbefore:\n%s\n\nafter:\n%s", before, after)
	}
}
