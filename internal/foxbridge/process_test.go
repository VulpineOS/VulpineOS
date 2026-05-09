package foxbridge

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"vulpineos/internal/juggler"
	"vulpineos/internal/testutil"
)

func TestStartExternalFailureDoesNotBlockRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script test binary is unix-specific")
	}

	dir := t.TempDir()
	t.Chdir(dir)
	foxbridgePath := filepath.Join(dir, "foxbridge")
	if err := os.WriteFile(foxbridgePath, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("write non-executable foxbridge: %v", err)
	}

	process := New()
	if err := process.Start(Config{}); err == nil {
		t.Fatal("expected non-executable foxbridge start to fail")
	}
	if process.cmd != nil {
		t.Fatal("failed start retained command state")
	}
	if process.logFile != nil {
		t.Fatal("failed start retained log file state")
	}

	if err := os.WriteFile(foxbridgePath, []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatalf("write executable foxbridge: %v", err)
	}
	if err := os.Chmod(foxbridgePath, 0o755); err != nil {
		t.Fatalf("chmod executable foxbridge: %v", err)
	}

	originalWait := waitForProcessPort
	waitForProcessPort = func(int, time.Duration) error {
		return errors.New("forced readiness failure")
	}
	t.Cleanup(func() { waitForProcessPort = originalWait })

	err := process.Start(Config{})
	if err == nil {
		process.Stop()
		t.Fatal("expected readiness failure on retry")
	}
	if !strings.Contains(err.Error(), "forced readiness failure") {
		t.Fatalf("retry error = %v, want forced readiness failure", err)
	}
	if process.cmd != nil {
		process.Stop()
		t.Fatal("readiness failure retained command state")
	}
}

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
