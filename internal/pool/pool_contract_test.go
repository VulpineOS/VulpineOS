package pool

import (
	"encoding/json"
	"testing"

	"vulpineos/internal/juggler"
	"vulpineos/internal/testutil"
)

func TestPoolUsesBrowserContextJugglerContract(t *testing.T) {
	transport := testutil.NewFakeJugglerTransport(t)
	transport.RespondJSON("Browser.createBrowserContext", struct {
		BrowserContextID string `json:"browserContextId"`
	}{BrowserContextID: "ctx-123"})

	client := juggler.NewClient(transport)
	pool := New(client, Config{PreWarm: 0, MaxActive: 1})
	defer pool.Close()

	slot, err := pool.Acquire()
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	createCall, ok := transport.LastCall("Browser.createBrowserContext")
	if !ok {
		t.Fatal("expected Browser.createBrowserContext call")
	}
	if createCall.Method != "Browser.createBrowserContext" {
		t.Fatalf("Method = %q, want %q", createCall.Method, "Browser.createBrowserContext")
	}

	var params struct {
		RemoveOnDetach bool `json:"removeOnDetach"`
	}
	if err := json.Unmarshal(createCall.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.RemoveOnDetach {
		t.Fatal("expected removeOnDetach=true")
	}

	if slot.ContextID != "ctx-123" {
		t.Fatalf("ContextID = %q, want %q", slot.ContextID, "ctx-123")
	}

	pool.Release(slot)
	pool.Close()

	removeCall, ok := transport.LastCall("Browser.removeBrowserContext")
	if !ok {
		t.Fatal("expected Browser.removeBrowserContext call")
	}
	if removeCall.Method != "Browser.removeBrowserContext" {
		t.Fatalf("Method = %q, want %q", removeCall.Method, "Browser.removeBrowserContext")
	}

	var removeParams struct {
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(removeCall.Params, &removeParams); err != nil {
		t.Fatalf("unmarshal remove params: %v", err)
	}
	if removeParams.BrowserContextID != "ctx-123" {
		t.Fatalf("browserContextId = %q, want %q", removeParams.BrowserContextID, "ctx-123")
	}
}

func TestPoolRejectsMalformedContextResponse(t *testing.T) {
	transport := testutil.NewFakeJugglerTransport(t)
	transport.RespondJSON("Browser.createBrowserContext", struct {
		WrongField string `json:"wrongField"`
	}{WrongField: "no-browserContextId"})

	client := juggler.NewClient(transport)
	pool := New(client, Config{PreWarm: 0, MaxActive: 1})
	defer pool.Close()

	slot, err := pool.Acquire()
	if err != nil {
		t.Fatalf("Acquire failed unexpectedly: %v", err)
	}
	if slot.ContextID != "" {
		t.Fatalf("ContextID = %q, want empty string for missing browserContextId", slot.ContextID)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}