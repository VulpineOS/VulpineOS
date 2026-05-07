package remote

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"vulpineos/internal/config"
	"vulpineos/internal/juggler"
	"vulpineos/internal/orchestrator"
)

type runtimePageTransport struct {
	mu      sync.Mutex
	calls   []runtimeCall
	recvCh  chan *juggler.Message
	closeCh chan struct{}
	closed  bool
}

type runtimeCall struct {
	Method string
	Params json.RawMessage
}

func newRuntimePageTransport() *runtimePageTransport {
	return &runtimePageTransport{
		recvCh:  make(chan *juggler.Message, 32),
		closeCh: make(chan struct{}),
	}
}

func (t *runtimePageTransport) Send(msg *juggler.Message) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return fmt.Errorf("transport closed")
	}
	params := append(json.RawMessage(nil), msg.Params...)
	t.calls = append(t.calls, runtimeCall{Method: msg.Method, Params: params})

	switch msg.Method {
	case "Browser.createBrowserContext":
		t.recvCh <- &juggler.Message{ID: msg.ID, Result: json.RawMessage(`{"browserContextId":"ctx-1"}`)}
	case "Browser.newPage":
		t.recvCh <- &juggler.Message{ID: msg.ID, Result: json.RawMessage(`{"targetId":"target-1"}`)}
		t.recvCh <- &juggler.Message{
			Method: "Browser.attachedToTarget",
			Params: json.RawMessage(`{"sessionId":"sess-1","targetInfo":{"targetId":"target-1","type":"page","browserContextId":"ctx-1","url":"about:blank"}}`),
		}
		t.recvCh <- &juggler.Message{
			Method:    "Page.frameAttached",
			SessionID: "sess-1",
			Params:    json.RawMessage(`{"frameId":"frame-1","parentFrameId":""}`),
		}
		t.recvCh <- &juggler.Message{
			Method:    "Runtime.executionContextCreated",
			SessionID: "sess-1",
			Params:    json.RawMessage(`{"context":{"id":"ctx-eval-1","auxData":{"frameId":"frame-1"}}}`),
		}
	case "Page.navigate":
		t.recvCh <- &juggler.Message{ID: msg.ID, Result: json.RawMessage(`{}`)}
		t.recvCh <- &juggler.Message{
			Method:    "Runtime.executionContextCreated",
			SessionID: "sess-1",
			Params:    json.RawMessage(`{"context":{"id":"ctx-eval-1","auxData":{"frameId":"frame-1"}}}`),
		}
	case "Runtime.evaluate":
		var params struct {
			Expression string `json:"expression"`
		}
		_ = json.Unmarshal(msg.Params, &params)
		value := `"ok"`
		if strings.Contains(params.Expression, `document.querySelector("h1")`) {
			value = `"Welcome"`
		}
		t.recvCh <- &juggler.Message{ID: msg.ID, Result: json.RawMessage(`{"result":{"value":` + value + `}}`)}
	case "Page.screenshot":
		t.recvCh <- &juggler.Message{ID: msg.ID, Result: json.RawMessage(`{"data":"c2NyZWVuc2hvdA=="}`)}
	default:
		t.recvCh <- &juggler.Message{ID: msg.ID, Result: json.RawMessage(`{}`)}
	}
	return nil
}

func (t *runtimePageTransport) getCalls() []runtimeCall {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]runtimeCall, len(t.calls))
	copy(result, t.calls)
	return result
}

func (t *runtimePageTransport) Receive() (*juggler.Message, error) {
	select {
	case msg := <-t.recvCh:
		return msg, nil
	case <-t.closeCh:
		return nil, fmt.Errorf("transport closed")
	}
}

func (t *runtimePageTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.closed = true
		close(t.closeCh)
	}
	return nil
}

