package sentinelcapture

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"time"

	"vulpineos/internal/extensions"
	"vulpineos/internal/juggler"
	"vulpineos/internal/monitor"
	"vulpineos/internal/proxy"
)

// RecordRuntimeSignal writes a runtime-signal event into Sentinel when
// a real provider is available.
func RecordRuntimeSignal(ctx context.Context, name string, attributes map[string]string) error {
	return recordEvent(ctx, extensions.SentinelEvent{
		Kind:       extensions.SentinelEventKindRuntimeSignal,
		Source:     "runtime",
		Name:       name,
		Attributes: cloneAttributes(attributes),
		Timestamp:  time.Now().UTC(),
	})
}

// RecordMonitorAlert maps a runtime alert to both a raw evidence event
// and a normalized outcome label.
func RecordMonitorAlert(ctx context.Context, alert monitor.Alert) error {
	attributes := map[string]string{
		"alert_type": string(alert.Type),
		"details":    alert.Details,
	}
	scope := extensions.SentinelScope{AgentID: alert.AgentID}
	eventErr := recordEvent(ctx, extensions.SentinelEvent{
		Kind:       extensions.SentinelEventKindChallengeSignal,
		Source:     "monitor",
		Name:       "monitor." + string(alert.Type),
		Scope:      scope,
		Attributes: attributes,
		Timestamp:  alert.Timestamp.UTC(),
	})
	outcome := extensions.SentinelOutcome{
		Outcome:    monitorAlertOutcome(alert.Type),
		Source:     "monitor",
		Scope:      scope,
		Attributes: cloneAttributes(attributes),
		Timestamp:  alert.Timestamp.UTC(),
	}
	if outcome.Outcome == extensions.SentinelOutcomeSoftChallenge {
		outcome.ChallengeVendor = "unknown"
	}
	outcomeErr := recordOutcome(ctx, outcome)
	if eventErr != nil {
		return eventErr
	}
	return outcomeErr
}

// RecordProxyRotation writes a transport-observation event for a
// successful proxy transition.
func RecordProxyRotation(ctx context.Context, event proxy.RotationEvent) error {
	attributes := map[string]string{
		"reason":         event.Reason,
		"previous_proxy": scrubProxyEndpoint(event.PreviousProxy),
		"new_proxy":      scrubProxyEndpoint(event.NewProxy),
	}
	return recordEvent(ctx, extensions.SentinelEvent{
		Kind:       extensions.SentinelEventKindTransportObservation,
		Source:     "proxy",
		Name:       "proxy.rotate",
		Scope:      extensions.SentinelScope{AgentID: event.AgentID},
		Attributes: attributes,
		Timestamp:  event.Timestamp.UTC(),
	})
}

// RecordBrowserProbe writes page-level browser probe evidence into
// Sentinel when a real provider is available.
func RecordBrowserProbe(ctx context.Context, sessionID string, probe juggler.BrowserProbe) error {
	attributes := map[string]string{
		"probe_type": probe.ProbeType,
		"api":        probe.API,
		"detail":     probe.Detail,
		"count":      jsonInt(probe.Count),
		"frame_id":   probe.FrameID,
	}
	payload, _ := json.Marshal(probe)
	return recordEvent(ctx, extensions.SentinelEvent{
		Kind:   extensions.SentinelEventKindBrowserProbe,
		Source: "juggler",
		Name:   probeName(probe),
		Scope: extensions.SentinelScope{
			SessionID: sessionID,
			Domain:    scrubDomain(probe.URL),
			URL:       probe.URL,
			ScriptURL: probe.ScriptURL,
		},
		Attributes: attributes,
		Payload:    payload,
		Timestamp:  probeTime(probe.Timestamp),
	})
}

func recordEvent(ctx context.Context, event extensions.SentinelEvent) error {
	provider := extensions.Registry.Sentinel()
	if provider == nil || !provider.Available() {
		return nil
	}
	return provider.RecordEvent(ctx, event)
}

func recordOutcome(ctx context.Context, outcome extensions.SentinelOutcome) error {
	provider := extensions.Registry.Sentinel()
	if provider == nil || !provider.Available() {
		return nil
	}
	return provider.RecordOutcome(ctx, outcome)
}

func monitorAlertOutcome(alertType monitor.AlertType) string {
	switch alertType {
	case monitor.AlertCaptcha:
		return extensions.SentinelOutcomeSoftChallenge
	case monitor.AlertIPBlock:
		return extensions.SentinelOutcomeBlock
	default:
		return extensions.SentinelOutcomeDegraded
	}
}

func cloneAttributes(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func scrubProxyEndpoint(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	return parsed.Host
}

func scrubDomain(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func probeName(probe juggler.BrowserProbe) string {
	if probe.API == "" {
		return probe.ProbeType
	}
	return probe.ProbeType + "." + probe.API
}

func probeTime(timestamp float64) time.Time {
	if timestamp <= 0 {
		return time.Now().UTC()
	}
	return time.UnixMilli(int64(timestamp)).UTC()
}

func jsonInt(v int) string {
	return strconv.Itoa(v)
}
