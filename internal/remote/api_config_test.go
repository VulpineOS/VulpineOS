package remote

import (
	"encoding/json"
	"os"
	"testing"

	"vulpineos/internal/config"
)

func TestConfigSetMarksSetupCompleteAndRegeneratesProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	api := &PanelAPI{
		Config: &config.Config{},
	}

	params := json.RawMessage(`{"provider":"zai","model":"zai/glm-4.7","apiKey":"zai-test-key","defaultBudgetMaxCostUsd":1.25,"defaultBudgetMaxTokens":4000}`)
	payload, err := api.HandleMessage("config.set", params)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal result: %v", err)
	}
	if result["status"] != "ok" {
		t.Fatalf("status = %q, want ok", result["status"])
	}
	if !api.Config.SetupComplete {
		t.Fatal("setupComplete = false, want true")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != "zai" || cfg.Model != "zai/glm-4.7" || cfg.APIKey != "zai-test-key" {
		t.Fatalf("unexpected saved config: %#v", cfg)
	}
	if cfg.DefaultBudgetMaxCostUSD != 1.25 || cfg.DefaultBudgetMaxTokens != 4000 {
		t.Fatalf("unexpected saved default budget: %#v", cfg)
	}

	if _, err := os.Stat(config.OpenClawConfigPath()); err != nil {
		t.Fatalf("expected generated openclaw.json: %v", err)
	}
}

func TestConfigProvidersReturnsRegistry(t *testing.T) {
	api := &PanelAPI{}

	payload, err := api.HandleMessage("config.providers", nil)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result struct {
		Providers []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			NeedsKey bool   `json:"needsKey"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal result: %v", err)
	}
	if len(result.Providers) == 0 {
		t.Fatal("providers = empty, want registry entries")
	}
	if result.Providers[0].ID == "" || result.Providers[0].Name == "" {
		t.Fatalf("unexpected first provider: %#v", result.Providers[0])
	}
}
