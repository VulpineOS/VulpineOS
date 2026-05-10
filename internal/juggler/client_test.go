package juggler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// memTransport is an in-memory Transport for testing.
type memTransport struct {
	incoming chan *Message // messages to be received by the client
	outgoing chan *Message // messages sent by the client
	closed   chan struct{}
	once     sync.Once
}

func newMemTransport() *memTransport {
	return &memTransport{
		incoming: make(chan *Message, 64),
		outgoing: make(chan *Message, 64),
		closed:   make(chan struct{}),
	}
}

func (t *memTransport) Send(msg *Message) error {
	select {
	case <-t.closed:
		return fmt.Errorf("transport closed")
	case t.outgoing <- msg:
		return nil
	}
}

func (t *memTransport) Receive() (*Message, error) {
	select {
	case <-t.closed:
		return nil, fmt.Errorf("transport closed")
	case msg := <-t.incoming:
		return msg, nil
	}
}

func (t *memTransport) Close() error {
	t.once.Do(func() { close(t.closed) })
	return nil
}

type blockingSendTransport struct {
	sendStarted chan struct{}
	closed      chan struct{}
	once        sync.Once
}

func newBlockingSendTransport() *blockingSendTransport {
	return &blockingSendTransport{
		sendStarted: make(chan struct{}),
		closed:      make(chan struct{}),
	}
}

func (t *blockingSendTransport) Send(*Message) error {
	t.once.Do(func() { close(t.sendStarted) })
	<-t.closed
	return fmt.Errorf("transport closed")
}

func (t *blockingSendTransport) Receive() (*Message, error) {
	<-t.closed
	return nil, fmt.Errorf("transport closed")
}

func (t *blockingSendTransport) Close() error {
	select {
	case <-t.closed:
	default:
		close(t.closed)
	}
	return nil
}

// respondToRequests reads outgoing messages and replies with canned responses.
func respondToRequests(mt *memTransport) {
	for {
		select {
		case <-mt.closed:
			return
		case req := <-mt.outgoing:
			result, _ := json.Marshal(map[string]string{"echo": req.Method})
			mt.incoming <- &Message{
				ID:     req.ID,
				Result: result,
			}
		}
	}
}

func TestClient_Call_ReturnsResponse(t *testing.T) {
	mt := newMemTransport()
	go respondToRequests(mt)

	c := NewClient(mt)
	defer c.Close()

	result, err := c.Call("", "Test.method", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["echo"] != "Test.method" {
		t.Errorf("expected echo=Test.method, got %s", parsed["echo"])
	}
}

func TestClient_Subscribe_ReceivesEvents(t *testing.T) {
	mt := newMemTransport()
	c := NewClient(mt)
	defer c.Close()

	received := make(chan json.RawMessage, 1)
	c.Subscribe("Page.loadFired", func(_ string, params json.RawMessage) {
		received <- params
	})

	payload, _ := json.Marshal(map[string]int{"timestamp": 42})
	mt.incoming <- &Message{
		Method: "Page.loadFired",
		Params: payload,
	}

	select {
	case params := <-received:
		var parsed map[string]int
		if err := json.Unmarshal(params, &parsed); err != nil {
			t.Fatalf("unmarshal params: %v", err)
		}
		if parsed["timestamp"] != 42 {
			t.Errorf("expected timestamp=42, got %d", parsed["timestamp"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestClient_SubscribeWithCancel_RemovesHandler(t *testing.T) {
	mt := newMemTransport()
	c := NewClient(mt)
	defer c.Close()

	received := make(chan json.RawMessage, 1)
	cancel := c.SubscribeWithCancel("Page.loadFired", func(_ string, params json.RawMessage) {
		received <- params
	})
	cancel()
	cancel()

	payload, _ := json.Marshal(map[string]int{"timestamp": 42})
	mt.incoming <- &Message{
		Method: "Page.loadFired",
		Params: payload,
	}

	select {
	case params := <-received:
		t.Fatalf("handler received event after cancellation: %s", params)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestClient_EventHandlerDoesNotBlockResponses(t *testing.T) {
	mt := newMemTransport()
	c := NewClient(mt)
	defer c.Close()

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	c.Subscribe("Page.slowEvent", func(_ string, _ json.RawMessage) {
		close(handlerStarted)
		<-releaseHandler
	})

	mt.incoming <- &Message{
		Method: "Page.slowEvent",
		Params: json.RawMessage(`{}`),
	}
	select {
	case <-handlerStarted:
	case <-time.After(time.Second):
		t.Fatal("event handler did not start")
	}
	defer close(releaseHandler)

	go func() {
		req := <-mt.outgoing
		mt.incoming <- &Message{
			ID:     req.ID,
			Result: json.RawMessage(`{"ok":true}`),
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := c.CallWithContext(ctx, "", "Browser.getInfo", nil)
	if err != nil {
		t.Fatalf("CallWithContext while event handler blocked: %v", err)
	}
	if string(result) != `{"ok":true}` {
		t.Fatalf("result = %s, want ok response", result)
	}
}

func TestClient_QueueEventBackpressuresWhenBufferFull(t *testing.T) {
	c := &Client{
		events:   make(chan *Message, 1),
		done:     make(chan struct{}),
		handlers: make(map[string][]eventSubscription),
	}
	c.events <- &Message{Method: "Page.first"}

	queued := make(chan struct{})
	go func() {
		c.queueEvent(&Message{Method: "Page.second"})
		close(queued)
	}()

	select {
	case <-queued:
		t.Fatal("queueEvent returned while event buffer was full")
	case <-time.After(50 * time.Millisecond):
	}

	<-c.events
	select {
	case <-queued:
	case <-time.After(time.Second):
		t.Fatal("queueEvent did not unblock after buffer space was available")
	}
}

func TestClient_CallWithContext_TimesOut(t *testing.T) {
	mt := newMemTransport()
	// Don't respond — let it hang
	c := NewClient(mt)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.CallWithContext(ctx, "", "Slow.method", nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !contains(err.Error(), "context deadline exceeded") {
		t.Errorf("expected context deadline exceeded, got: %v", err)
	}
}

func TestClient_CallWithContext_TimesOutWhenSendBlocks(t *testing.T) {
	bt := newBlockingSendTransport()
	c := NewClient(bt)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.CallWithContext(ctx, "", "Blocked.method", nil)
		done <- err
	}()

	select {
	case <-bt.sendStarted:
	case <-time.After(time.Second):
		t.Fatal("Send was not reached")
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		if !contains(err.Error(), "context deadline exceeded") {
			t.Fatalf("expected context deadline exceeded, got: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("CallWithContext did not return after context timeout")
	}

	select {
	case <-bt.closed:
	default:
		t.Fatal("client did not close the blocked transport")
	}
}

func TestClient_ConcurrentCalls(t *testing.T) {
	mt := newMemTransport()
	go respondToRequests(mt)

	c := NewClient(mt)
	defer c.Close()

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			method := fmt.Sprintf("Test.method%d", n)
			result, err := c.Call("", method, nil)
			if err != nil {
				errs <- fmt.Errorf("call %d failed: %w", n, err)
				return
			}
			var parsed map[string]string
			if err := json.Unmarshal(result, &parsed); err != nil {
				errs <- fmt.Errorf("call %d unmarshal: %w", n, err)
				return
			}
			if parsed["echo"] != method {
				errs <- fmt.Errorf("call %d: expected echo=%s, got %s", n, method, parsed["echo"])
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
