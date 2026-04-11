package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"vulpineos/internal/extensions"
	"vulpineos/internal/juggler"
)

// extensionTools returns the MCP tool definitions backed by the
// extensions package (credentials, audio capture, mobile bridge, and
// annotated screenshots). Each tool returns a graceful "feature
// unavailable" error when no provider is registered for the backing
// feature.
func extensionTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "vulpine_annotated_screenshot",
			Description: "Capture a screenshot of the current page and return it as base64 PNG. Future: will overlay element labels when an extension provider is available.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId":   {Type: "string", Description: "Target page session ID"},
					"format":      {Type: "string", Description: "Image format (default: png)"},
					"maxElements": {Type: "number", Description: "Max labeled elements to return (future use)"},
				},
				Required: []string{"sessionId"},
			},
		},
		{
			Name:        "vulpine_get_credential",
			Description: "Look up stored credential metadata for a given site URL. Returns metadata only (never the password). Returns 'credential provider unavailable' when no provider is registered.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"site_url": {Type: "string", Description: "Site URL to look up credentials for"},
				},
				Required: []string{"site_url"},
			},
		},
		{
			Name:        "vulpine_autofill",
			Description: "Look up a credential for site_url and ask the registered credential provider to fill the username and password fields. Plaintext never crosses the tool boundary.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"site_url":          {Type: "string", Description: "Site URL to look up credentials for"},
					"page_id":           {Type: "string", Description: "Browser page identifier"},
					"frame_id":          {Type: "string", Description: "Optional frame identifier"},
					"username_selector": {Type: "string", Description: "CSS selector for the username field"},
					"password_selector": {Type: "string", Description: "CSS selector for the password field"},
				},
				Required: []string{"site_url", "page_id", "username_selector", "password_selector"},
			},
		},
		{
			Name:        "vulpine_start_audio_capture",
			Description: "Start an audio capture session. Returns the capture handle ID. Requires a registered audio capture provider.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"format":      {Type: "string", Description: "Audio format (pcm, opus, wav)"},
					"sample_rate": {Type: "number", Description: "Sample rate in Hz (e.g. 48000)"},
					"channels":    {Type: "number", Description: "Channel count (1=mono, 2=stereo)"},
				},
			},
		},
		{
			Name:        "vulpine_stop_audio_capture",
			Description: "Stop an active audio capture session.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"handle_id": {Type: "string", Description: "Capture handle ID from vulpine_start_audio_capture"},
				},
				Required: []string{"handle_id"},
			},
		},
		{
			Name:        "vulpine_read_audio_chunk",
			Description: "Read a chunk of data from an active audio capture session. Returns base64-encoded bytes and an EOF flag.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"handle_id": {Type: "string", Description: "Capture handle ID"},
					"max_bytes": {Type: "number", Description: "Max bytes to read (default 65536)"},
				},
				Required: []string{"handle_id"},
			},
		},
		{
			Name:        "vulpine_click_label",
			Description: "Click an element previously labeled by vulpine_annotated_screenshot. The label is the `@N` identifier (or backend-supplied name) returned alongside the annotated PNG. Requires a prior annotated screenshot in the same session.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"session_id": {Type: "string", Description: "Target page session ID"},
					"label":      {Type: "string", Description: "Label identifier (e.g. @3)"},
				},
				Required: []string{"session_id", "label"},
			},
		},
		{
			Name:        "vulpine_list_mobile_devices",
			Description: "List mobile devices visible to the registered mobile bridge provider. Returns an empty list / unavailable error when no provider is registered.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
			},
		},
	}
}

// handleExtensionTool dispatches an extension-backed tool call. Returns
// (nil, false) if the name is not recognized so the caller can fall
// through to other handlers. The ctx argument is threaded through to
// every provider method so call-scoped deadlines, cancellation, and
// test sentinels propagate into extension backends.
func handleExtensionTool(ctx context.Context, client *juggler.Client, name string, args json.RawMessage) (*ToolCallResult, bool) {
	switch name {
	case "vulpine_annotated_screenshot":
		return handleAnnotatedScreenshot(ctx, client, args), true
	case "vulpine_get_credential":
		return handleGetCredential(ctx, args), true
	case "vulpine_autofill":
		return handleAutofill(ctx, client, args), true
	case "vulpine_start_audio_capture":
		return handleStartAudioCapture(ctx, args), true
	case "vulpine_stop_audio_capture":
		return handleStopAudioCapture(ctx, args), true
	case "vulpine_read_audio_chunk":
		return handleReadAudioChunk(ctx, args), true
	case "vulpine_list_mobile_devices":
		return handleListMobileDevices(ctx, args), true
	case "vulpine_click_label":
		return handleClickLabel(ctx, client, args), true
	}
	return nil, false
}

