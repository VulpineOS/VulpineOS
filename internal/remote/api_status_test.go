package remote

import (
	"encoding/json"
	"os"
	"testing"

	"vulpineos/internal/config"
	"vulpineos/internal/extensions"
	"vulpineos/internal/extensions/extensionstest"
	"vulpineos/internal/kernel"
)

type stubGateway struct{ running bool }

func (s stubGateway) Running() bool { return s.running }

func TestStatusGetIncludesBrowserRoute(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
	if got := result["openclaw_profile_configured"]; got != false {
		t.Fatalf("openclaw_profile_configured = %v, want false", got)
	}
	if got := result["sentinel_available"]; got != false {
		t.Fatalf("sentinel_available = %v, want false", got)
	}
	if got := result["sentinel_mode"]; got != extensions.SentinelModePublicNoop {
		t.Fatalf("sentinel_mode = %v, want %s", got, extensions.SentinelModePublicNoop)
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
	if got := result["openclaw_profile_configured"]; got != true {
		t.Fatalf("openclaw_profile_configured = %v, want true", got)
	}
}

func TestStatusGetWithoutKernelReportsDisabledRoute(t *testing.T) {
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

func TestStatusGetIncludesSentinelStatus(t *testing.T) {
	original := extensions.Registry.Sentinel()
	t.Cleanup(func() { extensions.Registry.SetSentinel(original) })
	fake := &extensionstest.FakeSentinelProvider{
		AvailableFlag: true,
		StatusValue: extensions.SentinelStatus{
			Provider:       "sentinel-private",
			Mode:           "private_scaffold",
			EventSink:      "memory",
			OutcomeSink:    "memory",
			VariantSource:  "memory",
			VariantBundles: 2,
			TrustRecipes:   1,
		},
	}
	extensions.Registry.SetSentinel(fake)

	api := &PanelAPI{Config: &config.Config{}}
	payload, err := api.HandleMessage("status.get", nil)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got := result["sentinel_available"]; got != true {
		t.Fatalf("sentinel_available = %v, want true", got)
	}
	if got := result["sentinel_provider"]; got != "sentinel-private" {
		t.Fatalf("sentinel_provider = %v, want sentinel-private", got)
	}
	if got := result["sentinel_mode"]; got != "private_scaffold" {
		t.Fatalf("sentinel_mode = %v, want private_scaffold", got)
	}
	if got := result["sentinel_variant_bundles"]; got != float64(2) {
		t.Fatalf("sentinel_variant_bundles = %v, want 2", got)
	}
	if got := result["sentinel_trust_recipes"]; got != float64(1) {
		t.Fatalf("sentinel_trust_recipes = %v, want 1", got)
	}
}

func TestSentinelGetReturnsVariantsAndTrustRecipes(t *testing.T) {
	original := extensions.Registry.Sentinel()
	t.Cleanup(func() { extensions.Registry.SetSentinel(original) })
	fake := &extensionstest.FakeSentinelProvider{
		AvailableFlag: true,
		StatusValue: extensions.SentinelStatus{
			Provider:       "sentinel-private",
			Mode:           "private_scaffold",
			VariantBundles: 1,
			TrustRecipes:   1,
		},
		VariantBundles: []extensions.SentinelVariantBundle{
			{ID: "control", Name: "Control", Enabled: true, Weight: 100},
		},
		TrustRecipes: []extensions.SentinelTrustRecipe{
			{ID: "baseline-warmup", Name: "Baseline warmup", WarmupStrategy: "generic_revisit"},
		},
	}
	extensions.Registry.SetSentinel(fake)

	api := &PanelAPI{}
	payload, err := api.HandleMessage("sentinel.get", nil)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result struct {
		Available      bool                               `json:"available"`
		VariantBundles []extensions.SentinelVariantBundle `json:"variantBundles"`
		TrustRecipes   []extensions.SentinelTrustRecipe   `json:"trustRecipes"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !result.Available {
		t.Fatal("expected sentinel to be available")
	}
	if len(result.VariantBundles) != 1 || result.VariantBundles[0].ID != "control" {
		t.Fatalf("variantBundles = %+v", result.VariantBundles)
	}
	if len(result.TrustRecipes) != 1 || result.TrustRecipes[0].ID != "baseline-warmup" {
		t.Fatalf("trustRecipes = %+v", result.TrustRecipes)
	}
}
