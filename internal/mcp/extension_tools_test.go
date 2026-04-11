package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

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
	res, ok := handleExtensionTool(nil, name, raw)
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
	res := runExtTool(t, "vulpine_list_mobile_devices", map[string]interface{}{})
	assertUnavailable(t, res, "mobile bridge unavailable")
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
