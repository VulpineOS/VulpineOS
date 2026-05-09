package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"vulpineos/internal/config"
	"vulpineos/internal/extensions"
	"vulpineos/internal/juggler"
	"vulpineos/internal/kernel"
	"vulpineos/internal/monitor"
	"vulpineos/internal/openclaw"
	"vulpineos/internal/orchestrator"
	"vulpineos/internal/proxy"
	"vulpineos/internal/runtimeaudit"
	"vulpineos/internal/sentinelcapture"
	"vulpineos/internal/tui/agentdetail"
	"vulpineos/internal/tui/agentlist"
	"vulpineos/internal/tui/contextlist"
	"vulpineos/internal/tui/conversation"
	"vulpineos/internal/tui/settings"
	"vulpineos/internal/tui/shared"
	"vulpineos/internal/tui/systeminfo"
	"vulpineos/internal/vault"
)

var startExternalCommand = func(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}

// Focus panel identifiers.
const (
	FocusAgentList    = 0
	FocusConversation = 1
	FocusAgentDetail  = 2
	FocusContextList  = 3
	FocusSettings     = 4
	FocusNormalCount  = 4 // number of panels in normal Tab cycle (excludes settings)
)

// statusNotice is a transient message shown in the status bar.
type statusNotice struct {
	text string
}

// ControlClient sends panel/control commands over a remote connection.
type ControlClient interface {
	ControlCall(ctx context.Context, method string, params any, result any) error
}

type remoteAgentsLoadedMsg struct {
	Agents          []vault.Agent
	SelectedAgentID string
}

type remoteMessagesLoadedMsg struct {
	AgentID  string
	Messages []vault.AgentMessage
}

// App is the root Bubbletea model for the 3-column agent workbench.
type App struct {
	kernel  *kernel.Kernel
	client  *juggler.Client
	orch    *orchestrator.Orchestrator
	vault   *vault.DB
	cfg     *config.Config
	monitor *monitor.Monitor
	control ControlClient

	width, height int
	leftWidth     int // adjustable left sidebar width
	rightWidth    int // adjustable right sidebar width
	leftSplit     int // height of system info in left (agent list gets remainder)
	rightSplit    int // height of agent detail in right (contexts gets remainder)
	focus         int // 0=agentlist, 1=conversation, 2=agentdetail, 3=contexts

	// Panels
	systemInfo   systeminfo.Model
	agentList    agentlist.Model
	agentDetail  agentdetail.Model
	conversation conversation.Model
	contextList  contextlist.Model
	settings     settings.Model

	// State
	selectedAgentID         string
	inputMode               string // "" | "new-agent-name" | "new-agent-desc" | "chat"
	newAgentName            string // temp storage during agent creation
	newAgentContext         string
	notice                  string
	noticeTTL               int  // number of ticks before notice is cleared
	confirmDelete           bool // true when waiting for delete confirmation
	confirmKillAll          bool // true when waiting for bulk kill confirmation
	resizeMode              bool
	pendingChatFocusAgentID string

	// Text inputs
	nameInput textinput.Model
	taskInput textinput.Model

	eventCh  chan tea.Msg
	stopCh   chan struct{}
	stopOnce *sync.Once
}

// NewApp creates the root TUI model.
func NewApp(k *kernel.Kernel, client *juggler.Client, orch *orchestrator.Orchestrator, v *vault.DB, cfg *config.Config, audit *runtimeaudit.Manager) App {
	return NewAppWithControl(k, client, orch, v, cfg, audit, nil)
}

// NewAppWithControl creates the root TUI model with an optional remote control client.
func NewAppWithControl(k *kernel.Kernel, client *juggler.Client, orch *orchestrator.Orchestrator, v *vault.DB, cfg *config.Config, audit *runtimeaudit.Manager, control ControlClient) App {
	eventCh := make(chan tea.Msg, 64)
	stopCh := make(chan struct{})

	nameIn := textinput.New()
	nameIn.Placeholder = "Agent name..."
	nameIn.CharLimit = 64
	nameIn.Width = 40

	taskIn := textinput.New()
	taskIn.Placeholder = "Brief description of this agent's purpose..."
	taskIn.CharLimit = 500
	taskIn.Width = 60

	mon := monitor.New()

	app := App{
		kernel:       k,
		client:       client,
		orch:         orch,
		vault:        v,
		cfg:          cfg,
		monitor:      mon,
		control:      control,
		leftWidth:    18,
		rightWidth:   18,
		leftSplit:    13, // system info height (includes pool stats now)
		rightSplit:   10, // agent detail height in right column
		nameInput:    nameIn,
		taskInput:    taskIn,
		systemInfo:   systeminfo.New(),
		agentList:    agentlist.New(),
		agentDetail:  agentdetail.New(),
		conversation: conversation.New(),
		contextList:  contextlist.New(),
		settings:     settings.New(),
		eventCh:      eventCh,
		stopCh:       stopCh,
		stopOnce:     &sync.Once{},
	}
	if control != nil {
		app.agentDetail.SetRemote(true)
	}
	emitEvent := app.emitEvent
	if cfg != nil {
		app.resizeMode = cfg.ResizePanelsWithArrows
	}

	if audit != nil {
		if events, err := audit.List(vault.RuntimeEventFilter{Limit: 3}); err == nil {
			seed := make([]shared.RuntimeEventMsg, 0, len(events))
			for _, event := range events {
				seed = append(seed, shared.RuntimeEventMsg{Event: event})
			}
			app.systemInfo.SetRuntimeEvents(seed)
		}
		sub := audit.Subscribe()
		go func() {
			for {
				select {
				case <-stopCh:
					return
				case event, ok := <-sub:
					if !ok {
						return
					}
					emitEvent(shared.RuntimeEventMsg{Event: event})
				}
			}
		}()
	}

	// Load existing agents from vault and reconcile status
	if v != nil {
		agents, err := v.ListAgents()
		if err != nil {
			log.Printf("tui: failed to load agents from vault: %v", err)
		} else {
			// Reconcile status: agents that were "active" or "starting" or "running"
			// are now "paused" since no process is running on startup
			for i := range agents {
				if agents[i].Status == "active" || agents[i].Status == "starting" || agents[i].Status == "running" {
					agents[i].Status = "paused"
					v.UpdateAgentStatus(agents[i].ID, "paused")
				}
			}
			app.agentList.SetAgents(agents)
			// Select first agent if any
			if len(agents) > 0 {
				app.selectedAgentID = agents[0].ID
				app.conversation.SetAgentID(agents[0].ID)
				app.conversation.SetAgentName(agents[0].Name)
				msgs, err := v.GetMessages(agents[0].ID)
				if err == nil {
					app.conversation.LoadMessages(msgs)
				}
				app.updateAgentDetail(&agents[0])
			}
		}
	}

	// Subscribe to Juggler events
	if client != nil {
		client.Subscribe("Browser.attachedToTarget", func(sid string, params json.RawMessage) {
			var e juggler.AttachedToTarget
			json.Unmarshal(params, &e)
			emitEvent(shared.TargetAttachedMsg{
				SessionID: e.SessionID,
				TargetID:  e.TargetInfo.TargetID,
				ContextID: e.TargetInfo.BrowserContextID,
				URL:       e.TargetInfo.URL,
			})
		})
		client.Subscribe("Browser.detachedFromTarget", func(sid string, params json.RawMessage) {
			var e juggler.DetachedFromTarget
			json.Unmarshal(params, &e)
			emitEvent(shared.TargetDetachedMsg{
				SessionID: e.SessionID,
				TargetID:  e.TargetID,
			})
		})
		client.Subscribe("Browser.trustWarmingStateChanged", func(sid string, params json.RawMessage) {
			var e juggler.TrustWarmingState
			json.Unmarshal(params, &e)
			_ = sentinelcapture.RecordTrustActivity(context.Background(), e)
			emitEvent(shared.TrustWarmMsg{State: e.State, CurrentSite: e.CurrentSite})
		})
		client.Subscribe("Browser.telemetryUpdate", func(sid string, params json.RawMessage) {
			var e juggler.TelemetryUpdate
			json.Unmarshal(params, &e)
			emitEvent(shared.TelemetryMsg{
				MemoryMB:           e.MemoryMB,
				EventLoopLagMs:     e.EventLoopLagMs,
				DetectionRiskScore: e.DetectionRiskScore,
				ActiveContexts:     e.ActiveContexts,
				ActivePages:        e.ActivePages,
			})
		})
		client.Subscribe("Browser.injectionAttemptDetected", func(sid string, params json.RawMessage) {
			var e juggler.InjectionAttempt
			json.Unmarshal(params, &e)
			emitEvent(shared.AlertMsg{
				Timestamp: time.Now(),
				Type:      e.AttemptType,
				URL:       e.URL,
				Details:   e.Details,
				Blocked:   e.Blocked,
			})
		})
		client.Subscribe("Page.browserProbeDetected", func(sid string, params json.RawMessage) {
			var e juggler.BrowserProbe
			if err := json.Unmarshal(params, &e); err != nil {
				return
			}
			_ = sentinelcapture.RecordBrowserProbe(context.Background(), sid, e)
		})
		client.Subscribe("Page.navigationCommitted", func(sid string, params json.RawMessage) {
			var e struct {
				FrameID string `json:"frameId"`
				URL     string `json:"url"`
			}
			json.Unmarshal(params, &e)
			emitEvent(shared.NavigationMsg{
				SessionID: sid,
				FrameID:   e.FrameID,
				URL:       e.URL,
			})
		})
		client.Subscribe("Page.eventFired", func(sid string, params json.RawMessage) {
			var e struct {
				FrameID string `json:"frameId"`
				Name    string `json:"name"`
			}
			json.Unmarshal(params, &e)
			emitEvent(shared.PageLoadMsg{
				SessionID: sid,
				FrameID:   e.FrameID,
				Name:      e.Name,
			})
		})
		client.Subscribe("Page.frameAttached", func(sid string, params json.RawMessage) {
			var e struct {
				FrameID       string `json:"frameId"`
				ParentFrameID string `json:"parentFrameId"`
			}
			json.Unmarshal(params, &e)
			emitEvent(shared.FrameAttachedMsg{
				SessionID:     sid,
				FrameID:       e.FrameID,
				ParentFrameID: e.ParentFrameID,
			})
		})
		client.Subscribe("Runtime.executionContextCreated", func(sid string, params json.RawMessage) {
			var e struct {
				ExecutionContextID string `json:"executionContextId"`
				AuxData            struct {
					FrameID string `json:"frameId"`
				} `json:"auxData"`
			}
			json.Unmarshal(params, &e)
			emitEvent(shared.ExecContextCreatedMsg{
				SessionID:          sid,
				ExecutionContextID: e.ExecutionContextID,
				FrameID:            e.AuxData.FrameID,
			})
		})
		client.Subscribe("Vulpine.agentStatus", func(sid string, params json.RawMessage) {
			var e struct {
				AgentID   string `json:"agentId"`
				ContextID string `json:"contextId"`
				Status    string `json:"status"`
				Objective string `json:"objective"`
				Tokens    int    `json:"tokens"`
			}
			if err := json.Unmarshal(params, &e); err != nil {
				return
			}
			emitEvent(shared.AgentStatusMsg{
				AgentID:   e.AgentID,
				ContextID: e.ContextID,
				Status:    e.Status,
				Objective: e.Objective,
				Tokens:    e.Tokens,
			})
		})
		client.Subscribe("Vulpine.conversation", func(sid string, params json.RawMessage) {
			var e struct {
				AgentID string `json:"agentId"`
				Role    string `json:"role"`
				Content string `json:"content"`
				Tokens  int    `json:"tokens"`
			}
			if err := json.Unmarshal(params, &e); err != nil {
				return
			}
			emitEvent(shared.ConversationEntryMsg{
				AgentID:   e.AgentID,
				Role:      e.Role,
				Content:   e.Content,
				Tokens:    e.Tokens,
				Timestamp: time.Now(),
			})
		})
		client.Subscribe("Vulpine.runtimeEvent", func(sid string, params json.RawMessage) {
			var event vault.RuntimeEvent
			if err := json.Unmarshal(params, &event); err != nil {
				return
			}
			emitEvent(shared.RuntimeEventMsg{Event: event})
		})
	}

	// Forward agent status updates from orchestrator to TUI
	if orch != nil {
		statusCh := orch.Agents.StatusChan()
		go func() {
			for {
				select {
				case <-stopCh:
					return
				case status, ok := <-statusCh:
					if !ok {
						return
					}
					emitEvent(shared.AgentStatusMsg{
						AgentID:   status.AgentID,
						ContextID: status.ContextID,
						Status:    status.Status,
						Objective: status.Objective,
						Tokens:    status.Tokens,
					})
				}
			}
		}()

		// Forward conversation messages from orchestrator
		conversationCh := orch.Agents.ConversationChan()
		go func() {
			for {
				select {
				case <-stopCh:
					return
				case msg, ok := <-conversationCh:
					if !ok {
						return
					}
					emitEvent(shared.ConversationEntryMsg{
						AgentID:   msg.AgentID,
						Role:      msg.Role,
						Content:   msg.Content,
						Tokens:    msg.Tokens,
						Timestamp: time.Now(),
					})
				}
			}
		}()
	}

	// Forward rate limit monitor alerts to TUI
	go func() {
		alertCh := mon.AlertChan()
		for {
			select {
			case <-stopCh:
				return
			case alert, ok := <-alertCh:
				if !ok {
					return
				}
				_ = sentinelcapture.RecordMonitorAlert(context.Background(), alert)
				emitEvent(statusNotice{text: fmt.Sprintf("WARNING %s: %s on agent %s", alert.Type, alert.Details, alert.AgentID)})
			}
		}
	}()

	return app
}