// handleAnnotatedScreenshot first tries the Juggler binding
// Page.getAnnotatedScreenshot, which returns the PNG plus a structured
// element list. On success the element list is stashed into the
// per-session label index so that vulpine_click_label can look up an
// objectID by label afterwards. On any failure (including "method not
// found" from a backend that doesn't implement the protocol yet) it
// falls back to a plain Page.screenshot so the tool degrades
// gracefully.
func handleAnnotatedScreenshot(ctx context.Context, client *juggler.Client, args json.RawMessage) *ToolCallResult {
	var p struct {
		SessionID   string `json:"sessionId"`
		Format      string `json:"format"`
		MaxElements int    `json:"maxElements"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err)
	}
	if client == nil {
		return errorResult(fmt.Errorf("annotated screenshot: juggler client unavailable"))
	}

	// Attempt the real annotated screenshot binding first.
	if img, elements, err := client.GetAnnotatedScreenshot(ctx, p.SessionID, p.Format, p.MaxElements); err == nil {
		globalLabels.Set(p.SessionID, elements)
		b64 := base64.StdEncoding.EncodeToString(img)
		elementsJSON, _ := json.Marshal(elements)
		return &ToolCallResult{
			Content: []ContentBlock{
				{Type: "image", Data: b64, MimeType: "image/png"},
				{Type: "text", Text: string(elementsJSON)},
			},
		}
	}

	// Fallback: plain Page.screenshot.
	result, err := client.Call(p.SessionID, "Page.screenshot", map[string]interface{}{
		"mimeType": "image/png",
		"clip":     map[string]interface{}{"x": 0, "y": 0, "width": 1280, "height": 720},
	})
	if err != nil {
		return errorResult(err)
	}
	var shot struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(result, &shot); err != nil {
		return errorResult(err)
	}
	return &ToolCallResult{
		Content: []ContentBlock{{
			Type:     "image",
			Data:     shot.Data,
			MimeType: "image/png",
		}},
	}
}

// handleClickLabel looks up an objectID in the per-session label index
// and clicks it via Page.click. Returns an error when the label is
// unknown (e.g. no annotated screenshot has been taken in this session
// yet) or when the underlying click call fails.
func handleClickLabel(ctx context.Context, client *juggler.Client, args json.RawMessage) *ToolCallResult {
	_ = ctx // reserved for future cancellation of Page.click
	var p struct {
		SessionID string `json:"session_id"`
		Label     string `json:"label"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err)
	}
	if client == nil {
		return errorResult(fmt.Errorf("click_label: juggler client unavailable"))
	}
	if p.SessionID == "" || p.Label == "" {
		return errorResult(fmt.Errorf("click_label: session_id and label are required"))
	}
	objectID, ok := globalLabels.Get(p.SessionID, p.Label)
	if !ok {
		return errorResult(fmt.Errorf("click_label: label %q not found for session %q (take an annotated screenshot first)", p.Label, p.SessionID))
	}
	if _, err := client.Call(p.SessionID, "Page.click", map[string]interface{}{
		"objectId": objectID,
	}); err != nil {
		return errorResult(fmt.Errorf("click_label: Page.click %q: %w", objectID, err))
	}
	return textResult(fmt.Sprintf("clicked label %s (objectId=%s)", p.Label, objectID))
}

