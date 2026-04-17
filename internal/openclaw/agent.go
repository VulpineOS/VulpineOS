package openclaw

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ConversationMsg represents a captured conversation message from an agent.
type ConversationMsg struct {
	AgentID string
	Role    string
	Content string
	Tokens  int
}

// AgentStatus represents an agent's current state.
type AgentStatus struct {
	AgentID   string `json:"agent_id"`
	ContextID string `json:"context_id"`
	Status    string `json:"status"`    // starting, running, thinking, completed, error
	Objective string `json:"objective"` // current task description
	Tokens    int    `json:"tokens"`    // tokens consumed
}

// AgentOutput is a JSON message from the agent's stdout.
type AgentOutput struct {
	// Standard JSON-lines format (if agent outputs line-by-line)
	Type      string `json:"type"`      // status, log, result, error
	Status    string `json:"status"`    // for type=status
	Objective string `json:"objective"` // for type=status
	Tokens    int    `json:"tokens"`    // for type=status
	Message   string `json:"message"`   // for type=log or type=error
	Result    string `json:"result"`    // for type=result
	Role      string `json:"role"`      // for type=log (user, assistant, etc.)

	// OpenClaw `agent --json` response format
	Payloads []struct {
		Text string `json:"text"`
	} `json:"payloads"`
	Meta *struct {
		DurationMs int `json:"durationMs"`
		AgentMeta  *struct {
			SessionID string `json:"sessionId"`
			Provider  string `json:"provider"`
			Model     string `json:"model"`
			Usage     *struct {
				Input  int `json:"input"`
				Output int `json:"output"`
				Total  int `json:"total"`
			} `json:"usage"`
		} `json:"agentMeta"`
	} `json:"meta"`
}

// Agent manages a single OpenClaw subprocess.
type Agent struct {
	ID             string
	ContextID      string
	cmd            *exec.Cmd
	stdinPipe      io.WriteCloser
	stderrBuf      *bytes.Buffer // captures stderr for error reporting
	statusCh       chan AgentStatus
	conversationCh chan ConversationMsg
	doneCh         chan struct{}
	mu             sync.Mutex
	status         AgentStatus
	startedAt      time.Time
	env            map[string]string // extra environment variables for the subprocess
	RestartCount   int               // number of automatic restarts after crashes
	binary         string            // binary path used to start this agent
	args           []string          // args used to start this agent
	waitState      *agentWaitState
	stopStatus     string
	sessionLogPath string
	lastToolName   string
	lastToolFailed bool
	lastToolError  string
}

type agentWaitState struct {
	once sync.Once
	err  error
}

// newAgent creates a new agent instance (not yet started).
func newAgent(id, contextID string, statusCh chan AgentStatus) *Agent {
	return &Agent{
		ID:             id,
		ContextID:      contextID,
		statusCh:       statusCh,
		conversationCh: make(chan ConversationMsg, 64),
		doneCh:         make(chan struct{}),
		status: AgentStatus{
			AgentID:   id,
			ContextID: contextID,
			Status:    "starting",
		},
	}
}

