package openclaw

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"vulpineos/internal/config"
)

// Manager manages multiple OpenClaw agent subprocesses.
type Manager struct {
	agents         map[string]*Agent
	statusCh       chan AgentStatus
	conversationCh chan ConversationMsg
	mu             sync.RWMutex
	binary         string
}

// NewManager creates a new agent manager.
func NewManager(binary string) *Manager {
	if binary == "" {
		binary = "openclaw"
	}
	return &Manager{
		agents:         make(map[string]*Agent),
		statusCh:       make(chan AgentStatus, 64),
		conversationCh: make(chan ConversationMsg, 64),
		binary:         binary,
	}
}

// StatusChan returns the channel for agent status updates.
func (m *Manager) StatusChan() <-chan AgentStatus {
	return m.statusCh
}

// ConversationChan returns the channel for agent conversation messages.
func (m *Manager) ConversationChan() <-chan ConversationMsg {
	return m.conversationCh
}

// SpawnWithSession creates and starts a new agent with a named session for state persistence.
// Each call spawns a fresh OpenClaw process for one turn of conversation.
// If a previous process for this agentID is still running, it is stopped first.
func (m *Manager) SpawnWithSession(agentID, task, sessionName, configPath string) (string, error) {
	openclawBin := m.findOpenClaw()
	if openclawBin == "" {
		return "", fmt.Errorf("OpenClaw not found. Run 'npm install' in the VulpineOS directory or install globally: npm install -g openclaw")
	}

	// Stop any previous process for this agent before spawning a new one.
	m.mu.Lock()
	if old, ok := m.agents[agentID]; ok {
		delete(m.agents, agentID)
		m.mu.Unlock()
		old.Stop()
	} else {
		m.mu.Unlock()
	}

	args := agentTurnArgs(sessionName, task)

	agent := newAgent(agentID, "openclaw", m.statusCh)

	if err := agent.start(openclawBin, args); err != nil {
		return "", fmt.Errorf("spawn failed (binary=%s): %w", openclawBin, err)
	}

	m.mu.Lock()
	m.agents[agentID] = agent
	m.mu.Unlock()

	// Wire agent conversation channel to manager's channel
	go m.forwardConversation(agent)

	// Auto-cleanup when agent exits — only delete if this is still the current agent
	go func() {
		agent.Wait()
		m.mu.Lock()
		if m.agents[agentID] == agent {
			delete(m.agents, agentID)
		}
		m.mu.Unlock()
	}()

	return agentID, nil
}

// SpawnPersistent is a compatibility shim over the current one-turn OpenClaw CLI.
func (m *Manager) SpawnPersistent(agentID, initialMessage, sessionName string) (string, error) {
	return m.SpawnWithSession(agentID, initialMessage, sessionName, config.OpenClawConfigPath())
}

// SendMessageOrRespawn sends a one-turn message using the current session id.
func (m *Manager) SendMessageOrRespawn(agentID, text, sessionName string) error {
	_, err := m.SpawnWithSession(agentID, text, sessionName, config.OpenClawConfigPath())
	return err
}

// ResumeWithSession reactivates a saved session without forcing a model turn.
func (m *Manager) ResumeWithSession(agentID, sessionName, configPath string) (string, error) {
	openclawBin := m.findOpenClaw()
	if openclawBin == "" {
		return "", fmt.Errorf("OpenClaw not found. Run 'npm install' in the VulpineOS directory or install globally: npm install -g openclaw")
	}
	_ = openclawBin
	_ = sessionName
	_ = configPath

	// Stop any previous process for this agent before resuming.
	m.mu.Lock()
	if old, ok := m.agents[agentID]; ok {
		delete(m.agents, agentID)
		m.mu.Unlock()
		old.Stop()
	} else {
		m.mu.Unlock()
	}

	return agentID, nil
}

// PauseAgent saves state and stops an agent.
func (m *Manager) PauseAgent(agentID string) error {
	m.mu.RLock()
	agent, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}

	return agent.Stop()
}

// forwardConversation reads from an agent's conversationCh and sends to the manager's channel.
func (m *Manager) forwardConversation(agent *Agent) {
	for msg := range agent.conversationCh {
		select {
		case m.conversationCh <- msg:
		default:
			// Manager channel full, drop
		}
	}
}

// SendMessage sends a message to a running agent's stdin.
func (m *Manager) SendMessage(agentID, text string) error {
	m.mu.RLock()
	agent, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}
	return agent.SendMessage(text)
}