func handleGetCredential(ctx context.Context, args json.RawMessage) *ToolCallResult {
	var p struct {
		SiteURL string `json:"site_url"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err)
	}
	provider := extensions.Registry.Credentials()
	if provider == nil || !provider.Available() {
		return errorResult(fmt.Errorf("credential provider unavailable"))
	}
	cred, err := provider.Lookup(ctx, p.SiteURL)
	if err != nil {
		return errorResult(err)
	}
	if cred == nil {
		return textResult("{}")
	}
	meta := map[string]interface{}{
		"id":       cred.ID,
		"site":     cred.Site,
		"username": cred.Username,
		"hasTOTP":  cred.HasTOTP,
		"notes":    cred.Notes,
	}
	b, _ := json.Marshal(meta)
	return textResult(string(b))
}

func handleAutofill(ctx context.Context, client *juggler.Client, args json.RawMessage) *ToolCallResult {
	_ = client // reserved for selector-value fallback path
	var p struct {
		SiteURL          string `json:"site_url"`
		PageID           string `json:"page_id"`
		FrameID          string `json:"frame_id"`
		UsernameSelector string `json:"username_selector"`
		PasswordSelector string `json:"password_selector"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err)
	}
	provider := extensions.Registry.Credentials()
	if provider == nil || !provider.Available() {
		return errorResult(fmt.Errorf("credential provider unavailable"))
	}
	cred, err := provider.Lookup(ctx, p.SiteURL)
	if err != nil {
		return errorResult(err)
	}
	if cred == nil {
		return errorResult(fmt.Errorf("no credential found for %s", p.SiteURL))
	}
	if err := provider.Fill(ctx, cred.ID, extensions.FillTarget{
		PageID:   p.PageID,
		FrameID:  p.FrameID,
		Selector: p.UsernameSelector,
		Field:    "username",
	}); err != nil {
		return errorResult(fmt.Errorf("fill username: %w", err))
	}
	if err := provider.Fill(ctx, cred.ID, extensions.FillTarget{
		PageID:   p.PageID,
		FrameID:  p.FrameID,
		Selector: p.PasswordSelector,
		Field:    "password",
	}); err != nil {
		return errorResult(fmt.Errorf("fill password: %w", err))
	}
	return textResult(fmt.Sprintf("autofilled credential %s", cred.ID))
}

func handleStartAudioCapture(ctx context.Context, args json.RawMessage) *ToolCallResult {
	var p struct {
		Format     string `json:"format"`
		SampleRate int    `json:"sample_rate"`
		Channels   int    `json:"channels"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err)
	}
	// Apply defaults for unset fields. 16kHz mono PCM is a reasonable
	// baseline for speech capture and keeps backend implementations
	// from having to special-case zero values.
	if p.Format == "" {
		p.Format = "pcm"
	}
	if p.SampleRate == 0 {
		p.SampleRate = 16000
	}
	if p.Channels == 0 {
		p.Channels = 1
	}
	cap := extensions.Registry.Audio()
	if cap == nil || !cap.Available() {
		return errorResult(fmt.Errorf("audio capture unavailable"))
	}
	handle, err := cap.Start(ctx, extensions.CaptureRequest{
		Format:     p.Format,
		SampleRate: p.SampleRate,
		Channels:   p.Channels,
	})
	if err != nil {
		return errorResult(err)
	}
	return textResult(fmt.Sprintf(`{"handle_id":%q,"format":%q}`, handle.ID, handle.Format))
}

func handleStopAudioCapture(ctx context.Context, args json.RawMessage) *ToolCallResult {
	var p struct {
		HandleID string `json:"handle_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err)
	}
	cap := extensions.Registry.Audio()
	if cap == nil || !cap.Available() {
		return errorResult(fmt.Errorf("audio capture unavailable"))
	}
	if err := cap.Stop(ctx, p.HandleID); err != nil {
		return errorResult(err)
	}
	return textResult("stopped")
}

func handleReadAudioChunk(ctx context.Context, args json.RawMessage) *ToolCallResult {
	var p struct {
		HandleID string `json:"handle_id"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err)
	}
	if p.MaxBytes <= 0 {
		p.MaxBytes = 65536
	}
	cap := extensions.Registry.Audio()
	if cap == nil || !cap.Available() {
		return errorResult(fmt.Errorf("audio capture unavailable"))
	}
	chunk, eof, err := cap.Read(ctx, p.HandleID, p.MaxBytes)
	if err != nil {
		return errorResult(err)
	}
	payload := map[string]interface{}{
		"data": base64.StdEncoding.EncodeToString(chunk),
		"eof":  eof,
	}
	b, _ := json.Marshal(payload)
	return textResult(string(b))
}

func handleListMobileDevices(ctx context.Context, args json.RawMessage) *ToolCallResult {
	_ = args
	m := extensions.Registry.Mobile()
	if m == nil || !m.Available() {
		return errorResult(fmt.Errorf("mobile bridge unavailable"))
	}
	devices, err := m.ListDevices(ctx)
	if err != nil {
		return errorResult(err)
	}
	b, _ := json.Marshal(devices)
	return textResult(string(b))
}
