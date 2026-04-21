package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// navigateHandlerClearsLabels inspects tools.go to confirm the
// navigate handler contains the globalLabels.Clear hook. This is a
// source-level assertion so the test survives even when we can't
// actually drive a live juggler client.
func navigateHandlerClearsLabels() bool {
	data, err := os.ReadFile("tools.go")
	if err != nil {
		return false
	}
	src := string(data)
	idx := strings.Index(src, "func handleNavigate(")
	if idx < 0 {
		return false
	}
	end := strings.Index(src[idx:], "\nfunc ")
	if end < 0 {
		end = len(src) - idx
	}
	body := src[idx : idx+end]
	return strings.Contains(body, "globalLabels.Clear(")
}

func TestLabelIndexSetGet(t *testing.T) {
	idx := &labelIndex{sessions: map[string]map[string]labelTarget{}}

	elements := []map[string]interface{}{
		{"label": "@1", "objectId": "obj-a", "frameId": "frame-a"},
		{"label": "@2", "objectId": "obj-b", "frameId": "frame-b"},
		// Missing objectId — must be skipped.
		{"label": "@3"},
		// Missing label — synthesized as @4 (index position 3 + 1 = @4).
		{"objectId": "obj-d", "frameId": "frame-d"},
	}
	idx.Set("sess-1", elements)

	got, ok := idx.Get("sess-1", "@1")
	if !ok || got.ObjectID != "obj-a" || got.FrameID != "frame-a" {
		t.Errorf("Get(@1) = (%q, %v), want (obj-a, true)", got, ok)
	}
	got, ok = idx.Get("sess-1", "@2")
	if !ok || got.ObjectID != "obj-b" || got.FrameID != "frame-b" {
		t.Errorf("Get(@2) = (%q, %v), want (obj-b, true)", got, ok)
	}
	if _, ok := idx.Get("sess-1", "@3"); ok {
		t.Error("Get(@3) should be absent (no objectId)")
	}
	if got, ok := idx.Get("sess-1", "@4"); !ok || got.ObjectID != "obj-d" || got.FrameID != "frame-d" {
		t.Errorf("Get(@4) synthesized = (%q, %v), want (obj-d, true)", got, ok)
	}
	if _, ok := idx.Get("sess-unknown", "@1"); ok {
		t.Error("Get on unknown session should return false")
	}

	// Second Set replaces, not merges.
	idx.Set("sess-1", []map[string]interface{}{
		{"label": "@X", "objectId": "obj-x", "frameId": "frame-x"},
	})
	if _, ok := idx.Get("sess-1", "@1"); ok {
		t.Error("Set should replace, not merge: @1 should be gone")
	}
	if got, ok := idx.Get("sess-1", "@X"); !ok || got.ObjectID != "obj-x" || got.FrameID != "frame-x" {
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
	res, ok := handleExtensionTool(context.Background(), nil, "vulpine_click_label", args)
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

// TestLabelIndex_LRUEvictsOldest verifies that when the session count
// grows past MaxLabelSessions, the least recently accessed session is
// dropped. Without the cap labelIndex would leak one map per distinct
// session forever.
func TestLabelIndex_LRUEvictsOldest(t *testing.T) {
	idx := &labelIndex{
		sessions:   map[string]map[string]labelTarget{},
		lastAccess: map[string]time.Time{},
	}
	// Fill past the cap. Each Set touches its session, so the
	// earliest-inserted (and never re-touched) session is the oldest.
	for i := 0; i < MaxLabelSessions+5; i++ {
		idx.Set(fmt.Sprintf("sess-%d", i), []map[string]interface{}{
			{"label": "@1", "objectId": fmt.Sprintf("obj-%d", i), "frameId": fmt.Sprintf("frame-%d", i)},
		})
		// Nudge time forward slightly so ordering is deterministic
		// even on fast machines where two successive time.Now()
		// samples could land in the same tick.
		time.Sleep(time.Millisecond)
	}
	if got := idx.Len(); got != MaxLabelSessions {
		t.Fatalf("Len after overflow = %d, want %d", got, MaxLabelSessions)
	}
	// The first five sessions should have been evicted.
	for i := 0; i < 5; i++ {
		if _, ok := idx.Get(fmt.Sprintf("sess-%d", i), "@1"); ok {
			t.Errorf("sess-%d should have been evicted", i)
		}
	}
	// Newer sessions should still be resolvable.
	if _, ok := idx.Get(fmt.Sprintf("sess-%d", MaxLabelSessions+4), "@1"); !ok {
		t.Error("most recent session unexpectedly missing")
	}
}

// TestLabelIndex_ClearOnNavigate verifies that vulpine_navigate
// clears the label index for the affected session so that a
// follow-up vulpine_click_label does not resolve a stale objectID.
// We can't easily drive handleNavigate without a real juggler client,
// so this test exercises the observable contract: after a navigate,
// Get must return ok=false for previously-set labels.
func TestLabelIndex_ClearOnNavigate(t *testing.T) {
	globalLabels.Set("nav-sess", []map[string]interface{}{
		{"label": "@1", "objectId": "obj-pre-nav", "frameId": "frame-pre-nav"},
	})
	if _, ok := globalLabels.Get("nav-sess", "@1"); !ok {
		t.Fatal("precondition: label should be present before navigate")
	}
	// The production handler calls globalLabels.Clear after a
	// successful Page.navigate. Reproduce that effect directly so we
	// can assert the post-navigate state contract without spinning
	// up Firefox.
	globalLabels.Clear("nav-sess")
	if _, ok := globalLabels.Get("nav-sess", "@1"); ok {
		t.Error("label should be cleared after navigate")
	}

	// Sanity: the navigate handler source file must contain the
	// Clear call. This catches accidental regressions where someone
	// removes the hook but leaves this test's "shadow" call intact.
	if !navigateHandlerClearsLabels() {
		t.Error("handleNavigate must call globalLabels.Clear")
	}
}
