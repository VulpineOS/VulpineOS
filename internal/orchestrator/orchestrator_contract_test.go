package orchestrator

import (
	"encoding/json"
	"os"
	"testing"

	"vulpineos/internal/juggler"
	"vulpineos/internal/pool"
	"vulpineos/internal/testutil"
	"vulpineos/internal/vault"
)

func TestOrchestratorTracksContextOwnership(t *testing.T) {
	fake := testutil.NewFakeJugglerTransport(t)
	fake.RespondJSON("Browser.createBrowserContext", map[string]string{"browserContextId": "ctx-owned"})
	fake.RespondJSON("Browser.removeBrowserContext", map[string]any{})

	client := juggler.NewClient(fake)
	defer client.Close()

	vdb := openTestVault(t)
	defer vdb.Close()

	o := New(nil, client, vdb, pool.Config{PreWarm: 0, MaxActive: 1, MaxUsesPerSlot: 1}, "")
	o.Pool.Start()

	slot, err := o.Pool.Acquire()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	o.agentToSlotMu.Lock()
	o.agentToSlot["agent-x"] = slot
	o.agentToSlotMu.Unlock()

	o.agentToSlotMu.Lock()
	tracked := o.agentToSlot["agent-x"]
	o.agentToSlotMu.Unlock()

	if tracked == nil {
		t.Fatal("context ownership not tracked")
	}
	if tracked.ContextID != "ctx-owned" {
		t.Fatalf("tracked contextID = %q, want ctx-owned", tracked.ContextID)
	}

	o.agentToSlotMu.Lock()
	delete(o.agentToSlot, "agent-x")
	o.agentToSlotMu.Unlock()
	o.Pool.Release(slot)
}

func TestOrchestratorReleasesContextOnAgentKill(t *testing.T) {
	fake := testutil.NewFakeJugglerTransport(t)
	fake.RespondJSON("Browser.createBrowserContext", map[string]string{"browserContextId": "ctx-kill"})
	fake.RespondJSON("Browser.removeBrowserContext", map[string]any{})

	client := juggler.NewClient(fake)
	defer client.Close()

	vdb := openTestVault(t)
	defer vdb.Close()

	o := New(nil, client, vdb, pool.Config{PreWarm: 0, MaxActive: 1, MaxUsesPerSlot: 1}, "", Opts{})
	o.Pool.Start()

	slot, err := o.Pool.Acquire()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	o.agentToSlotMu.Lock()
	o.agentToSlot["agent-kill"] = slot
	o.agentToSlotMu.Unlock()

	o.agentToSlotMu.Lock()
	delete(o.agentToSlot, "agent-kill")
	o.agentToSlotMu.Unlock()
	o.Pool.Release(slot)

	calls := fake.CallsByMethod("Browser.removeBrowserContext")
	if len(calls) != 1 {
		t.Fatalf("remove calls = %d, want 1", len(calls))
	}

	removeParams := testutil.ParamsAs[struct {
		BrowserContextID string `json:"browserContextId"`
	}](t, calls[0].Params)
	if removeParams.BrowserContextID != "ctx-kill" {
		t.Fatalf("removed contextId = %q, want ctx-kill", removeParams.BrowserContextID)
	}
}

func TestOrchestratorCloseLeavesSharedClientUsable(t *testing.T) {
	fake := testutil.NewFakeJugglerTransport(t)
	fake.RespondJSON("Browser.createBrowserContext", map[string]string{"browserContextId": "ctx-close"})
	fake.RespondJSON("Browser.removeBrowserContext", map[string]any{})
	fake.RespondJSON("Browser.close", map[string]any{})

	client := juggler.NewClient(fake)
	defer client.Close()

	vdb := openTestVault(t)
	o := New(nil, client, vdb, pool.Config{PreWarm: 0, MaxActive: 1, MaxUsesPerSlot: 1}, "", Opts{})

	if _, err := o.Pool.Acquire(); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	o.Close()

	if _, err := client.Call("", "Browser.close", map[string]any{}); err != nil {
		t.Fatalf("shared client unusable after orchestrator close: %v", err)
	}
	if len(fake.CallsByMethod("Browser.removeBrowserContext")) != 1 {
		t.Fatalf("active pool context was not removed on close")
	}
}

func TestOrchestratorAppliesFingerprintViaJuggler(t *testing.T) {
	fake := testutil.NewFakeJugglerTransport(t)
	fake.RespondJSON("Browser.createBrowserContext", map[string]string{"browserContextId": "ctx-fp"})
	fake.RespondJSON("Browser.setCookies", map[string]any{})
	fake.RespondJSON("Browser.setUserAgentOverride", map[string]any{})
	fake.RespondJSON("Browser.setLocaleOverride", map[string]any{})
	fake.RespondJSON("Browser.setTimezoneOverride", map[string]any{})

	client := juggler.NewClient(fake)
	defer client.Close()

	vdb := openTestVault(t)
	defer vdb.Close()

	fp := vault.FingerprintData{
		Platform:     "mac",
		UserAgent:    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
		ScreenWidth:  1920,
		ScreenHeight: 1080,
		Language:     "en-US",
	}
	fpJSON, _ := json.Marshal(fp)

	cit := &vault.Citizen{
		ID:          "citizen-fp",
		Fingerprint: string(fpJSON),
		Locale:      "en-US",
		Timezone:    "America/New_York",
	}

	o := New(nil, client, vdb, pool.Config{PreWarm: 0, MaxActive: 1, MaxUsesPerSlot: 1}, "")

	if err := o.applyCitizenToContext("ctx-fp", cit); err != nil {
		t.Fatalf("applyCitizenToContext: %v", err)
	}

	uaCalls := fake.CallsByMethod("Browser.setUserAgentOverride")
	if len(uaCalls) != 1 {
		t.Fatalf("setUserAgentOverride calls = %d, want 1", len(uaCalls))
	}
	uaParams := testutil.ParamsAs[struct {
		BrowserContextID string `json:"browserContextId"`
		UserAgent        string `json:"userAgent"`
	}](t, uaCalls[0].Params)
	if uaParams.UserAgent != fp.UserAgent {
		t.Fatalf("userAgent = %q, want %q", uaParams.UserAgent, fp.UserAgent)
	}

	localeCalls := fake.CallsByMethod("Browser.setLocaleOverride")
	if len(localeCalls) != 1 {
		t.Fatalf("setLocaleOverride calls = %d, want 1", len(localeCalls))
	}
}

func openTestVault(t *testing.T) *vault.DB {
	f, err := os.CreateTemp("", "orchestrator-contract-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := vault.OpenPath(f.Name())
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
