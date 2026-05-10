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
	t.Cleanup(tracker.Close)
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

func TestContextTrackerCloseUnsubscribesEvents(t *testing.T) {
	transport := testutil.NewFakeJugglerTransport(t)
	client := juggler.NewClient(transport)
	t.Cleanup(func() { _ = client.Close() })

	tracker := NewContextTracker(client)
	tracker.Close()
	transport.InjectEvent("", "Browser.attachedToTarget", map[string]any{
		"sessionId": "session-closed",
	})
	time.Sleep(50 * time.Millisecond)

	if got := tracker.Get("session-closed"); got != nil {
		t.Fatalf("closed tracker recorded context: %+v", got)
	}
}

func TestServerCleansSessionStateOnDetach(t *testing.T) {
	transport := testutil.NewFakeJugglerTransport(t)
	client := juggler.NewClient(transport)
	t.Cleanup(func() { _ = client.Close() })

	server := NewServer(client)
	t.Cleanup(server.Close)
	server.screenshots.Set("session-cleanup", "screenshot-data")
	recordSnapshotProfile("session-cleanup", defaultSnapshotProfile(), true)

	transport.InjectEvent("", "Browser.detachedFromTarget", map[string]any{
		"sessionId": "session-cleanup",
	})

	waitForSessionCleanup(t, server.screenshots, "session-cleanup")
}

func TestCloseContextCleansTrackedSessionState(t *testing.T) {
	transport := testutil.NewFakeJugglerTransport(t)
	transport.RespondJSON("Browser.removeBrowserContext", map[string]any{})
	client := juggler.NewClient(transport)
	t.Cleanup(func() { _ = client.Close() })

	server := NewServer(client)
	t.Cleanup(server.Close)
	transport.InjectEvent("", "Browser.attachedToTarget", map[string]any{
		"sessionId": "session-close",
		"targetInfo": map[string]any{
			"browserContextId": "context-close",
		},
	})
	waitForContext(t, server.tracker, "session-close", true)
	server.screenshots.Set("session-close", "screenshot-data")
	recordSnapshotProfile("session-close", defaultSnapshotProfile(), true)

	result, err := handleToolCallFull(
		t.Context(),
		client,
		server.tracker,
		server.screenshots,
		"vulpine_close_context",
		[]byte(`{"contextId":"context-close"}`),
	)
	if err != nil {
		t.Fatalf("close context returned error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("close context result = %+v", result)
	}

	waitForSessionCleanup(t, server.screenshots, "session-close")
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

func waitForSessionCleanup(t *testing.T, screenshots *ScreenshotTracker, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		snapshotRetryState.Lock()
		_, hasSnapshotRetry := snapshotRetryState.bySession[sessionID]
		snapshotRetryState.Unlock()
		if screenshots.Get(sessionID) == "" && !hasSnapshotRetry {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session state for %s was not cleaned up", sessionID)
}
