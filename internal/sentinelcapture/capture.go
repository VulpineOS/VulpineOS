package sentinelcapture

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"vulpineos/internal/extensions"
	"vulpineos/internal/juggler"
	"vulpineos/internal/monitor"
	"vulpineos/internal/proxy"
	"vulpineos/internal/vault"
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
	attributes := withExperimentDefaults(ctx, map[string]string{
		"alert_type": string(alert.Type),
		"details":    alert.Details,
	})
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
	return RecordProxyRotationWithScope(ctx, event, extensions.SentinelScope{AgentID: event.AgentID})
}

// RecordProxyRotationWithScope writes a transport-observation event for
// a successful proxy transition using any known agent/session/context
// scope gathered by the caller.
func RecordProxyRotationWithScope(ctx context.Context, event proxy.RotationEvent, scope extensions.SentinelScope) error {
	attributes := withExperimentDefaults(ctx, map[string]string{
		"reason":         event.Reason,
		"previous_proxy": scrubProxyEndpoint(event.PreviousProxy),
		"new_proxy":      scrubProxyEndpoint(event.NewProxy),
	})
	if scope.AgentID == "" {
		scope.AgentID = event.AgentID
	}
	return recordEvent(ctx, extensions.SentinelEvent{
		Kind:       extensions.SentinelEventKindTransportObservation,
		Source:     "proxy",
		Name:       "proxy.rotate",
		Scope:      scope,
		Attributes: attributes,
		Timestamp:  event.Timestamp.UTC(),
	})
}

