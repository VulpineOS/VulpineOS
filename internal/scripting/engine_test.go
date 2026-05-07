package scripting

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"vulpineos/internal/juggler"
)

// mockTransport records calls and returns canned responses.
type mockTransport struct {
	mu             sync.Mutex
	calls          []mockCall
	nextID         int
	sendCh         chan *juggler.Message
	recvCh         chan *juggler.Message
	closed         bool
	closeCh        chan struct{}
	errorForMethod string
	errorMessage   string
}

type mockCall struct {
	Method string
	Params json.RawMessage
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		sendCh:  make(chan *juggler.Message, 100),
		recvCh:  make(chan *juggler.Message, 100),
		closeCh: make(chan struct{}),
	}
}

func (m *mockTransport) Send(msg *juggler.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("transport closed")
	}
	m.calls = append(m.calls, mockCall{Method: msg.Method, Params: msg.Params})
	if m.errorForMethod == msg.Method {
		m.recvCh <- &juggler.Message{
			ID:    msg.ID,
			Error: &juggler.Error{Message: m.errorMessage},
		}
		return nil
	}

	// Auto-respond with success.
	resp := &juggler.Message{
		ID:     msg.ID,
		Result: json.RawMessage(`{"result":{"value":"mock"}}`),
	}
	m.recvCh <- resp
	return nil
}

func (m *mockTransport) Receive() (*juggler.Message, error) {
	select {
	case msg := <-m.recvCh:
		return msg, nil
	case <-m.closeCh:
		return nil, fmt.Errorf("transport closed")
	}
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.closeCh)
	}
	return nil
}

func (m *mockTransport) getCalls() []mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]mockCall, len(m.calls))
	copy(result, m.calls)
	return result
}

func TestParseScript(t *testing.T) {
	data := []byte(`{"steps":[
		{"action":"navigate","target":"https://example.com"},
		{"action":"extract","target":"h1","store":"heading"},
		{"action":"screenshot","store":"page.png"}
	]}`)

	script, err := ParseScript(data)
	if err != nil {
		t.Fatalf("ParseScript error: %v", err)
	}
	if len(script.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(script.Steps))
	}
	if script.Steps[0].Action != "navigate" {
		t.Errorf("step 0: expected navigate, got %s", script.Steps[0].Action)
	}
	if script.Steps[1].Store != "heading" {
		t.Errorf("step 1: expected store=heading, got %s", script.Steps[1].Store)
	}
	if script.Steps[2].Action != "screenshot" {
		t.Errorf("step 2: expected screenshot, got %s", script.Steps[2].Action)
	}
}

func TestParseScriptInvalidJSON(t *testing.T) {
	_, err := ParseScript([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestVariableSetGet(t *testing.T) {
	transport := newMockTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)

	engine.SetVar("foo", "bar")
	if v := engine.GetVar("foo"); v != "bar" {
		t.Errorf("expected 'bar', got %q", v)
	}
	if v := engine.GetVar("missing"); v != "" {
		t.Errorf("expected empty string for missing var, got %q", v)
	}
}

func TestSetActionStoresVariable(t *testing.T) {
	transport := newMockTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)

	script := &Script{
		Steps: []Step{
			{Action: "set", Target: "greeting", Value: "hello"},
		},
	}

	if err := engine.Execute(script); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if v := engine.GetVar("greeting"); v != "hello" {
		t.Errorf("expected 'hello', got %q", v)
	}
}

func TestExecuteWithResultsRedactsSensitiveValues(t *testing.T) {
	transport := newMockTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)

	script := &Script{
		Steps: []Step{
			{Action: "set", Target: "api_token", Value: "secret-token"},
			{Action: "type", Target: "input[type=password]", Value: "${api_token}"},
			{Action: "set", Target: "search", Value: "public query"},
		},
	}

	results, err := engine.ExecuteWithResults(script)
	if err != nil {
		t.Fatalf("ExecuteWithResults error: %v", err)
	}
	encodedResults, _ := json.Marshal(results)
	if string(encodedResults) == "" {
		t.Fatal("expected encoded results")
	}
	if strings.Contains(string(encodedResults), "secret-token") || strings.Contains(string(encodedResults), "api_token") || strings.Contains(string(encodedResults), "input[type=password]") {
		t.Fatalf("sensitive script data leaked in results: %s", encodedResults)
	}
	if results[2].Value != "public query" || results[2].Output != "public query" {
		t.Fatalf("public script values should be preserved: %#v", results[2])
	}
	if engine.GetVar("api_token") != "secret-token" {
		t.Fatalf("execution vars should keep raw values for script expansion")
	}
	vars := engine.RedactedVars()
	if vars["api_token"] != redactedScriptValue {
		t.Fatalf("redacted vars leaked sensitive value: %#v", vars)
	}
	if vars["search"] != "public query" {
		t.Fatalf("public var should be preserved: %#v", vars)
	}
}

