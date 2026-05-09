package foxbridge

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"vulpineos/internal/juggler"
	"vulpineos/internal/testutil"

	"github.com/VulpineOS/foxbridge/pkg/backend"
)

func TestStartEmbeddedNilClient(t *testing.T) {
	_, err := StartEmbedded(nil, 0)
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if err.Error() != "juggler client is nil" {
		t.Errorf("error = %q, want %q", err.Error(), "juggler client is nil")
	}
}

func TestCDPURL(t *testing.T) {
	es := &EmbeddedServer{port: 9222}
	url := es.CDPURL()
	expected := "ws://127.0.0.1:9222"
	if url != expected {
		t.Errorf("CDPURL() = %q, want %q", url, expected)
	}
}

func TestCDPURLCustomPort(t *testing.T) {
	es := &EmbeddedServer{port: 12345}
	url := es.CDPURL()
	expected := "ws://127.0.0.1:12345"
	if url != expected {
		t.Errorf("CDPURL() = %q, want %q", url, expected)
	}
}

func TestPort(t *testing.T) {
	es := &EmbeddedServer{port: 8080}
	if es.Port() != 8080 {
		t.Errorf("Port() = %d, want 8080", es.Port())
	}
}

func TestStopDoesNotPanic(t *testing.T) {
	es := &EmbeddedServer{port: 9222, done: make(chan struct{})}
	// Should not panic even without a running server
	es.Stop()
}

type embeddedTestBackend struct {
	closeCount int
}

func (b *embeddedTestBackend) Call(sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (b *embeddedTestBackend) Subscribe(event string, handler backend.EventHandler) {}

func (b *embeddedTestBackend) Close() error {
	b.closeCount++
	return nil
}

func TestEmbeddedStopReleasesPort(t *testing.T) {
	port, err := reservePort()
	if err != nil {
		t.Fatalf("reservePort: %v", err)
	}
	es, err := startEmbeddedWithBackend(&embeddedTestBackend{}, port, true)
	if err != nil {
		t.Fatalf("startEmbeddedWithBackend: %v", err)
	}
	if err := waitForPort(port, 2*time.Second); err != nil {
		es.Stop()
		t.Fatalf("waitForPort: %v", err)
	}

	es.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := ensurePortAvailable(port); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("port %d was not released", port)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEmbeddedStopClosesBackend(t *testing.T) {
	port, err := reservePort()
	if err != nil {
		t.Fatalf("reservePort: %v", err)
	}
	be := &embeddedTestBackend{}
	es, err := startEmbeddedWithBackend(be, port, true)
	if err != nil {
		t.Fatalf("startEmbeddedWithBackend: %v", err)
	}

	es.Stop()

	if be.closeCount != 1 {
		t.Fatalf("backend close count = %d, want 1", be.closeCount)
	}
}

func TestCDPURLFormat(t *testing.T) {
	ports := []int{9222, 0, 1, 65535}
	for _, port := range ports {
		es := &EmbeddedServer{port: port}
		expected := fmt.Sprintf("ws://127.0.0.1:%d", port)
		if es.CDPURL() != expected {
			t.Errorf("CDPURL() with port %d = %q, want %q", port, es.CDPURL(), expected)
		}
	}
}

func TestJugglerAdapterClose(t *testing.T) {
	// jugglerAdapter.Close() should be a no-op (not close the underlying client)
	a := &jugglerAdapter{client: nil}
	if err := a.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestJugglerAdapterSuppressesDuplicateAttachedTargets(t *testing.T) {
	transport := testutil.NewFakeJugglerTransport(t)
	client := juggler.NewClient(transport)
	adapter := &jugglerAdapter{client: client}

	attached := make(chan string, 4)
	adapter.Subscribe("Browser.attachedToTarget", func(_ string, params json.RawMessage) {
		attached <- string(params)
	})

	target := map[string]any{
		"sessionId": "session-1",
		"targetInfo": map[string]any{
			"targetId": "target-1",
		},
	}

	transport.InjectEvent("", "Browser.attachedToTarget", target)
	expectAdapterEvent(t, attached)

	transport.InjectEvent("", "Browser.attachedToTarget", target)
	expectNoAdapterEvent(t, attached)

	transport.InjectEvent("", "Browser.attachedToTarget", map[string]any{
		"sessionId": "session-2",
		"targetInfo": map[string]any{
			"targetId": "target-1",
		},
	})
	expectNoAdapterEvent(t, attached)
}

func TestJugglerAdapterAllowsAttachAfterDetach(t *testing.T) {
	transport := testutil.NewFakeJugglerTransport(t)
	client := juggler.NewClient(transport)
	adapter := &jugglerAdapter{client: client}

	attached := make(chan string, 4)
	detached := make(chan string, 4)
	adapter.Subscribe("Browser.attachedToTarget", func(_ string, params json.RawMessage) {
		attached <- string(params)
	})
	adapter.Subscribe("Browser.detachedFromTarget", func(_ string, params json.RawMessage) {
		detached <- string(params)
	})

	target := map[string]any{
		"sessionId": "session-1",
		"targetInfo": map[string]any{
			"targetId": "target-1",
		},
	}

	transport.InjectEvent("", "Browser.attachedToTarget", target)
	expectAdapterEvent(t, attached)

	transport.InjectEvent("", "Browser.detachedFromTarget", map[string]any{
		"sessionId": "session-1",
		"targetId":  "target-1",
	})
	expectAdapterEvent(t, detached)

	transport.InjectEvent("", "Browser.attachedToTarget", target)
	expectAdapterEvent(t, attached)
}

func TestJugglerAdapterCloseCancelsSubscriptions(t *testing.T) {
	transport := testutil.NewFakeJugglerTransport(t)
	client := juggler.NewClient(transport)
	adapter := &jugglerAdapter{client: client}

	attached := make(chan string, 4)
	adapter.Subscribe("Browser.attachedToTarget", func(_ string, params json.RawMessage) {
		attached <- string(params)
	})
	if err := adapter.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	transport.InjectEvent("", "Browser.attachedToTarget", map[string]any{
		"sessionId": "session-1",
		"targetInfo": map[string]any{
			"targetId": "target-1",
		},
	})
	expectNoAdapterEvent(t, attached)
}

func expectAdapterEvent(t *testing.T, ch <-chan string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for adapter event")
	}
}

func expectNoAdapterEvent(t *testing.T, ch <-chan string) {
	t.Helper()
	select {
	case event := <-ch:
		t.Fatalf("unexpected adapter event: %s", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestEnsurePortAvailableDetectsBusyPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	if err := ensurePortAvailable(port); err == nil {
		t.Fatal("expected busy-port error")
	}
}

func TestEnsurePortAvailableAllowsFreePort(t *testing.T) {
	port, err := reservePort()
	if err != nil {
		t.Fatalf("reservePort: %v", err)
	}
	if err := ensurePortAvailable(port); err != nil {
		t.Fatalf("ensurePortAvailable(%d): %v", port, err)
	}
}
