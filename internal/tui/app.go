package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"vulpineos/internal/config"
	"vulpineos/internal/juggler"
	"vulpineos/internal/kernel"
	"vulpineos/internal/monitor"
	"vulpineos/internal/openclaw"
	"vulpineos/internal/orchestrator"
	"vulpineos/internal/proxy"
	"vulpineos/internal/runtimeaudit"
	"vulpineos/internal/tui/agentdetail"
	"vulpineos/internal/tui/agentlist"
	"vulpineos/internal/tui/contextlist"
	"vulpineos/internal/tui/conversation"
	"vulpineos/internal/tui/poolstats"
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

// App is the root Bubbletea model for the 3-column agent workbench.
type App struct {
	kernel  *kernel.Kernel
	client  *juggler.Client
	orch    *orchestrator.Orchestrator
	vault   *vault.DB
	cfg     *config.Config
	monitor *monitor.Monitor

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
	poolStats    poolstats.Model
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

	eventCh chan tea.Msg
}

// NewApp creates the root TUI model.
func NewApp(k *kernel.Kernel, client *juggler.Client, orch *orchestrator.Orchestrator, v *vault.DB, cfg *config.Config, audit *runtimeaudit.Manager) App {
	eventCh := make(chan tea.Msg, 64)

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
		poolStats:    poolstats.New(),
		settings:     settings.New(),
		eventCh:      eventCh,
	}
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
			for event := range sub {
				eventCh <- shared.RuntimeEventMsg{Event: event}
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
			eventCh <- shared.TargetAttachedMsg{
				SessionID: e.SessionID,
				TargetID:  e.TargetInfo.TargetID,
				ContextID: e.TargetInfo.BrowserContextID,
				URL:       e.TargetInfo.URL,
			}
		})
		client.Subscribe("Browser.detachedFromTarget", func(sid string, params json.RawMessage) {
			var e juggler.DetachedFromTarget
			json.Unmarshal(params, &e)
			eventCh <- shared.TargetDetachedMsg{
				SessionID: e.SessionID,
				TargetID:  e.TargetID,
			}
		})
		client.Subscribe("Browser.trustWarmingStateChanged", func(sid string, params json.RawMessage) {
			var e juggler.TrustWarmingState
			json.Unmarshal(params, &e)
			eventCh <- shared.TrustWarmMsg{State: e.State, CurrentSite: e.CurrentSite}
		})
		client.Subscribe("Browser.telemetryUpdate", func(sid string, params json.RawMessage) {
			var e juggler.TelemetryUpdate
			json.Unmarshal(params, &e)
			eventCh <- shared.TelemetryMsg{
				MemoryMB:           e.MemoryMB,
				EventLoopLagMs:     e.EventLoopLagMs,
				DetectionRiskScore: e.DetectionRiskScore,
				ActiveContexts:     e.ActiveContexts,
				ActivePages:        e.ActivePages,
			}
		})
		client.Subscribe("Browser.injectionAttemptDetected", func(sid string, params json.RawMessage) {
			var e juggler.InjectionAttempt
			json.Unmarshal(params, &e)
			eventCh <- shared.AlertMsg{
				Timestamp: time.Now(),
				Type:      e.AttemptType,
				URL:       e.URL,
				Details:   e.Details,
				Blocked:   e.Blocked,
			}
		})
		client.Subscribe("Page.navigationCommitted", func(sid string, params json.RawMessage) {
			var e struct {
				FrameID string `json:"frameId"`
				URL     string `json:"url"`
			}
			json.Unmarshal(params, &e)
			eventCh <- shared.NavigationMsg{
				SessionID: sid,
				FrameID:   e.FrameID,
				URL:       e.URL,
			}
		})
		client.Subscribe("Page.eventFired", func(sid string, params json.RawMessage) {
			var e struct {
				FrameID string `json:"frameId"`
				Name    string `json:"name"`
			}
			json.Unmarshal(params, &e)
			eventCh <- shared.PageLoadMsg{
				SessionID: sid,
				FrameID:   e.FrameID,
				Name:      e.Name,
			}
		})
		client.Subscribe("Page.frameAttached", func(sid string, params json.RawMessage) {
			var e struct {
				FrameID       string `json:"frameId"`
				ParentFrameID string `json:"parentFrameId"`
			}
			json.Unmarshal(params, &e)
			eventCh <- shared.FrameAttachedMsg{
				SessionID:     sid,
				FrameID:       e.FrameID,
				ParentFrameID: e.ParentFrameID,
			}
		})
		client.Subscribe("Runtime.executionContextCreated", func(sid string, params json.RawMessage) {
			var e struct {
				ExecutionContextID string `json:"executionContextId"`
				AuxData            struct {
					FrameID string `json:"frameId"`
				} `json:"auxData"`
			}
			json.Unmarshal(params, &e)
			eventCh <- shared.ExecContextCreatedMsg{
				SessionID:          sid,
				ExecutionContextID: e.ExecutionContextID,
				FrameID:            e.AuxData.FrameID,
			}
		})
	}

	// Forward agent status updates from orchestrator to TUI
	if orch != nil {
		go func() {
			for status := range orch.Agents.StatusChan() {
				eventCh <- shared.AgentStatusMsg{
					AgentID:   status.AgentID,
					ContextID: status.ContextID,
					Status:    status.Status,
					Objective: status.Objective,
					Tokens:    status.Tokens,
				}
			}
		}()

		// Forward conversation messages from orchestrator
		go func() {
			for msg := range orch.Agents.ConversationChan() {
				eventCh <- shared.ConversationEntryMsg{
					AgentID:   msg.AgentID,
					Role:      msg.Role,
					Content:   msg.Content,
					Tokens:    msg.Tokens,
					Timestamp: time.Now(),
				}
			}
		}()
	}

	// Forward rate limit monitor alerts to TUI
	go func() {
		for alert := range mon.AlertChan() {
			eventCh <- statusNotice{text: fmt.Sprintf("WARNING %s: %s on agent %s", alert.Type, alert.Details, alert.AgentID)}
		}
	}()

	return app
}