func (a App) Init() tea.Cmd {
	cmds := []tea.Cmd{
		a.waitForEvent(),
		a.tick(),
		a.replayBrowserTargets(),
	}
	if a.control != nil {
		cmds = append(cmds, a.loadRemoteAgents())
	}
	return tea.Batch(cmds...)
}

func (a App) replayBrowserTargets() tea.Cmd {
	if a.client == nil {
		return nil
	}
	return func() tea.Msg {
		_, _ = a.client.Call("", "Browser.enable", map[string]interface{}{
			"attachToDefaultContext": true,
		})
		return nil
	}
}

type remoteAgentSummary struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Status             string `json:"status"`
	Task               string `json:"task"`
	TotalTokens        int    `json:"totalTokens"`
	Fingerprint        string `json:"fingerprint"`
	FingerprintSummary string `json:"fingerprintSummary"`
	ContextID          string `json:"contextId"`
}

func (a App) loadRemoteAgents() tea.Cmd {
	if a.control == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		agents, err := a.fetchRemoteAgents(ctx)
		if err != nil {
			return statusNotice{text: "Remote agents failed: " + err.Error()}
		}
		return remoteAgentsLoadedMsg{Agents: agents}
	}
}

func (a App) loadRemoteMessages(agentID string) tea.Cmd {
	if a.control == nil || agentID == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var result struct {
			Messages []vault.AgentMessage `json:"messages"`
		}
		if err := a.control.ControlCall(ctx, "agents.getMessages", map[string]any{"agentId": agentID}, &result); err != nil {
			return statusNotice{text: "Remote messages failed: " + err.Error()}
		}
		return remoteMessagesLoadedMsg{AgentID: agentID, Messages: result.Messages}
	}
}

func (a App) fetchRemoteAgents(ctx context.Context) ([]vault.Agent, error) {
	var result struct {
		Agents []remoteAgentSummary `json:"agents"`
	}
	if err := a.control.ControlCall(ctx, "agents.list", map[string]any{}, &result); err != nil {
		return nil, err
	}
	agents := make([]vault.Agent, 0, len(result.Agents))
	for _, item := range result.Agents {
		agent := remoteSummaryToAgent(item)
		agents = append(agents, agent)
	}
	return agents, nil
}

func remoteSummaryToAgent(item remoteAgentSummary) vault.Agent {
	fingerprint := item.Fingerprint
	if fingerprint == "" {
		fingerprint = item.FingerprintSummary
	}
	agent := vault.Agent{
		ID:          item.ID,
		Name:        item.Name,
		Task:        item.Task,
		Status:      item.Status,
		TotalTokens: item.TotalTokens,
		Fingerprint: fingerprint,
	}
	if item.ContextID != "" {
		agent.Metadata = vault.MarshalAgentMetadata(vault.AgentMetadata{ContextID: item.ContextID})
	}
	return agent
}

func (a App) emitEvent(msg tea.Msg) {
	if a.eventCh == nil {
		return
	}
	if a.stopCh == nil {
		select {
		case a.eventCh <- msg:
		default:
		}
		return
	}
	select {
	case <-a.stopCh:
	case a.eventCh <- msg:
	default:
	}
}

func (a App) stopForwarders() {
	if a.stopOnce == nil || a.stopCh == nil {
		if a.monitor != nil {
			a.monitor.Dispose()
		}
		return
	}
	a.stopOnce.Do(func() {
		close(a.stopCh)
		if a.monitor != nil {
			a.monitor.Dispose()
		}
	})
}

