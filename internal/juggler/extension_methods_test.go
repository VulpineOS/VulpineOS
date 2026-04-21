package juggler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// routeResponder reads outgoing messages and replies using a
// per-method handler. It lets extension-method tests match on both the
// request's method name and the params that were sent.
type routeResponder struct {
	t        *testing.T
	mt       *memTransport
	handlers map[string]func(req *Message) *Message
}

func newRouteResponder(t *testing.T, mt *memTransport) *routeResponder {
	return &routeResponder{t: t, mt: mt, handlers: map[string]func(req *Message) *Message{}}
}

func (r *routeResponder) on(method string, fn func(req *Message) *Message) {
	r.handlers[method] = fn
}

func (r *routeResponder) run() {
	go func() {
		for {
			select {
			case <-r.mt.closed:
				return
			case req := <-r.mt.outgoing:
				fn, ok := r.handlers[req.Method]
				if !ok {
					// unknown method: return a generic error
					r.mt.incoming <- &Message{ID: req.ID, Error: &Error{Message: "no handler for " + req.Method}}
					continue
				}
				resp := fn(req)
				if resp == nil {
					continue
				}
				resp.ID = req.ID
				r.mt.incoming <- resp
			}
		}
	}()
}

// testErr is a shorthand for constructing an RPC error response.
func testErr(msg string) *Error { return &Error{Message: msg} }

func okResult(t *testing.T, v interface{}) *Message {
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &Message{Result: raw}
}

