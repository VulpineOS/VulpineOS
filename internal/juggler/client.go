package juggler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// EventHandler is called when a Juggler event is received.
// sessionID identifies which page session the event belongs to (empty for browser events).
type EventHandler func(sessionID string, params json.RawMessage)

type eventSubscription struct {
	id      int64
	handler EventHandler
}

// Client is a high-level Juggler protocol client.
type Client struct {
	transport     Transport
	nextID        atomic.Int64
	nextHandlerID atomic.Int64
	pending       map[int]chan *Message
	pendingMu     sync.Mutex
	handlers      map[string][]eventSubscription
	handlerMu     sync.RWMutex
	done          chan struct{}
	closeOnce     sync.Once
}

// NewClient creates a Juggler client using the given transport.
func NewClient(transport Transport) *Client {
	c := &Client{
		transport: transport,
		pending:   make(map[int]chan *Message),
		handlers:  make(map[string][]eventSubscription),
		done:      make(chan struct{}),
	}
	go c.readLoop()
	return c
}

// DefaultCallTimeout is the default timeout for Call().
const DefaultCallTimeout = 30 * time.Second

// Call sends a synchronous RPC request and waits for the response with a default 30-second timeout.
func (c *Client) Call(sessionID, method string, params interface{}) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultCallTimeout)
	defer cancel()
	return c.CallWithContext(ctx, sessionID, method, params)
}

// CallWithContext sends a synchronous RPC request and waits for the response, respecting the given context.
func (c *Client) CallWithContext(ctx context.Context, sessionID, method string, params interface{}) (json.RawMessage, error) {
	id := int(c.nextID.Add(1))

	var rawParams json.RawMessage
	if params != nil {
		var err error
		rawParams, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
	}

	msg := &Message{
		ID:        id,
		Method:    method,
		Params:    rawParams,
		SessionID: sessionID,
	}

	ch := make(chan *Message, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.transport.Send(msg); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("call %s: %w", method, ctx.Err())
	case <-c.done:
		return nil, fmt.Errorf("client closed")
	}
}

// Subscribe registers a handler for a Juggler event.
func (c *Client) Subscribe(event string, handler EventHandler) {
	c.SubscribeWithCancel(event, handler)
}

// SubscribeWithCancel registers a handler and returns a cancellation function.
func (c *Client) SubscribeWithCancel(event string, handler EventHandler) func() {
	id := c.nextHandlerID.Add(1)
	c.handlerMu.Lock()
	c.handlers[event] = append(c.handlers[event], eventSubscription{id: id, handler: handler})
	c.handlerMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			c.handlerMu.Lock()
			defer c.handlerMu.Unlock()
			subs := c.handlers[event]
			for i, sub := range subs {
				if sub.id == id {
					c.handlers[event] = append(subs[:i], subs[i+1:]...)
					if len(c.handlers[event]) == 0 {
						delete(c.handlers, event)
					}
					return
				}
			}
		})
	}
}

// Close shuts down the client and transport.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
	})
	return c.transport.Close()
}

func (c *Client) readLoop() {
	for {
		msg, err := c.transport.Receive()
		if err != nil {
			select {
			case <-c.done:
				return
			default:
				// Transport error — close client
				c.Close()
				return
			}
		}

		if msg.IsResponse() {
			c.pendingMu.Lock()
			ch, ok := c.pending[msg.ID]
			if ok {
				delete(c.pending, msg.ID)
			}
			c.pendingMu.Unlock()
			if ok {
				ch <- msg
			}
		} else if msg.IsEvent() {
			c.handlerMu.RLock()
			subs := append([]eventSubscription(nil), c.handlers[msg.Method]...)
			c.handlerMu.RUnlock()
			for _, sub := range subs {
				sub.handler(msg.SessionID, msg.Params)
			}
		}
	}
}