func (a *App) shutdown() tea.Cmd {
	a.gracefulShutdown()
	a.stopForwarders()
	return tea.Quit
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle input modes first
		switch a.inputMode {
		case "new-agent-name":
			return a.updateNameInput(msg)
		case "new-agent-desc":
			return a.updateDescInput(msg)
		case "chat":
			if a.conversation.Focused() {
				if a.allowFocusedChatShortcut(msg) {
					break
				}
				return a.updateChatInput(msg)
			}
			if msg.String() != "v" && msg.String() != "t" && msg.String() != "o" {
				return a.updateChatInput(msg)
			}
		}

		// Route to settings panel when active
		if a.focus == FocusSettings {
			var cmd tea.Cmd
			a.settings, cmd = a.settings.Update(msg)
			// If settings closed itself (via Esc or Tab cycling out)
			if !a.settings.IsActive() {
				a.focus = FocusAgentList
			}
			return a, cmd
		}

		// Cancel delete confirmation on any key except x
		if a.confirmDelete && msg.String() != "x" {
			a.confirmDelete = false
			a.notice = ""
		}
		// Cancel bulk kill confirmation on any key except X
		if a.confirmKillAll && msg.String() != "X" {
			a.confirmKillAll = false
			a.notice = ""
		}

		// Normal keybinds
		switch msg.String() {
		case "q", "ctrl+c":
			// Graceful shutdown: pause all running agents so they save state
			return a, a.shutdown()
		case "p":
			if a.selectedAgentID == "" {
				a.notice = "No agent selected"
				a.noticeTTL = 3
				return a, nil
			}
			cmds = append(cmds, a.pauseSelectedAgent())
		case "r":
			if a.selectedAgentID == "" {
				a.notice = "No agent selected"
				a.noticeTTL = 3
				return a, nil
			}
			cmds = append(cmds, a.resumeSelectedAgent())
		case "P":
			cmds = append(cmds, a.pauseAllAgents())
		case "R":
			cmds = append(cmds, a.resumePausedAgents())
		case "X":
			if a.confirmKillAll {
				a.confirmKillAll = false
				cmds = append(cmds, a.killAllAgents())
			} else {
				a.confirmKillAll = true
				a.notice = "Press X again to kill all live agents, or any other key to cancel"
				a.noticeTTL = 5
			}
		case "tab":
			// Cycle: AgentList → Conversation → AgentDetail → ContextList → AgentList
			a.focus = (a.focus + 1) % FocusNormalCount
		case "m":
			enabled := !a.resizeModeEnabled()
			a.resizeMode = enabled
			if enabled {
				a.notice = "Resize mode enabled — arrow keys resize panels"
			} else {
				a.notice = "Resize mode disabled — arrow keys navigate and scroll"
			}
			a.noticeTTL = 3
		case "t":
			a.handleTraceToggle()
		case "j":
			switch a.focus {
			case FocusAgentList:
				a.agentList.MoveDown()
				cmds = append(cmds, a.selectCurrentAgent())
			case FocusContextList:
				a.contextList.MoveDown()
			}
		case "k":
			switch a.focus {
			case FocusAgentList:
				a.agentList.MoveUp()
				cmds = append(cmds, a.selectCurrentAgent())
			case FocusContextList:
				a.contextList.MoveUp()
			}
		case "n":
			if a.orch != nil || a.control != nil {
				a.newAgentContext = ""
				if a.focus == FocusContextList {
					a.newAgentContext = a.contextList.SelectedContextID()
				}
				a.inputMode = "new-agent-name"
				a.nameInput.Focus()
				return a, textinput.Blink
			}
			a.notice = "No orchestrator available"
			a.noticeTTL = 3
		case "x":
			// Delete selected agent — ask for confirmation
			if a.selectedAgentID != "" {
				if a.confirmDelete {
					// Second press = confirmed
					a.confirmDelete = false
					cmds = append(cmds, a.deleteAgent(a.selectedAgentID))
				} else {
					a.confirmDelete = true
					a.notice = "Press x again to delete agent, or any other key to cancel"
					a.noticeTTL = 5
				}
			}
		case "v":
			a.handleBrowserToggle()
		case "o":
			a.handleOpenSessionLog()
		case "enter":
			switch a.focus {
			case FocusAgentList, FocusAgentDetail, FocusConversation:
				// Focus conversation input — always allow chatting with a selected agent
				if a.selectedAgentID != "" {
					a.focus = FocusConversation
					a.inputMode = "chat"
					a.conversation.SetAwake(true) // ensure input is enabled
					cmd := a.conversation.Focus()
					return a, cmd
				}
			}
		case "esc":
			a.conversation.Blur()
			a.inputMode = ""
			a.focus = FocusAgentList
		case "left":
			if a.resizeModeEnabled() {
				switch a.focus {
				case FocusAgentList:
					if a.leftWidth > 12 {
						a.leftWidth -= 2
						a.updatePanelSizes()
					}
				case FocusContextList:
					// Left arrow on right panel = expand (pull edge left)
					if a.rightWidth < 30 {
						a.rightWidth += 2
						a.updatePanelSizes()
					}
				}
			}
		case "right":
			if a.resizeModeEnabled() {
				switch a.focus {
				case FocusAgentList:
					if a.leftWidth < 30 {
						a.leftWidth += 2
						a.updatePanelSizes()
					}
				case FocusContextList:
					// Right arrow on right panel = shrink (push edge right)
					if a.rightWidth > 12 {
						a.rightWidth -= 2
						a.updatePanelSizes()
					}
				}
			}
		case "up":
			maxH := a.height - 2
			switch a.focus {
			case FocusConversation:
				_ = maxH
				var cmd tea.Cmd
				a.conversation, cmd = a.conversation.Update(msg)
				return a, cmd
			case FocusAgentList:
				if a.resizeModeEnabled() {
					if a.leftSplit > minSplit {
						a.leftSplit--
						a.updatePanelSizes()
					}
				} else {
					a.agentList.MoveUp()
					cmds = append(cmds, a.selectCurrentAgent())
				}
			case FocusAgentDetail:
				if a.resizeModeEnabled() && a.rightSplit > minSplit {
					a.rightSplit--
					a.updatePanelSizes()
				}
			case FocusContextList:
				if a.resizeModeEnabled() {
					if a.rightSplit > minSplit {
						a.rightSplit--
						a.updatePanelSizes()
					}
				} else {
					a.contextList.MoveUp()
				}
			}
		case "down":
			maxH := a.height - 2
			switch a.focus {
			case FocusConversation:
				_ = maxH
				var cmd tea.Cmd
				a.conversation, cmd = a.conversation.Update(msg)
				return a, cmd
			case FocusAgentList:
				if a.resizeModeEnabled() {
					if a.leftSplit < maxH-minSplit {
						a.leftSplit++
						a.updatePanelSizes()
					}
				} else {
					a.agentList.MoveDown()
					cmds = append(cmds, a.selectCurrentAgent())
				}
			case FocusAgentDetail:
				if a.resizeModeEnabled() && a.rightSplit < maxH*maxSplitRatio/100 {
					a.rightSplit++
					a.updatePanelSizes()
				}
			case FocusContextList:
				if a.resizeModeEnabled() {
					if a.rightSplit < maxH*maxSplitRatio/100 {
						a.rightSplit++
						a.updatePanelSizes()
					}
				} else {
					a.contextList.MoveDown()
				}
			}
		case "S":
			a.focus = FocusSettings
			a.settings.SetActive(true)
			a.settings.SetConfig(a.cfg)
			// Load proxies from vault
			if a.vault != nil {
				storedProxies, err := a.vault.ListProxies()
				if err == nil {
					items := make([]settings.ProxyItem, len(storedProxies))
					for i, sp := range storedProxies {
						items[i] = settings.ProxyItem{
							ID:      sp.ID,
							Label:   safeProxyLabel(sp.Label),
							Latency: "untested",
						}
						// Try to parse config for display
						var pc struct {
							Type string `json:"type"`
							Host string `json:"host"`
							Port int    `json:"port"`
						}
						if json.Unmarshal([]byte(sp.Config), &pc) == nil {
							items[i].Type = pc.Type
							items[i].Host = pc.Host
							items[i].Port = pc.Port
						}
						// Try to parse geo for country
						var geo struct {
							Country string `json:"country"`
						}
						if json.Unmarshal([]byte(sp.Geo), &geo) == nil {
							items[i].Country = geo.Country
						}
					}
					a.settings.SetProxies(items)
				}
			}
			return a, nil
		case "c":
			// Request the setup wizard on next launch without mutating the active config first.
			return a, a.requestReconfigure()
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.updatePanelSizes()

	case shared.TickMsg:
		// Decrement notice TTL; only clear when it reaches 0
		if a.noticeTTL > 0 {
			a.noticeTTL--
			if a.noticeTTL == 0 {
				a.notice = ""
			}
		}
		if a.kernel != nil {
			sentinelStatus, sentinelAvailable, _ := extensions.SentinelSnapshot(context.Background())
			ksMsg := shared.KernelStatusMsg{
				Running:           a.kernel.Running(),
				PID:               a.kernel.PID(),
				Uptime:            a.kernel.Uptime(),
				Headless:          a.kernel.IsHeadless(),
				BrowserRoute:      a.browserRouteLabel(),
				BrowserWindow:     a.browserWindowLabel(),
				SentinelAvailable: sentinelAvailable,
				SentinelMode:      sentinelStatus.Mode,
			}
			a.systemInfo, _ = a.systemInfo.Update(ksMsg)
		}
		if a.orch != nil {
			avail, active, total := a.orch.Pool.Stats()
			a.systemInfo.SetPoolStats(avail, active, total)
		}
		cmds = append(cmds, a.tick())

	// Juggler events
	case shared.TargetAttachedMsg:
		a.contextList, _ = a.contextList.Update(msg)
		cmds = append(cmds, a.waitForEvent())
	case shared.TargetDetachedMsg:
		a.contextList, _ = a.contextList.Update(msg)
		cmds = append(cmds, a.waitForEvent())
	case shared.NavigationMsg:
		a.contextList, _ = a.contextList.Update(msg)
		cmds = append(cmds, a.waitForEvent())
	case shared.FrameAttachedMsg:
		a.contextList, _ = a.contextList.Update(msg)
		cmds = append(cmds, a.waitForEvent())
	case shared.ExecContextCreatedMsg:
		cmds = append(cmds, a.waitForEvent())
	case shared.PageLoadMsg:
		a.contextList, _ = a.contextList.Update(msg)
		cmds = append(cmds, a.waitForEvent())
	case shared.TelemetryMsg:
		a.systemInfo, _ = a.systemInfo.Update(msg)
		cmds = append(cmds, a.waitForEvent())
	case shared.RuntimeEventMsg:
		a.systemInfo, _ = a.systemInfo.Update(msg)
		cmds = append(cmds, a.waitForEvent())
	case shared.TrustWarmMsg:
		cmds = append(cmds, a.waitForEvent())
	case shared.AlertMsg:
		cmds = append(cmds, a.waitForEvent())

	case shared.AgentStatusMsg:
		a.agentList, _ = a.agentList.Update(msg)
		// Update vault status
		if a.vault != nil {
			a.vault.UpdateAgentStatus(msg.AgentID, msg.Status)
			if msg.Tokens > 0 {
				a.vault.UpdateAgentTokens(msg.AgentID, msg.Tokens)
			}
		}
		// Update agent detail if this is the selected agent
		if msg.AgentID == a.selectedAgentID {
			if isTerminalAgentStatus(msg.Status) {
				a.conversation.SetThinking(false)
			}
			if a.pendingChatFocusAgentID == msg.AgentID && msg.Status != "starting" && msg.Status != "running" && msg.Status != "active" && msg.Status != "thinking" {
				a.pendingChatFocusAgentID = ""
				a.focus = FocusConversation
				a.inputMode = "chat"
				a.conversation.SetAwake(true)
				cmds = append(cmds, a.conversation.Focus())
			}
			if a.control != nil && a.vault == nil {
				a.updateRemoteAgentDetailFromList(msg.AgentID)
			} else {
				a.refreshAgentDetail(msg.AgentID)
			}
			if a.focus == FocusConversation && a.inputMode == "chat" && !a.conversation.Focused() {
				cmds = append(cmds, a.conversation.Focus())
			}
		}
		cmds = append(cmds, a.waitForEvent())

	case shared.ConversationEntryMsg:
		// Save to vault always
		if a.vault != nil {
			a.vault.AppendMessage(msg.AgentID, msg.Role, msg.Content, msg.Tokens)
		}
		// Check for rate limit / captcha / block patterns
		if a.monitor != nil && (msg.Role == "assistant" || msg.Role == "system") {
			a.monitor.CheckMessage(msg.AgentID, msg.Content)
		}
		// If matches selected agent, add to conversation panel + clear thinking
		if msg.AgentID == a.selectedAgentID {
			a.conversation.SetThinking(false)
			a.conversation.AddEntry(msg.Role, msg.Content)
			a.agentList.ClearUnread(msg.AgentID)
			if msg.Role == "assistant" && a.pendingChatFocusAgentID == msg.AgentID {
				a.pendingChatFocusAgentID = ""
				a.focus = FocusConversation
				a.inputMode = "chat"
				a.conversation.SetAwake(true)
				cmds = append(cmds, a.conversation.Focus())
			} else if a.focus == FocusConversation && a.inputMode == "chat" && !a.conversation.Focused() {
				cmds = append(cmds, a.conversation.Focus())
			}
		} else {
			a.agentList.MarkUnread(msg.AgentID)
		}
		cmds = append(cmds, a.waitForEvent())

	case remoteAgentsLoadedMsg:
		a.agentList.SetAgents(msg.Agents)
		if len(msg.Agents) == 0 {
			a.selectedAgentID = ""
			a.conversation.SetAgentID("")
			a.agentDetail.Clear()
			break
		}
		selectedID := msg.SelectedAgentID
		if selectedID == "" {
			selectedID = a.selectedAgentID
		}
		if selectedID == "" {
			selectedID = msg.Agents[0].ID
		}
		if !a.agentList.SelectAgentID(selectedID) {
			selectedID = msg.Agents[0].ID
			a.agentList.SelectAgentID(selectedID)
		}
		a.selectedAgentID = selectedID
		for i := range msg.Agents {
			if msg.Agents[i].ID == selectedID {
				a.conversation.SetAgentID(msg.Agents[i].ID)
				a.conversation.SetAgentName(msg.Agents[i].Name)
				a.updateAgentDetail(&msg.Agents[i])
				cmds = append(cmds, a.loadRemoteMessages(selectedID))
				break
			}
		}

	case remoteMessagesLoadedMsg:
		if msg.AgentID == a.selectedAgentID {
			a.conversation.LoadMessages(msg.Messages)
		}

	case shared.PoolStatsMsg:
		a.systemInfo, _ = a.systemInfo.Update(msg)
		cmds = append(cmds, a.waitForEvent())

	case shared.AgentCreatedMsg:
		a.agentList, _ = a.agentList.Update(msg)
		// Auto-select the newly created agent
		a.selectedAgentID = msg.Agent.ID
		a.conversation.SetAgentID(msg.Agent.ID)
		a.conversation.SetAgentName(msg.Agent.Name)
		// Select the new agent, load any messages (includes errors or "starting")
		if a.vault != nil {
			msgs, err := a.vault.GetMessages(msg.Agent.ID)
			if err == nil {
				a.conversation.LoadMessages(msgs)
			}
		}
		// Show thinking if agent is active (not error)
		if msg.Agent.Status == "active" {
			a.conversation.SetThinking(true)
			cmds = append(cmds, conversation.ThinkingTick())
			a.notice = "Agent starting — waiting for response..."
			a.pendingChatFocusAgentID = msg.Agent.ID
		} else if msg.Agent.Status == "error" {
			a.conversation.SetThinking(false)
			a.notice = "Agent created with errors — check conversation"
			a.pendingChatFocusAgentID = ""
		}
		agentCopy := msg.Agent
		a.updateAgentDetail(&agentCopy)
		a.focus = FocusConversation
		a.inputMode = "chat"
		a.conversation.SetAwake(msg.Agent.Status != "active")
		a.noticeTTL = 3
		cmds = append(cmds, a.conversation.Focus())
		cmds = append(cmds, a.waitForEvent())

	case shared.AgentDeletedMsg:
		a.agentList.RemoveAgent(msg.AgentID)
		// If deleted agent was selected, clear selection
		if a.selectedAgentID == msg.AgentID {
			a.selectedAgentID = ""
			a.conversation.SetAgentID("")
			a.agentDetail.Clear()
			// Select next agent if any
			newID := a.agentList.SelectedAgentID()
			if newID != "" {
				a.selectedAgentID = newID
				a.conversation.SetAgentID(newID)
				if a.vault != nil {
					msgs, err := a.vault.GetMessages(newID)
					if err == nil {
						a.conversation.LoadMessages(msgs)
					}
				}
				a.refreshAgentDetail(newID)
			}
		}
		a.notice = "Agent deleted"
		a.noticeTTL = 3

	case shared.BulkAgentStatusMsg:
		for _, agentID := range msg.AgentIDs {
			a.agentList.UpdateStatus(agentID, msg.Status)
			if a.vault != nil {
				a.vault.UpdateAgentStatus(agentID, msg.Status)
			}
			if agentID == a.selectedAgentID {
				a.conversation.SetThinking(false)
				a.refreshAgentDetail(agentID)
			}
		}
		a.notice = msg.Notice
		a.noticeTTL = 3

	case shared.SettingsClosedMsg:
		a.settings.SetActive(false)
		if a.focus == FocusSettings {
			a.focus = FocusAgentList
		}

	case shared.ReconfigureRequestedMsg:
		return a, a.requestReconfigure()

	case shared.ProxyAddMsg:
		pc, err := proxy.ParseProxyURL(msg.URL)
		if err != nil {
			a.notice = "Invalid proxy: " + err.Error()
			a.noticeTTL = 3
		} else {
			configJSON, _ := json.Marshal(pc)
			if a.vault != nil {
				a.vault.AddProxy(string(configJSON), "", pc.String())
			}
			a.notice = "Proxy added: " + pc.String()
			a.noticeTTL = 3
		}
		a.reloadSettingsProxies()

	case shared.ProxyDeleteMsg:
		if a.vault != nil {
			a.vault.DeleteProxy(msg.ProxyID)
		}
		a.notice = "Proxy deleted"
		a.noticeTTL = 3
		a.reloadSettingsProxies()

	case shared.SkillToggleMsg:
		if a.cfg != nil {
			if msg.Enabled {
				a.cfg.AddGlobalSkill(msg.Name, nil)
			} else {
				a.cfg.RemoveGlobalSkill(msg.Name)
			}
			a.cfg.Save()
			exe, _ := os.Executable()
			a.cfg.GenerateOpenClawConfig(exe, a.cfg.BinaryPath)
			state := "disabled"
			if msg.Enabled {
				state = "enabled"
			}
			a.notice = "Skill " + msg.Name + " " + state
			a.noticeTTL = 3
		}

	case shared.ResizeModeToggleMsg:
		a.resizeMode = msg.Enabled
		if a.cfg != nil {
			a.cfg.ResizePanelsWithArrows = msg.Enabled
			_ = a.cfg.Save()
			a.settings.SetConfig(a.cfg)
		}
		if msg.Enabled {
			a.notice = "Resize mode enabled — arrow keys resize panels"
		} else {
			a.notice = "Resize mode disabled — arrow keys navigate and scroll"
		}
		a.noticeTTL = 3

	case shared.ProxyTestRequestMsg:
		cmds = append(cmds, a.testProxy(msg.ProxyID, msg.Config))

	case shared.ProxyTestedMsg:
		a.settings, _ = a.settings.Update(msg)

	case tea.MouseMsg:
		// Forward mouse events to conversation for scroll
		if a.focus == FocusConversation || a.inputMode == "chat" {
			var cmd tea.Cmd
			a.conversation, cmd = a.conversation.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case conversation.ThinkingTickMsg:
		var cmd tea.Cmd
		a.conversation, cmd = a.conversation.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case statusNotice:
		a.notice = msg.text
		a.noticeTTL = 3
	}

	return a, tea.Batch(cmds...)
}

// updateNameInput handles keystrokes in "new-agent-name" mode.
func (a App) updateNameInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(a.nameInput.Value())
		if name != "" {
			a.newAgentName = name
			a.inputMode = "new-agent-desc"
			a.nameInput.Blur()
			a.nameInput.Reset()
			a.taskInput.Focus()
			return a, textinput.Blink
		}
		return a, nil
	case "esc":
		a.inputMode = ""
		a.newAgentName = ""
		a.newAgentContext = ""
		a.nameInput.Blur()
		a.nameInput.Reset()
		return a, nil
	default:
		var cmd tea.Cmd
		a.nameInput, cmd = a.nameInput.Update(msg)
		return a, cmd
	}
}

