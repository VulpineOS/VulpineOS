package scripting

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"vulpineos/internal/humaninput"
	"vulpineos/internal/juggler"
)

const (
	redactedScriptValue       = "[redacted]"
	maxScriptResultFieldBytes = 4096
	maxScriptWaitDuration     = 30 * time.Second
	truncatedScriptValue      = "... [truncated]"
)

// Step is a single instruction in a script.
type Step struct {
	Action string `json:"action"` // navigate, click, type, wait, extract, screenshot, if, set
	Target string `json:"target"` // CSS selector or URL
	Value  string `json:"value"`  // text to type, variable name, etc.
	Store  string `json:"store"`  // variable name to store result
	WPM    int    `json:"wpm"`    // optional typing speed for type actions
}

// Script is a sequence of steps to execute.
type Script struct {
	Steps []Step `json:"steps"`
}

// StepResult describes the outcome of a single executed step.
type StepResult struct {
	Index      int    `json:"index"`
	Action     string `json:"action"`
	Target     string `json:"target,omitempty"`
	Value      string `json:"value,omitempty"`
	Store      string `json:"store,omitempty"`
	Status     string `json:"status"`
	Output     string `json:"output,omitempty"`
	DurationMS int64  `json:"durationMs"`
}

// Engine executes scripts against a Juggler client.
type Engine struct {
	client             *juggler.Client
	sessionID          string
	frameID            string
	executionContextID string
	mu                 sync.RWMutex
	unsubscribes       []func()
	vars               map[string]string
}

// NewEngine creates a scripting engine backed by the given Juggler client.
func NewEngine(client *juggler.Client) *Engine {
	e := &Engine{
		client: client,
		vars:   make(map[string]string),
	}
	e.unsubscribes = append(e.unsubscribes, client.SubscribeWithCancel("Runtime.executionContextCreated", func(sessionID string, params json.RawMessage) {
		var ev struct {
			Context struct {
				ID       interface{} `json:"id"`
				UniqueID string      `json:"uniqueId"`
				AuxData  struct {
					FrameID string `json:"frameId"`
				} `json:"auxData"`
			} `json:"context"`
			ExecutionContextID interface{} `json:"executionContextId"`
			AuxData            struct {
				FrameID string `json:"frameId"`
			} `json:"auxData"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			return
		}
		id := ev.Context.UniqueID
		if id == "" {
			id = fmt.Sprint(ev.Context.ID)
		}
		if id == "<nil>" || id == "" {
			id = fmt.Sprint(ev.ExecutionContextID)
		}
		frameID := ev.Context.AuxData.FrameID
		if frameID == "" {
			frameID = ev.AuxData.FrameID
		}
		if id == "<nil>" || id == "" {
			return
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.sessionID == "" || sessionID == e.sessionID || (frameID != "" && frameID == e.frameID) {
			e.executionContextID = id
			if frameID != "" {
				e.frameID = frameID
			}
		}
	}))
	e.unsubscribes = append(e.unsubscribes, client.SubscribeWithCancel("Runtime.executionContextDestroyed", func(sessionID string, params json.RawMessage) {
		var ev struct {
			ExecutionContextID       interface{} `json:"executionContextId"`
			ExecutionContextUniqueID string      `json:"executionContextUniqueId"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			return
		}
		id := ev.ExecutionContextUniqueID
		if id == "" {
			id = fmt.Sprint(ev.ExecutionContextID)
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		if sessionID == e.sessionID && id == e.executionContextID {
			e.executionContextID = ""
		}
	}))
	return e
}

// Close releases event subscriptions held by the engine.
func (e *Engine) Close() {
	e.mu.Lock()
	unsubscribes := e.unsubscribes
	e.unsubscribes = nil
	e.mu.Unlock()
	for _, unsubscribe := range unsubscribes {
		unsubscribe()
	}
}

// SetSession sets the Juggler session ID used for protocol calls.
func (e *Engine) SetSession(sessionID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sessionID = sessionID
}

// SetFrame sets the main frame ID used for navigation calls.
func (e *Engine) SetFrame(frameID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.frameID = frameID
}

// GetVar returns the value of a variable, or empty string if unset.
func (e *Engine) GetVar(name string) string {
	return e.vars[name]
}

// SetVar sets a variable value.
func (e *Engine) SetVar(name, value string) {
	e.vars[name] = value
}

// ParseScript deserializes a Script from JSON bytes.
func ParseScript(data []byte) (*Script, error) {
	var s Script
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse script: %w", err)
	}
	return &s, nil
}

// Execute runs all steps in the script sequentially.
func (e *Engine) Execute(script *Script) error {
	for i, step := range script.Steps {
		if err := e.executeStep(step); err != nil {
			return fmt.Errorf("step %d (%s): %w", i, step.Action, err)
		}
	}
	return nil
}

