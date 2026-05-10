package openclaw

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func writeGatewayTestBinary(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openclaw-test")
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
	if g.logFile != nil {
		t.Fatal("gateway log file was not cleared after readiness failure")
	}
}

func TestGatewayStartFailureClearsLogFile(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0600); err != nil {
		t.Fatalf("write gateway test binary: %v", err)
	}
	g := NewGateway(bin)

	if err := g.Start(); err == nil {
		t.Fatal("expected start failure")
	}
	if g.logFile != nil {
		t.Fatal("gateway log file was retained after start failure")
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
	if g.cmd != nil {
		t.Fatal("gateway command was not cleared after process exit")
	}
	if g.logFile != nil {
		t.Fatal("gateway log file was not cleared after process exit")
	}
}

func TestGatewayStartDoesNotHangOnStaleStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script test binary is unix-specific")
	}
	originalTimeout := gatewayStopCommandTimeout
	gatewayStopCommandTimeout = 50 * time.Millisecond
	t.Cleanup(func() { gatewayStopCommandTimeout = originalTimeout })

	bin := writeGatewayTestBinary(t, `#!/bin/sh
if [ "$4" = "stop" ]; then
  sleep 60
fi
exit 0
`)
	g := NewGateway(bin)
	g.waitReadyFunc = func(string) error { return nil }
	defer g.Stop()

	started := time.Now()
	if err := g.Start(); err != nil {
		t.Fatalf("start gateway: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("gateway start took %s with wedged stale stop", elapsed)
	}
}

func TestGatewayStopKillsChildProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script test binary is unix-specific")
	}

	dir := t.TempDir()
	childPath := filepath.Join(dir, "child.pid")
	bin := writeGatewayTestBinary(t, "#!/bin/sh\nif [ \"$4\" = \"run\" ]; then\n  sleep 60 &\n  echo $! > "+childPath+"\n  wait\nfi\nexit 0\n")
	g := NewGateway(bin)
	g.waitReadyFunc = func(string) error { return nil }

	if err := g.Start(); err != nil {
		t.Fatalf("start gateway: %v", err)
	}
	childPID := waitForGatewayPIDFile(t, childPath)
	g.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !gatewayProcessExists(childPID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	proc, err := os.FindProcess(childPID)
	if err == nil {
		_ = proc.Kill()
	}
	t.Fatalf("gateway child process %d survived stop", childPID)
}

func waitForGatewayPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pidText := strings.TrimSpace(string(data))
			var pid int
			if _, scanErr := fmt.Sscanf(pidText, "%d", &pid); scanErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child pid file was not written")
	return 0
}

func gatewayProcessExists(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
