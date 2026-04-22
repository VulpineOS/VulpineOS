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
			Provider:        "sentinel-private",
			Mode:            "private_scaffold",
			EventSink:       "memory",
			OutcomeSink:     "memory",
			VariantSource:   "memory",
			VariantBundles:  2,
			TrustRecipes:    1,
			MaturityMetrics: 3,
			AssignmentRules: 2,
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
	if got := result["sentinel_maturity_metrics"]; got != float64(3) {
		t.Fatalf("sentinel_maturity_metrics = %v, want 3", got)
	}
	if got := result["sentinel_assignment_rules"]; got != float64(2) {
		t.Fatalf("sentinel_assignment_rules = %v, want 2", got)
	}
}

func TestSentinelGetReturnsLabData(t *testing.T) {
	original := extensions.Registry.Sentinel()
	t.Cleanup(func() { extensions.Registry.SetSentinel(original) })
	fake := &extensionstest.FakeSentinelProvider{
		AvailableFlag: true,
		StatusValue: extensions.SentinelStatus{
			Provider:        "sentinel-private",
			Mode:            "private_scaffold",
			VariantBundles:  1,
			TrustRecipes:    1,
			MaturityMetrics: 1,
			AssignmentRules: 1,
		},
		VariantBundles: []extensions.SentinelVariantBundle{
			{ID: "control", Name: "Control", Enabled: true, Weight: 100},
		},
		TrustRecipes: []extensions.SentinelTrustRecipe{
			{ID: "baseline-warmup", Name: "Baseline warmup", WarmupStrategy: "generic_revisit"},
		},
		MaturityMetrics: []extensions.SentinelMaturityMetric{
			{ID: "session_age_seconds", Name: "Session age", Unit: "seconds"},
		},
		AssignmentRules: []extensions.SentinelAssignmentRule{
			{ID: "cold-holdout", Name: "Cold holdout", VariantBundleID: "control", TrustRecipeID: "baseline-warmup"},
		},
		OutcomeLabels: []extensions.SentinelOutcomeLabel{
			{ID: extensions.SentinelOutcomeSoftChallenge, Name: "Soft challenge", Category: "challenge", Severity: "medium"},
		},
		OutcomeSummary: []extensions.SentinelOutcomeSummary{
			{Outcome: extensions.SentinelOutcomeSoftChallenge, Count: 1, Vendors: []string{"cloudflare"}},
		},
		ProbeSummary: []extensions.SentinelProbeSummary{
			{Domain: "example.com", ScriptURL: "https://cdn.example.com/fp.js", ProbeType: "canvas_probe", API: "toDataURL", Count: 2},
		},
		PatchQueue: []extensions.SentinelPatchCandidate{
			{Domain: "example.com", ProbeType: "canvas_probe", API: "toDataURL", Priority: "high", Score: 10, Recommendation: "Review canvas surface coherence and pixel-read behavior."},
		},
		SessionTimelines: []extensions.SentinelSessionTimeline{
			{
				SessionID:    "session-1",
				AgentID:      "agent-1",
				Domain:       "example.com",
				EventCount:   1,
				OutcomeCount: 1,
				Items: []extensions.SentinelTimelineItem{
					{Type: "event", Kind: extensions.SentinelEventKindBrowserProbe, Name: "canvas.toDataURL"},
					{Type: "outcome", Outcome: extensions.SentinelOutcomeSoftChallenge},
				},
			},
		},
	}
	extensions.Registry.SetSentinel(fake)

	api := &PanelAPI{}
	payload, err := api.HandleMessage("sentinel.get", nil)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result struct {
		Available       bool                                `json:"available"`
		VariantBundles  []extensions.SentinelVariantBundle  `json:"variantBundles"`
		TrustRecipes    []extensions.SentinelTrustRecipe    `json:"trustRecipes"`
		MaturityMetrics []extensions.SentinelMaturityMetric `json:"maturityMetrics"`
		AssignmentRules []extensions.SentinelAssignmentRule `json:"assignmentRules"`
		OutcomeLabels   []extensions.SentinelOutcomeLabel   `json:"outcomeLabels"`
		OutcomeSummary  []extensions.SentinelOutcomeSummary `json:"outcomeSummary"`
		ProbeSummary    []extensions.SentinelProbeSummary   `json:"probeSummary"`
		PatchQueue      []extensions.SentinelPatchCandidate `json:"patchQueue"`
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
	if len(result.MaturityMetrics) != 1 || result.MaturityMetrics[0].ID != "session_age_seconds" {
		t.Fatalf("maturityMetrics = %+v", result.MaturityMetrics)
	}
	if len(result.AssignmentRules) != 1 || result.AssignmentRules[0].ID != "cold-holdout" {
		t.Fatalf("assignmentRules = %+v", result.AssignmentRules)
	}
	if len(result.OutcomeLabels) != 1 || result.OutcomeLabels[0].ID != extensions.SentinelOutcomeSoftChallenge {
		t.Fatalf("outcomeLabels = %+v", result.OutcomeLabels)
	}
	if len(result.OutcomeSummary) != 1 || result.OutcomeSummary[0].Outcome != extensions.SentinelOutcomeSoftChallenge {
		t.Fatalf("outcomeSummary = %+v", result.OutcomeSummary)
	}
	if len(result.ProbeSummary) != 1 || result.ProbeSummary[0].API != "toDataURL" {
		t.Fatalf("probeSummary = %+v", result.ProbeSummary)
	}
	if len(result.PatchQueue) != 1 || result.PatchQueue[0].Priority != "high" {
		t.Fatalf("patchQueue = %+v", result.PatchQueue)
	}
}

func TestSentinelTimelineReturnsSessions(t *testing.T) {
	original := extensions.Registry.Sentinel()
	t.Cleanup(func() { extensions.Registry.SetSentinel(original) })
	fake := &extensionstest.FakeSentinelProvider{
		AvailableFlag: true,
		SessionTimelines: []extensions.SentinelSessionTimeline{
			{
				SessionID:    "session-1",
				AgentID:      "agent-1",
				Domain:       "example.com",
				EventCount:   1,
				OutcomeCount: 1,
				Items: []extensions.SentinelTimelineItem{
					{Type: "event", Kind: extensions.SentinelEventKindBrowserProbe, Name: "canvas.toDataURL"},
					{Type: "outcome", Outcome: extensions.SentinelOutcomeSoftChallenge},
				},
			},
		},
	}
	extensions.Registry.SetSentinel(fake)

	api := &PanelAPI{}
	payload, err := api.HandleMessage("sentinel.timeline", json.RawMessage(`{"limit":2}`))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result struct {
		Sessions []extensions.SentinelSessionTimeline `json:"sessions"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(result.Sessions) != 1 || result.Sessions[0].SessionID != "session-1" {
		t.Fatalf("sessions = %+v", result.Sessions)
	}
	if len(result.Sessions[0].Items) != 2 {
		t.Fatalf("items = %+v", result.Sessions[0].Items)
	}
}
