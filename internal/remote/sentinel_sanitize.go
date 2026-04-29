package remote

import (
	"encoding/json"
	"strings"

	"vulpineos/internal/extensions"
	"vulpineos/internal/proxy"
)

func sanitizeSentinelStatus(status extensions.SentinelStatus) extensions.SentinelStatus {
	status.EventSink = redactSessionLogString(status.EventSink)
	status.OutcomeSink = redactSessionLogString(status.OutcomeSink)
	status.VariantSource = redactSessionLogString(status.VariantSource)
	return status
}

func sanitizeSentinelProbeSummaries(rows []extensions.SentinelProbeSummary) []extensions.SentinelProbeSummary {
	out := append([]extensions.SentinelProbeSummary(nil), rows...)
	for i := range out {
		out[i].ScriptURL = redactPanelURLSecrets(out[i].ScriptURL)
		out[i].LastURL = redactPanelURLSecrets(out[i].LastURL)
		out[i].Detail = redactSessionLogString(out[i].Detail)
	}
	return out
}

func sanitizeSentinelTransportEvidence(rows []extensions.SentinelTransportEvidenceSummary) []extensions.SentinelTransportEvidenceSummary {
	out := append([]extensions.SentinelTransportEvidenceSummary(nil), rows...)
	for i := range out {
		out[i].Reasons = redactSentinelStrings(out[i].Reasons)
		out[i].ProxyEndpoints = redactSentinelProxyEndpoints(out[i].ProxyEndpoints)
	}
	return out
}

func sanitizeSentinelPatchCandidates(rows []extensions.SentinelPatchCandidate) []extensions.SentinelPatchCandidate {
	out := append([]extensions.SentinelPatchCandidate(nil), rows...)
	for i := range out {
		out[i].ScriptURL = redactPanelURLSecrets(out[i].ScriptURL)
		out[i].Recommendation = redactSessionLogString(out[i].Recommendation)
	}
	return out
}

func sanitizeSentinelSiteIntelligence(rows []extensions.SentinelSiteIntelligenceSummary) []extensions.SentinelSiteIntelligenceSummary {
	out := append([]extensions.SentinelSiteIntelligenceSummary(nil), rows...)
	for i := range out {
		out[i].TopScriptURL = redactPanelURLSecrets(out[i].TopScriptURL)
	}
	return out
}

func sanitizeSentinelProbeSequences(rows []extensions.SentinelProbeSequenceSummary) []extensions.SentinelProbeSequenceSummary {
	out := append([]extensions.SentinelProbeSequenceSummary(nil), rows...)
	for i := range out {
		out[i].ScriptURL = redactPanelURLSecrets(out[i].ScriptURL)
		out[i].Sequence = redactSessionLogString(out[i].Sequence)
	}
	return out
}

func sanitizeSentinelTimelines(rows []extensions.SentinelSessionTimeline) []extensions.SentinelSessionTimeline {
	out := append([]extensions.SentinelSessionTimeline(nil), rows...)
	for i := range out {
		out[i].URL = redactPanelURLSecrets(out[i].URL)
		out[i].Items = sanitizeSentinelTimelineItems(out[i].Items)
	}
	return out
}

func sanitizeSentinelTimelineItems(items []extensions.SentinelTimelineItem) []extensions.SentinelTimelineItem {
	out := append([]extensions.SentinelTimelineItem(nil), items...)
	for i := range out {
		out[i].Name = redactSessionLogString(out[i].Name)
		out[i].Source = redactSessionLogString(out[i].Source)
		out[i].Scope = sanitizeSentinelScope(out[i].Scope)
		out[i].Attributes = sanitizeSentinelAttributes(out[i].Attributes)
		out[i].Payload = sanitizeSentinelPayload(out[i].Payload)
	}
	return out
}

func sanitizeSentinelScope(scope extensions.SentinelScope) extensions.SentinelScope {
	scope.URL = redactPanelURLSecrets(scope.URL)
	scope.ScriptURL = redactPanelURLSecrets(scope.ScriptURL)
	return scope
}

func sanitizeSentinelAttributes(attrs map[string]string) map[string]string {
	if attrs == nil {
		return nil
	}
	out := make(map[string]string, len(attrs))
	for key, value := range attrs {
		if sensitiveSessionLogKey(key) {
			out[key] = "[redacted]"
			continue
		}
		out[key] = redactSessionLogString(value)
	}
	return out
}

func sanitizeSentinelPayload(payload json.RawMessage) json.RawMessage {
	if len(payload) == 0 {
		return payload
	}
	var decoded interface{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		encoded, marshalErr := json.Marshal(redactSessionLogString(string(payload)))
		if marshalErr != nil {
			return json.RawMessage(`"[redacted]"`)
		}
		return encoded
	}
	encoded, err := json.Marshal(sanitizeSessionLogValue(decoded))
	if err != nil {
		return json.RawMessage(`"[redacted]"`)
	}
	return encoded
}

func redactSentinelStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = redactSessionLogString(value)
	}
	return out
}

func redactSentinelProxyEndpoints(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = redactSentinelProxyEndpoint(value)
	}
	return out
}

func redactSentinelProxyEndpoint(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return trimmed
	}
	if strings.Contains(trimmed, "://") {
		return redactPanelURLSecrets(redactProxyURL(trimmed))
	}
	if at := strings.LastIndex(trimmed, "@"); at >= 0 {
		hostPort := trimmed[at+1:]
		if hostPort != "" {
			return "redacted@" + hostPort
		}
	}
	if parsed, err := proxy.ParseProxyURL(trimmed); err == nil {
		return parsed.String()
	}
	return redactSessionLogString(trimmed)
}
