package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"vulpineos/internal/extensions/extensionstest"
	"vulpineos/internal/juggler"
)

// routedTransport is a minimal in-process juggler.Transport for tests.
// Each outgoing request is dispatched to the handler registered for
// its method; the handler returns the Result/Error that should be fed
// back to the client. This lets us drive mcp handlers that talk to a
// real juggler.Client without spinning up Firefox.
type routedTransport struct {
	mu       sync.Mutex
	handlers map[string]func(req *juggler.Message) *juggler.Message
	incoming chan *juggler.Message
	closed   chan struct{}
	once     sync.Once
}

func newRoutedTransport() *routedTransport {
	return &routedTransport{
		handlers: map[string]func(req *juggler.Message) *juggler.Message{},
		incoming: make(chan *juggler.Message, 64),
		closed:   make(chan struct{}),
	}
}

func (r *routedTransport) on(method string, fn func(req *juggler.Message) *juggler.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[method] = fn
}

func (r *routedTransport) Send(msg *juggler.Message) error {
	select {
	case <-r.closed:
		return fmt.Errorf("transport closed")
	default:
	}
	r.mu.Lock()
	fn, ok := r.handlers[msg.Method]
	r.mu.Unlock()
	var resp *juggler.Message
	if ok {
		resp = fn(msg)
	} else {
		resp = &juggler.Message{Error: &juggler.Error{Message: "no handler for " + msg.Method}}
	}
	if resp == nil {
		return nil
	}
	resp.ID = msg.ID
	select {
	case r.incoming <- resp:
	case <-r.closed:
	}
	return nil
}

func (r *routedTransport) Receive() (*juggler.Message, error) {
	select {
	case <-r.closed:
		return nil, fmt.Errorf("transport closed")
	case m := <-r.incoming:
		return m, nil
	}
}

func (r *routedTransport) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

func okResultMessage(t *testing.T, v interface{}) *juggler.Message {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return &juggler.Message{Result: raw}
}

// TestExtensionToolsRegistered verifies that the tools() list exposes
// every extension-backed tool by name so the MCP server advertises them.
func TestExtensionToolsRegistered(t *testing.T) {
	want := []string{
		"vulpine_annotated_screenshot",
		"vulpine_get_credential",
		"vulpine_autofill",
		"vulpine_start_audio_capture",
		"vulpine_stop_audio_capture",
		"vulpine_read_audio_chunk",
		"vulpine_list_mobile_devices",
		"vulpine_click_label",
	}
	got := map[string]bool{}
	for _, tool := range tools() {
		got[tool.Name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("tools() missing %q", name)
		}
	}
}

// runExtTool dispatches through handleExtensionTool and returns the
// result text for assertion. client is nil since the default provider
// path never reaches Juggler.
func runExtTool(t *testing.T, name string, args map[string]interface{}) *ToolCallResult {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	res, ok := handleExtensionTool(context.Background(), nil, name, raw)
	if !ok {
		t.Fatalf("handleExtensionTool: %q not dispatched", name)
	}
	if res == nil {
		t.Fatalf("handleExtensionTool: nil result for %q", name)
	}
	return res
}

func assertUnavailable(t *testing.T, res *ToolCallResult, wantSubstr string) {
	t.Helper()
	if !res.IsError {
		t.Fatalf("expected IsError, got success: %+v", res)
	}
	if len(res.Content) == 0 {
		t.Fatalf("expected content, got empty")
	}
	text := res.Content[0].Text
	if !strings.Contains(text, wantSubstr) {
		t.Fatalf("expected error containing %q, got %q", wantSubstr, text)
	}
}

// TestExtensionTools_NilArgsAccepted ensures every extension handler
// tolerates both a nil args payload and a literal JSON null, because
// some MCP clients omit the arguments field entirely or serialize it
// as null for zero-arg tools. The handler should route to the normal
// "unavailable" path (which is what the default stub providers
// return) rather than reporting a parse error up the JSON-RPC stack.
func TestExtensionTools_NilArgsAccepted(t *testing.T) {
	withFakeMobile(t, &extensionstest.FakeMobileBridge{})

	type kase struct {
		name      string
		args      json.RawMessage
		wantParse bool // true if we expect to see a parse error leak through
	}
	names := []string{
		"vulpine_annotated_screenshot",
		"vulpine_get_credential",
		"vulpine_autofill",
		"vulpine_start_audio_capture",
		"vulpine_stop_audio_capture",
		"vulpine_read_audio_chunk",
		"vulpine_list_mobile_devices",
		"vulpine_click_label",
	}
	inputs := []json.RawMessage{nil, json.RawMessage("null")}
	for _, n := range names {
		for _, in := range inputs {
			res, ok := handleExtensionTool(context.Background(), nil, n, in)
			if !ok {
				t.Errorf("%s: not dispatched", n)
				continue
			}
			if res == nil {
				t.Errorf("%s: nil result", n)
				continue
			}
			if len(res.Content) > 0 {
				txt := res.Content[0].Text
				if strings.Contains(txt, "unexpected end of JSON") ||
					strings.Contains(txt, "cannot unmarshal") {
					t.Errorf("%s args=%q: parse error leaked: %q", n, string(in), txt)
				}
			}
		}
	}
}

