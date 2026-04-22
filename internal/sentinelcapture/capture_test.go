package sentinelcapture

import (
	"context"
	"testing"
	"time"

	"vulpineos/internal/extensions"
	"vulpineos/internal/extensions/extensionstest"
	"vulpineos/internal/monitor"
	"vulpineos/internal/proxy"
)

func TestRecordRuntimeSignal(t *testing.T) {
	original := extensions.Registry.Sentinel()
	t.Cleanup(func() { extensions.Registry.SetSentinel(original) })
	fake := &extensionstest.FakeSentinelProvider{AvailableFlag: true}
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
	fake := &extensionstest.FakeSentinelProvider{AvailableFlag: true}
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
	outcomes := fake.RecordedOutcomes()
	if len(outcomes) != 1 || outcomes[0].Outcome != extensions.SentinelOutcomeSoftChallenge {
		t.Fatalf("outcomes = %+v", outcomes)
	}
}

func TestRecordProxyRotationScrubsCredentials(t *testing.T) {
	original := extensions.Registry.Sentinel()
	t.Cleanup(func() { extensions.Registry.SetSentinel(original) })
	fake := &extensionstest.FakeSentinelProvider{AvailableFlag: true}
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
}
