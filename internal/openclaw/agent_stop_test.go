//go:build darwin || linux

package openclaw

import (
	"testing"
	"time"
)

func TestAgentStopKillsProcessGroup(t *testing.T) {
	statusCh := make(chan AgentStatus, 8)
	agent := newAgent("test-agent", "test-context", statusCh)

	if err := agent.start("/bin/sh", []string{"-c", "sleep 30 & wait"}); err != nil {
		t.Fatalf("start agent: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- agent.stopWithStatus("completed")
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop agent: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stopWithStatus timed out waiting for descendant process exit")
	}
}