func TestScriptsRunExecutesScriptAgainstRealSession(t *testing.T) {
	transport := newRuntimePageTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	api := &PanelAPI{
		Client:   client,
		Contexts: NewContextRegistry(),
		Config:   &config.Config{},
	}

	params := json.RawMessage(`{"script":"{\"steps\":[{\"action\":\"navigate\",\"target\":\"https://example.com\"},{\"action\":\"extract\",\"target\":\"h1\",\"store\":\"heading\"},{\"action\":\"screenshot\",\"store\":\"capture.png\"}]}"}`)
	payload, err := api.HandleMessage("scripts.run", params)
	if err != nil {
		t.Fatalf("HandleMessage scripts.run: %v", err)
	}

	var result struct {
		OK        bool                     `json:"ok"`
		ContextID string                   `json:"contextId"`
		SessionID string                   `json:"sessionId"`
		Results   []map[string]interface{} `json:"results"`
		Vars      map[string]string        `json:"vars"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal script result: %v", err)
	}
	if !result.OK || result.ContextID != "ctx-1" || result.SessionID != "sess-1" {
		t.Fatalf("unexpected script result header: %#v", result)
	}
	if len(result.Results) != 3 {
		t.Fatalf("results len = %d, want 3", len(result.Results))
	}
	if result.Vars["heading"] != "Welcome" || result.Vars["capture.png"] != "capture.png" {
		t.Fatalf("unexpected script vars: %#v", result.Vars)
	}
}

func TestScriptsRunTypeUsesHumanTypingThroughPanelAPI(t *testing.T) {
	transport := newRuntimePageTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	api := &PanelAPI{
		Client:   client,
		Contexts: NewContextRegistry(),
		Config:   &config.Config{},
	}

	params := json.RawMessage(`{"script":"{\"steps\":[{\"action\":\"type\",\"target\":\"#code\",\"value\":\"123\",\"wpm\":1000}]}"}`)
	payload, err := api.HandleMessage("scripts.run", params)
	if err != nil {
		t.Fatalf("HandleMessage scripts.run: %v", err)
	}

	var result struct {
		OK      bool                     `json:"ok"`
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal script result: %v", err)
	}
	if !result.OK || len(result.Results) != 1 || result.Results[0]["status"] != "ok" {
		t.Fatalf("unexpected script result: %#v", result)
	}

	var evalExpressions []string
	for _, call := range transport.getCalls() {
		if call.Method != "Runtime.evaluate" {
			continue
		}
		var params struct {
			Expression string `json:"expression"`
		}
		if err := json.Unmarshal(call.Params, &params); err != nil {
			t.Fatalf("unmarshal Runtime.evaluate params: %v", err)
		}
		evalExpressions = append(evalExpressions, params.Expression)
	}
	if len(evalExpressions) != 4 {
		t.Fatalf("expected focus + 3 human typing evaluations, got %d", len(evalExpressions))
	}
	if !strings.Contains(evalExpressions[0], "const selector = \"#code\"") {
		t.Fatalf("first evaluation should focus the target selector: %s", evalExpressions[0])
	}
	for i, expr := range evalExpressions[1:] {
		if !strings.Contains(expr, `document.querySelector("#code")`) {
			t.Fatalf("character evaluation %d should resolve the target selector: %s", i, expr)
		}
		if !strings.Contains(expr, "selectionStart") || !strings.Contains(expr, "dispatchEvent(new Event('input'") {
			t.Fatalf("character evaluation %d should dispatch editable input events: %s", i, expr)
		}
	}
}

func TestScriptsRunRejectsOversizedScript(t *testing.T) {
	transport := newRuntimePageTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	api := &PanelAPI{Client: client}
	params, err := json.Marshal(map[string]string{
		"script": strings.Repeat("x", maxPanelScriptBytes+1),
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	_, err = api.HandleMessage("scripts.run", params)
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("expected byte limit error, got %v", err)
	}
}

func TestScriptsRunRejectsUnsafeContextID(t *testing.T) {
	transport := newRuntimePageTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	api := &PanelAPI{Client: client}
	params := json.RawMessage(`{"contextId":"../ctx-secret","script":"{\"steps\":[{\"action\":\"wait\",\"ms\":1}]}"}`)
	_, err := api.HandleMessage("scripts.run", params)
	if err == nil || !strings.Contains(err.Error(), "invalid contextId") {
		t.Fatalf("error = %v, want invalid contextId", err)
	}
	if strings.Contains(err.Error(), "ctx-secret") {
		t.Fatalf("context error leaked input: %v", err)
	}
}

func TestContextsRemoveRejectsUnsafeBrowserContextID(t *testing.T) {
	transport := newRuntimePageTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	api := &PanelAPI{Client: client}
	_, err := api.HandleMessage("contexts.remove", json.RawMessage(`{"browserContextId":"../ctx-secret"}`))
	if err == nil || !strings.Contains(err.Error(), "invalid contextId") {
		t.Fatalf("error = %v, want invalid contextId", err)
	}
	if strings.Contains(err.Error(), "ctx-secret") {
		t.Fatalf("context remove error leaked input: %v", err)
	}
}

func TestScriptsRunRejectsTooManySteps(t *testing.T) {
	transport := newRuntimePageTransport()
	client := juggler.NewClient(transport)
	defer client.Close()

	steps := make([]map[string]string, maxPanelScriptSteps+1)
	for i := range steps {
		steps[i] = map[string]string{"action": "set", "target": "item", "value": "ok"}
	}
	script, err := json.Marshal(map[string]interface{}{"steps": steps})
	if err != nil {
		t.Fatalf("marshal script: %v", err)
	}
	params, err := json.Marshal(map[string]string{"script": string(script)})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	api := &PanelAPI{Client: client}
	_, err = api.HandleMessage("scripts.run", params)
	if err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("expected step limit error, got %v", err)
	}
}

func TestSecurityStatusReflectsRuntimeState(t *testing.T) {
	api := &PanelAPI{
		Config:       &config.Config{},
		Orchestrator: &orchestrator.Orchestrator{SecurityEnabled: true},
	}

	payload, err := api.HandleMessage("security.status", nil)
	if err != nil {
		t.Fatalf("HandleMessage security.status: %v", err)
	}

	var result struct {
		BrowserActive         bool                     `json:"browserActive"`
		SecurityEnabled       bool                     `json:"securityEnabled"`
		SignaturePatternCount int                      `json:"signaturePatternCount"`
		SandboxBlockedAPIs    []string                 `json:"sandboxBlockedAPIs"`
		Protections           []map[string]interface{} `json:"protections"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal security status: %v", err)
	}
	if result.BrowserActive {
		t.Fatal("browserActive = true, want false")
	}
	if !result.SecurityEnabled {
		t.Fatal("securityEnabled = false, want true")
	}
	if result.SignaturePatternCount == 0 || len(result.SandboxBlockedAPIs) == 0 {
		t.Fatalf("unexpected security metadata: %#v", result)
	}
	if len(result.Protections) != 7 {
		t.Fatalf("protections len = %d, want 7", len(result.Protections))
	}
}