func TestExecuteWithResultsRedactsSensitiveErrors(t *testing.T) {
	transport := newMockTransport()
	transport.errorForMethod = "Runtime.evaluate"
	transport.errorMessage = `evaluation failed with "secret-token"`
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)
	script := &Script{
		Steps: []Step{
			{Action: "type", Target: "input[type=password]", Value: "secret-token"},
		},
	}

	results, err := engine.ExecuteWithResults(script)
	if err == nil {
		t.Fatal("expected sensitive step error")
	}
	encodedResults, _ := json.Marshal(results)
	if strings.Contains(err.Error(), "secret-token") || strings.Contains(string(encodedResults), "secret-token") {
		t.Fatalf("sensitive error leaked: err=%q results=%s", err.Error(), encodedResults)
	}
	if !strings.Contains(err.Error(), "redacted sensitive details") {
		t.Fatalf("error should explain redaction: %v", err)
	}
}

func TestExecuteWithResultsLimitsDisplayedValues(t *testing.T) {
	transport := newMockTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)
	longValue := strings.Repeat("a", maxScriptResultFieldBytes+32)
	script := &Script{
		Steps: []Step{
			{Action: "set", Target: "public_value", Value: longValue},
		},
	}

	results, err := engine.ExecuteWithResults(script)
	if err != nil {
		t.Fatalf("ExecuteWithResults error: %v", err)
	}
	if engine.GetVar("public_value") != longValue {
		t.Fatal("raw script var should remain available for later expansion")
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if !strings.Contains(results[0].Value, truncatedScriptValue) || !strings.Contains(results[0].Output, truncatedScriptValue) {
		t.Fatalf("display values were not truncated: %#v", results[0])
	}
	vars := engine.RedactedVars()
	if !strings.Contains(vars["public_value"], truncatedScriptValue) {
		t.Fatalf("redacted vars should truncate large public values: %#v", vars)
	}
}

func TestExecuteNavigate(t *testing.T) {
	transport := newMockTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)

	script := &Script{
		Steps: []Step{
			{Action: "navigate", Target: "https://example.com"},
		},
	}

	if err := engine.Execute(script); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	calls := transport.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Method != "Page.navigate" {
		t.Errorf("expected Page.navigate, got %s", calls[0].Method)
	}

	// Verify params contain the URL.
	var params map[string]interface{}
	if err := json.Unmarshal(calls[0].Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["url"] != "https://example.com" {
		t.Errorf("expected url=https://example.com, got %v", params["url"])
	}
}

