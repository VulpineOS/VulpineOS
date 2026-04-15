package runtimeaudit

import (
	"testing"
	"time"

	"vulpineos/internal/vault"
)

func TestManagerLogAndSubscribe(t *testing.T) {
	db, err := vault.OpenPath(t.TempDir() + "/vault.db")
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	manager := New(db)
	t.Cleanup(manager.Close)

	sub := manager.Subscribe()
	_, err = manager.Log("kernel", "error", "crashed", "kernel exited", map[string]string{"pid": "44"})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	select {
	case event := <-sub:
		if event.Component != "kernel" || event.Event != "crashed" {
			t.Fatalf("unexpected event: %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runtime event")
	}

	events, err := manager.List(10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 stored event, got %d", len(events))
	}
}
