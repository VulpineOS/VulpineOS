package foxbridge

import (
	"encoding/json"
	"testing"

	"vulpineos/internal/juggler"
)

type fakeJugglerBackend struct {
	lastSession string
	lastMethod  string
	lastParams  json.RawMessage
	handlers    map[string]juggler.EventHandler
}

func (f *fakeJugglerBackend) Call(sessionID, method string, params interface{}) (json.RawMessage, error) {
	f.lastSession = sessionID
	f.lastMethod = method
	switch raw := params.(type) {
	case json.RawMessage:
		f.lastParams = append(json.RawMessage(nil), raw...)
	default:
		data, _ := json.Marshal(params)
		f.lastParams = data
	}
	return json.RawMessage(`{}`), nil
}

func (f *fakeJugglerBackend) Subscribe(event string, handler juggler.EventHandler) {
	f.SubscribeWithCancel(event, handler)
}

func (f *fakeJugglerBackend) SubscribeWithCancel(event string, handler juggler.EventHandler) func() {
	if f.handlers == nil {
		f.handlers = make(map[string]juggler.EventHandler)
	}
	f.handlers[event] = handler
	return func() {
		delete(f.handlers, event)
	}
}

func TestScopedBackendCreateBrowserContext(t *testing.T) {
	be := newScopedBackend(&fakeJugglerBackend{}, "ctx-42")
	result, err := be.Call("", "Browser.createBrowserContext", nil)
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}

	var payload struct {
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if payload.BrowserContextID != "ctx-42" {
		t.Fatalf("browserContextId = %q, want ctx-42", payload.BrowserContextID)
	}
}