func TestGetCredentialUnavailable(t *testing.T) {
	res := runExtTool(t, "vulpine_get_credential", map[string]interface{}{
		"site_url": "https://example.com",
	})
	assertUnavailable(t, res, "credential provider unavailable")
}

func TestAutofillUnavailable(t *testing.T) {
	res := runExtTool(t, "vulpine_autofill", map[string]interface{}{
		"site_url":          "https://example.com",
		"page_id":           "page-1",
		"username_selector": "#user",
		"password_selector": "#pass",
	})
	assertUnavailable(t, res, "credential provider unavailable")
}

func TestStartAudioCaptureUnavailable(t *testing.T) {
	res := runExtTool(t, "vulpine_start_audio_capture", map[string]interface{}{
		"format":      "pcm",
		"sample_rate": 48000,
		"channels":    2,
	})
	assertUnavailable(t, res, "audio capture unavailable")
}

func TestStopAudioCaptureUnavailable(t *testing.T) {
	res := runExtTool(t, "vulpine_stop_audio_capture", map[string]interface{}{
		"handle_id": "h1",
	})
	assertUnavailable(t, res, "audio capture unavailable")
}

func TestReadAudioChunkUnavailable(t *testing.T) {
	res := runExtTool(t, "vulpine_read_audio_chunk", map[string]interface{}{
		"handle_id": "h1",
		"max_bytes": 1024,
	})
	assertUnavailable(t, res, "audio capture unavailable")
}

func TestListMobileDevicesUnavailable(t *testing.T) {
	withFakeMobile(t, &extensionstest.FakeMobileBridge{})

	res := runExtTool(t, "vulpine_list_mobile_devices", map[string]interface{}{})
	assertUnavailable(t, res, "mobile bridge unavailable")
}

// TestHandleAnnotatedScreenshot_FallbackToPageScreenshot verifies the
// fallback path: when Page.getAnnotatedScreenshot errors, the handler
// must degrade to a plain Page.screenshot capture, return exactly one
// image content block (no element text block), and leave the
// per-session label index untouched so stale click_label attempts
// fail instead of resolving to random objectIDs from a prior page.
func TestHandleAnnotatedScreenshot_FallbackToPageScreenshot(t *testing.T) {
	const sessionID = "fallback-sess"
	// Make sure there are no stale labels for this session from
	// any previously-run test.
	globalLabels.Clear(sessionID)

	rt := newRoutedTransport()
	rt.on("Page.getAnnotatedScreenshot", func(req *juggler.Message) *juggler.Message {
		return &juggler.Message{Error: &juggler.Error{Message: "method not implemented"}}
	})
	fakePNG := []byte("fake-png-bytes")
	rt.on("Page.screenshot", func(req *juggler.Message) *juggler.Message {
		return okResultMessage(t, map[string]interface{}{
			"data": base64.StdEncoding.EncodeToString(fakePNG),
		})
	})
	client := juggler.NewClient(rt)
	defer client.Close()

	args, _ := json.Marshal(map[string]interface{}{"sessionId": sessionID})
	res, ok := handleExtensionTool(context.Background(), client, "vulpine_annotated_screenshot", args)
	if !ok {
		t.Fatal("vulpine_annotated_screenshot not dispatched")
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if res.IsError {
		t.Fatalf("fallback path should succeed, got error: %+v", res.Content)
	}
	if len(res.Content) != 1 {
		t.Fatalf("fallback should return exactly 1 content block, got %d: %+v", len(res.Content), res.Content)
	}
	if res.Content[0].Type != "image" {
		t.Errorf("want image content block, got type=%q", res.Content[0].Type)
	}
	if res.Content[0].MimeType != "image/png" {
		t.Errorf("want image/png, got %q", res.Content[0].MimeType)
	}
	// Decode and compare to ensure the fallback returned the
	// Page.screenshot payload, not something stale.
	decoded, err := base64.StdEncoding.DecodeString(res.Content[0].Data)
	if err != nil {
		t.Fatalf("decode fallback image: %v", err)
	}
	if string(decoded) != string(fakePNG) {
		t.Errorf("fallback image mismatch: got %q want %q", decoded, fakePNG)
	}
	// Label index must NOT have been populated on the fallback path.
	if _, ok := globalLabels.Get(sessionID, "@1"); ok {
		t.Error("fallback path must not populate label index")
	}
}

// TestAnnotatedScreenshotRequiresClient verifies the tool gracefully
// errors out when no juggler client is available (nil client path).
// With a real client the tool would capture a PNG via Page.screenshot.
func TestAnnotatedScreenshotNilClient(t *testing.T) {
	res := runExtTool(t, "vulpine_annotated_screenshot", map[string]interface{}{
		"sessionId": "s1",
	})
	if !res.IsError {
		t.Fatalf("expected error for nil client, got success")
	}
	if !strings.Contains(res.Content[0].Text, "juggler client unavailable") {
		t.Fatalf("unexpected error: %q", res.Content[0].Text)
	}
}