func isTerminalAgentStatus(status string) bool {
	switch status {
	case "completed", "error", "failed", "interrupted":
		return true
	default:
		return false
	}
}

// updateDescInput handles keystrokes in "new-agent-desc" mode.
func (a App) updateDescInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		desc := strings.TrimSpace(a.taskInput.Value())
		if desc == "" {
			desc = a.newAgentName // use name as description if empty
		}
		cmd := a.createAgent(a.newAgentName, desc, a.newAgentContext)
		a.inputMode = ""
		a.newAgentName = ""
		a.newAgentContext = ""
		a.taskInput.Blur()
		a.taskInput.Reset()
		return a, cmd
	case "esc":
		a.inputMode = ""
		a.newAgentName = ""
		a.newAgentContext = ""
		a.taskInput.Blur()
		a.taskInput.Reset()
		return a, nil
	default:
		var cmd tea.Cmd
		a.taskInput, cmd = a.taskInput.Update(msg)
		return a, cmd
	}
}

// updateChatInput handles keystrokes in "chat" mode.
func (a App) updateChatInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+v":
		a.handleBrowserToggle()
		return a, nil
	case "ctrl+o":
		a.handleOpenSessionLog()
		return a, nil
	case "ctrl+t":
		a.handleTraceToggle()
		return a, nil
	case "enter":
		text := a.conversation.InputValue()
		if text != "" && a.selectedAgentID != "" {
			// Add to conversation view + show thinking with animation
			a.conversation.AddEntry("user", text)
			a.conversation.SetThinking(true)
			// Save to vault
			if a.vault != nil {
				a.vault.AppendMessage(a.selectedAgentID, "user", text, 0)
			}
			// Run one agent turn
			cmd := a.sendMessageToAgent(a.selectedAgentID, text)
			return a, tea.Batch(cmd, conversation.ThinkingTick())
		}
		return a, nil
	case "esc":
		a.conversation.Blur()
		a.inputMode = ""
		a.focus = FocusAgentList
		return a, nil
	case "pgup", "pgdown", "up", "down":
		// Forward scroll keys to conversation
		var cmd tea.Cmd
		a.conversation, cmd = a.conversation.Update(msg)
		return a, cmd
	default:
		ti := a.conversation.TextInput()
		if !a.conversation.Focused() {
			ti.Focus()
		}
		var cmd tea.Cmd
		*ti, cmd = ti.Update(msg)
		return a, cmd
	}
}

