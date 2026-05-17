package nanoclaw

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDaemonStartCreatesSocket(t *testing.T) {
	// Skip if NanoClaw isn't installed
	mgr := NewManager("")
	if !mgr.NanoClawInstalled() {
		t.Skip("NanoClaw not installed")
	}

	daemon := NewDaemon("")

	// Start should succeed and create the socket
	err := daemon.Start()
	if err != nil {
		t.Fatalf("daemon.Start() error = %v", err)
	}
	defer daemon.Stop()

	// Verify socket exists
	socketPath := filepath.Join(GetNanoclawDir(), "data", "cli.sock")

	// Wait up to 10 seconds for socket
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			return // Success
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatal("cli.sock was not created within 10 seconds")
}
