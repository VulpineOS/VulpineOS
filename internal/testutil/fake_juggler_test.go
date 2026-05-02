package testutil

import (
	"encoding/json"
	"testing"
	"time"

	"vulpineos/internal/juggler"
)

func TestFakeJugglerTransportRecordsSessionMethodAndParams(t *testing.T) {
	transport := NewFakeJugglerTransport(t)
	client := juggler.NewClient(transport)
	defer client.Close()

	_, err := client.Call("session-1", "Browser.getInfo", map[string]any{
		"includeUserAgent": true,
	})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	call, ok := transport.LastCall("Browser.getInfo")
	if !ok {
		t.Fatal("expected Browser.getInfo call")
	}
	if call.ID == 0 {
		t.Fatal("expected non-zero call ID")
	}
	if call.SessionID != "session-1" {
		t.Fatalf("SessionID = %q, want %q", call.SessionID, "session-1")
	}
	if call.Method != "Browser.getInfo" {
		t.Fatalf("Method = %q, want %q", call.Method, "Browser.getInfo")
	}

	params := ParamsAs[struct {
		IncludeUserAgent bool `json:"includeUserAgent"`
	}](t, call.Params)
	if !params.IncludeUserAgent {
		t.Fatal("expected includeUserAgent param")
	}
}

func TestFakeJugglerTransportRespondJSONReturnsResultToClientCall(t *testing.T) {
	transport := NewFakeJugglerTransport(t)
	transport.RespondJSON("Browser.getVersion", struct {
		Product string `json:"product"`
	}{Product: "Firefox"})

	client := juggler.NewClient(transport)
	defer client.Close()

	result, err := client.Call("", "Browser.getVersion", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	version := ParamsAs[struct {
		Product string `json:"product"`
	}](t, result)
	if version.Product != "Firefox" {
		t.Fatalf("Product = %q, want %q", version.Product, "Firefox")
	}
}

func TestFakeJugglerTransportParamsAsDecodesTypedStructs(t *testing.T) {
	raw := json.RawMessage(`{"frameId":"frame-1","timeout":250}`)

	params := ParamsAs[struct {
		FrameID string `json:"frameId"`
		Timeout int    `json:"timeout"`
	}](t, raw)

	if params.FrameID != "frame-1" {
		t.Fatalf("FrameID = %q, want %q", params.FrameID, "frame-1")
	}
	if params.Timeout != 250 {
		t.Fatalf("Timeout = %d, want %d", params.Timeout, 250)
	}
}

func TestFakeJugglerTransportInjectEventDeliversToSubscribedClient(t *testing.T) {
	transport := NewFakeJugglerTransport(t)
	client := juggler.NewClient(transport)
	defer client.Close()

	type eventParams struct {
		URL string `json:"url"`
	}

	received := make(chan struct {
		sessionID string
		url       string
		err       error
	}, 1)
	client.Subscribe("Page.navigationStarted", func(sessionID string, params json.RawMessage) {
		var decoded eventParams
		err := json.Unmarshal(params, &decoded)
		received <- struct {
			sessionID string
			url       string
			err       error
		}{
			sessionID: sessionID,
			url:       decoded.URL,
			err:       err,
		}
	})

	transport.InjectEvent("session-2", "Page.navigationStarted", eventParams{URL: "https://example.test"})

	select {
	case event := <-received:
		if event.err != nil {
			t.Fatalf("unmarshal event params: %v", event.err)
		}
		if event.sessionID != "session-2" {
			t.Fatalf("sessionID = %q, want %q", event.sessionID, "session-2")
		}
		if event.url != "https://example.test" {
			t.Fatalf("URL = %q, want %q", event.url, "https://example.test")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}
