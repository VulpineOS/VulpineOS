package juggler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// SecureSetInputValue invokes Page.secureSetInputValue on the given page
// session, writing the value into the element identified by objectId
// inside the given frame. The protocol-level handler does not fire
// keystroke events, so the value never appears in JavaScript event
// listeners.
//
// Returns the injection method used by the backend (e.g. "native-setter")
// and whether injection succeeded.
func (c *Client) SecureSetInputValue(ctx context.Context, sessionID, frameID, objectID, value string) (injected bool, method string, err error) {
	params := map[string]interface{}{
		"frameId":  frameID,
		"objectId": objectID,
		"value":    value,
	}
	raw, err := c.CallWithContext(ctx, sessionID, "Page.secureSetInputValue", params)
	if err != nil {
		return false, "", err
	}
	var resp struct {
		Injected bool   `json:"injected"`
		Method   string `json:"method"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return false, "", fmt.Errorf("secureSetInputValue: decode response: %w", err)
	}
	return resp.Injected, resp.Method, nil
}

// StartAudioCapture invokes Browser.startAudioCapture and returns the
// capture ID assigned by the backend. Format defaults to "pcm" when
// empty. SampleRate and Channels are passed through only when non-zero.
func (c *Client) StartAudioCapture(ctx context.Context, format string, sampleRate, channels int) (captureID string, err error) {
	if format == "" {
		format = "pcm"
	}
	params := map[string]interface{}{
		"format": format,
	}
	if sampleRate > 0 {
		params["sampleRate"] = sampleRate
	}
	if channels > 0 {
		params["channels"] = channels
	}
	raw, err := c.CallWithContext(ctx, "", "Browser.startAudioCapture", params)
	if err != nil {
		return "", err
	}
	var resp struct {
		CaptureID string `json:"captureId"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("startAudioCapture: decode response: %w", err)
	}
	return resp.CaptureID, nil
}

// StopAudioCapture invokes Browser.stopAudioCapture and returns the
// total duration and byte count of the finished capture.
func (c *Client) StopAudioCapture(ctx context.Context, captureID string) (durationMs int64, bytesRecorded int64, err error) {
	params := map[string]interface{}{
		"captureId": captureID,
	}
	raw, err := c.CallWithContext(ctx, "", "Browser.stopAudioCapture", params)
	if err != nil {
		return 0, 0, err
	}
	var resp struct {
		DurationMs    int64 `json:"durationMs"`
		BytesRecorded int64 `json:"bytesRecorded"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 0, 0, fmt.Errorf("stopAudioCapture: decode response: %w", err)
	}
	return resp.DurationMs, resp.BytesRecorded, nil
}

// GetAudioChunk invokes Browser.getAudioChunk and returns the next
// chunk of audio data along with an EOF flag. The wire format delivers
// audio data as a base64 string; this wrapper decodes it so callers
// receive raw bytes.
func (c *Client) GetAudioChunk(ctx context.Context, captureID string, maxBytes int) (data []byte, eof bool, err error) {
	params := map[string]interface{}{
		"captureId": captureID,
	}
	if maxBytes > 0 {
		params["maxBytes"] = maxBytes
	}
	raw, err := c.CallWithContext(ctx, "", "Browser.getAudioChunk", params)
	if err != nil {
		return nil, false, err
	}
	var resp struct {
		Data string `json:"data"`
		EOF  bool   `json:"eof"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, false, fmt.Errorf("getAudioChunk: decode response: %w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(resp.Data)
	if err != nil {
		return nil, false, fmt.Errorf("getAudioChunk: decode base64 payload: %w", err)
	}
	return decoded, resp.EOF, nil
}

// GetAnnotatedScreenshot invokes Page.getAnnotatedScreenshot and
// returns the annotated PNG as raw bytes plus the structured element
// map. The wire-level image field is base64; this wrapper decodes it
// so callers receive raw PNG data.
func (c *Client) GetAnnotatedScreenshot(ctx context.Context, sessionID, format string, maxElements int) (image []byte, elements []map[string]interface{}, err error) {
	params := map[string]interface{}{}
	if format != "" {
		params["format"] = format
	}
	if maxElements > 0 {
		params["maxElements"] = maxElements
	}
	raw, err := c.CallWithContext(ctx, sessionID, "Page.getAnnotatedScreenshot", params)
	if err != nil {
		return nil, nil, err
	}
	var resp struct {
		Image    string                   `json:"image"`
		Elements []map[string]interface{} `json:"elements"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, nil, fmt.Errorf("getAnnotatedScreenshot: decode response: %w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(resp.Image)
	if err != nil {
		return nil, nil, fmt.Errorf("getAnnotatedScreenshot: decode base64 image: %w", err)
	}
	return decoded, resp.Elements, nil
}