func TestExecuteTypeUsesHumanTypingPath(t *testing.T) {
	transport := newMockTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)
	engine.SetSession("sess-1")

	script := &Script{
		Steps: []Step{
			{Action: "type", Target: "#code", Value: "123", WPM: 1000},
		},
	}

	if err := engine.Execute(script); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	calls := transport.getCalls()
	if len(calls) != 4 {
		t.Fatalf("expected focus + 3 character calls, got %d", len(calls))
	}
	for i, call := range calls {
		if call.Method != "Runtime.evaluate" {
			t.Fatalf("call %d method = %s, want Runtime.evaluate", i, call.Method)
		}
		var params map[string]string
		if err := json.Unmarshal(call.Params, &params); err != nil {
			t.Fatalf("unmarshal params for call %d: %v", i, err)
		}
		expr := params["expression"]
		if strings.Contains(expr, "Page.insertText") {
			t.Fatalf("script type should not use direct insertText path: %s", expr)
		}
		if i == 0 {
			if !strings.Contains(expr, "const selector = \"#code\"") || !strings.Contains(expr, "document.querySelector(selector)") {
				t.Fatalf("first call should focus the target selector: %s", expr)
			}
			continue
		}
		if !strings.Contains(expr, `document.querySelector("#code")`) {
			t.Fatalf("character call %d should resolve the target selector: %s", i, expr)
		}
		if !strings.Contains(expr, "selectionStart") || !strings.Contains(expr, "dispatchEvent(new Event('input'") {
			t.Fatalf("character call %d should emulate editable-field input events: %s", i, expr)
		}
	}
}

func TestExecuteEmptyScript(t *testing.T) {
	transport := newMockTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)

	script := &Script{Steps: []Step{}}
	if err := engine.Execute(script); err != nil {
		t.Fatalf("Execute empty script error: %v", err)
	}

	calls := transport.getCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for empty script, got %d", len(calls))
	}
}

func TestExecuteMultipleSteps(t *testing.T) {
	transport := newMockTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)

	script := &Script{
		Steps: []Step{
			{Action: "navigate", Target: "https://example.com"},
			{Action: "screenshot", Store: "page.png"},
		},
	}

	if err := engine.Execute(script); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	calls := transport.getCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Method != "Page.navigate" {
		t.Errorf("call 0: expected Page.navigate, got %s", calls[0].Method)
	}
	if calls[1].Method != "Page.screenshot" {
		t.Errorf("call 1: expected Page.screenshot, got %s", calls[1].Method)
	}

	// Screenshot should store variable.
	if v := engine.GetVar("page.png"); v != "page.png" {
		t.Errorf("expected screenshot store='page.png', got %q", v)
	}
}

func TestWaitRejectsLongOrNegativeDurations(t *testing.T) {
	transport := newMockTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)
	cases := []Step{
		{Action: "wait", Value: (maxScriptWaitDuration + time.Second).String()},
		{Action: "wait", Value: "-1s"},
	}
	for _, step := range cases {
		start := time.Now()
		err := engine.Execute(&Script{Steps: []Step{step}})
		if err == nil {
			t.Fatalf("expected wait duration error for %#v", step)
		}
		if time.Since(start) > time.Second {
			t.Fatalf("invalid wait duration blocked too long: %s", time.Since(start))
		}
	}
}

func TestExecuteUnknownAction(t *testing.T) {
	transport := newMockTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)

	script := &Script{
		Steps: []Step{
			{Action: "bogus"},
		},
	}

	err := engine.Execute(script)
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestIfCondition(t *testing.T) {
	transport := newMockTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)
	engine.SetVar("status", "ok")

	// Passing condition.
	script := &Script{
		Steps: []Step{
			{Action: "if", Target: "status", Value: "ok"},
		},
	}
	if err := engine.Execute(script); err != nil {
		t.Fatalf("if with matching value should pass: %v", err)
	}

	// Failing condition.
	script = &Script{
		Steps: []Step{
			{Action: "if", Target: "status", Value: "error"},
		},
	}
	if err := engine.Execute(script); err == nil {
		t.Error("if with non-matching value should fail")
	}
}

func TestVariableExpansion(t *testing.T) {
	transport := newMockTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	engine := NewEngine(client)

	script := &Script{
		Steps: []Step{
			{Action: "set", Target: "host", Value: "https://example.com"},
			{Action: "navigate", Target: "${host}/page"},
		},
	}

	if err := engine.Execute(script); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	calls := transport.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 juggler call, got %d", len(calls))
	}

	var params map[string]interface{}
	if err := json.Unmarshal(calls[0].Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["url"] != "https://example.com/page" {
		t.Errorf("expected expanded URL, got %v", params["url"])
	}
}
