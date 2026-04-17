package openclaw

import (
	"encoding/json"
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
	"vulpineos/internal/runtimeaudit"
)

// Manager manages multiple OpenClaw agent subprocesses.
type Manager struct {
	agents map[string]*managedAgent
	mu     sync.RWMutex
	binary string
	closed bool

	statusSource       chan AgentStatus
	conversationSource chan ConversationMsg
	statusSubs         map[chan AgentStatus]struct{}
	conversationSubs   map[chan ConversationMsg]struct{}
	audit              *runtimeaudit.Manager
}

type managedAgent struct {
	agent   *Agent
	cleanup func()
}

// NewManager creates a new agent manager.
func NewManager(binary string) *Manager {
	if binary == "" {
		binary = "openclaw"
	}
	m := &Manager{
		agents:             make(map[string]*managedAgent),
		binary:             binary,
		statusSource:       make(chan AgentStatus, 64),
		conversationSource: make(chan ConversationMsg, 64),
		statusSubs:         make(map[chan AgentStatus]struct{}),
		conversationSubs:   make(map[chan ConversationMsg]struct{}),
	}
	go m.fanOutStatus()
	go m.fanOutConversation()
	return m
}

// SetRuntimeAudit attaches a runtime audit manager.
func (m *Manager) SetRuntimeAudit(audit *runtimeaudit.Manager) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit = audit
}

// StatusChan returns the channel for agent status updates.
func (m *Manager) StatusChan() <-chan AgentStatus {
	ch := make(chan AgentStatus, 64)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		close(ch)
		return ch
	}
	m.statusSubs[ch] = struct{}{}
	m.mu.Unlock()
	return ch
}

// ConversationChan returns the channel for agent conversation messages.
func (m *Manager) ConversationChan() <-chan ConversationMsg {
	ch := make(chan ConversationMsg, 64)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		close(ch)
		return ch
	}
	m.conversationSubs[ch] = struct{}{}
	m.mu.Unlock()
	return ch
}

// SpawnWithSession creates and starts a new agent with a named session for state persistence.
// Each call spawns a fresh OpenClaw process for one turn of conversation.
// If a previous process for this agentID is still running, it is stopped first.
func (m *Manager) SpawnWithSession(agentID, task, sessionName, configPath string) (string, error) {
	return m.SpawnWithSessionIsolated(agentID, task, sessionName, configPath, nil)
}

// SpawnWithSessionIsolated creates and starts a new agent with an optional cleanup hook.
func (m *Manager) SpawnWithSessionIsolated(agentID, task, sessionName, configPath string, cleanup func()) (string, error) {
	openclawBin := m.findOpenClaw()
	if openclawBin == "" {
		return "", fmt.Errorf("OpenClaw not found. Run 'npm install' in the VulpineOS directory or install globally: npm install -g openclaw")
	}

	args := agentTurnArgs(sessionName, task)
	return m.startManagedAgent(agentID, "openclaw", openclawBin, args, configPath, cleanup)
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
	if strings.TrimSpace(configPath) == "" {
		configPath = config.OpenClawConfigPath()
	}
	return m.SpawnWithSession(agentID, "Continue from the saved session and resume the current task.", sessionName, configPath)
}

// PauseAgent saves state and stops an agent.
func (m *Manager) PauseAgent(agentID string) error {
	m.mu.RLock()
	entry, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}

	return entry.agent.stopWithStatus("paused")
}

// forwardConversation reads from an agent's conversationCh and sends to the manager's channel.
func (m *Manager) forwardConversation(agent *Agent) {
	for msg := range agent.conversationCh {
		select {
		case m.conversationSource <- msg:
		default:
			// Manager channel full, drop
		}
	}
}

// SendMessage sends a message to a running agent's stdin.
func (m *Manager) SendMessage(agentID, text string) error {
	m.mu.RLock()
	entry, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}
	return entry.agent.SendMessage(text)
}

// Spawn creates and starts a new agent turn using the current OpenClaw session CLI.
// The newer CLI no longer accepts legacy context-id or SOP flags directly, so the
// template SOP content is sent as the first turn and browser attachment is handled
// by the generated OpenClaw profile config.
func (m *Manager) Spawn(contextID string, sopFile string, extraArgs ...string) (string, error) {
	return m.SpawnIsolated(contextID, sopFile, "", nil, extraArgs...)
}

// SpawnIsolated starts an agent with an optional per-run OpenClaw config and cleanup hook.
func (m *Manager) SpawnIsolated(contextID string, sopFile string, configPath string, cleanup func(), extraArgs ...string) (string, error) {
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
	return m.startManagedAgent(id, contextID, openclawBin, args, configPath, cleanup)
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

	return m.startManagedAgent(id, "openclaw", openclawBin, args, "", nil)
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
	entry, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}
	return entry.agent.Stop()
}

// List returns the status of all active agents.
func (m *Manager) List() []AgentStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]AgentStatus, 0, len(m.agents))
	for _, entry := range m.agents {
		statuses = append(statuses, entry.agent.Status())
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
	agents := make([]*managedAgent, 0, len(m.agents))
	for _, a := range m.agents {
		agents = append(agents, a)
	}
	m.mu.RUnlock()

	for _, a := range agents {
		a.agent.Stop()
		if a.cleanup != nil {
			a.cleanup()
		}
	}
}