func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.waitForEvent(),
		a.tick(),
	)
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
			if a.conversation.Focused() || (msg.String() != "v" && msg.String() != "t" && msg.String() != "o") {
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
			a.gracefulShutdown()
			return a, tea.Quit
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
			enabled := !a.conversation.TraceOnly()
			a.conversation.SetTraceOnly(enabled)
			if enabled {
				a.notice = "Trace mode enabled — showing tool actions and results"
			} else {
				a.notice = "Trace mode disabled — showing full conversation"
			}
			a.noticeTTL = 3
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
			if a.orch != nil {
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
			// View: toggle browser window visibility
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
			} else if a.kernel != nil && a.kernel.IsHeadless() {
				a.notice = "Cannot show browser in headless mode — run without --headless"
				a.noticeTTL = 4
			} else {
				// Fallback: open URL in system browser
				url := a.contextList.SelectedURL()
				if url != "" && url != "about:blank" {
					_ = startExternalCommand("open", url)
					a.notice = "Opened " + url
					a.noticeTTL = 3
				}
			}
		case "o":
			if a.selectedAgentID == "" {
				a.notice = "No agent selected"
				a.noticeTTL = 3
				return a, nil
			}
			logPath := agentSessionLogPath(a.selectedAgentID)
			if _, err := os.Stat(logPath); err != nil {
				a.notice = "No session log yet for selected agent"
				a.noticeTTL = 4
				return a, nil
			}
			if err := startExternalCommand("open", logPath); err != nil {
				a.notice = "Failed to open session log: " + err.Error()
				a.noticeTTL = 4
				return a, nil
			}
			a.notice = "Opened session log"
			a.noticeTTL = 3
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
							Label:   sp.Label,
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
			if err := config.RequestReconfigure(); err != nil {
				a.notice = "Failed to queue reconfigure: " + err.Error()
				a.noticeTTL = 4
				return a, nil
			}
			a.gracefulShutdown()
			return a, tea.Quit
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
			ksMsg := shared.KernelStatusMsg{
				Running: a.kernel.Running(),
				PID:     a.kernel.PID(),
				Uptime:  a.kernel.Uptime(),
			}
			a.systemInfo, _ = a.systemInfo.Update(ksMsg)
		}
		if a.orch != nil {
			avail, active, total := a.orch.Pool.Stats()
			a.poolStats.SetStats(avail, active, total)
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
			if msg.Status == "completed" || msg.Status == "error" {
				a.conversation.SetThinking(false)
			}
			a.refreshAgentDetail(msg.AgentID)
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
		if a.monitor != nil {
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

	case shared.PoolStatsMsg:
		a.poolStats, _ = a.poolStats.Update(msg)
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
		a.conversation.SetAwake(true)
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

	case shared.ProxyAddMsg:
		pc, err := proxy.ParseProxyURL(msg.URL)
		if err != nil {
			a.notice = "Invalid proxy: " + err.Error()
			a.noticeTTL = 3
		} else {
			configJSON, _ := json.Marshal(pc)
			if a.vault != nil {
				a.vault.AddProxy(string(configJSON), "", msg.URL)
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

// detailHeight is the fixed height for the agent detail area.
// Min/max constraints for panel sizes
const (
	minSplit      = 5
	maxSplitRatio = 80 // percent of column height
)

func (a App) View() string {
	if a.width == 0 {
		return "Loading..."
	}

	leftWidth := a.leftWidth
	rightWidth := a.rightWidth
	// Each panel adds 4 cols (2 border + 2 padding). Three panels = 12 cols overhead.
	centerWidth := a.width - leftWidth - rightWidth - 12
	if centerWidth < 20 {
		centerWidth = 20
	}

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
		centerContent = a.settings.View()
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

	bar := shared.TitleStyle.Render("VULPINE") +
		shared.MutedStyle.Render(" | ") +
		shared.RunningStyle.Render("* "+mode) +
		shared.MutedStyle.Render("  n:new  p/r:agent  P/R:all  X:kill-all  x:del  v:view  o:log  m:mode  S:settings  Enter:chat  Tab:focus  ") +
		shared.MutedStyle.Render("t:trace  ") +
		arrowMode +
		shared.MutedStyle.Render("  q:quit") +
		ctxHint

	return lipgloss.NewStyle().MaxWidth(a.width).Render(bar)
}

// updatePanelSizes recalculates panel dimensions after a resize.
func (a *App) updatePanelSizes() {
	leftWidth := a.leftWidth
	rightWidth := a.rightWidth
	centerWidth := a.width - leftWidth - rightWidth - 12
	if centerWidth < 20 {
		centerWidth = 20
	}
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
	a.poolStats.SetWidth(rightWidth)

	// Update text input widths to fit center panel
	inputWidth := centerWidth - 6
	if inputWidth < 10 {
		inputWidth = 10
	}
	a.nameInput.Width = inputWidth
	a.taskInput.Width = inputWidth
}

func (a *App) resizeModeEnabled() bool {
	return a.resizeMode
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

// selectCurrentAgent loads the currently highlighted agent's data.
func (a *App) selectCurrentAgent() tea.Cmd {
	newID := a.agentList.SelectedAgentID()
	if newID == a.selectedAgentID || newID == "" {
		return nil
	}
	a.selectedAgentID = newID
	a.conversation.SetAgentID(newID)
	a.agentList.ClearUnread(newID)

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
		return <-a.eventCh
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
		introMsg := "You are an AI agent named '" + name + "'. Your assigned runtime name for this session is exactly '" + name + "' and you must not claim a different name or inherited persona. Your purpose: " + description + ". Introduce yourself briefly (1-2 sentences), use the assigned name exactly, and ask how you can help."
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

// pauseAgent pauses an agent.
func (a App) pauseAgent(agentID string) tea.Cmd {
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
		_, err = a.orch.Agents.ResumeWithSession(agentID, sessionName, configPath)
		if cleanup != nil {
			cleanup()
		}
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

// deleteAgent removes an agent.
func (a *App) deleteAgent(agentID string) tea.Cmd {
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

func agentSessionLogPath(agentID string) string {
	return filepath.Join(config.OpenClawProfileDir(), "agents", "main", "sessions", "vulpine-"+agentID+".jsonl")
}

func (a App) selectedAgentStatus() string {
	if a.selectedAgentID == "" || a.vault == nil {
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
			if _, err := a.orch.Agents.ResumeWithSession(agents[i].ID, sessionName, configPath); err == nil {
				if cleanup != nil {
					cleanup()
				}
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
			Label:   sp.Label,
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
