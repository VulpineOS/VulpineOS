package mcp

import (
	"testing"
	"time"

	"vulpineos/internal/juggler"
	"vulpineos/internal/testutil"
)

func TestContextTrackerRemovesSessionOnDetach(t *testing.T) {
	transport := testutil.NewFakeJugglerTransport(t)
	client := juggler.NewClient(transport)
	t.Cleanup(func() { _ = client.Close() })

	tracker := NewContextTracker(client)
	transport.InjectEvent("", "Browser.attachedToTarget", map[string]any{
		"sessionId": "session-1",
	})
	waitForContext(t, tracker, "session-1", true)

	transport.InjectEvent("", "Browser.detachedFromTarget", map[string]any{
		"sessionId": "session-1",
		"targetId":  "target-1",
	})
	waitForContext(t, tracker, "session-1", false)
}

func waitForContext(t *testing.T, tracker *ContextTracker, sessionID string, wantPresent bool) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		present := tracker.Get(sessionID) != nil
		if present == wantPresent {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("context presence for %s did not become %v", sessionID, wantPresent)
}
