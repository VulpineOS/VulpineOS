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

	params := json.RawMessage(`{"provider":"zai","model":"zai/glm-4.7","apiKey":"zai-test-key"}`)
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

	if _, err := os.Stat(config.OpenClawConfigPath()); err != nil {
		t.Fatalf("expected generated openclaw.json: %v", err)
	}
}
