package remote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nhooyr.io/websocket"

	"vulpineos/internal/juggler"
)

func TestClientConcurrentSendSerializesWebsocketWrites(t *testing.T) {
	const messageCount = 25

	received := make(chan []int, 1)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		ids := make([]int, 0, messageCount)
		for len(ids) < messageCount {
			_, data, err := conn.Read(ctx)
			if err != nil {
				t.Errorf("read websocket: %v", err)
				return
			}

			var env Envelope
			if err := json.Unmarshal(data, &env); err != nil {
				t.Errorf("unmarshal envelope: %v", err)
				return
			}
			if env.Type != "juggler" {
				t.Errorf("unexpected envelope type %q", env.Type)
				return
			}

			var msg juggler.Message
			if err := json.Unmarshal(env.Payload, &msg); err != nil {
				t.Errorf("unmarshal payload: %v", err)
				return
			}
			ids = append(ids, msg.ID)
		}
		received <- ids
	}))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	client, err := Dial(ctx, wsURL, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	var wg sync.WaitGroup
	errs := make(chan error, messageCount)
	for i := 1; i <= messageCount; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- client.Send(&juggler.Message{ID: i, Method: "Browser.enable"})
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	select {
	case ids := <-received:
		if len(ids) != messageCount {
			t.Fatalf("received %d messages, want %d", len(ids), messageCount)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for server to receive concurrent sends")
	}
}
