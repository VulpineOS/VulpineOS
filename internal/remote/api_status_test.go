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

func TestStatusGetDisablesRouteWhenKernelIsStopped(t *testing.T) {
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

	if got := result["browser_route"]; got != "disabled" {
		t.Fatalf("browser_route = %v, want disabled", got)
	}
	if got := result["browser_route_source"]; got != "server" {
		t.Fatalf("browser_route_source = %v, want server", got)
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

	if got := result["browser_route"]; got != "disabled" {
		t.Fatalf("browser_route = %v, want disabled", got)
	}
	if got := result["browser_route_source"]; got != "server" {
		t.Fatalf("browser_route_source = %v, want server", got)
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

	if got := result["browser_route"]; got != "disabled" {
		t.Fatalf("browser_route = %v, want disabled", got)
	}
	if got := result["browser_route_source"]; got != "server" {
		t.Fatalf("browser_route_source = %v, want server", got)
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

func TestStatusGetReportsVaultDegradedState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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

	if got := result["vault_available"]; got != false {
		t.Fatalf("vault_available = %v, want false", got)
	}
	if got := result["degraded"]; got != true {
		t.Fatalf("degraded = %v, want true", got)
	}
	reasons, ok := result["degraded_reasons"].([]interface{})
	if !ok || len(reasons) == 0 {
		t.Fatalf("degraded_reasons = %#v, want non-empty list", result["degraded_reasons"])
	}
	first, ok := reasons[0].(map[string]interface{})
	if !ok || first["component"] != "vault" {
		t.Fatalf("first degraded reason = %#v, want vault component", reasons[0])
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
