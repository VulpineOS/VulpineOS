package nanoclaw

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeGatewayTestBinary(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nanoclaw-test")
	if err := os.WriteFile(path, []byte(body), 0700); err != nil {
		t.Fatalf("write gateway test binary: %v", err)
	}
	return path
}

func TestGatewayStartCleansUpOnReadinessFailure(t *testing.T) {
	bin := writeGatewayTestBinary(t, `#!/bin/sh
if [ "$4" = "run" ]; then
  sleep 30
fi
exit 0
`)
	g := NewGateway(bin)
	g.waitReadyFunc = func(string) error {
		return errors.New("not ready")
	}

	if err := g.Start(); err == nil {
		t.Fatal("expected readiness failure")
	}
	if g.Running() {
		t.Fatal("gateway still reports running after readiness failure")
	}
	if g.cmd != nil {
		t.Fatal("gateway command was not cleared after readiness failure")
	}
}

func TestGatewayRunningTracksExitedProcess(t *testing.T) {
	bin := writeGatewayTestBinary(t, `#!/bin/sh
exit 0
`)
	g := NewGateway(bin)
	g.waitReadyFunc = func(string) error { return nil }

	if err := g.Start(); err != nil {
		t.Fatalf("start gateway: %v", err)
	}
	t.Cleanup(g.Stop)

	deadline := time.Now().Add(2 * time.Second)
	for g.Running() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if g.Running() {
		t.Fatal("gateway still reports running after process exit")
	}
}
