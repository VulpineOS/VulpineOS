package remote

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"vulpineos/internal/config"
	"vulpineos/internal/juggler"
	"vulpineos/internal/orchestrator"
)

type runtimePageTransport struct {
	mu      sync.Mutex
	recvCh  chan *juggler.Message
	closeCh chan struct{}
	closed  bool
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

	switch msg.Method {
	case "Browser.createBrowserContext":
		t.recvCh <- &juggler.Message{ID: msg.ID, Result: json.RawMessage(`{"browserContextId":"ctx-1"}`)}
	case "Browser.newPage":
		t.recvCh <- &juggler.Message{ID: msg.ID, Result: json.RawMessage(`{"targetId":"target-1"}`)}
		t.recvCh <- &juggler.Message{
			Method: "Browser.attachedToTarget",
			Params: json.RawMessage(`{"sessionId":"sess-1","targetInfo":{"targetId":"target-1","type":"page","browserContextId":"ctx-1","url":"about:blank"}}`),
		}
	case "Page.navigate":
		t.recvCh <- &juggler.Message{ID: msg.ID, Result: json.RawMessage(`{}`)}
	case "Runtime.evaluate":
		var params struct {
			Expression string `json:"expression"`
		}
		_ = json.Unmarshal(msg.Params, &params)
		value := `"ok"`
		if params.Expression == `document.querySelector("h1").textContent` {
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
