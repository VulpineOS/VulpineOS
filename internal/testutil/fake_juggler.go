package testutil

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"vulpineos/internal/juggler"
)

type JugglerCall struct {
	ID        int
	SessionID string
	Method    string
	Params    json.RawMessage
}

type JugglerResponder func(*juggler.Message) (json.RawMessage, *juggler.Error)

type FakeJugglerTransport struct {
	t testing.TB

	incoming chan *juggler.Message
	closed   chan struct{}
	once     sync.Once

	mu         sync.RWMutex
	closedFlag bool
	calls      []JugglerCall
	responders map[string]JugglerResponder
}

func NewFakeJugglerTransport(t testing.TB) *FakeJugglerTransport {
	t.Helper()
	transport := &FakeJugglerTransport{
		t:          t,
		incoming:   make(chan *juggler.Message, 256),
		closed:     make(chan struct{}),
		responders: make(map[string]JugglerResponder),
	}
	t.Cleanup(func() { _ = transport.Close() })
	return transport
}

func (f *FakeJugglerTransport) Send(msg *juggler.Message) error {
	if f.isClosed() {
		return fmt.Errorf("transport closed")
	}

	params := append(json.RawMessage(nil), msg.Params...)
	call := JugglerCall{
		ID:        msg.ID,
		SessionID: msg.SessionID,
		Method:    msg.Method,
		Params:    params,
	}

	f.mu.Lock()
	f.calls = append(f.calls, call)
	responder := f.responders[msg.Method]
	f.mu.Unlock()

	result := json.RawMessage(`{}`)
	var jugglerErr *juggler.Error
	if responder != nil {
		result, jugglerErr = responder(msg)
	}

	return f.enqueue(&juggler.Message{
		ID:        msg.ID,
		Result:    append(json.RawMessage(nil), result...),
		Error:     jugglerErr,
		SessionID: msg.SessionID,
	})
}

func (f *FakeJugglerTransport) Receive() (*juggler.Message, error) {
	select {
	case <-f.closed:
		return nil, fmt.Errorf("transport closed")
	default:
	}

	select {
	case <-f.closed:
		return nil, fmt.Errorf("transport closed")
	case msg := <-f.incoming:
		return msg, nil
	}
}

func (f *FakeJugglerTransport) Close() error {
	f.once.Do(func() {
		f.mu.Lock()
		f.closedFlag = true
		f.mu.Unlock()
		close(f.closed)
	})
	return nil
}

func (f *FakeJugglerTransport) RespondJSON(method string, result any) {
	f.t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		f.t.Fatalf("marshal response for %s: %v", method, err)
	}
	f.RespondFunc(method, func(*juggler.Message) (json.RawMessage, *juggler.Error) {
		return append(json.RawMessage(nil), data...), nil
	})
}

func (f *FakeJugglerTransport) RespondError(method string, message string) {
	f.RespondFunc(method, func(*juggler.Message) (json.RawMessage, *juggler.Error) {
		return nil, &juggler.Error{Message: message}
	})
}

func (f *FakeJugglerTransport) RespondFunc(method string, responder JugglerResponder) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responders[method] = responder
}

func (f *FakeJugglerTransport) Calls() []JugglerCall {
	f.mu.RLock()
	defer f.mu.RUnlock()

	calls := make([]JugglerCall, len(f.calls))
	for i, call := range f.calls {
		calls[i] = copyJugglerCall(call)
	}
	return calls
}

func (f *FakeJugglerTransport) CallsByMethod(method string) []JugglerCall {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var calls []JugglerCall
	for _, call := range f.calls {
		if call.Method == method {
			calls = append(calls, copyJugglerCall(call))
		}
	}
	return calls
}

func (f *FakeJugglerTransport) LastCall(method string) (JugglerCall, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].Method == method {
			return copyJugglerCall(f.calls[i]), true
		}
	}
	return JugglerCall{}, false
}

func (f *FakeJugglerTransport) InjectEvent(sessionID string, method string, params any) {
	f.t.Helper()
	data, err := json.Marshal(params)
	if err != nil {
		f.t.Fatalf("marshal event params for %s: %v", method, err)
	}
	if err := f.enqueue(&juggler.Message{
		SessionID: sessionID,
		Method:    method,
		Params:    data,
	}); err != nil {
		f.t.Fatalf("inject event %s: %v", method, err)
	}
}

func ParamsAs[T any](t testing.TB, raw json.RawMessage) T {
	t.Helper()
	var params T
	if err := json.Unmarshal(raw, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	return params
}

func (f *FakeJugglerTransport) enqueue(msg *juggler.Message) error {
	select {
	case <-f.closed:
		return fmt.Errorf("transport closed")
	default:
	}

	select {
	case <-f.closed:
		return fmt.Errorf("transport closed")
	case f.incoming <- msg:
		return nil
	}
}

func (f *FakeJugglerTransport) isClosed() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.closedFlag
}

func copyJugglerCall(call JugglerCall) JugglerCall {
	call.Params = append(json.RawMessage(nil), call.Params...)
	return call
}
