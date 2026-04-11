package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLabelIndexSetGet(t *testing.T) {
	idx := &labelIndex{sessions: map[string]map[string]string{}}

	elements := []map[string]interface{}{
		{"label": "@1", "objectId": "obj-a"},
		{"label": "@2", "objectId": "obj-b"},
		// Missing objectId — must be skipped.
		{"label": "@3"},
		// Missing label — synthesized as @4 (index position 3 + 1 = @4).
		{"objectId": "obj-d"},
	}
	idx.Set("sess-1", elements)

	got, ok := idx.Get("sess-1", "@1")
	if !ok || got != "obj-a" {
		t.Errorf("Get(@1) = (%q, %v), want (obj-a, true)", got, ok)
	}
	got, ok = idx.Get("sess-1", "@2")
	if !ok || got != "obj-b" {
		t.Errorf("Get(@2) = (%q, %v), want (obj-b, true)", got, ok)
	}
	if _, ok := idx.Get("sess-1", "@3"); ok {
		t.Error("Get(@3) should be absent (no objectId)")
	}
	if got, ok := idx.Get("sess-1", "@4"); !ok || got != "obj-d" {
		t.Errorf("Get(@4) synthesized = (%q, %v), want (obj-d, true)", got, ok)
	}
	if _, ok := idx.Get("sess-unknown", "@1"); ok {
		t.Error("Get on unknown session should return false")
	}

	// Second Set replaces, not merges.
	idx.Set("sess-1", []map[string]interface{}{
		{"label": "@X", "objectId": "obj-x"},
	})
	if _, ok := idx.Get("sess-1", "@1"); ok {
		t.Error("Set should replace, not merge: @1 should be gone")
	}
	if got, ok := idx.Get("sess-1", "@X"); !ok || got != "obj-x" {
		t.Errorf("Get(@X) = (%q, %v)", got, ok)
	}

	// Clear drops the session.
	idx.Clear("sess-1")
	if _, ok := idx.Get("sess-1", "@X"); ok {
		t.Error("Clear should drop the session")
	}
}

// TestVulpineClickLabelUnavailable verifies the tool fails gracefully
// when no label has been indexed for the session — this is the path
// agents hit if they call click_label without first running an
// annotated screenshot.
func TestVulpineClickLabelUnavailable(t *testing.T) {
	// Ensure a clean slate.
	globalLabels.Clear("no-such-session")
	args, _ := json.Marshal(map[string]interface{}{
		"session_id": "no-such-session",
		"label":      "@1",
	})
	res, ok := handleExtensionTool(nil, "vulpine_click_label", args)
	if !ok {
		t.Fatal("vulpine_click_label not dispatched")
	}
	if !res.IsError {
		t.Fatal("expected IsError")
	}
	text := res.Content[0].Text
	// nil client hits the client==nil branch first.
	if !strings.Contains(text, "juggler client unavailable") {
		t.Errorf("unexpected error text: %q", text)
	}
}