// start launches the agent subprocess.
func (a *Agent) start(binary string, args []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.binary = binary
	a.args = make([]string, len(args))
	copy(a.args, args)
	a.cmd = exec.Command(binary, args...)
	configureAgentProcess(a.cmd)
	a.waitState = &agentWaitState{}

	// Apply extra environment variables (e.g. OPENCLAW_CONFIG_PATH)
	if len(a.env) > 0 {
		a.cmd.Env = os.Environ()
		for k, v := range a.env {
			a.cmd.Env = append(a.cmd.Env, k+"="+v)
		}
	}

	stdout, err := a.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}

	stdinPipe, err := a.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}
	a.stdinPipe = stdinPipe

	// Capture stderr so we can report errors to the user
	a.stderrBuf = &bytes.Buffer{}
	a.cmd.Stderr = a.stderrBuf

	if err := a.cmd.Start(); err != nil {
		return fmt.Errorf("start agent process: %w", err)
	}

	a.startedAt = time.Now()
	a.status.Status = "running"
	a.emitStatusLocked() // already holding a.mu

	// Start activity watchdog — warns with escalating timeouts
	activityCh := make(chan struct{}, 1)
	go func() {
		firstTimeout := 30 * time.Second
		longTimeout := 90 * time.Second
		timer := time.NewTimer(firstTimeout)
		defer timer.Stop()
		warnings := 0
		for {
			select {
			case <-activityCh:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(firstTimeout)
				warnings = 0
			case <-timer.C:
				a.mu.Lock()
				status := a.status.Status
				a.mu.Unlock()
				if status == "running" || status == "thinking" || status == "active" {
					warnings++
					var msg string
					switch warnings {
					case 1:
						msg = "Agent is working — waiting for response..."
					case 2:
						msg = "Still processing. The agent may be performing a browser action. Press v to view the page."
					default:
						msg = fmt.Sprintf("Agent has been working for %ds with no response. It may be stuck — try sending another message or check the browser with v.", warnings*30+30)
					}
					a.emitConversation(ConversationMsg{
						AgentID: a.ID,
						Role:    "system",
						Content: msg,
					})
				}
				timer.Reset(longTimeout)
			case <-a.doneCh:
				return
			}
		}
	}()

	if a.sessionLogPath != "" {
		go a.streamSessionEvents()
	}

	// Read JSON objects from stdout using a streaming decoder.
	// OpenClaw outputs pretty-printed multi-line JSON, so a line-based
	// scanner would fail to parse it. json.Decoder handles this correctly.
	go func() {
		defer close(a.doneCh)
		defer close(a.conversationCh)

		decoder := json.NewDecoder(stdout)
		for decoder.More() {
			var output AgentOutput
			if err := decoder.Decode(&output); err != nil {
				// If we hit a decode error, try to skip past it
				// by draining to find the next valid JSON
				if err == io.EOF {
					break
				}
				continue
			}
			// Signal activity to watchdog
			select {
			case activityCh <- struct{}{}:
			default:
			}
			a.handleOutput(output)
		}

		// Wait for the process to exit so we can inspect the exit code
		waitErr := a.waitProcess()

		a.mu.Lock()
		if a.stopStatus != "" {
			a.status.Status = a.stopStatus
		} else if a.status.Status != "completed" && a.status.Status != "error" {
			// Process exited without producing a valid response
			if waitErr != nil {
				a.status.Status = "error"
				// Emit stderr content as an error message to the conversation
				stderrText := strings.TrimSpace(a.stderrBuf.String())
				if stderrText != "" {
					a.emitConversation(ConversationMsg{
						AgentID: a.ID,
						Role:    "system",
						Content: fmt.Sprintf("Agent error (exit %v):\n%s", waitErr, truncate(stderrText, 500)),
					})
				} else {
					a.emitConversation(ConversationMsg{
						AgentID: a.ID,
						Role:    "system",
						Content: fmt.Sprintf("Agent exited with error: %v", waitErr),
					})
				}
			} else {
				a.status.Status = "completed"
			}
		} else if a.status.Status == "error" && a.stderrBuf.Len() > 0 {
			// Status was already set to error by handleOutput, but add stderr details
			stderrText := strings.TrimSpace(a.stderrBuf.String())
			if stderrText != "" {
				a.emitConversation(ConversationMsg{
					AgentID: a.ID,
					Role:    "system",
					Content: fmt.Sprintf("Agent stderr:\n%s", truncate(stderrText, 500)),
				})
			}
		}
		a.emitStatusLocked()
		a.mu.Unlock()
	}()

	return nil
}

func (a *Agent) handleOutput(output AgentOutput) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Handle OpenClaw `agent --json` response format (single JSON blob with payloads + meta)
	if len(output.Payloads) > 0 {
		for _, p := range output.Payloads {
			if p.Text != "" {
				a.emitConversation(ConversationMsg{
					AgentID: a.ID,
					Role:    "assistant",
					Content: p.Text,
				})
			}
		}
		if output.Meta != nil && output.Meta.AgentMeta != nil && output.Meta.AgentMeta.Usage != nil {
			a.status.Tokens = output.Meta.AgentMeta.Usage.Total
		}
		a.status.Status = "completed"
		a.emitStatusLocked()
		return
	}

	switch output.Type {
	case "status":
		if output.Status != "" {
			a.status.Status = output.Status
		}
		if output.Objective != "" {
			a.status.Objective = output.Objective
		}
		if output.Tokens > 0 {
			a.status.Tokens = output.Tokens
		}
	case "log":
		if output.Message != "" {
			role := output.Role
			if role == "" {
				role = "assistant"
			}
			a.emitConversation(ConversationMsg{
				AgentID: a.ID,
				Role:    role,
				Content: output.Message,
				Tokens:  output.Tokens,
			})
		}
	case "result":
		content := output.Result
		if content == "" {
			content = output.Message
		}
		if content != "" {
			a.emitConversation(ConversationMsg{
				AgentID: a.ID,
				Role:    "assistant",
				Content: content,
				Tokens:  output.Tokens,
			})
		}
	case "error":
		a.status.Status = "error"
		errMsg := output.Message
		if errMsg != "" {
			a.emitConversation(ConversationMsg{
				AgentID: a.ID,
				Role:    "system",
				Content: "Agent error: " + errMsg,
			})
		}
	}

	a.emitStatusLocked()
}