func TestScopedBackendInjectsContextOnNewPage(t *testing.T) {
	client := &fakeJugglerBackend{}
	be := newScopedBackend(client, "ctx-7")

	if _, err := be.Call("", "Browser.newPage", json.RawMessage(`{"foo":"bar"}`)); err != nil {
		t.Fatalf("Call returned error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(client.lastParams, &payload); err != nil {
		t.Fatalf("unmarshal forwarded params: %v", err)
	}
	if payload["browserContextId"] != "ctx-7" {
		t.Fatalf("browserContextId = %v, want ctx-7", payload["browserContextId"])
	}
}

func TestScopedBackendInjectsContextOnContextScopedBrowserMethods(t *testing.T) {
	client := &fakeJugglerBackend{}
	be := newScopedBackend(client, "ctx-7")

	if _, err := be.Call("", "Browser.setCookies", json.RawMessage(`{"cookies":[]}`)); err != nil {
		t.Fatalf("Call returned error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(client.lastParams, &payload); err != nil {
		t.Fatalf("unmarshal forwarded params: %v", err)
	}
	if client.lastMethod != "Browser.setCookies" {
		t.Fatalf("lastMethod = %q, want Browser.setCookies", client.lastMethod)
	}
	if payload["browserContextId"] != "ctx-7" {
		t.Fatalf("browserContextId = %v, want ctx-7", payload["browserContextId"])
	}
}

func TestScopedBackendRejectsOtherBrowserContext(t *testing.T) {
	client := &fakeJugglerBackend{}
	be := newScopedBackend(client, "ctx-7")

	if _, err := be.Call("", "Browser.setCookies", json.RawMessage(`{"browserContextId":"ctx-other","cookies":[]}`)); err == nil {
		t.Fatal("expected context mismatch error")
	}
	if client.lastMethod != "" {
		t.Fatalf("forwarded forbidden call to %s", client.lastMethod)
	}
}

func TestScopedBackendBlocksBrowserClose(t *testing.T) {
	client := &fakeJugglerBackend{}
	be := newScopedBackend(client, "ctx-7")

	if _, err := be.Call("", "Browser.close", nil); err == nil {
		t.Fatal("expected Browser.close to be blocked")
	}
	if client.lastMethod != "" {
		t.Fatalf("forwarded forbidden call to %s", client.lastMethod)
	}
}

func TestScopedBackendAllowsSafeGlobalBrowserMethods(t *testing.T) {
	client := &fakeJugglerBackend{}
	be := newScopedBackend(client, "ctx-7")

	if _, err := be.Call("", "Browser.getInfo", nil); err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if client.lastMethod != "Browser.getInfo" {
		t.Fatalf("lastMethod = %q, want Browser.getInfo", client.lastMethod)
	}
}

func TestScopedBackendBlocksUnknownBrowserMethods(t *testing.T) {
	client := &fakeJugglerBackend{}
	be := newScopedBackend(client, "ctx-7")

	if _, err := be.Call("", "Browser.setGlobalDanger", nil); err == nil {
		t.Fatal("expected unknown Browser method to be blocked")
	}
	if client.lastMethod != "" {
		t.Fatalf("forwarded forbidden call to %s", client.lastMethod)
	}
}

func TestScopedBackendFiltersEvents(t *testing.T) {
	client := &fakeJugglerBackend{}
	be := newScopedBackend(client, "ctx-1")

	var attachedCount, pageCount int
	be.Subscribe("Browser.attachedToTarget", func(sessionID string, params json.RawMessage) {
		attachedCount++
	})
	be.Subscribe("Page.navigationCommitted", func(sessionID string, params json.RawMessage) {
		pageCount++
	})

	client.handlers["Browser.attachedToTarget"]("", json.RawMessage(`{
		"sessionId":"page-1",
		"targetInfo":{"targetId":"target-1","browserContextId":"ctx-1"}
	}`))
	client.handlers["Page.navigationCommitted"]("page-1", json.RawMessage(`{"url":"https://example.com"}`))
	client.handlers["Browser.attachedToTarget"]("", json.RawMessage(`{
		"sessionId":"page-2",
		"targetInfo":{"targetId":"target-2","browserContextId":"ctx-other"}
	}`))
	client.handlers["Page.navigationCommitted"]("page-2", json.RawMessage(`{"url":"https://example.com/other"}`))

	if attachedCount != 1 {
		t.Fatalf("attachedCount = %d, want 1", attachedCount)
	}
	if pageCount != 1 {
		t.Fatalf("pageCount = %d, want 1", pageCount)
	}
}

func TestScopedBackendSuppressesDuplicateAttachedTargets(t *testing.T) {
	client := &fakeJugglerBackend{}
	be := newScopedBackend(client, "ctx-1")

	var attachedCount int
	be.Subscribe("Browser.attachedToTarget", func(sessionID string, params json.RawMessage) {
		attachedCount++
	})

	client.handlers["Browser.attachedToTarget"]("", json.RawMessage(`{
		"sessionId":"page-1",
		"targetInfo":{"targetId":"target-1","browserContextId":"ctx-1"}
	}`))
	client.handlers["Browser.attachedToTarget"]("", json.RawMessage(`{
		"sessionId":"page-1",
		"targetInfo":{"targetId":"target-1","browserContextId":"ctx-1"}
	}`))
	client.handlers["Browser.attachedToTarget"]("", json.RawMessage(`{
		"sessionId":"page-2",
		"targetInfo":{"targetId":"target-1","browserContextId":"ctx-1"}
	}`))

	if attachedCount != 1 {
		t.Fatalf("attachedCount = %d, want 1", attachedCount)
	}
}

func TestScopedBackendAllowsAttachAfterDetach(t *testing.T) {
	client := &fakeJugglerBackend{}
	be := newScopedBackend(client, "ctx-1")

	var attachedCount, detachedCount int
	be.Subscribe("Browser.attachedToTarget", func(sessionID string, params json.RawMessage) {
		attachedCount++
	})
	be.Subscribe("Browser.detachedFromTarget", func(sessionID string, params json.RawMessage) {
		detachedCount++
	})

	client.handlers["Browser.attachedToTarget"]("", json.RawMessage(`{
		"sessionId":"page-1",
		"targetInfo":{"targetId":"target-1","browserContextId":"ctx-1"}
	}`))
	client.handlers["Browser.detachedFromTarget"]("", json.RawMessage(`{
		"sessionId":"page-1",
		"targetId":"target-1"
	}`))
	client.handlers["Browser.attachedToTarget"]("", json.RawMessage(`{
		"sessionId":"page-1",
		"targetInfo":{"targetId":"target-1","browserContextId":"ctx-1"}
	}`))

	if attachedCount != 2 {
		t.Fatalf("attachedCount = %d, want 2", attachedCount)
	}
	if detachedCount != 1 {
		t.Fatalf("detachedCount = %d, want 1", detachedCount)
	}
}

func TestScopedBackendCloseCancelsSubscriptions(t *testing.T) {
	client := &fakeJugglerBackend{}
	be := newScopedBackend(client, "ctx-1")

	be.Subscribe("Browser.attachedToTarget", func(sessionID string, params json.RawMessage) {})
	if _, ok := client.handlers["Browser.attachedToTarget"]; !ok {
		t.Fatal("expected test subscription to be installed")
	}

	if err := be.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if _, ok := client.handlers["Browser.attachedToTarget"]; ok {
		t.Fatal("subscription still installed after Close")
	}
}
