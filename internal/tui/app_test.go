package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
