package sentinelcapture

import (
	"context"
	"testing"
	"time"

	"vulpineos/internal/extensions"
	"vulpineos/internal/extensions/extensionstest"
	"vulpineos/internal/juggler"
	"vulpineos/internal/monitor"
	"vulpineos/internal/proxy"
)

func TestRecordRuntimeSignal(t *testing.T) {
	original := extensions.Registry.Sentinel()
	t.Cleanup(func() { extensions.Registry.SetSentinel(original) })
	fake := &extensionstest.FakeSentinelProvider{
		AvailableFlag: true,
		VariantBundles: []extensions.SentinelVariantBundle{
			{ID: "control", Enabled: true},
		},
		TrustRecipes: []extensions.SentinelTrustRecipe{
			{ID: "baseline-warmup"},
		},
	}
	extensions.Registry.SetSentinel(fake)

	if err := RecordRuntimeSignal(context.Background(), "provider_ready", map[string]string{"mode": "private_scaffold"}); err != nil {
		t.Fatalf("RecordRuntimeSignal: %v", err)
	}

	events := fake.RecordedEvents()
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Kind != extensions.SentinelEventKindRuntimeSignal || events[0].Name != "provider_ready" {
		t.Fatalf("event = %+v", events[0])
	}
}

func TestRecordMonitorAlertMapsToEventAndOutcome(t *testing.T) {
	original := extensions.Registry.Sentinel()
	t.Cleanup(func() { extensions.Registry.SetSentinel(original) })
	fake := &extensionstest.FakeSentinelProvider{
		AvailableFlag: true,
		VariantBundles: []extensions.SentinelVariantBundle{
			{ID: "control", Enabled: true},
		},
		TrustRecipes: []extensions.SentinelTrustRecipe{
			{ID: "baseline-warmup"},
		},
	}
	extensions.Registry.SetSentinel(fake)

	alert := monitor.Alert{
		AgentID:   "agent-1",
		Type:      monitor.AlertCaptcha,
		Details:   "Captcha detected",
		Timestamp: time.Unix(1713830400, 0).UTC(),
	}
	if err := RecordMonitorAlert(context.Background(), alert); err != nil {
		t.Fatalf("RecordMonitorAlert: %v", err)
	}

	events := fake.RecordedEvents()
	if len(events) != 1 || events[0].Name != "monitor.captcha" {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Attributes["variant_bundle_id"] != "control" || events[0].Attributes["trust_recipe_id"] != "baseline-warmup" {
		t.Fatalf("event attrs = %+v", events[0].Attributes)
	}
	outcomes := fake.RecordedOutcomes()
	if len(outcomes) != 1 || outcomes[0].Outcome != extensions.SentinelOutcomeSoftChallenge {
		t.Fatalf("outcomes = %+v", outcomes)
	}
	if outcomes[0].Attributes["variant_bundle_id"] != "control" || outcomes[0].Attributes["trust_recipe_id"] != "baseline-warmup" {
		t.Fatalf("outcome attrs = %+v", outcomes[0].Attributes)
	}
}

func TestRecordProxyRotationScrubsCredentials(t *testing.T) {
	original := extensions.Registry.Sentinel()
	t.Cleanup(func() { extensions.Registry.SetSentinel(original) })
	fake := &extensionstest.FakeSentinelProvider{
		AvailableFlag: true,
		VariantBundles: []extensions.SentinelVariantBundle{
			{ID: "control", Enabled: true},
		},
		TrustRecipes: []extensions.SentinelTrustRecipe{
			{ID: "baseline-warmup"},
		},
	}
	extensions.Registry.SetSentinel(fake)

	err := RecordProxyRotation(context.Background(), proxy.RotationEvent{
		AgentID:       "agent-1",
		Reason:        "rate_limit",
		PreviousProxy: "http://user:pass@old.example:8080",
		NewProxy:      "http://user:pass@new.example:8080",
		Timestamp:     time.Unix(1713830400, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("RecordProxyRotation: %v", err)
	}

	events := fake.RecordedEvents()
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	if got := events[0].Attributes["previous_proxy"]; got != "old.example:8080" {
		t.Fatalf("previous_proxy = %q", got)
	}
	if got := events[0].Attributes["new_proxy"]; got != "new.example:8080" {
		t.Fatalf("new_proxy = %q", got)
	}
	if events[0].Attributes["variant_bundle_id"] != "control" || events[0].Attributes["trust_recipe_id"] != "baseline-warmup" {
		t.Fatalf("attrs = %+v", events[0].Attributes)
	}
}

func TestRecordBrowserProbeMapsScopeAndPayload(t *testing.T) {
	original := extensions.Registry.Sentinel()
	t.Cleanup(func() { extensions.Registry.SetSentinel(original) })
	fake := &extensionstest.FakeSentinelProvider{
		AvailableFlag: true,
		VariantBundles: []extensions.SentinelVariantBundle{
			{ID: "control", Enabled: true},
		},
		TrustRecipes: []extensions.SentinelTrustRecipe{
			{ID: "baseline-warmup"},
		},
	}
	extensions.Registry.SetSentinel(fake)

	err := RecordBrowserProbe(context.Background(), "session-1", juggler.BrowserProbe{
		FrameID:   "frame-1",
		URL:       "https://ticketmaster.example/product/123",
		ScriptURL: "https://cdn.ticketmaster.example/fp.js",
		ProbeType: "webgl_probe",
		API:       "getParameter",
		Detail:    "37445",
		Count:     3,
		Timestamp: float64(time.Unix(1713830400, 0).UnixMilli()),
	})
	if err != nil {
		t.Fatalf("RecordBrowserProbe: %v", err)
	}

	events := fake.RecordedEvents()
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	event := events[0]
	if event.Kind != extensions.SentinelEventKindBrowserProbe {
		t.Fatalf("event.Kind = %q", event.Kind)
	}
	if event.Name != "webgl_probe.getParameter" {
		t.Fatalf("event.Name = %q", event.Name)
	}
	if event.Scope.SessionID != "session-1" || event.Scope.Domain != "ticketmaster.example" {
		t.Fatalf("event.Scope = %+v", event.Scope)
	}
	if event.Scope.ScriptURL != "https://cdn.ticketmaster.example/fp.js" {
		t.Fatalf("event.Scope.ScriptURL = %q", event.Scope.ScriptURL)
	}
	if got := event.Attributes["frame_id"]; got != "frame-1" {
		t.Fatalf("frame_id = %q", got)
	}
	if got := event.Attributes["count"]; got != "3" {
		t.Fatalf("count = %q", got)
	}
	if event.Attributes["variant_bundle_id"] != "control" || event.Attributes["trust_recipe_id"] != "baseline-warmup" {
		t.Fatalf("experiment attrs = %+v", event.Attributes)
	}
	if len(event.Payload) == 0 {
		t.Fatalf("event.Payload is empty")
	}
}
