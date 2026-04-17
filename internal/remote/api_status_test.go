package remote

import (
	"encoding/json"
	"os"
	"testing"

	"vulpineos/internal/config"
	"vulpineos/internal/kernel"
)

type stubGateway struct{ running bool }

func (s stubGateway) Running() bool { return s.running }

func TestStatusGetIncludesBrowserRoute(t *testing.T) {
	api := &PanelAPI{
		Kernel: kernel.New(),
		Config: &config.Config{FoxbridgeCDPURL: "ws://127.0.0.1:9222"},
		Gateway: stubGateway{
			running: true,
		},
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
	if got := result["browser_route_source"]; got != "runtime" {
		t.Fatalf("browser_route_source = %v, want runtime", got)
	}
	if got := result["browser_window"]; got != "unavailable" {
		t.Fatalf("browser_window = %v, want unavailable", got)
	}
	if got := result["gateway_running"]; got != true {
		t.Fatalf("gateway_running = %v, want true", got)
	}
	if got := result["kernel_headless"]; got != false {
		t.Fatalf("kernel_headless = %v, want false", got)
	}
}

func TestStatusGetFallsBackToOpenClawProfileRoute(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := os.MkdirAll(config.OpenClawProfileDir(), 0700); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	if err := os.WriteFile(config.OpenClawConfigPath(), []byte(`{"browser":{"enabled":true,"cdpUrl":"ws://127.0.0.1:9222"}}`), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	api := &PanelAPI{
		Kernel: kernel.New(),
		Config: &config.Config{},
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
	if got := result["browser_route_source"]; got != "profile" {
		t.Fatalf("browser_route_source = %v, want profile", got)
	}
}
