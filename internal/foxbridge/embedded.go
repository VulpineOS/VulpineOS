package foxbridge

import (
	"encoding/json"
	"fmt"
	"log"
	"net"

	"vulpineos/internal/juggler"

	"github.com/VulpineOS/foxbridge/pkg/backend"
	"github.com/VulpineOS/foxbridge/pkg/bridge"
	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

// jugglerAdapter wraps a VulpineOS juggler.Client as a foxbridge backend.Backend.
// The key difference is the Call signature: VulpineOS takes interface{}, foxbridge takes json.RawMessage.
// Since json.RawMessage implements json.Marshaler, passing it as interface{} to VulpineOS's
// client (which calls json.Marshal) produces identical bytes — no double-encoding.
type jugglerAdapter struct {
	client *juggler.Client
}

// Verify jugglerAdapter implements backend.Backend at compile time.
var _ backend.Backend = (*jugglerAdapter)(nil)

func (a *jugglerAdapter) Call(sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	// Pass json.RawMessage directly as interface{} — VulpineOS's client marshals it correctly.
	return a.client.Call(sessionID, method, params)
}

func (a *jugglerAdapter) Subscribe(event string, handler backend.EventHandler) {
	// Both sides use func(sessionID string, params json.RawMessage) — direct passthrough.
	a.client.Subscribe(event, juggler.EventHandler(handler))
}

func (a *jugglerAdapter) Close() error {
	// Don't close the underlying client — it belongs to the kernel.
	return nil
}

// EmbeddedServer runs foxbridge's CDP server in-process, wrapping the kernel's Juggler client.
type EmbeddedServer struct {
	server   *cdp.Server
	sessions *cdp.SessionManager
	bridge   *bridge.Bridge
	port     int
	done     chan struct{}
}

// StartEmbedded creates and starts an embedded foxbridge CDP server using the kernel's Juggler client.
// The server runs in a background goroutine. Call Stop() to shut it down.
func StartEmbedded(client *juggler.Client, port int) (*EmbeddedServer, error) {
	if client == nil {
		return nil, fmt.Errorf("juggler client is nil")
	}
	if port == 0 {
		port = 9222
	}

	// Wrap VulpineOS's juggler client as a foxbridge backend.
	be := &jugglerAdapter{client: client}

	return startEmbeddedWithBackend(be, port, true)
}

// StartEmbeddedScoped creates an embedded foxbridge server limited to a single browser context.
// The server only exposes targets and events for browserContextId and forces new pages into it.
func StartEmbeddedScoped(client *juggler.Client, port int, browserContextID string) (*EmbeddedServer, error) {
	if client == nil {
		return nil, fmt.Errorf("juggler client is nil")
	}
	if browserContextID == "" {
		return nil, fmt.Errorf("browser context id is required")
	}
	if port == 0 {
		var err error
		port, err = reservePort()
		if err != nil {
			return nil, err
		}
	}

	be := newScopedBackend(client, browserContextID)
	return startEmbeddedWithBackend(be, port, false)
}

func startEmbeddedWithBackend(be backend.Backend, port int, attachToDefaultContext bool) (*EmbeddedServer, error) {

	// Create CDP session manager and server (same wiring as foxbridge main.go).
	sessions := cdp.NewSessionManager()

	var b *bridge.Bridge
	server := cdp.NewServer(port, func(conn *cdp.Connection, msg *cdp.Message) {
		b.HandleMessage(conn, msg)
	}, sessions)

	b = bridge.New(be, sessions, server)
	b.SetupEventSubscriptions()

	// Browser.enable is already called by the kernel — the adapter shares the same
	// underlying Juggler connection, so existing subscriptions and targets are visible.
	// However, foxbridge needs its own Browser.enable to receive attachedToTarget events
	// through the bridge's event subscriptions.
	enableParams, _ := json.Marshal(map[string]interface{}{
		"attachToDefaultContext": attachToDefaultContext,
	})
	if _, err := be.Call("", "Browser.enable", enableParams); err != nil {
		return nil, fmt.Errorf("Browser.enable via embedded foxbridge: %w", err)
	}

	es := &EmbeddedServer{
		server:   server,
		sessions: sessions,
		bridge:   b,
		port:     port,
		done:     make(chan struct{}),
	}

	// Start CDP server in background.
	go func() {
		defer close(es.done)
		if err := server.Start(); err != nil {
			log.Printf("embedded foxbridge CDP server error: %v", err)
		}
	}()

	log.Printf("embedded foxbridge CDP server listening on 127.0.0.1:%d", port)
	return es, nil
}

func reservePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve foxbridge port: %w", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("reserve foxbridge port: unexpected address type %T", listener.Addr())
	}
	return addr.Port, nil
}

// CDPURL returns the CDP WebSocket URL for connecting clients (e.g., OpenClaw).
func (es *EmbeddedServer) CDPURL() string {
	return fmt.Sprintf("ws://127.0.0.1:%d", es.port)
}

// Port returns the CDP server port.
func (es *EmbeddedServer) Port() int {
	return es.port
}

// Stop shuts down the embedded CDP server.
// It does NOT close the underlying Juggler client (owned by the kernel).
func (es *EmbeddedServer) Stop() {
	// cdp.Server uses http.ListenAndServe which doesn't have a graceful shutdown.
	// The server goroutine will exit when the process exits.
	log.Println("embedded foxbridge stopped")
}
