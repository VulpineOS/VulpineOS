package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type snapshotProfile struct {
	Name          string `json:"profile"`
	MaxDepth      int    `json:"maxDepth"`
	MaxNodes      int    `json:"maxNodes"`
	MaxTextLength int    `json:"maxTextLength"`
}

var snapshotProfiles = []snapshotProfile{
	{Name: "compact", MaxDepth: 10, MaxNodes: 180, MaxTextLength: 90},
	{Name: "expanded", MaxDepth: 12, MaxNodes: 360, MaxTextLength: 160},
	{Name: "full", MaxDepth: 14, MaxNodes: 800, MaxTextLength: 240},
}

type snapshotRetryRecord struct {
	profile   string
	truncated bool
}

var snapshotRetryState = struct {
	sync.Mutex
	bySession map[string]snapshotRetryRecord
}{
	bySession: make(map[string]snapshotRetryRecord),
}

func defaultSnapshotProfile() snapshotProfile {
	return snapshotProfiles[0]
}

func snapshotProfileByName(name string) (snapshotProfile, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return defaultSnapshotProfile(), nil
	}
	for _, profile := range snapshotProfiles {
		if profile.Name == name {
			return profile, nil
		}
	}
	return snapshotProfile{}, fmt.Errorf("unknown snapshot profile %q; use compact, expanded, or full", name)
}

func nextSnapshotProfile(name string) snapshotProfile {
	name = strings.ToLower(strings.TrimSpace(name))
	for i, profile := range snapshotProfiles {
		if profile.Name == name {
			if i+1 < len(snapshotProfiles) {
				return snapshotProfiles[i+1]
			}
			return profile
		}
	}
	if len(snapshotProfiles) > 1 {
		return snapshotProfiles[1]
	}
	return defaultSnapshotProfile()
}

func retrySnapshotProfile(sessionID string) snapshotProfile {
	snapshotRetryState.Lock()
	defer snapshotRetryState.Unlock()

	if record, ok := snapshotRetryState.bySession[sessionID]; ok && record.truncated {
		return nextSnapshotProfile(record.profile)
	}
	return nextSnapshotProfile(defaultSnapshotProfile().Name)
}

func recordSnapshotProfile(sessionID string, profile snapshotProfile, truncated bool) {
	snapshotRetryState.Lock()
	defer snapshotRetryState.Unlock()
	snapshotRetryState.bySession[sessionID] = snapshotRetryRecord{
		profile:   profile.Name,
		truncated: truncated,
	}
}

func resetSnapshotProfile(sessionID string) {
	snapshotRetryState.Lock()
	defer snapshotRetryState.Unlock()
	delete(snapshotRetryState.bySession, sessionID)
}

func snapshotRetryHint(profile snapshotProfile, truncated bool) string {
	if !truncated {
		return ""
	}
	if profile.Name == "custom" {
		return "Snapshot was truncated with custom limits. If exhaustive content is required, pass larger explicit maxNodes and maxTextLength values."
	}
	next := nextSnapshotProfile(profile.Name)
	if next.Name != profile.Name {
		return fmt.Sprintf("Snapshot was truncated. If the target is missing, retry vulpine_snapshot with retry:true or profile:%q (%d nodes, %d chars).", next.Name, next.MaxNodes, next.MaxTextLength)
	}
	return "Snapshot was truncated even at the full profile. If exhaustive content is required, pass explicit maxNodes and maxTextLength values."
}

func annotateSnapshotPayload(raw json.RawMessage, profile snapshotProfile) ([]byte, bool, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false, err
	}

	truncated, _ := payload["truncated"].(bool)
	payload["profile"] = profile.Name
	payload["limits"] = map[string]int{
		"maxDepth":      profile.MaxDepth,
		"maxNodes":      profile.MaxNodes,
		"maxTextLength": profile.MaxTextLength,
	}
	if hint := snapshotRetryHint(profile, truncated); hint != "" {
		payload["retryHint"] = hint
	}

	annotated, err := json.Marshal(payload)
	return annotated, truncated, err
}
