package foxbridge

import (
	"errors"
	"testing"
	"time"

	"vulpineos/internal/juggler"
	"vulpineos/internal/testutil"
)

func TestStartEmbeddedModeStopsServerWhenReadinessFails(t *testing.T) {
	port, err := reservePort()
	if err != nil {
		t.Fatalf("reservePort: %v", err)
	}
	transport := testutil.NewFakeJugglerTransport(t)
	transport.RespondJSON("Browser.enable", map[string]any{})
	client := juggler.NewClient(transport)

	originalWait := waitForProcessPort
	waitForProcessPort = func(int, time.Duration) error {
		return errors.New("forced readiness failure")
	}
	t.Cleanup(func() { waitForProcessPort = originalWait })

	process := New()
	if err := process.StartEmbeddedMode(client, port); err == nil {
		process.Stop()
		t.Fatal("expected forced readiness failure")
	}
	if process.embedded != nil {
		process.Stop()
		t.Fatal("embedded server retained after failed startup")
	}
	if err := ensurePortAvailable(port); err != nil {
		t.Fatalf("embedded server was not stopped after failed startup: %v", err)
	}
}