// RecordBrowserProbe writes page-level browser probe evidence into
// Sentinel when a real provider is available.
func RecordBrowserProbe(ctx context.Context, sessionID string, probe juggler.BrowserProbe) error {
	attributes := withExperimentDefaults(ctx, map[string]string{
		"probe_type": probe.ProbeType,
		"api":        probe.API,
		"detail":     probe.Detail,
		"count":      jsonInt(probe.Count),
		"frame_id":   probe.FrameID,
	})
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

// RecordTrustActivity writes trust-warming lifecycle activity into
// Sentinel when a real provider is available.
func RecordTrustActivity(ctx context.Context, state juggler.TrustWarmingState) error {
	name := "trust_warming.update"
	if state.State != "" {
		name = "trust_warming." + strings.ToLower(state.State)
	}
	now := time.Now().UTC()
	attributes := withExperimentDefaults(ctx, map[string]string{
		"state":        state.State,
		"current_site": state.CurrentSite,
	})
	attributes = withPriorDomainEvidence(ctx, scrubDomain(state.CurrentSite), now, attributes)
	return recordEvent(ctx, extensions.SentinelEvent{
		Kind:   extensions.SentinelEventKindTrustActivity,
		Source: "juggler",
		Name:   name,
		Scope: extensions.SentinelScope{
			Domain: scrubDomain(state.CurrentSite),
			URL:    state.CurrentSite,
		},
		Attributes: attributes,
		Timestamp:  now,
	})
}

// RecordTrustAssets writes per-domain carry-forward state from the
// vault into Sentinel so trust recipes can be compared against the
// actual cookie/storage maturity each session starts with.
func RecordTrustAssets(ctx context.Context, scope extensions.SentinelScope, citizen *vault.Citizen, cookies []vault.CitizenCookies, storage []vault.CitizenStorage) error {
	if citizen == nil {
		return nil
	}
	type aggregate struct {
		cookieCount       int
		storageEntryCount int
		hasCookieState    bool
		hasStorageState   bool
		lastAssetUpdate   time.Time
	}
	assets := make(map[string]*aggregate)
	ensure := func(domain string) *aggregate {
		if domain == "" {
			return nil
		}
		if existing, ok := assets[domain]; ok {
			return existing
		}
		created := &aggregate{}
		assets[domain] = created
		return created
	}
	for _, cc := range cookies {
		domain := normalizeAssetDomain(cc.Domain)
		agg := ensure(domain)
		if agg == nil {
			continue
		}
		count := jsonArrayLen(cc.Cookies)
		agg.cookieCount += count
		if count > 0 {
			agg.hasCookieState = true
		}
		agg.lastAssetUpdate = maxTimestamp(agg.lastAssetUpdate, cc.UpdatedAt.UTC())
	}
	for _, cs := range storage {
		domain := scrubDomain(cs.Origin)
		agg := ensure(domain)
		if agg == nil {
			continue
		}
		count := jsonObjectLen(cs.Data)
		agg.storageEntryCount += count
		if count > 0 {
			agg.hasStorageState = true
		}
		agg.lastAssetUpdate = maxTimestamp(agg.lastAssetUpdate, cs.UpdatedAt.UTC())
	}
	if len(assets) == 0 {
		return nil
	}
	now := time.Now().UTC()
	lastSeen := citizen.LastUsedAt.UTC()
	if lastSeen.IsZero() {
		lastSeen = citizen.CreatedAt.UTC()
	}
	domains := make([]string, 0, len(assets))
	for domain := range assets {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	var firstErr error
	for _, domain := range domains {
		agg := assets[domain]
		eventScope := scope
		if eventScope.CitizenID == "" {
			eventScope.CitizenID = citizen.ID
		}
		eventScope.Domain = domain
		if scrubDomain(eventScope.URL) != domain {
			eventScope.URL = ""
		}
		attrs := withExperimentDefaults(ctx, map[string]string{
			"cookie_count":        jsonInt(agg.cookieCount),
			"storage_entry_count": jsonInt(agg.storageEntryCount),
			"has_cookie_state":    strconv.FormatBool(agg.hasCookieState),
			"has_storage_state":   strconv.FormatBool(agg.hasStorageState),
			"total_sessions_seen": jsonInt(citizen.TotalSessions),
		})
		if !lastSeen.IsZero() {
			attrs["hours_since_last_seen"] = formatHours(now.Sub(lastSeen).Hours())
		}
		if !agg.lastAssetUpdate.IsZero() {
			attrs["hours_since_last_asset_update"] = formatHours(now.Sub(agg.lastAssetUpdate).Hours())
		}
		if err := recordEvent(ctx, extensions.SentinelEvent{
			Kind:       extensions.SentinelEventKindTrustActivity,
			Source:     "vault",
			Name:       "trust_asset.snapshot",
			Scope:      eventScope,
			Attributes: attrs,
			Timestamp:  now,
		}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
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

func withExperimentDefaults(ctx context.Context, attributes map[string]string) map[string]string {
	out := cloneAttributes(attributes)
	provider := extensions.Registry.Sentinel()
	if provider == nil || !provider.Available() {
		return out
	}
	if out["variant_bundle_id"] != "" && out["trust_recipe_id"] != "" {
		return out
	}
	variantID, trustID := defaultExperimentAssignment(ctx, provider)
	if out["variant_bundle_id"] == "" && variantID != "" {
		out["variant_bundle_id"] = variantID
	}
	if out["trust_recipe_id"] == "" && trustID != "" {
		out["trust_recipe_id"] = trustID
	}
	return out
}

func withPriorDomainEvidence(ctx context.Context, domain string, now time.Time, attributes map[string]string) map[string]string {
	if domain == "" {
		return attributes
	}
	provider := extensions.Registry.Sentinel()
	if provider == nil || !provider.Available() {
		return attributes
	}
	timelines, err := provider.ListSessionTimelines(ctx, extensions.SentinelTimelineFilter{Domain: domain, Limit: 64})
	if err != nil {
		return attributes
	}
	out := cloneAttributes(attributes)
	sessionIDs := make(map[string]struct{})
	distinctDays := make(map[string]struct{})
	priorTrustEvents := 0
	var firstSeen time.Time
	var lastSeen time.Time
	for _, timeline := range timelines {
		if timeline.Domain != "" && timeline.Domain != domain {
			continue
		}
		if timeline.SessionID != "" {
			sessionIDs[timeline.SessionID] = struct{}{}
		}
		for _, item := range timeline.Items {
			ts := item.Timestamp.UTC()
			if ts.IsZero() {
				continue
			}
			distinctDays[ts.Format("2006-01-02")] = struct{}{}
			if firstSeen.IsZero() || ts.Before(firstSeen) {
				firstSeen = ts
			}
			if lastSeen.IsZero() || ts.After(lastSeen) {
				lastSeen = ts
			}
			if item.Type == "event" && item.Kind == extensions.SentinelEventKindTrustActivity {
				priorTrustEvents++
			}
		}
		if len(timeline.Items) == 0 && !timeline.LastActivityAt.IsZero() {
			ts := timeline.LastActivityAt.UTC()
			distinctDays[ts.Format("2006-01-02")] = struct{}{}
			if firstSeen.IsZero() || ts.Before(firstSeen) {
				firstSeen = ts
			}
			if lastSeen.IsZero() || ts.After(lastSeen) {
				lastSeen = ts
			}
		}
	}
	if len(sessionIDs) == 0 && priorTrustEvents == 0 && len(distinctDays) == 0 {
		return out
	}
	out["prior_session_count"] = jsonInt(len(sessionIDs))
	out["prior_trust_event_count"] = jsonInt(priorTrustEvents)
	out["distinct_days_seen"] = jsonInt(len(distinctDays))
	if !firstSeen.IsZero() {
		out["hours_since_first_seen"] = formatHours(now.Sub(firstSeen).Hours())
	}
	if !lastSeen.IsZero() {
		out["hours_since_last_seen"] = formatHours(now.Sub(lastSeen).Hours())
	}
	return out
}

func defaultExperimentAssignment(ctx context.Context, provider extensions.SentinelProvider) (string, string) {
	variantID := ""
	bundles, err := provider.ListVariantBundles(ctx)
	if err == nil {
		for _, bundle := range bundles {
			if bundle.Enabled {
				variantID = bundle.ID
				break
			}
		}
		if variantID == "" && len(bundles) > 0 {
			variantID = bundles[0].ID
		}
	}
	trustID := ""
	recipes, err := provider.ListTrustRecipes(ctx)
	if err == nil && len(recipes) > 0 {
		trustID = recipes[0].ID
	}
	return variantID, trustID
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

func normalizeAssetDomain(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		return scrubDomain(raw)
	}
	return strings.ToLower(strings.TrimPrefix(raw, "."))
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

func jsonArrayLen(raw string) int {
	if raw == "" {
		return 0
	}
	var rows []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return 0
	}
	return len(rows)
}

func jsonObjectLen(raw string) int {
	if raw == "" {
		return 0
	}
	var rows map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return 0
	}
	return len(rows)
}

func maxTimestamp(current, candidate time.Time) time.Time {
	if candidate.IsZero() {
		return current
	}
	if current.IsZero() || candidate.After(current) {
		return candidate
	}
	return current
}

func formatHours(hours float64) string {
	return strconv.FormatFloat(hours, 'f', 1, 64)
}
