package nanoclaw

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"vulpineos/internal/config"
	"vulpineos/internal/opencode"
	"vulpineos/internal/runtimeaudit"
)

// Manager manages multiple NanoClaw agent subprocesses.
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

	opencodeClient *opencode.Client
}

type managedAgent struct {
	agent   *Agent
	cleanup func()
}

// NewManager creates a new agent manager.
func NewManager(binary string) *Manager {
	if binary == "" {
		binary = "nanoclaw"
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
// Each call spawns a fresh NanoClaw process for one turn of conversation.
// If a previous process for this agentID is still running, it is stopped first.
func (m *Manager) SpawnWithSession(agentID, task, sessionName, configPath string) (string, error) {
	return m.SpawnWithSessionIsolated(agentID, task, sessionName, configPath, nil)
}

// SpawnWithSessionIsolated creates and starts a new agent with an optional cleanup hook.
func (m *Manager) SpawnWithSessionIsolated(agentID, task, sessionName, configPath string, cleanup func()) (string, error) {
	sessionName, err := safeSessionName(agentID, sessionName)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return "", err
	}

	if err := provisionOpenRouterIfNeeded(); err != nil {
		log.Printf("Warning: provision OpenRouter: %v", err)
	}

	useDefaultBinary := m.binary == "" || m.binary == "nanoclaw"
	if useDefaultBinary {
		if cfg, err := config.Load(); err == nil && cfg.Provider == "opencode-local" {
			return m.SpawnOpenCode(task, agentID)
		}

		_, socketFound := FindNanoclawSocket()
		if socketFound {
			return m.spawnViaSocket(agentID, sessionName, task, configPath, cleanup)
		}
	}

	nanoclawBin := m.findNanoClaw()
	if nanoclawBin == "" {
		if cleanup != nil {
			cleanup()
		}
		return "", fmt.Errorf("NanoClaw not found. Install: git clone https://github.com/qwibitai/nanoclaw.git && cd nanoclaw && pnpm tsx src/index.ts")
	}

	args := nanoclawArgs(sessionName, task)
	return m.startManagedAgent(agentID, "nanoclaw", nanoclawBin, args, configPath, cleanup)
}

// SpawnPersistent is a compatibility shim over the current one-turn NanoClaw CLI.
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
	return m.ResumeWithSessionIsolated(agentID, sessionName, configPath, nil)
}

// ResumeWithSessionIsolated reactivates a saved session without forcing a model turn
// and ties any scoped runtime cleanup to the managed agent lifecycle.
func (m *Manager) ResumeWithSessionIsolated(agentID, sessionName, configPath string, cleanup func()) (string, error) {
	if strings.TrimSpace(configPath) == "" {
		configPath = config.OpenClawConfigPath()
	}
	return m.SpawnWithSessionIsolated(agentID, "Continue from the saved session and resume the current task.", sessionName, configPath, cleanup)
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
		m.safeForwardConversation(msg)
	}
}