// Spawn creates and starts a new agent turn using the current OpenClaw session CLI.
// The newer CLI no longer accepts legacy context-id or SOP flags directly, so the
// template SOP content is sent as the first turn and browser attachment is handled
// by the generated OpenClaw profile config.
func (m *Manager) Spawn(contextID string, sopFile string, extraArgs ...string) (string, error) {
	openclawBin := m.findOpenClaw()
	if openclawBin == "" {
		return "", fmt.Errorf("OpenClaw not found. Run 'npm install' in the VulpineOS directory or install globally: npm install -g openclaw")
	}

	id := uuid.New().String()[:8]

	message := ""
	if sopFile != "" {
		data, err := os.ReadFile(sopFile)
		if err != nil {
			return "", fmt.Errorf("read SOP: %w", err)
		}
		message = strings.TrimSpace(string(data))
	}
	if message == "" && len(extraArgs) > 0 {
		message = strings.Join(extraArgs, " ")
	}
	if message == "" {
		message = "Start."
	}

	args := agentTurnArgs("vulpine-"+id, message)

	agent := newAgent(id, contextID, m.statusCh)
	if err := agent.start(openclawBin, args); err != nil {
		return "", fmt.Errorf("spawn agent: %w", err)
	}

	m.mu.Lock()
	m.agents[id] = agent
	m.mu.Unlock()

	// Auto-restart on crash (up to 3 attempts), then cleanup
	go func() {
		for {
			agent.Wait()
			exitCode := agent.ExitCode()

			// Only restart on non-zero exit (crash), not clean completion
			if exitCode != 0 && agent.RestartCount < 3 {
				agent.RestartCount++
				fmt.Printf("agent %s crashed (exit %d), restarting (attempt %d/3)\n", id, exitCode, agent.RestartCount)
				time.Sleep(time.Duration(agent.RestartCount) * time.Second) // backoff
				if err := agent.restart(); err != nil {
					fmt.Printf("agent %s restart failed: %v\n", id, err)
					break
				}
				go m.forwardConversation(agent)
				continue
			}
			break
		}
		m.mu.Lock()
		delete(m.agents, id)
		m.mu.Unlock()
	}()

	return id, nil
}

// SpawnOpenClaw spawns a real OpenClaw agent using the VulpineOS-generated config.
// It sends a task message to OpenClaw's gateway to start an agent run.
func (m *Manager) SpawnOpenClaw(task string, agentSkills []config.SkillEntry) (string, error) {
	// Find OpenClaw binary
	openclawBin := m.findOpenClaw()
	if openclawBin == "" {
		return "", fmt.Errorf("OpenClaw not found. Run ./scripts/bundle-openclaw.sh or install globally: npm install -g openclaw")
	}

	id := uuid.New().String()[:8]

	// OpenClaw args: run with our config, send a task
	args := []string{
		"--profile", "vulpine",
		"agent",
		"--local",
		"--session-id", "vulpine-" + id,
		"-m", task,
		"--json",
	}
	_ = agentSkills

	agent := newAgent(id, "openclaw", m.statusCh)
	if err := agent.start(openclawBin, args); err != nil {
		return "", fmt.Errorf("spawn openclaw: %w", err)
	}

	m.mu.Lock()
	m.agents[id] = agent
	m.mu.Unlock()

	go func() {
		agent.Wait()
		m.mu.Lock()
		delete(m.agents, id)
		m.mu.Unlock()
	}()

	return id, nil
}

// findOpenClaw looks for the OpenClaw binary in common locations.
// Searches: explicit binary path, exe dir, cwd, parent dirs (for monorepo), global PATH.
func (m *Manager) findOpenClaw() string {
	// If binary was explicitly set, treat it as authoritative.
	if m.binary != "" && m.binary != "openclaw" {
		if isRunnable(m.binary) {
			return m.binary
		}
		return ""
	}

	// Get the directory containing the vulpineos binary
	exePath, err := os.Executable()
	if err != nil {
		exePath = "."
	}
	exeDir := filepath.Dir(exePath)

	// Also check the working directory and its parents (up to 3 levels)
	cwd, _ := os.Getwd()

	searchDirs := []string{exeDir, cwd}
	// Walk up from cwd to find node_modules (handles being in subdirectories)
	dir := cwd
	for i := 0; i < 5; i++ {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		searchDirs = append(searchDirs, parent)
		dir = parent
	}

	for _, d := range searchDirs {
		candidates := []string{
			filepath.Join(d, "node_modules", ".bin", "openclaw"),
			filepath.Join(d, "node_modules", "openclaw", "openclaw.mjs"),
			filepath.Join(d, "openclaw", "start.sh"),
		}
		for _, c := range candidates {
			if isRunnable(c) {
				abs, _ := filepath.Abs(c)
				return abs
			}
		}
	}

	// Check global install
	if path, err := exec.LookPath("openclaw"); err == nil {
		return path
	}

	return ""
}

// OpenClawInstalled returns true if OpenClaw is available.
func (m *Manager) OpenClawInstalled() bool {
	return m.findOpenClaw() != ""
}

func agentTurnArgs(sessionName, message string) []string {
	args := []string{
		"--profile", "vulpine",
		"agent",
		"--local",
		"--session-id", sessionName,
		"--json",
	}
	if message != "" {
		args = append(args, "-m", message)
	}
	return args
}

func isRunnable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Type() == fs.FileMode(0) && info.Mode()&0111 != 0
}

// Kill stops an agent by ID.
func (m *Manager) Kill(agentID string) error {
	m.mu.RLock()
	agent, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}
	return agent.Stop()
}

// List returns the status of all active agents.
func (m *Manager) List() []AgentStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]AgentStatus, 0, len(m.agents))
	for _, agent := range m.agents {
		statuses = append(statuses, agent.Status())
	}
	return statuses
}

// Count returns the number of active agents.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.agents)
}

// KillAll stops all agents.
func (m *Manager) KillAll() {
	m.mu.RLock()
	agents := make([]*Agent, 0, len(m.agents))
	for _, a := range m.agents {
		agents = append(agents, a)
	}
	m.mu.RUnlock()

	for _, a := range agents {
		a.Stop()
	}
}

// Dispose kills all agents and closes channels.
func (m *Manager) Dispose() {
	m.KillAll()
	close(m.statusCh)
	close(m.conversationCh)
}