func TestSecureSetInputValue_HappyPath(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Page.secureSetInputValue", func(req *Message) *Message {
		var p struct {
			FrameID  string `json:"frameId"`
			ObjectID string `json:"objectId"`
			Value    string `json:"value"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			t.Errorf("unmarshal params: %v", err)
		}
		if p.FrameID != "frame-1" || p.ObjectID != "obj-42" || p.Value != "s3cret" {
			t.Errorf("unexpected params: %+v", p)
		}
		if req.SessionID != "sess-1" {
			t.Errorf("expected sessionID=sess-1, got %q", req.SessionID)
		}
		return okResult(t, map[string]interface{}{"injected": true, "method": "native-setter"})
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	injected, method, err := c.SecureSetInputValue(ctx, "sess-1", "frame-1", "obj-42", "s3cret")
	if err != nil {
		t.Fatalf("SecureSetInputValue: %v", err)
	}
	if !injected || method != "native-setter" {
		t.Errorf("unexpected result: injected=%v method=%q", injected, method)
	}
}

func TestSecureSetInputValue_ProtocolError(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Page.secureSetInputValue", func(req *Message) *Message {
		return &Message{Error: testErr("element not found")}
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := c.SecureSetInputValue(ctx, "s", "f", "o", "v")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "element not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSecureSetInputValueBySelector_HappyPath(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Runtime.evaluate", func(req *Message) *Message {
		var p struct {
			Expression    string `json:"expression"`
			FrameID       string `json:"frameId"`
			ReturnByValue bool   `json:"returnByValue"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			t.Errorf("unmarshal evaluate params: %v", err)
		}
		if !strings.Contains(p.Expression, `document.querySelector("#user")`) {
			t.Errorf("expression missing selector: %q", p.Expression)
		}
		if p.FrameID != "frame-1" {
			t.Errorf("expected frameId=frame-1, got %q", p.FrameID)
		}
		if p.ReturnByValue {
			t.Errorf("expected returnByValue=false")
		}
		return okResult(t, map[string]interface{}{
			"result": map[string]interface{}{"objectId": "obj-77"},
		})
	})
	r.on("Page.secureSetInputValue", func(req *Message) *Message {
		var p struct {
			FrameID  string `json:"frameId"`
			ObjectID string `json:"objectId"`
			Value    string `json:"value"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			t.Errorf("unmarshal secure params: %v", err)
		}
		if p.ObjectID != "obj-77" || p.Value != "s3cret" || p.FrameID != "frame-1" {
			t.Errorf("unexpected params: %+v", p)
		}
		return okResult(t, map[string]interface{}{"injected": true, "method": "native-setter"})
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.SecureSetInputValueBySelector(ctx, "sess-1", "frame-1", "#user", "s3cret"); err != nil {
		t.Fatalf("SecureSetInputValueBySelector: %v", err)
	}
}

func TestSecureSetInputValueBySelector_NotFound(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Runtime.evaluate", func(req *Message) *Message {
		return okResult(t, map[string]interface{}{
			"result": map[string]interface{}{"subtype": "null"},
		})
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := c.SecureSetInputValueBySelector(ctx, "sess-1", "frame-1", "#missing", "x")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), `selector "#missing" not found`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSecureSetInputValueBySelector_EvaluateError(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Runtime.evaluate", func(req *Message) *Message {
		return &Message{Error: testErr("execution context destroyed")}
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := c.SecureSetInputValueBySelector(ctx, "s", "f", "#user", "v")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Runtime.evaluate failed") {
		t.Errorf("expected wrapped Runtime.evaluate error, got: %v", err)
	}
}

func TestStartAudioCapture_HappyPath(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Browser.startAudioCapture", func(req *Message) *Message {
		var p map[string]interface{}
		_ = json.Unmarshal(req.Params, &p)
		if p["format"] != "pcm" {
			t.Errorf("expected format=pcm, got %v", p["format"])
		}
		if p["sampleRate"].(float64) != 48000 {
			t.Errorf("expected sampleRate=48000, got %v", p["sampleRate"])
		}
		return okResult(t, map[string]interface{}{"captureId": "cap-123"})
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	ctx := context.Background()
	id, err := c.StartAudioCapture(ctx, "", 48000, 2)
	if err != nil {
		t.Fatalf("StartAudioCapture: %v", err)
	}
	if id != "cap-123" {
		t.Errorf("expected cap-123, got %q", id)
	}
}

func TestStartAudioCapture_Error(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Browser.startAudioCapture", func(req *Message) *Message {
		return &Message{Error: testErr("audio capture disabled")}
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	_, err := c.StartAudioCapture(context.Background(), "pcm", 0, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStopAudioCapture_HappyPath(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Browser.stopAudioCapture", func(req *Message) *Message {
		return okResult(t, map[string]interface{}{"durationMs": 1500, "bytesRecorded": 96000})
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	dur, n, err := c.StopAudioCapture(context.Background(), "cap-1")
	if err != nil {
		t.Fatalf("StopAudioCapture: %v", err)
	}
	if dur != 1500 || n != 96000 {
		t.Errorf("unexpected dur=%d bytes=%d", dur, n)
	}
}

func TestStopAudioCapture_Error(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Browser.stopAudioCapture", func(req *Message) *Message {
		return &Message{Error: testErr("unknown captureId")}
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	_, _, err := c.StopAudioCapture(context.Background(), "bad")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetAudioChunk_DecodesBase64(t *testing.T) {
	payload := []byte{0x00, 0x11, 0x22, 0x33}
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Browser.getAudioChunk", func(req *Message) *Message {
		return okResult(t, map[string]interface{}{
			"data": base64.StdEncoding.EncodeToString(payload),
			"eof":  false,
		})
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	data, eof, err := c.GetAudioChunk(context.Background(), "cap-1", 1024)
	if err != nil {
		t.Fatalf("GetAudioChunk: %v", err)
	}
	if eof {
		t.Error("expected eof=false")
	}
	if !bytes.Equal(data, payload) {
		t.Errorf("expected %v, got %v", payload, data)
	}
}

func TestGetAudioChunk_InvalidBase64(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Browser.getAudioChunk", func(req *Message) *Message {
		return okResult(t, map[string]interface{}{"data": "not!valid!b64!", "eof": true})
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	_, _, err := c.GetAudioChunk(context.Background(), "cap-1", 0)
	if err == nil {
		t.Fatal("expected base64 decode error, got nil")
	}
}

func TestGetAnnotatedScreenshot_HappyPath(t *testing.T) {
	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Page.getAnnotatedScreenshot", func(req *Message) *Message {
		return okResult(t, map[string]interface{}{
			"image": base64.StdEncoding.EncodeToString(pngBytes),
			"elements": []map[string]interface{}{
				{"label": "button", "x": 10.0, "y": 20.0},
			},
		})
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	img, elements, err := c.GetAnnotatedScreenshot(context.Background(), "sess-1", "png", 50)
	if err != nil {
		t.Fatalf("GetAnnotatedScreenshot: %v", err)
	}
	if !bytes.Equal(img, pngBytes) {
		t.Errorf("png mismatch")
	}
	if len(elements) != 1 || elements[0]["label"] != "button" {
		t.Errorf("unexpected elements: %+v", elements)
	}
}

func TestGetAnnotatedScreenshot_Error(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Page.getAnnotatedScreenshot", func(req *Message) *Message {
		return &Message{Error: testErr("screenshot failed")}
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	_, _, err := c.GetAnnotatedScreenshot(context.Background(), "s", "", 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClickByObjectID_HappyPath(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Page.scrollIntoViewIfNeeded", func(req *Message) *Message {
		var p struct {
			FrameID  string `json:"frameId"`
			ObjectID string `json:"objectId"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			t.Fatalf("unmarshal scroll params: %v", err)
		}
		if p.FrameID != "frame-1" || p.ObjectID != "obj-1" {
			t.Fatalf("unexpected scroll params: %+v", p)
		}
		return okResult(t, map[string]interface{}{})
	})
	r.on("Page.getContentQuads", func(req *Message) *Message {
		return okResult(t, map[string]interface{}{
			"quads": []map[string]interface{}{
				{
					"p1": map[string]float64{"x": 10, "y": 20},
					"p2": map[string]float64{"x": 30, "y": 20},
					"p3": map[string]float64{"x": 30, "y": 40},
					"p4": map[string]float64{"x": 10, "y": 40},
				},
			},
		})
	})
	var dispatches []map[string]interface{}
	r.on("Page.dispatchMouseEvent", func(req *Message) *Message {
		var p map[string]interface{}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			t.Fatalf("unmarshal mouse params: %v", err)
		}
		dispatches = append(dispatches, p)
		return okResult(t, map[string]interface{}{})
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.ClickByObjectID(ctx, "sess-1", "frame-1", "obj-1"); err != nil {
		t.Fatalf("ClickByObjectID: %v", err)
	}
	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2", len(dispatches))
	}
	if dispatches[0]["type"] != "mousedown" || dispatches[1]["type"] != "mouseup" {
		t.Fatalf("unexpected dispatches: %#v", dispatches)
	}
	if dispatches[0]["x"] != 20.0 || dispatches[0]["y"] != 30.0 {
		t.Fatalf("unexpected click coordinates: %#v", dispatches[0])
	}
}

func TestClickByObjectID_NoQuads(t *testing.T) {
	mt := newMemTransport()
	r := newRouteResponder(t, mt)
	r.on("Page.scrollIntoViewIfNeeded", func(req *Message) *Message {
		return okResult(t, map[string]interface{}{})
	})
	r.on("Page.getContentQuads", func(req *Message) *Message {
		return okResult(t, map[string]interface{}{"quads": []map[string]interface{}{}})
	})
	r.run()

	c := NewClient(mt)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := c.ClickByObjectID(ctx, "sess-1", "frame-1", "obj-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no quads returned") {
		t.Fatalf("unexpected error: %v", err)
	}
}