func (a App) allowFocusedChatShortcut(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "v", "t", "o":
		return strings.TrimSpace(a.conversation.InputValue()) == ""
	default:
		return false
	}
}

func (a *App) handleBrowserToggle() {
	if a.kernel != nil && a.kernel.Window() != nil {
		visible, err := a.kernel.Window().Toggle()
		if err != nil {
			a.notice = "Browser toggle failed: " + err.Error()
		} else if visible {
			a.notice = "Browser window shown — press v to hide"
		} else {
			a.notice = "Browser window hidden"
		}
		a.noticeTTL = 3
		return
	}
	if a.kernel != nil && a.kernel.IsHeadless() {
		a.notice = "Cannot show browser in headless mode — run with --headful"
		a.noticeTTL = 4
		return
	}
	url := a.contextList.SelectedURL()
	if url != "" && url != "about:blank" {
		_ = startExternalCommand("open", url)
		a.notice = "Opened " + contextlist.SafeDisplayURL(url)
		a.noticeTTL = 3
	}
}

func (a *App) handleOpenSessionLog() {
	if a.selectedAgentID == "" {
		a.notice = "No agent selected"
		a.noticeTTL = 3
		return
	}
	if a.control != nil {
		a.notice = "Remote session logs are available in the web panel"
		a.noticeTTL = 4
		return
	}
	logPath, err := agentSessionLogPath(a.selectedAgentID)
	if err != nil {
		a.notice = "Invalid agent id"
		a.noticeTTL = 4
		return
	}
	if _, err := os.Stat(logPath); err != nil {
		a.notice = "No session log yet for selected agent"
		a.noticeTTL = 4
		return
	}
	if err := startExternalCommand("open", logPath); err != nil {
		a.notice = "Failed to open session log: " + err.Error()
		a.noticeTTL = 4
		return
	}
	a.notice = "Opened session log"
	a.noticeTTL = 3
}

func (a *App) handleTraceToggle() {
	enabled := !a.conversation.TraceOnly()
	a.conversation.SetTraceOnly(enabled)
	if enabled {
		a.notice = "Trace mode enabled — showing tool actions and results"
	} else {
		a.notice = "Trace mode disabled — showing full conversation"
	}
	a.noticeTTL = 3
}

func (a *App) browserWindowLabel() string {
	if a.kernel == nil {
		return ""
	}
	if a.kernel.IsHeadless() {
		return "HEADLESS"
	}
	w := a.kernel.Window()
	if w == nil {
		return "N/A"
	}
	visible, found := w.Status()
	if !found {
		return "N/A"
	}
	if visible {
		return "VISIBLE"
	}
	return "HIDDEN"
}

// detailHeight is the fixed height for the agent detail area.
// Min/max constraints for panel sizes
const (
	minSplit      = 5
	maxSplitRatio = 80 // percent of column height

	minCenterWidth        = 20
	panelHorizontalChrome = 2 // Lipgloss Width includes horizontal padding; the border adds 2 columns.
	workbenchPanelCount   = 3
)

type workbenchWidths struct {
	left   int
	center int
	right  int
}

func (a App) View() string {
	if a.width == 0 {
		return "Loading..."
	}

	widths := resolveWorkbenchWidths(a.width, a.leftWidth, a.rightWidth)
	leftWidth := widths.left
	centerWidth := widths.center
	rightWidth := widths.right

	bodyHeight := a.height - 2 // status bar

	// Left column: systemInfo (with pool stats) on top, agentList below
	leftTop := a.leftSplit
	leftBottom := bodyHeight - leftTop - 4 // subtract borders
	if leftBottom < 3 {
		leftBottom = 3
		leftTop = bodyHeight - leftBottom - 4
	}
	if leftTop < 3 {
		leftTop = 3
	}
	sysView := a.renderPanel(FocusAgentList, a.systemInfo.View(), leftWidth, leftTop)
	agentView := a.renderFocusPanel(FocusAgentList, a.agentList.View(), leftWidth, leftBottom)
	leftColumn := lipgloss.JoinVertical(lipgloss.Left, sysView, agentView)

	// Center column: settings panel OR full-height conversation
	var centerContent string
	if a.focus == FocusSettings && a.settings.IsActive() {
		settingsView := a.settings.View()
		maxContentLines := bodyHeight - 2
		settingsLines := strings.Split(settingsView, "\n")
		if maxContentLines > 0 && len(settingsLines) > maxContentLines {
			settingsLines = settingsLines[:maxContentLines]
			settingsView = strings.Join(settingsLines, "\n")
		}
		centerContent = shared.ActivePanelStyle.Width(centerWidth).Height(bodyHeight - 2).Render(settingsView)
	} else {
		// Check if we need to show agent creation inputs overlaid on conversation
		var convView string
		switch a.inputMode {
		case "new-agent-name":
			convView = shared.TitleStyle.Render("NEW AGENT — NAME") + "\n\n" +
				a.nameInput.View() + "\n\n" +
				a.newAgentContextNotice() +
				shared.MutedStyle.Render("[Enter] confirm  [Esc] cancel")
		case "new-agent-desc":
			convView = shared.TitleStyle.Render("NEW AGENT — DESCRIPTION for "+a.newAgentName) + "\n\n" +
				a.taskInput.View() + "\n\n" +
				a.newAgentContextNotice() +
				shared.MutedStyle.Render("[Enter] create  [Esc] cancel")
		default:
			convView = a.conversation.View()
		}

		// Full-height conversation panel
		convStyle := shared.PanelStyle
		if a.focus == FocusConversation {
			convStyle = shared.ActivePanelStyle
		}

		// Hard-truncate conversation content to prevent overflow
		maxContentLines := bodyHeight - 2 // subtract panel borders
		convLines := strings.Split(convView, "\n")
		if len(convLines) > maxContentLines {
			convLines = convLines[:maxContentLines]
			convView = strings.Join(convLines, "\n")
		}

		centerContent = convStyle.Width(centerWidth).Height(bodyHeight - 2).Render(convView)
	}
	centerView := centerContent

	// Right column: agent detail on top, contexts below
	rightTop := a.rightSplit
	rightBottom := bodyHeight - rightTop - 4 // subtract borders
	if rightBottom < 3 {
		rightBottom = 3
		rightTop = bodyHeight - rightBottom - 4
	}
	if rightTop < 3 {
		rightTop = 3
	}
	detailView := a.renderFocusPanel(FocusAgentDetail, a.agentDetail.View(), rightWidth, rightTop)
	ctxView := a.renderFocusPanel(FocusContextList, a.contextList.View(), rightWidth, rightBottom)
	rightColumn := lipgloss.JoinVertical(lipgloss.Left, detailView, ctxView)

	// Hard-truncate each column to bodyHeight lines
	leftLines := strings.Split(leftColumn, "\n")
	if len(leftLines) > bodyHeight {
		leftColumn = strings.Join(leftLines[:bodyHeight], "\n")
	}
	rightLines := strings.Split(rightColumn, "\n")
	if len(rightLines) > bodyHeight {
		rightColumn = strings.Join(rightLines[:bodyHeight], "\n")
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, centerView, rightColumn)

	// Status bar (notice replaces status bar content when present)
	var statusBar string
	if a.notice != "" {
		statusBar = shared.WarmingStyle.Render("  " + a.notice)
	} else {
		statusBar = a.renderStatusBar()
	}

	output := lipgloss.JoinVertical(lipgloss.Left, body, statusBar)

	// Final safety: hard-truncate to terminal height.
	// Keep the last line (status bar) and trim overflow from the top of the body
	// so the status bar is never cut off.
	outputLines := strings.Split(output, "\n")
	if len(outputLines) > a.height {
		excess := len(outputLines) - a.height
		outputLines = append(outputLines[excess:len(outputLines)-1], outputLines[len(outputLines)-1])
		output = strings.Join(outputLines, "\n")
	}
	return output
}

// renderPanel renders content in a panel box without focus highlight.
func (a App) renderPanel(_ int, content string, width, height int) string {
	return shared.PanelStyle.
		Width(width).
		Height(height).
		Render(content)
}