type sessionLogLine struct {
	Type    string `json:"type"`
	Message *struct {
		Role       string `json:"role"`
		ToolName   string `json:"toolName"`
		ToolCallID string `json:"toolCallId"`
		IsError    bool   `json:"isError"`
		Content    []struct {
			Type      string                 `json:"type"`
			Text      string                 `json:"text"`
			Thinking  string                 `json:"thinking"`
			ID        string                 `json:"id"`
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		} `json:"content"`
		Details map[string]interface{} `json:"details"`
	} `json:"message"`
}

func (a *Agent) streamSessionEvents() {
	path := a.sessionLogPath
	offset := int64(0)
	if info, err := os.Stat(path); err == nil {
		offset = info.Size()
	} else if !os.IsNotExist(err) {
		return
	}

	var pending string
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-a.doneCh:
			return
		case <-ticker.C:
			data, newOffset, err := readSessionChunk(path, offset)
			if err != nil {
				continue
			}
			offset = newOffset
			if len(data) == 0 {
				continue
			}
			blob := pending + string(data)
			lines := strings.Split(blob, "\n")
			pending = ""
			if !strings.HasSuffix(blob, "\n") {
				pending = lines[len(lines)-1]
				lines = lines[:len(lines)-1]
			}
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				a.handleSessionLogLine(line)
			}
		}
	}
}

func readSessionChunk(path string, offset int64) ([]byte, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer file.Close()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, offset, err
	}
	return data, offset + int64(len(data)), nil
}

func (a *Agent) handleSessionLogLine(line string) {
	var event sessionLogLine
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return
	}
	if event.Type != "message" || event.Message == nil {
		return
	}

	switch event.Message.Role {
	case "assistant":
		if a.lastToolFailed {
			a.emitConversation(ConversationMsg{
				AgentID: a.ID,
				Role:    "system",
				Content: formatPostFailureWarning(a.lastToolName, a.lastToolError),
			})
			a.lastToolFailed = false
		}
		for _, item := range event.Message.Content {
			if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
				a.emitConversation(ConversationMsg{
					AgentID: a.ID,
					Role:    "assistant",
					Content: item.Text,
				})
			}
		}
		for _, item := range event.Message.Content {
			if item.Type != "toolCall" {
				continue
			}
			a.lastToolName = item.Name
			a.lastToolFailed = false
			a.lastToolError = ""
			if msg := summarizeToolCall(item.Name, item.Arguments); msg != "" {
				a.emitConversation(ConversationMsg{
					AgentID: a.ID,
					Role:    "system",
					Content: msg,
				})
			}
		}
	case "toolResult":
		if msg := summarizeToolResult(event.Message.ToolName, event.Message.Content, event.Message.Details); msg != "" {
			a.emitConversation(ConversationMsg{
				AgentID: a.ID,
				Role:    "system",
				Content: msg,
			})
		}
		status, errText := toolResultStatus(event.Message.IsError, event.Message.Content, event.Message.Details)
		a.lastToolName = event.Message.ToolName
		if status == "error" {
			a.lastToolFailed = true
			a.lastToolError = errText
		} else if status != "" {
			a.lastToolFailed = false
			a.lastToolError = ""
		}
	}
}

func formatPostFailureWarning(toolName, errText string) string {
	if toolName == "" {
		toolName = "tool"
	}
	if errText != "" {
		return fmt.Sprintf("Warning: assistant replied after failed %s action — %s", toolName, truncate(singleLine(errText), 180))
	}
	return fmt.Sprintf("Warning: assistant replied after failed %s action with no successful retry recorded", toolName)
}

