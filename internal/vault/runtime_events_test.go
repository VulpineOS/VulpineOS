package vault

import (
	"fmt"
	"testing"
)

func TestAppendAndListRuntimeEvents(t *testing.T) {
	db, err := OpenPath(t.TempDir() + "/vault.db")
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.AppendRuntimeEvent("kernel", "error", "crashed", "kernel exited unexpectedly", map[string]string{
		"pid": "1234",
	})
	if err != nil {
		t.Fatalf("AppendRuntimeEvent: %v", err)
	}
	_, err = db.AppendRuntimeEvent("openclaw", "warn", "restart_failed", "agent restart failed", nil)
	if err != nil {
		t.Fatalf("AppendRuntimeEvent: %v", err)
	}

	events, err := db.ListRuntimeEvents(10)
	if err != nil {
		t.Fatalf("ListRuntimeEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Component != "openclaw" || events[0].Event != "restart_failed" {
		t.Fatalf("unexpected newest event: %+v", events[0])
	}
	if got := events[1].Metadata["pid"]; got != "1234" {
		t.Fatalf("expected pid metadata, got %q", got)
	}
}

func TestAppendRuntimeEventTrimsHistory(t *testing.T) {
	db, err := OpenPath(t.TempDir() + "/vault.db")
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for i := 0; i < maxRuntimeEvents+5; i++ {
		if _, err := db.AppendRuntimeEvent("kernel", "info", "started", fmt.Sprintf("event-%d", i), map[string]string{
			"seq": fmt.Sprintf("%d", i),
		}); err != nil {
			t.Fatalf("AppendRuntimeEvent %d: %v", i, err)
		}
	}

	events, err := db.ListRuntimeEvents(maxRuntimeEvents + 10)
	if err != nil {
		t.Fatalf("ListRuntimeEvents: %v", err)
	}
	if len(events) != maxRuntimeEvents {
		t.Fatalf("expected %d retained events, got %d", maxRuntimeEvents, len(events))
	}
	if oldest := events[len(events)-1].Metadata["seq"]; oldest != "5" {
		t.Fatalf("expected oldest retained seq 5, got %q", oldest)
	}
}
