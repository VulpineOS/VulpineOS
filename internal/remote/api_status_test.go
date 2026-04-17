package remote

import (
	"encoding/json"
	"testing"

	"vulpineos/internal/config"
	"vulpineos/internal/kernel"
)

func TestStatusGetIncludesBrowserRoute(t *testing.T) {
	api := &PanelAPI{
		Kernel: kernel.New(),
		Config: &config.Config{FoxbridgeCDPURL: "ws://127.0.0.1:9222"},
	}

	payload, err := api.HandleMessage("status.get", nil)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got := result["browser_route"]; got != "camoufox" {
		t.Fatalf("browser_route = %v, want camoufox", got)
	}
	if got := result["kernel_headless"]; got != false {
		t.Fatalf("kernel_headless = %v, want false", got)
	}
}