func summarizeToolCall(name string, args map[string]interface{}) string {
	if name == "" {
		return ""
	}
	switch name {
	case "browser":
		return summarizeBrowserCall(args)
	case "web_fetch":
		if url, _ := args["url"].(string); url != "" {
			return fmt.Sprintf("Running tool: web_fetch %s", url)
		}
	case "web_search":
		if query, _ := args["query"].(string); query != "" {
			return fmt.Sprintf("Running tool: web_search %s", truncate(singleLine(query), 180))
		}
	case "read", "write":
		if path, _ := args["path"].(string); path != "" {
			return fmt.Sprintf("Running tool: %s %s", name, truncate(path, 180))
		}
	case "exec":
		if command, _ := args["command"].(string); command != "" {
			return fmt.Sprintf("Running tool: exec %s", truncate(singleLine(command), 180))
		}
	default:
		argText := compactJSON(args)
		if argText == "" {
			return fmt.Sprintf("Running tool: %s", name)
		}
		return fmt.Sprintf("Running tool: %s %s", name, truncate(argText, 180))
	}
	argText := compactJSON(args)
	if argText == "" {
		return fmt.Sprintf("Running tool: %s", name)
	}
	return fmt.Sprintf("Running tool: %s %s", name, truncate(argText, 180))
}

func summarizeToolResult(name string, content []struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text"`
	Thinking  string                 `json:"thinking"`
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}, details map[string]interface{}) string {
	if name == "" {
		name = "tool"
	}
	if status, errText := toolResultStatus(false, content, details); status != "" {
		if status == "error" {
			if errText != "" {
				return fmt.Sprintf("Tool failed: %s — %s", name, truncate(singleLine(errText), 180))
			}
			return fmt.Sprintf("Tool failed: %s", name)
		}
		if msg := summarizeToolSuccess(name, details); msg != "" {
			return msg
		}
		for _, item := range content {
			if item.Type != "text" || item.Text == "" {
				continue
			}
			var payload map[string]interface{}
			if json.Unmarshal([]byte(item.Text), &payload) != nil {
				continue
			}
			if msg := summarizeToolSuccess(name, payload); msg != "" {
				return msg
			}
		}
		return fmt.Sprintf("Tool completed: %s", name)
	}

	return fmt.Sprintf("Tool completed: %s", name)
}

func toolResultStatus(isError bool, content []struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text"`
	Thinking  string                 `json:"thinking"`
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}, details map[string]interface{}) (status string, errText string) {
	if isError {
		if errText, _ = details["error"].(string); errText != "" {
			return "error", errText
		}
		if aggregated, _ := details["aggregated"].(string); aggregated != "" {
			return "error", aggregated
		}
		return "error", ""
	}

	if exitCode, ok := numericDetail(details["exitCode"]); ok && exitCode != 0 {
		if aggregated, _ := details["aggregated"].(string); aggregated != "" {
			return "error", aggregated
		}
		if errText, _ = details["error"].(string); errText != "" {
			return "error", errText
		}
		return "error", fmt.Sprintf("command exited with code %d", exitCode)
	}
	if okValue, ok := details["ok"].(bool); ok && !okValue {
		if errText, _ = details["error"].(string); errText != "" {
			return "error", errText
		}
		return "error", ""
	}

	if status, _ = details["status"].(string); status != "" {
		errText, _ = details["error"].(string)
		return status, errText
	}

	for _, item := range content {
		if item.Type != "text" || item.Text == "" {
			continue
		}
		var payload map[string]interface{}
		if json.Unmarshal([]byte(item.Text), &payload) != nil {
			continue
		}
		status, _ = payload["status"].(string)
		errText, _ = payload["error"].(string)
		if status != "" {
			return status, errText
		}
	}

	return "", ""
}