// renderFocusPanel renders content in a panel box, highlighted if focused.
func (a App) renderFocusPanel(panel int, content string, width, height int) string {
	style := shared.PanelStyle
	if panel == a.focus {
		style = shared.ActivePanelStyle
	}
	return style.
		Width(width).
		Height(height).
		Render(content)
}

// renderStatusBar renders the bottom status bar.
func (a App) renderStatusBar() string {
	mode := "local"
	if a.client != nil && a.kernel == nil {
		mode = "remote"
	}

	ctxHint := ""
	if contextID := a.contextList.SelectedContextID(); contextID != "" && a.focus == FocusContextList {
		ctxHint = shared.MutedStyle.Render("  n:new-in-ctx " + shortContextID(contextID))
	}
	arrowMode := shared.MutedStyle.Render("mode:navigate")
	if a.resizeModeEnabled() {
		arrowMode = shared.WarmingStyle.Render("mode:resize")
	}

	controls := "  n:new  p/r:agent  P/R:all  X:kill-all  x:del  v:view  o:log  m:mode  S:settings  Enter:chat  Tab:focus  "
	if a.control != nil {
		controls = "  n:new  p/r:agent  P/R:all  X:kill-all  x:kill  v:view  m:mode  S:settings  Enter:chat  Tab:focus  "
	}
	bar := shared.TitleStyle.Render("VULPINE") +
		shared.MutedStyle.Render(" | ") +
		shared.RunningStyle.Render("* "+mode) +
		shared.MutedStyle.Render(controls) +
		shared.MutedStyle.Render("t:trace  ") +
		arrowMode +
		shared.MutedStyle.Render("  q:quit") +
		ctxHint

	return lipgloss.NewStyle().MaxWidth(a.width).Render(bar)
}

// updatePanelSizes recalculates panel dimensions after a resize.
func (a *App) updatePanelSizes() {
	widths := resolveWorkbenchWidths(a.width, a.leftWidth, a.rightWidth)
	leftWidth := widths.left
	centerWidth := widths.center
	rightWidth := widths.right
	bodyHeight := a.height - 2

	// Center is full-height conversation (minus panel border)
	convHeight := bodyHeight - 2
	if convHeight < minSplit {
		convHeight = minSplit
	}

	// Left column splits
	leftTop := a.leftSplit
	leftBottom := bodyHeight - leftTop - 4
	if leftBottom < 3 {
		leftBottom = 3
		leftTop = bodyHeight - leftBottom - 4
	}
	if leftTop < 3 {
		leftTop = 3
	}

	// Right column splits
	rightTop := a.rightSplit
	rightBottom := bodyHeight - rightTop - 4
	if rightBottom < 3 {
		rightBottom = 3
		rightTop = bodyHeight - rightBottom - 4
	}
	if rightTop < 3 {
		rightTop = 3
	}

	a.systemInfo.SetWidth(leftWidth)
	a.systemInfo.SetHeight(leftTop)
	a.agentList.SetWidth(leftWidth)
	a.agentList.SetHeight(leftBottom)
	a.agentDetail.SetSize(rightWidth, rightTop)
	a.conversation.SetSize(centerWidth, convHeight)
	a.settings.SetSize(centerWidth, bodyHeight)
	a.contextList.SetWidth(rightWidth)
	a.contextList.SetHeight(rightBottom)

	// Update text input widths to fit center panel
	inputWidth := centerWidth - 6
	if inputWidth < 10 {
		inputWidth = 10
	}
	a.nameInput.Width = inputWidth
	a.taskInput.Width = inputWidth
}

func resolveWorkbenchWidths(totalWidth, preferredLeft, preferredRight int) workbenchWidths {
	available := totalWidth - workbenchPanelCount*panelHorizontalChrome
	if available <= 0 {
		return workbenchWidths{}
	}

	left := max(0, preferredLeft)
	right := max(0, preferredRight)
	if left+right+minCenterWidth <= available {
		return workbenchWidths{
			left:   left,
			center: available - left - right,
			right:  right,
		}
	}

	sideBudget := available - minCenterWidth
	if sideBudget < 0 {
		sideBudget = 0
	}
	left, right = shrinkSideWidths(left, right, sideBudget)
	return workbenchWidths{
		left:   left,
		center: max(0, available-left-right),
		right:  right,
	}
}

func shrinkSideWidths(left, right, budget int) (int, int) {
	if budget <= 0 {
		return 0, 0
	}
	if left+right <= budget {
		return left, right
	}
	total := left + right
	if total <= 0 {
		return budget / 2, budget - budget/2
	}
	left = budget * left / total
	if left < 0 {
		left = 0
	}
	if left > budget {
		left = budget
	}
	return left, budget - left
}

func (a *App) resizeModeEnabled() bool {
	return a.resizeMode
}

func (a *App) requestReconfigure() tea.Cmd {
	if err := config.RequestReconfigure(); err != nil {
		a.notice = "Failed to queue reconfigure: " + err.Error()
		a.noticeTTL = 4
		return nil
	}
	return a.shutdown()
}

func (a *App) browserRouteLabel() string {
	if a.cfg != nil && strings.TrimSpace(a.cfg.FoxbridgeCDPURL) != "" {
		return "CAMOUFOX"
	}
	if a.kernel != nil && a.kernel.IsHeadless() {
		return "HEADLESS"
	}
	if a.kernel != nil {
		return "DIRECT"
	}
	return ""
}

// updateAgentDetail populates the agent detail panel from an Agent struct.
func (a *App) updateAgentDetail(agent *vault.Agent) {
	if agent == nil {
		a.agentDetail.Clear()
		return
	}
	fpSummary := vault.FingerprintSummary(agent.Fingerprint)
	proxyInfo := ""
	if agent.ProxyConfig != "" {
		proxyInfo = agent.ProxyConfig // simplified display
	}
	a.agentDetail.SetAgent(
		agent.ID, agent.Name, agent.Task, agent.Status,
		agent.TotalTokens, fpSummary, proxyInfo, agent.CreatedAt,
	)
	meta, err := vault.ParseAgentMetadata(agent.Metadata)
	if err == nil && meta.ContextID != "" {
		a.agentDetail.SetBrowserContext("pinned " + shortContextID(meta.ContextID))
	} else {
		a.agentDetail.SetBrowserContext("")
	}
}

// refreshAgentDetail reloads agent detail from vault.
func (a *App) refreshAgentDetail(agentID string) {
	if a.vault == nil {
		return
	}
	agent, err := a.vault.GetAgent(agentID)
	if err != nil {
		return
	}
	a.updateAgentDetail(agent)
}

func (a *App) updateRemoteAgentDetailFromList(agentID string) {
	item, ok := a.agentList.Agent(agentID)
	if !ok {
		return
	}
	agent := remoteSummaryToAgent(remoteAgentSummary{
		ID:          item.ID,
		Name:        item.Name,
		Status:      item.Status,
		TotalTokens: item.Tokens,
	})
	a.updateAgentDetail(&agent)
}

// selectCurrentAgent loads the currently highlighted agent's data.
func (a *App) selectCurrentAgent() tea.Cmd {
	newID := a.agentList.SelectedAgentID()
	if newID == a.selectedAgentID || newID == "" {
		return nil
	}
	a.selectedAgentID = newID
	a.conversation.SetAgentID(newID)
	a.agentList.ClearUnread(newID)

	if a.control != nil && a.vault == nil {
		if item, ok := a.agentList.SelectedAgent(); ok {
			a.conversation.SetAgentName(item.Name)
			agent := remoteSummaryToAgent(remoteAgentSummary{
				ID:          item.ID,
				Name:        item.Name,
				Status:      item.Status,
				TotalTokens: item.Tokens,
			})
			a.updateAgentDetail(&agent)
		}
		return a.loadRemoteMessages(newID)
	}

	if a.vault != nil {
		agent, err := a.vault.GetAgent(newID)
		if err == nil {
			a.conversation.SetAgentName(agent.Name)
		}
		msgs, err := a.vault.GetMessages(newID)
		if err == nil {
			a.conversation.LoadMessages(msgs)
		}
		a.refreshAgentDetail(newID)
	}
	return nil
}

func (a App) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		if a.stopCh == nil {
			return <-a.eventCh
		}
		select {
		case msg := <-a.eventCh:
			return msg
		case <-a.stopCh:
			return nil
		}
	}
}

func (a App) tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return shared.TickMsg{}
	})
}