// ExecuteWithResults runs all steps and returns per-step results.
func (e *Engine) ExecuteWithResults(script *Script) ([]StepResult, error) {
	results := make([]StepResult, 0, len(script.Steps))
	for i, step := range script.Steps {
		start := time.Now()
		err := e.executeStep(step)
		result := StepResult{
			Index:      i,
			Action:     step.Action,
			Target:     safeScriptField(step.Target),
			Value:      safeScriptStepValue(step),
			Store:      safeScriptField(step.Store),
			Status:     "ok",
			Output:     e.safeStepOutput(step),
			DurationMS: time.Since(start).Milliseconds(),
		}
		if err != nil {
			err = safeScriptStepError(step, err)
			result.Status = "error"
			result.Output = err.Error()
			results = append(results, result)
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

// Vars returns a copy of the current script variables.
func (e *Engine) Vars() map[string]string {
	out := make(map[string]string, len(e.vars))
	for key, value := range e.vars {
		out[key] = value
	}
	return out
}

// RedactedVars returns script variables safe for operator-facing API responses.
func (e *Engine) RedactedVars() map[string]string {
	out := make(map[string]string, len(e.vars))
	for key, value := range e.vars {
		if sensitiveScriptToken(key) {
			out[key] = redactedScriptValue
			continue
		}
		out[key] = limitScriptDisplayValue(value)
	}
	return out
}

func (e *Engine) executeStep(step Step) error {
	switch step.Action {
	case "navigate":
		return e.doNavigate(step)
	case "click":
		return e.doClick(step)
	case "type":
		return e.doType(step)
	case "wait":
		return e.doWait(step)
	case "extract":
		return e.doExtract(step)
	case "screenshot":
		return e.doScreenshot(step)
	case "set":
		return e.doSet(step)
	case "if":
		return e.doIf(step)
	default:
		return fmt.Errorf("unknown action %q", step.Action)
	}
}

func (e *Engine) doNavigate(step Step) error {
	target := e.expandVars(step.Target)
	e.mu.Lock()
	frameID := e.frameID
	e.executionContextID = ""
	e.mu.Unlock()

	params := map[string]interface{}{
		"url": target,
	}
	if frameID != "" {
		params["frameId"] = frameID
	}
	_, err := e.client.Call(e.sessionID, "Page.navigate", params)
	return err
}

func (e *Engine) doClick(step Step) error {
	target := e.expandVars(step.Target)
	// Evaluate querySelector to find the element, then dispatch click.
	expr := fmt.Sprintf(`(() => {
		const el = document.querySelector(%q);
		if (!el) throw new Error("element not found: %s");
		el.click();
		return "clicked";
	})()`, target, target)
	_, err := e.client.Call(e.sessionID, "Runtime.evaluate", map[string]interface{}{
		"expression": expr,
	})
	return err
}

func (e *Engine) doType(step Step) error {
	target := e.expandVars(step.Target)
	value := e.expandVars(step.Value)
	result, err := e.evaluateString(humaninput.FocusEditableExpression(target))
	if err != nil {
		return err
	}
	if result == "not_input" {
		return fmt.Errorf("type target is not editable: %s", target)
	}

	for _, ks := range humaninput.GenerateKeystrokes(value, step.WPM) {
		time.Sleep(ks.Delay)

		expr := humaninput.InsertTextIntoSelectorExpression(target, string(ks.Char))
		if ks.IsCorrection {
			expr = humaninput.BackspaceExpression()
		}

		result, err := e.evaluateString(expr)
		if err != nil {
			return err
		}
		if result == "not_input" {
			return fmt.Errorf("active element is not editable while typing: %s", target)
		}
	}
	return nil
}

func (e *Engine) doWait(step Step) error {
	// If value looks like a duration, sleep.
	if step.Value != "" {
		d, err := time.ParseDuration(step.Value)
		if err == nil {
			if d < 0 {
				return fmt.Errorf("wait duration must be non-negative")
			}
			if d > maxScriptWaitDuration {
				return fmt.Errorf("wait duration %s exceeds maximum %s", d, maxScriptWaitDuration)
			}
			time.Sleep(d)
			return nil
		}
	}
	// Otherwise wait for element by selector.
	target := e.expandVars(step.Target)
	expr := fmt.Sprintf(`(() => {
		const el = document.querySelector(%q);
		if (!el) throw new Error("element not found: %s");
		return "found";
	})()`, target, target)
	_, err := e.client.Call(e.sessionID, "Runtime.evaluate", map[string]interface{}{
		"expression": expr,
	})
	return err
}

func (e *Engine) doExtract(step Step) error {
	target := e.expandVars(step.Target)
	expr := fmt.Sprintf(`(() => {
		const el = document.querySelector(%q);
		if (!el) throw new Error("element not found: %s");
		return 'value' in el ? el.value : el.textContent;
	})()`, target, target)
	result, err := e.evaluateRaw(expr)
	if err != nil {
		return err
	}
	// Parse the result to extract the value string.
	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result, &evalResult); err != nil {
		return fmt.Errorf("parse extract result: %w", err)
	}
	if step.Store != "" {
		e.vars[step.Store] = evalResult.Result.Value
	}
	return nil
}

func (e *Engine) doScreenshot(step Step) error {
	_, err := e.client.Call(e.sessionID, "Page.screenshot", map[string]interface{}{})
	if err != nil {
		return err
	}
	// Store filename if requested (actual image data comes from Juggler response).
	if step.Store != "" {
		e.vars[step.Store] = step.Store
	}
	return nil
}

func (e *Engine) doSet(step Step) error {
	name := step.Target
	value := e.expandVars(step.Value)
	e.vars[name] = value
	return nil
}

func (e *Engine) evaluateString(expression string) (string, error) {
	result, err := e.evaluateRaw(expression)
	if err != nil {
		return "", err
	}

	var evalResult struct {
		Result struct {
			Value interface{} `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result, &evalResult); err != nil {
		return "", fmt.Errorf("parse evaluate result: %w", err)
	}
	if evalResult.Result.Value == nil {
		return "", nil
	}
	return fmt.Sprint(evalResult.Result.Value), nil
}

func (e *Engine) evaluateRaw(expression string) (json.RawMessage, error) {
	params := map[string]interface{}{
		"expression": expression,
	}
	e.mu.RLock()
	needsExecutionContext := e.frameID != ""
	e.mu.RUnlock()
	if needsExecutionContext {
		executionContextID := e.waitForExecutionContext(2 * time.Second)
		if executionContextID != "" {
			params["executionContextId"] = executionContextID
		}
	}
	return e.client.Call(e.sessionID, "Runtime.evaluate", params)
}

func (e *Engine) waitForExecutionContext(timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for {
		e.mu.RLock()
		id := e.executionContextID
		e.mu.RUnlock()
		if id != "" {
			return id
		}
		if time.Now().After(deadline) {
			return ""
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (e *Engine) doIf(step Step) error {
	// Conditional check: if vars[target] != value, return error to skip.
	actual := e.vars[step.Target]
	expected := e.expandVars(step.Value)
	if actual != expected {
		if sensitiveScriptToken(step.Target) {
			actual = redactedScriptValue
			expected = redactedScriptValue
		}
		return fmt.Errorf("condition failed: %s=%q, expected %q", step.Target, actual, expected)
	}
	return nil
}

func (e *Engine) stepOutput(step Step) string {
	switch step.Action {
	case "extract":
		if step.Store != "" {
			return e.vars[step.Store]
		}
	case "screenshot":
		if step.Store != "" {
			return e.vars[step.Store]
		}
	case "set":
		return e.vars[step.Target]
	case "if":
		return "condition passed"
	}
	return "ok"
}

func (e *Engine) safeStepOutput(step Step) string {
	if scriptStepSensitive(step) {
		switch step.Action {
		case "extract", "set":
			return redactedScriptValue
		}
	}
	return limitScriptDisplayValue(e.stepOutput(step))
}

func safeScriptStepValue(step Step) string {
	if step.Value == "" {
		return ""
	}
	if scriptStepSensitive(step) || sensitiveScriptToken(step.Value) {
		return redactedScriptValue
	}
	return limitScriptDisplayValue(step.Value)
}

func safeScriptField(value string) string {
	if sensitiveScriptToken(value) {
		return redactedScriptValue
	}
	return limitScriptDisplayValue(value)
}

func safeScriptStepError(step Step, err error) error {
	if err == nil {
		return nil
	}
	if scriptStepSensitive(step) || sensitiveScriptToken(err.Error()) {
		return fmt.Errorf("%s failed with redacted sensitive details", step.Action)
	}
	return fmt.Errorf("%s", limitScriptDisplayValue(err.Error()))
}

func scriptStepSensitive(step Step) bool {
	return sensitiveScriptToken(step.Target) || sensitiveScriptToken(step.Store)
}

func sensitiveScriptToken(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	for _, marker := range []string{"api_key", "apikey", "token", "secret", "password", "credential", "authorization", "cookie", "session"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return normalized == "auth" || strings.HasSuffix(normalized, "_auth")
}

func limitScriptDisplayValue(value string) string {
	if len(value) <= maxScriptResultFieldBytes {
		return value
	}
	cut := maxScriptResultFieldBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	if cut == 0 {
		cut = maxScriptResultFieldBytes
	}
	return value[:cut] + truncatedScriptValue
}

// expandVars replaces ${varName} references in a string with variable values.
func (e *Engine) expandVars(s string) string {
	result := s
	for name, val := range e.vars {
		result = replaceAll(result, "${"+name+"}", val)
	}
	return result
}

// replaceAll is a simple string replacement (avoids importing strings for one call).
func replaceAll(s, old, new string) string {
	for {
		i := indexOf(s, old)
		if i < 0 {
			return s
		}
		s = s[:i] + new + s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