func numericDetail(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func summarizeBrowserCall(args map[string]interface{}) string {
	action, _ := args["action"].(string)
	if action == "" {
		return "Running browser action"
	}
	switch action {
	case "open":
		if url, _ := args["url"].(string); url != "" {
			return fmt.Sprintf("Running browser action: open %s", url)
		}
	case "snapshot":
		if targetID, _ := args["targetId"].(string); targetID != "" {
			return fmt.Sprintf("Running browser action: snapshot target %s", truncate(targetID, 24))
		}
		return "Running browser action: snapshot"
	case "click":
		if text, _ := args["text"].(string); text != "" {
			return fmt.Sprintf("Running browser action: click %s", truncate(text, 120))
		}
		if selector, _ := args["selector"].(string); selector != "" {
			return fmt.Sprintf("Running browser action: click %s", truncate(selector, 120))
		}
	case "type":
		if text, _ := args["text"].(string); text != "" {
			return fmt.Sprintf("Running browser action: type %s", truncate(singleLine(text), 120))
		}
	case "scroll":
		if direction, _ := args["direction"].(string); direction != "" {
			return fmt.Sprintf("Running browser action: scroll %s", direction)
		}
	case "status", "start", "stop", "close":
		return fmt.Sprintf("Running browser action: %s", action)
	}
	argText := compactJSON(args)
	if argText == "" {
		return fmt.Sprintf("Running browser action: %s", action)
	}
	return fmt.Sprintf("Running browser action: %s %s", action, truncate(argText, 160))
}

func summarizeToolSuccess(name string, payload map[string]interface{}) string {
	switch name {
	case "browser":
		if targetID, _ := payload["targetId"].(string); targetID != "" {
			if url, _ := payload["url"].(string); url != "" {
				return fmt.Sprintf("Tool completed: browser — opened %s", truncate(url, 160))
			}
			return fmt.Sprintf("Tool completed: browser — opened target %s", truncate(targetID, 24))
		}
		if url, _ := payload["url"].(string); url != "" {
			return fmt.Sprintf("Tool completed: browser — at %s", truncate(url, 160))
		}
		if title, _ := payload["title"].(string); title != "" {
			return fmt.Sprintf("Tool completed: browser — %s", truncate(title, 160))
		}
	case "web_fetch":
		if url, _ := payload["url"].(string); url != "" {
			return fmt.Sprintf("Tool completed: web_fetch — fetched %s", truncate(url, 160))
		}
	case "web_search":
		if query, _ := payload["query"].(string); query != "" {
			return fmt.Sprintf("Tool completed: web_search — %s", truncate(query, 160))
		}
	}
	return ""
}

func compactJSON(v interface{}) string {
	if v == nil {
		return ""
	}
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func (a *Agent) emitStatus() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.emitStatusLocked()
}

func (a *Agent) emitStatusLocked() {
	select {
	case a.statusCh <- a.status:
	default:
		// Channel full, drop
	}
}

// SendMessage writes a message to the agent's stdin pipe.
func (a *Agent) SendMessage(text string) error {
	a.mu.Lock()
	pipe := a.stdinPipe
	a.mu.Unlock()

	if pipe == nil {
		return fmt.Errorf("stdin pipe not available")
	}
	_, err := fmt.Fprintf(pipe, "%s\n", text)
	return err
}

// emitConversation sends a conversation message to the channel (non-blocking).
func (a *Agent) emitConversation(msg ConversationMsg) {
	select {
	case a.conversationCh <- msg:
	default:
		// Channel full, drop
	}
}

// Stop kills the agent subprocess and waits for it to exit.
func (a *Agent) Stop() error {
	return a.stopWithStatus("completed")
}

func (a *Agent) stopWithStatus(status string) error {
	a.mu.Lock()
	cmd := a.cmd
	pipe := a.stdinPipe
	if status == "" {
		status = "completed"
	}
	a.stopStatus = status
	a.status.Status = status
	a.mu.Unlock()

	// Try to tell OpenClaw to save state before killing
	if pipe != nil {
		fmt.Fprintf(pipe, "/savestate\n")
		time.Sleep(500 * time.Millisecond)
	}

	if cmd != nil && cmd.Process != nil {
		_ = killAgentProcess(cmd)
		// Reap the process through the shared wait path so Stop and the stdout
		// reader goroutine do not race on exec.Cmd.Wait.
		a.waitProcess()
	}

	if pipe != nil {
		pipe.Close()
	}

	a.emitStatus()
	return nil
}

// Wait blocks until the agent exits.
func (a *Agent) Wait() {
	<-a.doneCh
}

func (a *Agent) waitProcess() error {
	a.mu.Lock()
	cmd := a.cmd
	state := a.waitState
	a.mu.Unlock()

	if cmd == nil || state == nil {
		return nil
	}

	state.once.Do(func() {
		state.err = cmd.Wait()
	})
	return state.err
}

func (a *Agent) StopRequested() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stopStatus != ""
}

// ExitCode returns the process exit code, or -1 if still running or not started.
func (a *Agent) ExitCode() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cmd == nil || a.cmd.ProcessState == nil {
		return -1
	}
	return a.cmd.ProcessState.ExitCode()
}

// restart re-launches the agent subprocess using the same binary and args.
func (a *Agent) restart() error {
	// Reset channels for the new process
	a.conversationCh = make(chan ConversationMsg, 64)
	a.doneCh = make(chan struct{})
	return a.start(a.binary, a.args)
}

// Status returns the current agent status.
func (a *Agent) Status() AgentStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
