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
	if f.handlers == nil {
		f.handlers = make(map[string]juggler.EventHandler)
	}
	f.handlers[event] = handler
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