// createAgent creates an agent profile in the vault AND immediately spawns OpenClaw.
// The agent wakes up and introduces itself — the user doesn't need to send the first message.
// ALL errors are visible to the user — either as notices (pre-creation) or in the conversation (post-creation).
func (a *App) createAgent(name, description, contextID string) tea.Cmd {
	if a.control != nil {
		return a.createRemoteAgent(name, description, contextID)
	}
	return func() tea.Msg {
		// Pre-creation checks — show errors as notices since there's no agent yet
		if a.vault == nil {
			return statusNotice{text: "ERROR: No vault available — cannot create agent"}
		}
		if a.orch == nil {
			return statusNotice{text: "ERROR: No orchestrator — is the browser running?"}
		}

		// Generate fingerprint
		fp, err := vault.GenerateFingerprint(name)
		if err != nil {
			fp = "{}" // use empty fingerprint as fallback, don't block creation
		}

		// Create in vault — this MUST succeed for anything else to work
		agent, err := a.vault.CreateAgent(name, description, fp)
		if err != nil {
			return statusNotice{text: "ERROR: Failed to create agent: " + err.Error()}
		}
		if contextID != "" {
			metadata := vault.MarshalAgentMetadata(vault.AgentMetadata{ContextID: contextID})
			if err := a.vault.UpdateAgentMetadata(agent.ID, metadata); err == nil {
				agent.Metadata = metadata
			}
		}

		// If agent has a proxy assigned, sync fingerprint geo
		if agent.ProxyConfig != "" {
			var pc proxy.ProxyConfig
			if json.Unmarshal([]byte(agent.ProxyConfig), &pc) == nil {
				geo, geoErr := proxy.ResolveGeo(pc)
				if geoErr == nil {
					synced, syncErr := proxy.SyncFingerprintToProxy(agent.Fingerprint, geo)
					if syncErr == nil {
						agent.Fingerprint = synced
						a.vault.UpdateAgentFingerprint(agent.ID, synced)
					}
				}
			}
		}

		// Spawn first turn — agent introduces itself
		introMsg := openclaw.IntroMessage(name, description)
		sessionName := "vulpine-" + agent.ID
		configPath, cleanup, configErr := a.agentRuntimeConfig(agent)
		if configErr != nil {
			agent.Status = "error"
			a.vault.UpdateAgentStatus(agent.ID, "error")
			a.vault.AppendMessage(agent.ID, "system", "Failed to prepare runtime: "+configErr.Error(), 0)
			return shared.AgentCreatedMsg{Agent: *agent}
		}
		_, spawnErr := a.orch.Agents.SpawnWithSessionIsolated(agent.ID, introMsg, sessionName, configPath, cleanup)
		if spawnErr != nil {
			// Agent is in vault but spawn failed — show it with error status
			// The user will see the agent in the list with error state + error in conversation
			agent.Status = "error"
			a.vault.UpdateAgentStatus(agent.ID, "error")
			a.vault.AppendMessage(agent.ID, "system", "Failed to start: "+spawnErr.Error(), 0)
		} else {
			agent.Status = "active"
			a.vault.UpdateAgentStatus(agent.ID, "active")
			a.vault.AppendMessage(agent.ID, "system", "Agent starting...", 0)
		}

		// ALWAYS return AgentCreatedMsg so the agent shows up in the list
		return shared.AgentCreatedMsg{Agent: *agent}
	}
}

func (a *App) createRemoteAgent(name, description, contextID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var result struct {
			AgentID string `json:"agentId"`
		}
		params := map[string]any{
			"name": name,
			"task": description,
		}
		if contextID != "" {
			params["contextId"] = contextID
		}
		if err := a.control.ControlCall(ctx, "agents.spawn", params, &result); err != nil {
			return statusNotice{text: "Remote agent failed: " + err.Error()}
		}
		agents, err := a.fetchRemoteAgents(ctx)
		if err != nil {
			return statusNotice{text: "Remote agents failed: " + err.Error()}
		}
		return remoteAgentsLoadedMsg{Agents: agents, SelectedAgentID: result.AgentID}
	}
}

// pauseAgent pauses an agent.
func (a App) pauseAgent(agentID string) tea.Cmd {
	if a.control != nil {
		return a.remoteAgentStatusCommand("agents.pause", agentID, "paused", "Remote agent paused: ")
	}
	return func() tea.Msg {
		if a.orch == nil {
			return statusNotice{text: "No orchestrator"}
		}
		if err := a.orch.Agents.PauseAgent(agentID); err != nil {
			return statusNotice{text: "Pause failed: " + err.Error()}
		}
		if a.vault != nil {
			a.vault.UpdateAgentStatus(agentID, "paused")
		}
		return shared.BulkAgentStatusMsg{
			AgentIDs: []string{agentID},
			Status:   "paused",
			Notice:   "Agent paused: " + agentID,
		}
	}
}

// resumeAgent resumes an agent from saved session.
func (a App) resumeAgent(agentID string) tea.Cmd {
	if a.control != nil {
		return a.remoteAgentStatusCommand("agents.resume", agentID, "active", "Remote agent resumed: ")
	}
	return func() tea.Msg {
		if a.orch == nil {
			return statusNotice{text: "No orchestrator"}
		}
		if a.vault == nil {
			return statusNotice{text: "No vault available"}
		}
		sessionName := "vulpine-" + agentID
		agent, err := a.vault.GetAgent(agentID)
		if err != nil {
			return statusNotice{text: "Resume failed: " + err.Error()}
		}
		configPath, cleanup, err := a.agentRuntimeConfig(agent)
		if err != nil {
			return statusNotice{text: "Resume failed: " + err.Error()}
		}
		_, err = a.orch.Agents.ResumeWithSessionIsolated(agentID, sessionName, configPath, cleanup)
		if err != nil {
			return statusNotice{text: "Resume failed: " + err.Error()}
		}
		if a.vault != nil {
			a.vault.UpdateAgentStatus(agentID, "active")
		}
		return shared.BulkAgentStatusMsg{
			AgentIDs: []string{agentID},
			Status:   "active",
			Notice:   "Agent resumed: " + agentID,
		}
	}
}

func (a App) remoteAgentStatusCommand(method, agentID, status, noticePrefix string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var result map[string]any
		if err := a.control.ControlCall(ctx, method, map[string]any{"agentId": agentID}, &result); err != nil {
			return statusNotice{text: "Remote command failed: " + err.Error()}
		}
		return shared.BulkAgentStatusMsg{
			AgentIDs: []string{agentID},
			Status:   status,
			Notice:   noticePrefix + agentID,
		}
	}
}

// deleteAgent removes an agent.
func (a *App) deleteAgent(agentID string) tea.Cmd {
	if a.control != nil {
		return a.remoteAgentStatusCommand("agents.kill", agentID, "interrupted", "Remote agent killed: ")
	}
	return func() tea.Msg {
		// Kill if running
		if a.orch != nil {
			a.orch.Agents.Kill(agentID)
		}
		// Remove from vault
		if a.vault != nil {
			a.vault.DeleteAgent(agentID)
		}
		return shared.AgentDeletedMsg{AgentID: agentID}
	}
}

// sendMessageToAgent spawns an OpenClaw process for one turn of conversation.
// Stateless per-turn like Claude Code: spawn → load session → respond → exit.
// OpenClaw's --session-id handles history and compaction automatically.
// Zero memory between messages. No idle processes.
func (a App) sendMessageToAgent(agentID, text string) tea.Cmd {
	if a.control != nil {
		return a.sendRemoteMessageToAgent(agentID, text)
	}
	return func() tea.Msg {
		if a.orch == nil {
			return shared.ConversationEntryMsg{
				AgentID: agentID,
				Role:    "system",
				Content: "Error: No orchestrator available. Is the browser running?",
			}
		}
		if a.vault == nil {
			return shared.ConversationEntryMsg{
				AgentID: agentID,
				Role:    "system",
				Content: "Error: No vault available.",
			}
		}

		sessionName := "vulpine-" + agentID
		agent, err := a.vault.GetAgent(agentID)
		if err != nil {
			return shared.ConversationEntryMsg{
				AgentID: agentID,
				Role:    "system",
				Content: "Error: " + err.Error(),
			}
		}
		configPath, cleanup, err := a.agentRuntimeConfig(agent)
		if err != nil {
			return shared.ConversationEntryMsg{
				AgentID: agentID,
				Role:    "system",
				Content: "Error: " + err.Error(),
			}
		}
		_, err = a.orch.Agents.SpawnWithSessionIsolated(agentID, text, sessionName, configPath, cleanup)
		if err != nil {
			return shared.ConversationEntryMsg{
				AgentID: agentID,
				Role:    "system",
				Content: "Error: " + err.Error(),
			}
		}

		if a.vault != nil {
			a.vault.UpdateAgentStatus(agentID, "active")
		}

		return nil
	}
}

func (a App) sendRemoteMessageToAgent(agentID, text string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var result struct {
			AgentID string `json:"agentId"`
		}
		if err := a.control.ControlCall(ctx, "agents.resume", map[string]any{
			"agentId": agentID,
			"message": text,
		}, &result); err != nil {
			return shared.ConversationEntryMsg{
				AgentID: agentID,
				Role:    "system",
				Content: "Error: " + err.Error(),
			}
		}
		return shared.BulkAgentStatusMsg{
			AgentIDs: []string{agentID},
			Status:   "active",
			Notice:   "Remote agent running: " + agentID,
		}
	}
}

func (a App) newAgentContextNotice() string {
	if a.newAgentContext == "" {
		return ""
	}
	return shared.MutedStyle.Render("Pinned browser context: "+shortContextID(a.newAgentContext)) + "\n\n"
}

func (a *App) agentRuntimeConfig(agent *vault.Agent) (string, func(), error) {
	if agent == nil {
		return "", nil, fmt.Errorf("agent not found")
	}
	if a.cfg != nil {
		if err := config.RepairOpenClawProfile(a.cfg.FoxbridgeCDPURL); err != nil {
			return "", nil, fmt.Errorf("repair openclaw profile: %w", err)
		}
	}
	meta, err := vault.ParseAgentMetadata(agent.Metadata)
	if err != nil {
		return "", nil, fmt.Errorf("parse agent metadata: %w", err)
	}
	if meta.ContextID == "" {
		return openclaw.PrepareRuntimeConfig(config.OpenClawConfigPath())
	}
	if a.orch == nil {
		return "", nil, fmt.Errorf("orchestrator not available")
	}
	return a.orch.PrepareScopedOpenClawConfig(meta.ContextID)
}

func shortContextID(contextID string) string {
	if len(contextID) <= 12 {
		return contextID
	}
	return contextID[:12]
}

