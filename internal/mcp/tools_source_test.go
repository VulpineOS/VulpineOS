package mcp

import (
	"os"
	"strings"
	"testing"
)

func TestHandleNewContextCancelsAttachSubscription(t *testing.T) {
	data, err := os.ReadFile("tools.go")
	if err != nil {
		t.Fatalf("read tools.go: %v", err)
	}
	source := string(data)
	if !strings.Contains(source, "cancelAttach := client.SubscribeWithCancel(\"Browser.attachedToTarget\"") {
		t.Fatal("handleNewContext does not use a cancellable attach subscription")
	}
	if !strings.Contains(source, "defer cancelAttach()") {
		t.Fatal("handleNewContext does not cancel the attach subscription")
	}
}
