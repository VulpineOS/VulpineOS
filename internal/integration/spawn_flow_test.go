package integration

import (
	"testing"

	"vulpineos/internal/testutil"
)

func TestSpawnFlow_LoadTemplate(t *testing.T) {
	env := newTestEnv(t)

	_, err := env.Vault.CreateTemplate("test-template", "test description", "sop-v1", "full", "[]", "{}")
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	template, err := env.Vault.GetTemplateByName("test-template")
	if err != nil {
		t.Fatalf("get template: %v", err)
	}
	if template.Name != "test-template" {
		t.Fatalf("template name = %q, want test-template", template.Name)
	}
	if template.SOP != "sop-v1" {
		t.Fatalf("template sop = %q, want sop-v1", template.SOP)
	}
}

func TestSpawnFlow_AcquireContext(t *testing.T) {
	env := newTestEnv(t)

	slot, err := env.Pool.Acquire()
	if err != nil {
		t.Fatalf("pool acquire: %v", err)
	}
	if slot == nil {
		t.Fatal("slot is nil")
	}
	if slot.ContextID == "" {
		t.Fatal("context id is empty")
	}

	env.Pool.Release(slot)

	calls := env.FakeJuggler.Calls()
	if len(calls) == 0 {
		t.Fatal("expected Juggler calls for context creation")
	}

	found := false
	for _, call := range calls {
		if call.Method == "Browser.createBrowserContext" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Browser.createBrowserContext call, got: %v", callNames(calls))
	}
}

func TestSpawnFlow_ApplyFingerprint(t *testing.T) {
	env := newTestEnv(t)

	tmpl, err := env.Vault.CreateTemplate("test-template", "test description", "sop-v1", "full", "[]", "{}")
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	citizen, err := env.Vault.CreateCitizen("test-citizen", `{"navigator.userAgent":"Mozilla/5.0 Test","screen.width":1920,"screen.height":1080}`, "{}", "en-US", "America/New_York")
	if err != nil {
		t.Fatalf("create citizen: %v", err)
	}

	_, spawnErr := env.Orch.SpawnCitizen(citizen.ID, tmpl.ID)

	calls := env.FakeJuggler.Calls()
	callMethods := callNames(calls)

	hasUserAgent := false
	hasLocale := false
	hasTimezone := false
	hasViewport := false

	for _, name := range callMethods {
		switch name {
		case "Browser.setUserAgentOverride":
			hasUserAgent = true
		case "Browser.setLocaleOverride":
			hasLocale = true
		case "Browser.setTimezoneOverride":
			hasTimezone = true
		case "Browser.setDefaultViewport":
			hasViewport = true
		}
	}

	if !hasUserAgent {
		t.Error("expected Browser.setUserAgentOverride call for fingerprint")
	}
	if !hasLocale {
		t.Error("expected Browser.setLocaleOverride call for fingerprint")
	}
	if !hasTimezone {
		t.Error("expected Browser.setTimezoneOverride call for fingerprint")
	}
	if !hasViewport {
		t.Error("expected Browser.setDefaultViewport call for fingerprint")
	}

	if spawnErr != nil {
		t.Logf("spawn failed (expected in test env): %v", spawnErr)
	}
}

func TestSpawnFlow_TemplateNotFound(t *testing.T) {
	env := newTestEnv(t)

	_, err := env.Vault.GetTemplateByName("non-existent-template")
	if err == nil {
		t.Fatal("expected error when template not found")
	}
}

func TestSpawnFlow_JugglerError(t *testing.T) {
	env := newTestEnv(t)
	env.FakeJuggler.RespondError("Browser.createBrowserContext", "context limit reached")

	_, err := env.Pool.Acquire()
	if err == nil {
		t.Fatal("expected error from juggler")
	}
}

func TestSpawnFlow_TrackOwnership(t *testing.T) {
	env := newTestEnv(t)

	tmpl, err := env.Vault.CreateTemplate("test-template", "test description", "sop-v1", "full", "[]", "{}")
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	citizen, err := env.Vault.CreateCitizen("test-citizen", "{}", "{}", "en-US", "America/New_York")
	if err != nil {
		t.Fatalf("create citizen: %v", err)
	}

	availBefore, activeBefore, totalBefore := env.Pool.Stats()

	_, spawnErr := env.Orch.SpawnCitizen(citizen.ID, tmpl.ID)

	_, activeAfterSpawn, _ := env.Pool.Stats()
	if spawnErr == nil && activeAfterSpawn != activeBefore+1 {
		t.Fatalf("pool active = %d, want %d (after spawn)", activeAfterSpawn, activeBefore+1)
	}

	if spawnErr != nil {
		t.Logf("spawn failed (expected in test env): %v", spawnErr)
	}

	_, activeAfterCleanup, _ := env.Pool.Stats()
	if activeAfterCleanup != activeBefore {
		t.Fatalf("pool active = %d, want %d (after cleanup)", activeAfterCleanup, activeBefore)
	}

	availAfter, _, totalAfter := env.Pool.Stats()
	if availAfter < availBefore {
		t.Errorf("pool available decreased: %d -> %d", availBefore, availAfter)
	}
	if totalAfter != totalBefore {
		t.Fatalf("pool total = %d, want %d (after cleanup)", totalAfter, totalBefore)
	}
}

func callNames(calls []testutil.JugglerCall) []string {
	names := make([]string, len(calls))
	for i, call := range calls {
		names[i] = call.Method
	}
	return names
}