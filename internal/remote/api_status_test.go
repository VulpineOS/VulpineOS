package remote

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"vulpineos/internal/config"
	"vulpineos/internal/kernel"
	"vulpineos/internal/vault"
)

type stubGateway struct{ running bool }

func (s stubGateway) Running() bool { return s.running }

func TestStatusGetIncludesBrowserRoute(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	api := &PanelAPI{
		Kernel: kernel.New(),
		Config: &config.Config{FoxbridgeCDPURL: "ws://127.0.0.1:9222"},
		FoxbridgeRunning: func() bool {
			return true
		},
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
	if got := result["openclaw_profile_configured"]; got != false {
		t.Fatalf("openclaw_profile_configured = %v, want false", got)
	}
}

func TestStatusGetReportsProfileConfiguredWithoutTreatingItAsLiveRoute(t *testing.T) {
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

	if got := result["browser_route"]; got != "direct" {
		t.Fatalf("browser_route = %v, want direct", got)
	}
	if got := result["browser_route_source"]; got != "kernel" {
		t.Fatalf("browser_route_source = %v, want kernel", got)
	}
	if got := result["openclaw_profile_configured"]; got != true {
		t.Fatalf("openclaw_profile_configured = %v, want true", got)
	}
}

func TestStatusGetIgnoresStoppedFoxbridgeRuntimeRoute(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	api := &PanelAPI{
		Kernel: kernel.New(),
		Config: &config.Config{FoxbridgeCDPURL: "ws://127.0.0.1:9222"},
		FoxbridgeRunning: func() bool {
			return false
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

	if got := result["browser_route"]; got != "direct" {
		t.Fatalf("browser_route = %v, want direct", got)
	}
	if got := result["browser_route_source"]; got != "kernel" {
		t.Fatalf("browser_route_source = %v, want kernel", got)
	}
}

func TestStatusGetWithoutKernelReportsDisabledRoute(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := os.MkdirAll(config.OpenClawProfileDir(), 0700); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	if err := os.WriteFile(config.OpenClawConfigPath(), []byte(`{"browser":{"enabled":true,"cdpUrl":"ws://127.0.0.1:9222"}}`), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	api := &PanelAPI{
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

	if got := result["browser_route"]; got != "disabled" {
		t.Fatalf("browser_route = %v, want disabled", got)
	}
	if got := result["browser_route_source"]; got != "server" {
		t.Fatalf("browser_route_source = %v, want server", got)
	}
	if got := result["browser_window"]; got != "n/a" {
		t.Fatalf("browser_window = %v, want n/a", got)
	}
	if got := result["kernel_running"]; got != false {
		t.Fatalf("kernel_running = %v, want false", got)
	}
	if got := result["gateway_running"]; got != false {
		t.Fatalf("gateway_running = %v, want false", got)
	}
}

func TestAgentRuntimeConfigClearsStoppedFoxbridgeURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := os.MkdirAll(config.OpenClawProfileDir(), 0700); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	if err := os.WriteFile(config.OpenClawConfigPath(), []byte(`{"browser":{"enabled":true,"headless":true,"cdpUrl":"ws://127.0.0.1:9222"}}`), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	api := &PanelAPI{
		Config: &config.Config{FoxbridgeCDPURL: "ws://127.0.0.1:9222"},
		FoxbridgeRunning: func() bool {
			return false
		},
	}
	path, cleanup, err := api.agentRuntimeConfig(&vault.Agent{ID: "agent-1", Metadata: "{}"})
	if err != nil {
		t.Fatalf("agentRuntimeConfig: %v", err)
	}
	defer cleanup()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	if strings.Contains(string(data), "cdpUrl") {
		t.Fatalf("runtime config kept stale cdpUrl: %s", data)
	}
}