func (m *Manager) safeForwardConversation(msg ConversationMsg) {
	defer func() {
		_ = recover()
	}()
	select {
	case m.conversationSource <- msg:
	default:
		// Manager channel full or closing, drop
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

// Spawn creates and starts a new agent turn using the current NanoClaw session CLI.
// The newer CLI no longer accepts legacy context-id or SOP flags directly, so the
// template SOP content is sent as the first turn and browser attachment is handled
// by the generated NanoClaw profile config.
func (m *Manager) Spawn(contextID string, sopFile string, extraArgs ...string) (string, error) {
	return m.SpawnIsolated(contextID, sopFile, "", nil, extraArgs...)
}

// SpawnIsolated starts an agent with an optional per-run NanoClaw config and cleanup hook.
func (m *Manager) SpawnIsolated(contextID string, sopFile string, configPath string, cleanup func(), extraArgs ...string) (string, error) {
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

	sessionName := "vulpine-" + id

	// Try socket first (daemon mode) when using default binary
	useDefaultBinary := m.binary == "" || m.binary == "nanoclaw"
	if useDefaultBinary {
		if _, socketFound := FindNanoclawSocket(); socketFound {
			return m.spawnViaSocket(id, sessionName, message, configPath, cleanup)
		}
	}

	nanoclawBin := m.findNanoClaw()
	if nanoclawBin == "" {
		if cleanup != nil {
			cleanup()
		}
		return "", fmt.Errorf("NanoClaw not found. Install: git clone https://github.com/nanocoai/nanoclaw.git")
	}

	args := nanoclawArgs(sessionName, message)
	return m.startManagedAgent(id, contextID, nanoclawBin, args, configPath, cleanup)
}

// SpawnNanoClaw spawns an agent using NanoClaw.
// It sends a task message to NanoClaw to start an agent run.
func (m *Manager) SpawnNanoClaw(task string, agentSkills []config.SkillEntry) (string, error) {
	// Find NanoClaw binary
	nanoclawBin := m.findNanoClaw()
	if nanoclawBin == "" {
		return "", fmt.Errorf("NanoClaw not found. Install: git clone https://github.com/nanocoai/nanoclaw.git")
	}

	id := uuid.New().String()[:8]

	// NanoClaw args: run with session and task
	args := []string{
		"run",
		"--session", "vulpine-" + id,
		task,
	}
	_ = agentSkills

	return m.startManagedAgent(id, "nanoclaw", nanoclawBin, args, "", nil)
}

// SpawnOpenCode spawns an agent using the local OpenCode server.
func (m *Manager) SpawnOpenCode(task string, vaultAgentID string) (string, error) {
	if m.opencodeClient == nil {
		m.opencodeClient = opencode.NewClient("opencode")
		if err := m.opencodeClient.Start(); err != nil {
			return "", fmt.Errorf("failed to start opencode server: %w", err)
		}
	}

	id := vaultAgentID
	if id == "" {
		id = uuid.New().String()[:8]
	}

	prompt := OpenCodePrompt(id, task)

	agent := newAgent(id, "opencode-local", m.statusSource)

	m.mu.Lock()
	m.agents[id] = &managedAgent{agent: agent, cleanup: nil}
	m.mu.Unlock()

	var totalTokens int

	go func() {
		log.Printf("opencode-local: starting agent %s with prompt: %s", id, prompt[:min(50, len(prompt))])
		_, _, err := m.opencodeClient.SendMessageWithCallback(prompt, func(text string, done bool, tok int) {
			if text != "" {
				log.Printf("opencode-local: streaming chunk: %s", text[:min(50, len(text))])
				agent.conversationCh <- ConversationMsg{
					AgentID: id,
					Role:    "assistant",
					Content: text,
					Tokens:  0,
				}
			}
			if done {
				log.Printf("opencode-local: done, tokens: %d", tok)
				totalTokens = tok
			}
		})
		if err != nil {
			agent.mu.Lock()
			agent.status.Status = "error"
			agent.status.Objective = err.Error()
			agent.mu.Unlock()
			agent.statusCh <- agent.status
			return
		}

		agent.mu.Lock()
		agent.status.Status = "completed"
		agent.status.Objective = task
		agent.status.Tokens = totalTokens
		agent.mu.Unlock()

		agent.statusCh <- agent.status
		close(agent.doneCh)
		close(agent.conversationCh)
	}()

	go m.forwardConversation(agent)

	return id, nil
}

func (m *Manager) spawnViaSocket(agentID, sessionName, task, configPath string, cleanup func()) (string, error) {
	nanoclawDir := GetNanoclawDir()
	if nanoclawDir == "" {
		return "", fmt.Errorf("nanoclaw directory not found")
	}

	client := NewNanoclawClient(nanoclawDir)

	if !client.IsRunning() {
		return "", fmt.Errorf("nanoclaw daemon not running. Start with: cd nanoclaw && pnpm tsx src/index.ts")
	}
	if err := ensureVulpineAgentRoute(nanoclawDir, agentID); err != nil {
		if cleanup != nil {
			cleanup()
		}
		return "", err
	}

	agent := newAgent(agentID, sessionName, m.statusSource)
	agent.sessionLogPath = ""

	m.mu.Lock()
	m.agents[agentID] = &managedAgent{agent: agent, cleanup: cleanup}
	m.mu.Unlock()

	go func() {
		err := client.SendAgentMessage(agentID, task, func(chunk string, done bool) {
			if chunk == "[superseded by a newer client]" {
				return
			}
			if chunk != "" {
				agent.conversationCh <- ConversationMsg{
					AgentID: agentID,
					Role:    "assistant",
					Content: chunk,
					Tokens:  0,
				}
			}
			if done {
				agent.mu.Lock()
				agent.status.Status = "completed"
				agent.status.Tokens = 0
				agent.mu.Unlock()
				agent.statusCh <- agent.status
				close(agent.doneCh)
				close(agent.conversationCh)
			}
		})
		if err != nil {
			agent.mu.Lock()
			agent.status.Status = "error"
			agent.status.Objective = err.Error()
			agent.mu.Unlock()
			agent.statusCh <- agent.status
		}
	}()

	go m.forwardConversation(agent)

	return agentID, nil
}

// findNanoClaw looks for the NanoClaw binary in common locations.
// Searches: explicit binary path, exe dir, cwd, parent dirs (for monorepo), global PATH.
func (m *Manager) findNanoClaw() string {
	// If binary was explicitly set, treat it as authoritative.
	if m.binary != "" && m.binary != "nanoclaw" {
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
			filepath.Join(d, "node_modules", ".bin", "nanoclaw"),
			filepath.Join(d, "node_modules", "nanoclaw", "bin", "nanoclaw"),
			filepath.Join(d, "nanoclaw", "nanoclaw.sh"),
			filepath.Join(d, "nanoclaw", "nanoclaw"),
		}
		for _, c := range candidates {
			if isRunnable(c) {
				abs, _ := filepath.Abs(c)
				return abs
			}
		}
	}

	for _, d := range searchDirs {
		bundledDir := filepath.Join(d, "nanoclaw")
		bundledMain := filepath.Join(bundledDir, "node_modules", "nanoclaw", "bin", "nanoclaw")
		if _, err := os.Stat(bundledMain); err == nil {
			abs, _ := filepath.Abs(bundledMain)
			return abs
		}
	}

	// Check global install
	if path, err := exec.LookPath("nanoclaw"); err == nil {
		return path
	}

	return ""
}

// NanoClawInstalled returns true if NanoClaw is available.
func (m *Manager) NanoClawInstalled() bool {
	return m.findNanoClaw() != ""
}

func nanoclawArgs(sessionName, message string) []string {
	args := []string{
		"run",
		"--session", sessionName,
	}
	if message != "" {
		args = append(args, message)
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
	return entry.agent.stopWithStatus("interrupted")
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
	m.mu.Lock()
	agents := make([]*managedAgent, 0, len(m.agents))
	for id, a := range m.agents {
		agents = append(agents, a)
		delete(m.agents, id)
	}
	m.mu.Unlock()

	for _, a := range agents {
		a.agent.stopWithStatus("interrupted")
		waitAgentDone(a.agent, 2*time.Second)
		if a.cleanup != nil {
			a.cleanup()
		}
	}
}

// Dispose kills all agents and closes channels.
func (m *Manager) Dispose() {
	m.KillAll()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	close(m.statusSource)
	close(m.conversationSource)
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

func waitAgentDone(agent *Agent, timeout time.Duration) {
	if agent == nil {
		return
	}
	select {
	case <-agent.doneCh:
	case <-time.After(timeout):
	}
}

func (m *Manager) startManagedAgent(agentID, contextID, nanoclawBin string, args []string, configPath string, cleanup func()) (string, error) {
	var old *managedAgent
	sessionLogPath := ""
	if sessionID := sessionIDFromArgs(args); sessionID != "" {
		path, err := sessionLogPathForSessionID(sessionID)
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return "", err
		}
		sessionLogPath = path
	}

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
	agent.sessionLogPath = sessionLogPath
	if configPath != "" {
		agent.env = runtimeEnvForConfig(configPath)
	}
	if err := agent.start(nanoclawBin, args); err != nil {
		if cleanup != nil {
			cleanup()
		}
		m.logRuntimeEvent("error", "start_failed", "failed to start NanoClaw agent", map[string]string{
			"binary": nanoclawBin,
			"error":  err.Error(),
		})
		return "", fmt.Errorf("spawn failed (binary=%s): %w", nanoclawBin, err)
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
				m.logRuntimeEvent("error", "crashed", "NanoClaw agent crashed", map[string]string{
					"agent_id":  agentID,
					"context":   contextID,
					"exit_code": fmt.Sprintf("%d", exitCode),
					"attempt":   fmt.Sprintf("%d", agent.RestartCount),
				})
				log.Printf("nanoclaw: agent %s crashed (exit %d), restarting (attempt %d/3)", agentID, exitCode, agent.RestartCount)
				time.Sleep(time.Duration(agent.RestartCount) * time.Second)
				if err := agent.restart(); err != nil {
					m.logRuntimeEvent("error", "restart_failed", "NanoClaw agent restart failed", map[string]string{
						"agent_id": agentID,
						"context":  contextID,
						"attempt":  fmt.Sprintf("%d", agent.RestartCount),
						"error":    err.Error(),
					})
					log.Printf("nanoclaw: agent %s restart failed: %v", agentID, err)
					break
				}
				m.logRuntimeEvent("warn", "restarted", "NanoClaw agent restarted", map[string]string{
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

func safeSessionName(agentID, sessionName string) (string, error) {
	name := strings.TrimSpace(sessionName)
	if name == "" {
		id := strings.TrimSpace(agentID)
		if id == "" {
			return "", fmt.Errorf("agentID is required")
		}
		name = "vulpine-" + id
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "", fmt.Errorf("invalid sessionName")
	}
	return name, nil
}

func sessionLogPathForSessionID(sessionID string) (string, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return "", nil
	}
	if strings.ContainsAny(id, `/\`) || id == "." || id == ".." {
		return "", fmt.Errorf("invalid sessionName")
	}
	sessionsDir := filepath.Join(config.OpenClawProfileDir(), "agents", "main", "sessions")
	path := filepath.Join(sessionsDir, id+".jsonl")
	rel, err := filepath.Rel(sessionsDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid sessionName")
	}
	return path, nil
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
	if _, err := audit.Log("nanoclaw", level, event, message, metadata); err != nil {
		log.Printf("runtime audit nanoclaw %s: %v", event, err)
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

func provisionOpenRouterIfNeeded() error {
	logFile, err := os.OpenFile("/tmp/vulpine-provision.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		defer logFile.Close()
		logFile.WriteString("=== provisionOpenRouterIfNeeded called ===\n")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Printf("PROVISIONING: config.Load failed: %v", err)
		if logFile != nil { logFile.WriteString(fmt.Sprintf("config.Load failed: %v\n", err)) }
		return nil
	}
	log.Printf("PROVISIONING: loaded config - provider=%s model=%s", cfg.Provider, cfg.Model)
	if logFile != nil { logFile.WriteString(fmt.Sprintf("config: provider=%s model=%s\n", cfg.Provider, cfg.Model)) }
	if cfg.Provider != "openrouter" {
		log.Printf("PROVISIONING: not openrouter provider (=%s), skipping", cfg.Provider)
		return nil
	}

	log.Printf("PROVISIONING: OpenRouter provider detected, provisioning NanoClaw...")
	if logFile != nil { logFile.WriteString("OpenRouter detected, provisioning...\n") }

	nanoclawDir := GetNanoclawDir()
	if nanoclawDir == "" {
		log.Printf("PROVISIONING: NanoClaw dir not found, skipping")
		if logFile != nil { logFile.WriteString("NanoClaw dir not found\n") }
		return nil
	}

	agentGroupID, err := LookupNanoclawAgentGroupID(nanoclawDir)
	if err != nil {
		return fmt.Errorf("lookup nanoclaw agent group: %w", err)
	}
	if agentGroupID == "" {
		log.Printf("PROVISIONING: No agent group ID found, skipping")
		if logFile != nil { logFile.WriteString("No agent group ID\n") }
		return nil
	}

	log.Printf("PROVISIONING: Setting container config for agent group %s", agentGroupID[:8])
	if logFile != nil { logFile.WriteString(fmt.Sprintf("Setting container config for %s\n", agentGroupID[:8])) }
	if err := SetContainerConfig(nanoclawDir, agentGroupID, "opencode", cfg.Model); err != nil {
		return fmt.Errorf("set container config: %w", err)
	}

	secretPath := filepath.Join(nanoclawDir, "data", "secrets.yaml")
	log.Printf("PROVISIONING: Creating OpenRouter secret at %s", secretPath)
	if logFile != nil { logFile.WriteString(fmt.Sprintf("Creating secret at %s\n", secretPath)) }
	if err := CreateOpenRouterSecret(secretPath, cfg.APIKey); err != nil {
		return fmt.Errorf("create openrouter secret: %w", err)
	}

	log.Printf("PROVISIONING: Done - provider=opencode, model=%s", cfg.Model)
	if logFile != nil { logFile.WriteString(fmt.Sprintf("DONE - provider=opencode model=%s\n", cfg.Model)) }
	return nil
}
