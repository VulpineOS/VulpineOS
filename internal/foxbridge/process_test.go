package foxbridge

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
	originalFind := findFoxbridgePath
	findFoxbridgePath = func() string {
		return foxbridgePath
	}
	t.Cleanup(func() { findFoxbridgePath = originalFind })

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

func TestStartExternalReapsExitedProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script test binary is unix-specific")
	}

	dir := t.TempDir()
	t.Chdir(dir)
	foxbridgePath := filepath.Join(dir, "foxbridge")
	if err := os.WriteFile(foxbridgePath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write foxbridge: %v", err)
	}
	if err := os.Chmod(foxbridgePath, 0o755); err != nil {
		t.Fatalf("chmod foxbridge: %v", err)
	}
	originalFind := findFoxbridgePath
	findFoxbridgePath = func() string {
		return foxbridgePath
	}
	t.Cleanup(func() { findFoxbridgePath = originalFind })

	originalWait := waitForProcessPort
	waitForProcessPort = func(int, time.Duration) error {
		return nil
	}
	t.Cleanup(func() { waitForProcessPort = originalWait })

	process := New()
	if err := process.Start(Config{}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	process.mu.Lock()
	waitDone := process.waitDone
	cmd := process.cmd
	logFile := process.logFile
	process.mu.Unlock()
	if waitDone == nil {
		if cmd != nil {
			t.Fatal("exited foxbridge command was not cleared")
		}
		if logFile != nil {
			t.Fatal("exited foxbridge log file was not cleared")
		}
		return
	}

	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		process.Stop()
		t.Fatal("foxbridge subprocess was not reaped")
	}

	process.mu.Lock()
	cmd = process.cmd
	logFile = process.logFile
	process.mu.Unlock()
	if cmd != nil {
		t.Fatal("exited foxbridge command was not cleared")
	}
	if logFile != nil {
		t.Fatal("exited foxbridge log file was not cleared")
	}
}

func TestStopExternalKillsChildProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script test binary is unix-specific")
	}

	dir := t.TempDir()
	t.Chdir(dir)
	childPath := filepath.Join(dir, "child.pid")
	foxbridgePath := filepath.Join(dir, "foxbridge")
	script := "#!/bin/sh\nsleep 60 &\necho $! > " + childPath + "\nwait\n"
	if err := os.WriteFile(foxbridgePath, []byte(script), 0o755); err != nil {
		t.Fatalf("write foxbridge: %v", err)
	}
	if err := os.Chmod(foxbridgePath, 0o755); err != nil {
		t.Fatalf("chmod foxbridge: %v", err)
	}
	originalFind := findFoxbridgePath
	findFoxbridgePath = func() string {
		return foxbridgePath
	}
	t.Cleanup(func() { findFoxbridgePath = originalFind })

	originalWait := waitForProcessPort
	waitForProcessPort = func(int, time.Duration) error {
		return nil
	}
	t.Cleanup(func() { waitForProcessPort = originalWait })

	process := New()
	if err := process.Start(Config{}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	childPID := waitForPIDFile(t, childPath)
	process.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !processExists(childPID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	proc, err := os.FindProcess(childPID)
	if err == nil {
		_ = proc.Kill()
	}
	t.Fatalf("child process %d survived foxbridge stop", childPID)
}

func waitForPIDFile(t *testing.T, path string) int {
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

func processExists(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
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