// Dispose kills all agents and closes channels.
func (m *Manager) Dispose() {
	m.KillAll()
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	close(m.statusSource)
	close(m.conversationSource)
	m.mu.Lock()
	for ch := range m.statusSubs {
		close(ch)
		delete(m.statusSubs, ch)
	}
	for ch := range m.conversationSubs {
		close(ch)
		delete(m.conversationSubs, ch)
	}
	m.mu.Unlock()
}

func (m *Manager) startManagedAgent(agentID, contextID, openclawBin string, args []string, configPath string, cleanup func()) (string, error) {
	var old *managedAgent

	m.mu.Lock()
	if existing, ok := m.agents[agentID]; ok {
		old = existing
		delete(m.agents, agentID)
	}
	m.mu.Unlock()

	if old != nil {
		old.agent.Stop()
		if old.cleanup != nil {
			old.cleanup()
		}
	}

	agent := newAgent(agentID, contextID, m.statusSource)
	if sessionID := sessionIDFromArgs(args); sessionID != "" {
		agent.sessionLogPath = filepath.Join(config.OpenClawProfileDir(), "agents", "main", "sessions", sessionID+".jsonl")
	}
	if configPath != "" {
		agent.env = runtimeEnvForConfig(configPath)
	}
	if err := agent.start(openclawBin, args); err != nil {
		if cleanup != nil {
			cleanup()
		}
		m.logRuntimeEvent("error", "start_failed", "failed to start OpenClaw agent", map[string]string{
			"agent_id": agentID,
			"context":  contextID,
			"error":    err.Error(),
		})
		return "", fmt.Errorf("spawn failed (binary=%s): %w", openclawBin, err)
	}

	entry := &managedAgent{agent: agent, cleanup: cleanup}

	m.mu.Lock()
	m.agents[agentID] = entry
	m.mu.Unlock()

	go m.forwardConversation(agent)

	go func() {
		for {
			agent.Wait()
			exitCode := agent.ExitCode()
			if exitCode != 0 && agent.RestartCount < 3 && !agent.StopRequested() {
				agent.RestartCount++
				m.logRuntimeEvent("error", "crashed", "OpenClaw agent crashed", map[string]string{
					"agent_id":  agentID,
					"context":   contextID,
					"exit_code": fmt.Sprintf("%d", exitCode),
					"attempt":   fmt.Sprintf("%d", agent.RestartCount),
				})
				fmt.Printf("agent %s crashed (exit %d), restarting (attempt %d/3)\n", agentID, exitCode, agent.RestartCount)
				time.Sleep(time.Duration(agent.RestartCount) * time.Second)
				if err := agent.restart(); err != nil {
					m.logRuntimeEvent("error", "restart_failed", "OpenClaw agent restart failed", map[string]string{
						"agent_id": agentID,
						"context":  contextID,
						"attempt":  fmt.Sprintf("%d", agent.RestartCount),
						"error":    err.Error(),
					})
					fmt.Printf("agent %s restart failed: %v\n", agentID, err)
					break
				}
				m.logRuntimeEvent("warn", "restarted", "OpenClaw agent restarted", map[string]string{
					"agent_id": agentID,
					"context":  contextID,
					"attempt":  fmt.Sprintf("%d", agent.RestartCount),
				})
				go m.forwardConversation(agent)
				continue
			}
			break
		}

		m.mu.Lock()
		current := m.agents[agentID]
		if current == entry {
			delete(m.agents, agentID)
		}
		m.mu.Unlock()

		if current == entry && entry.cleanup != nil {
			entry.cleanup()
		}
	}()

	return agentID, nil
}

func sessionIDFromArgs(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--session-id" {
			return args[i+1]
		}
	}
	return ""
}

func (m *Manager) logRuntimeEvent(level, event, message string, metadata map[string]string) {
	m.mu.RLock()
	audit := m.audit
	m.mu.RUnlock()
	if audit == nil {
		return
	}
	if _, err := audit.Log("openclaw", level, event, message, metadata); err != nil {
		fmt.Printf("runtime audit openclaw %s: %v\n", event, err)
	}
}

func runtimeEnvForConfig(configPath string) map[string]string {
	if strings.TrimSpace(configPath) == "" {
		return nil
	}

	env := map[string]string{
		"OPENCLAW_CONFIG_PATH": configPath,
	}
	if token, err := gatewayAuthToken(configPath); err == nil && strings.TrimSpace(token) != "" {
		env["OPENCLAW_GATEWAY_TOKEN"] = token
	}
	return env
}

func gatewayAuthToken(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", err
	}

	gateway, ok := cfg["gateway"].(map[string]interface{})
	if !ok || gateway == nil {
		return "", nil
	}
	auth, ok := gateway["auth"].(map[string]interface{})
	if !ok || auth == nil {
		return "", nil
	}
	token, _ := auth["token"].(string)
	return token, nil
}

func (m *Manager) fanOutStatus() {
	for status := range m.statusSource {
		m.mu.RLock()
		for ch := range m.statusSubs {
			select {
			case ch <- status:
			default:
			}
		}
		m.mu.RUnlock()
	}
}

func (m *Manager) fanOutConversation() {
	for msg := range m.conversationSource {
		m.mu.RLock()
		for ch := range m.conversationSubs {
			select {
			case ch <- msg:
			default:
			}
		}
		m.mu.RUnlock()
	}
}
