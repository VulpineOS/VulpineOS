package scripting

import (
	"encoding/json"
	"fmt"
	"time"

	"vulpineos/internal/juggler"
)

// Step is a single instruction in a script.
type Step struct {
	Action string `json:"action"` // navigate, click, type, wait, extract, screenshot, if, set
	Target string `json:"target"` // CSS selector or URL
	Value  string `json:"value"`  // text to type, variable name, etc.
	Store  string `json:"store"`  // variable name to store result
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
	client    *juggler.Client
	sessionID string
	vars      map[string]string
}

// NewEngine creates a scripting engine backed by the given Juggler client.
func NewEngine(client *juggler.Client) *Engine {
	return &Engine{
		client: client,
		vars:   make(map[string]string),
	}
}

// SetSession sets the Juggler session ID used for protocol calls.
func (e *Engine) SetSession(sessionID string) {
	e.sessionID = sessionID
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
			Target:     step.Target,
			Value:      step.Value,
			Store:      step.Store,
			Status:     "ok",
			Output:     e.stepOutput(step),
			DurationMS: time.Since(start).Milliseconds(),
		}
		if err != nil {
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
	_, err := e.client.Call(e.sessionID, "Page.navigate", map[string]interface{}{
		"url": target,
	})
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
	expr := fmt.Sprintf(`(() => {
		const el = document.querySelector(%q);
		if (!el) throw new Error("element not found: %s");
		el.focus();
		el.value = %q;
		el.dispatchEvent(new Event('input', {bubbles: true}));
		return "typed";
	})()`, target, target, value)
	_, err := e.client.Call(e.sessionID, "Runtime.evaluate", map[string]interface{}{
		"expression": expr,
	})
	return err
}

func (e *Engine) doWait(step Step) error {
	// If value looks like a duration, sleep.
	if step.Value != "" {
		d, err := time.ParseDuration(step.Value)
		if err == nil {
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
	expr := fmt.Sprintf(`document.querySelector(%q).textContent`, target)
	result, err := e.client.Call(e.sessionID, "Runtime.evaluate", map[string]interface{}{
		"expression": expr,
	})
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

func (e *Engine) doIf(step Step) error {
	// Conditional check: if vars[target] != value, return error to skip.
	actual := e.vars[step.Target]
	expected := e.expandVars(step.Value)
	if actual != expected {
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