func agentSessionLogPath(agentID string) (string, error) {
	id := strings.TrimSpace(agentID)
	if id == "" {
		return "", fmt.Errorf("agent id is required")
	}
	if strings.ContainsAny(id, `/\`) || id == "." || id == ".." {
		return "", fmt.Errorf("invalid agent id")
	}
	sessionsDir := filepath.Join(config.OpenClawProfileDir(), "agents", "main", "sessions")
	path := filepath.Join(sessionsDir, "vulpine-"+id+".jsonl")
	rel, err := filepath.Rel(sessionsDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid agent id")
	}
	return path, nil
}

func (a App) selectedAgentStatus() string {
	if a.selectedAgentID == "" {
		return ""
	}
	if a.control != nil && a.vault == nil {
		return a.agentList.Status(a.selectedAgentID)
	}
	if a.vault == nil {
		return ""
	}
	agent, err := a.vault.GetAgent(a.selectedAgentID)
	if err != nil {
		return ""
	}
	return agent.Status
}

func (a App) pauseSelectedAgent() tea.Cmd {
	status := a.selectedAgentStatus()
	switch status {
	case "":
		return statusNoticeCmd("Agent state unavailable")
	case "paused":
		return statusNoticeCmd("Agent already paused")
	case "completed", "error", "failed", "interrupted":
		return statusNoticeCmd("Agent is not running")
	}
	return a.pauseAgent(a.selectedAgentID)
}

func (a App) resumeSelectedAgent() tea.Cmd {
	status := a.selectedAgentStatus()
	switch status {
	case "":
		return statusNoticeCmd("Agent state unavailable")
	case "active", "running", "thinking", "starting":
		return statusNoticeCmd("Agent already active")
	case "completed":
		return statusNoticeCmd("Completed agents cannot be resumed")
	}
	return a.resumeAgent(a.selectedAgentID)
}

func (a App) pauseAllAgents() tea.Cmd {
	if a.control != nil {
		ids := a.agentList.IDsByStatus(map[string]bool{
			"running": true, "thinking": true, "starting": true, "active": true,
		})
		return a.remoteBulkAgentStatusCommand("agents.pauseMany", ids, "paused", "Paused %d remote agents")
	}
	return func() tea.Msg {
		if a.orch == nil || a.vault == nil {
			return statusNotice{text: "Pause all unavailable"}
		}
		statuses := a.orch.Agents.List()
		paused := 0
		affected := make([]string, 0, len(statuses))
		for _, status := range statuses {
			switch status.Status {
			case "running", "thinking", "starting", "active":
				if err := a.orch.Agents.PauseAgent(status.AgentID); err == nil {
					_ = a.vault.UpdateAgentStatus(status.AgentID, "paused")
					paused++
					affected = append(affected, status.AgentID)
				}
			}
		}
		if paused == 0 {
			return statusNotice{text: "No active agents to pause"}
		}
		return shared.BulkAgentStatusMsg{
			AgentIDs: affected,
			Status:   "paused",
			Notice:   fmt.Sprintf("Paused %d agents", paused),
		}
	}
}

func (a App) resumePausedAgents() tea.Cmd {
	if a.control != nil {
		ids := a.agentList.IDsByStatus(map[string]bool{"paused": true})
		return a.remoteBulkAgentStatusCommand("agents.resumeMany", ids, "active", "Resumed %d remote agents")
	}
	return func() tea.Msg {
		if a.orch == nil || a.vault == nil {
			return statusNotice{text: "Resume all unavailable"}
		}
		agents, err := a.vault.ListAgentsByStatus("paused")
		if err != nil {
			return statusNotice{text: "Resume all failed: " + err.Error()}
		}
		resumed := 0
		affected := make([]string, 0, len(agents))
		for i := range agents {
			configPath, cleanup, cfgErr := a.agentRuntimeConfig(&agents[i])
			if cfgErr != nil {
				continue
			}
			sessionName := "vulpine-" + agents[i].ID
			if _, err := a.orch.Agents.ResumeWithSessionIsolated(agents[i].ID, sessionName, configPath, cleanup); err == nil {
				_ = a.vault.UpdateAgentStatus(agents[i].ID, "active")
				resumed++
				affected = append(affected, agents[i].ID)
				continue
			}
			if cleanup != nil {
				cleanup()
			}
		}
		if resumed == 0 {
			return statusNotice{text: "No paused agents resumed"}
		}
		return shared.BulkAgentStatusMsg{
			AgentIDs: affected,
			Status:   "active",
			Notice:   fmt.Sprintf("Resumed %d agents", resumed),
		}
	}
}

func (a App) killAllAgents() tea.Cmd {
	if a.control != nil {
		ids := a.agentList.IDsByStatus(map[string]bool{
			"running": true, "thinking": true, "starting": true, "active": true,
		})
		return a.remoteBulkAgentStatusCommand("agents.killMany", ids, "interrupted", "Killed %d remote agents")
	}
	return func() tea.Msg {
		if a.orch == nil || a.vault == nil {
			return statusNotice{text: "Kill all unavailable"}
		}
		statuses := a.orch.Agents.List()
		if len(statuses) == 0 {
			return statusNotice{text: "No live agents to kill"}
		}

		affected := make([]string, 0, len(statuses))
		for _, status := range statuses {
			affected = append(affected, status.AgentID)
		}

		a.orch.Agents.KillAll()
		for _, agentID := range affected {
			_ = a.vault.UpdateAgentStatus(agentID, "interrupted")
		}
		return shared.BulkAgentStatusMsg{
			AgentIDs: affected,
			Status:   "interrupted",
			Notice:   fmt.Sprintf("Killed %d agents", len(affected)),
		}
	}
}

func (a App) remoteBulkAgentStatusCommand(method string, agentIDs []string, status string, noticeFormat string) tea.Cmd {
	return func() tea.Msg {
		if len(agentIDs) == 0 {
			return statusNotice{text: "No remote agents matched"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var result struct {
			Failures map[string]string `json:"failures"`
		}
		if err := a.control.ControlCall(ctx, method, map[string]any{"agentIds": agentIDs}, &result); err != nil {
			return statusNotice{text: "Remote command failed: " + err.Error()}
		}
		successful := make([]string, 0, len(agentIDs))
		for _, agentID := range agentIDs {
			if _, failed := result.Failures[agentID]; failed {
				continue
			}
			successful = append(successful, agentID)
		}
		if len(successful) == 0 {
			return statusNotice{text: fmt.Sprintf("Remote command failed for %d agents", len(agentIDs))}
		}
		notice := fmt.Sprintf(noticeFormat, len(successful))
		if len(result.Failures) > 0 {
			notice = fmt.Sprintf("%s (%d failed)", notice, len(result.Failures))
		}
		return shared.BulkAgentStatusMsg{
			AgentIDs: successful,
			Status:   status,
			Notice:   notice,
		}
	}
}

func statusNoticeCmd(text string) tea.Cmd {
	return func() tea.Msg {
		return statusNotice{text: text}
	}
}

// reloadSettingsProxies loads proxies from vault into the settings panel.
func (a *App) reloadSettingsProxies() {
	if a.vault == nil {
		return
	}
	storedProxies, err := a.vault.ListProxies()
	if err != nil {
		return
	}
	items := make([]settings.ProxyItem, len(storedProxies))
	for i, sp := range storedProxies {
		items[i] = settings.ProxyItem{
			ID:      sp.ID,
			Label:   safeProxyLabel(sp.Label),
			Latency: "untested",
		}
		var pc struct {
			Type string `json:"type"`
			Host string `json:"host"`
			Port int    `json:"port"`
		}
		if json.Unmarshal([]byte(sp.Config), &pc) == nil {
			items[i].Type = pc.Type
			items[i].Host = pc.Host
			items[i].Port = pc.Port
		}
		var geo struct {
			Country string `json:"country"`
		}
		if json.Unmarshal([]byte(sp.Geo), &geo) == nil {
			items[i].Country = geo.Country
		}
	}
	a.settings.SetProxies(items)
}

func safeProxyLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	if pc, err := proxy.ParseProxyURL(label); err == nil {
		return pc.String()
	}
	return label
}

// testProxy spawns a goroutine to test proxy latency and resolve geo.
func (a App) testProxy(proxyID, configJSON string) tea.Cmd {
	return func() tea.Msg {
		var pc proxy.ProxyConfig
		if err := json.Unmarshal([]byte(configJSON), &pc); err != nil {
			return shared.ProxyTestedMsg{ProxyID: proxyID, Latency: "error: invalid config"}
		}
		latency, err := proxy.TestProxy(pc)
		if err != nil {
			return shared.ProxyTestedMsg{ProxyID: proxyID, Latency: "error: " + err.Error()}
		}
		result := shared.ProxyTestedMsg{
			ProxyID: proxyID,
			Latency: fmt.Sprintf("%dms", latency),
		}
		// Also resolve geo
		geo, err := proxy.ResolveGeo(pc)
		if err == nil {
			result.ExitIP = geo.IP
			// Update vault with geo info
			if a.vault != nil {
				geoJSON, _ := json.Marshal(geo)
				a.vault.UpdateProxyGeo(proxyID, string(geoJSON))
			}
		}
		return result
	}
}

// gracefulShutdown pauses all running agents so they save state before exit.
func (a *App) gracefulShutdown() {
	if a.orch == nil || a.vault == nil {
		return
	}

	// Get all running agents and pause them
	agents := a.orch.Agents.List()
	for _, status := range agents {
		if shouldPauseOnShutdown(status.Status) {
			// Send /savestate and mark as paused in vault
			a.orch.Agents.PauseAgent(status.AgentID)
			a.vault.UpdateAgentStatus(status.AgentID, "paused")
		}
	}
}

func shouldPauseOnShutdown(status string) bool {
	switch status {
	case "running", "thinking", "starting", "active":
		return true
	default:
		return false
	}
}

// Header renders the VulpineOS header.
func Header() string {
	var b strings.Builder
	b.WriteString(shared.TitleStyle.Render("VulpineOS"))
	b.WriteString(shared.MutedStyle.Render(" -- Sovereign Agent Runtime"))
	return b.String()
}
