package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"vulpineos/internal/extensions"
	"vulpineos/internal/extensions/extensionstest"
)

// ctxKey is a private test sentinel type used by
// TestHandleAutofillThreadsContext to verify that handleAutofill
// passes a real per-call context into the credential provider's Fill
// method instead of dropping it for context.Background().
type ctxKey struct{ name string }

// missingCredProvider is a CredentialProvider whose Lookup always
// returns (nil, nil). Used to exercise the "no match" branch of
// handleGetCredential, which must now return {"found":false}.
type missingCredProvider struct{}

func (missingCredProvider) Available() bool { return true }
func (missingCredProvider) Lookup(_ context.Context, _ string) (*extensions.Credential, error) {
	return nil, nil
}
func (missingCredProvider) Fill(_ context.Context, _ string, _ extensions.FillTarget) error {
	return nil
}
func (missingCredProvider) GenerateCode(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (missingCredProvider) List(_ context.Context) ([]extensions.Credential, error) {
	return nil, nil
}

func TestGetCredentialMissReturnsFoundFalse(t *testing.T) {
	original := extensions.Registry.Credentials()
	t.Cleanup(func() { extensions.Registry.SetCredentials(original) })
	extensions.Registry.SetCredentials(missingCredProvider{})

	res := runExtTool(t, "vulpine_get_credential", map[string]interface{}{
		"site_url": "https://no-such-site.example",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(res.Content[0].Text), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if found, ok := parsed["found"].(bool); !ok || found {
		t.Errorf("expected {\"found\":false}, got %q", res.Content[0].Text)
	}
}

func TestHandleAutofillThreadsContext(t *testing.T) {
	sentinel := &ctxKey{name: "autofill-ctx"}
	var seen context.Context
	fake := withFakeCredentials(t, &extensionstest.FakeCredentialProvider{
		AvailableFlag: true,
		Cred: extensions.Credential{
			ID:       "cred-ctx",
			Site:     "https://example.com",
			Username: "alice",
		},
		FillFn: func(ctx context.Context, credID string, target extensions.FillTarget) error {
			if seen == nil {
				seen = ctx
			}
			return nil
		},
	})
	_ = fake

	ctx := context.WithValue(context.Background(), sentinel, "present")
	args, _ := json.Marshal(map[string]interface{}{
		"site_url":          "https://example.com",
		"page_id":           "p1",
		"frame_id":          "f1",
		"username_selector": "#user",
		"password_selector": "#pass",
	})
	res, ok := handleExtensionTool(ctx, nil, "vulpine_autofill", args)
	if !ok {
		t.Fatal("autofill not dispatched")
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res)
	}
	if seen == nil {
		t.Fatal("Fill was never called")
	}
	if got, _ := seen.Value(sentinel).(string); got != "present" {
		t.Errorf("sentinel not visible inside Fill: got %q", got)
	}
}

// withFakeCredentials installs a fake credential provider for the
// duration of the test and returns the fake so assertions can inspect
// recorded calls.
func withFakeCredentials(t *testing.T, fake *extensionstest.FakeCredentialProvider) *extensionstest.FakeCredentialProvider {
	t.Helper()
	original := extensions.Registry.Credentials()
	t.Cleanup(func() { extensions.Registry.SetCredentials(original) })
	extensions.Registry.SetCredentials(fake)
	return fake
}

func withFakeAudio(t *testing.T, fake *extensionstest.FakeAudioCapturer) *extensionstest.FakeAudioCapturer {
	t.Helper()
	original := extensions.Registry.Audio()
	t.Cleanup(func() { extensions.Registry.SetAudio(original) })
	extensions.Registry.SetAudio(fake)
	return fake
}

func withFakeMobile(t *testing.T, fake *extensionstest.FakeMobileBridge) *extensionstest.FakeMobileBridge {
	t.Helper()
	original := extensions.Registry.Mobile()
	t.Cleanup(func() { extensions.Registry.SetMobile(original) })
	extensions.Registry.SetMobile(fake)
	return fake
}

func TestGetCredentialReturnsCredJSON(t *testing.T) {
	withFakeCredentials(t, &extensionstest.FakeCredentialProvider{
		AvailableFlag: true,
		Cred: extensions.Credential{
			ID:       "cred-1",
			Site:     "https://example.com",
			Username: "alice",
			HasTOTP:  true,
			Notes:    "main",
		},
	})
	res := runExtTool(t, "vulpine_get_credential", map[string]interface{}{
		"site_url": "https://example.com",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	body := res.Content[0].Text
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["id"] != "cred-1" {
		t.Errorf("id = %v, want cred-1", parsed["id"])
	}
	if parsed["username"] != "alice" {
		t.Errorf("username = %v, want alice", parsed["username"])
	}
	if parsed["hasTOTP"] != true {
		t.Errorf("hasTOTP = %v, want true", parsed["hasTOTP"])
	}
	// Password must never appear in the tool boundary.
	if strings.Contains(strings.ToLower(body), "password") {
		t.Errorf("credential JSON leaked password-like field: %q", body)
	}
}

func TestAutofillCallsFillTwice(t *testing.T) {
	fake := withFakeCredentials(t, &extensionstest.FakeCredentialProvider{
		AvailableFlag: true,
		Cred: extensions.Credential{
			ID:       "cred-1",
			Site:     "https://example.com",
			Username: "alice",
		},
	})
	res := runExtTool(t, "vulpine_autofill", map[string]interface{}{
		"site_url":          "https://example.com",
		"page_id":           "p1",
		"frame_id":          "f1",
		"username_selector": "#user",
		"password_selector": "#pass",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	calls := fake.RecordedFills()
	if len(calls) != 2 {
		t.Fatalf("expected 2 Fill calls, got %d", len(calls))
	}
	if calls[0].Target.Field != "username" || calls[0].Target.Selector != "#user" {
		t.Errorf("first call = %+v, want username/#user", calls[0])
	}
	if calls[1].Target.Field != "password" || calls[1].Target.Selector != "#pass" {
		t.Errorf("second call = %+v, want password/#pass", calls[1])
	}
	if calls[0].CredID != "cred-1" || calls[1].CredID != "cred-1" {
		t.Errorf("Fill called with wrong credID: %+v", calls)
	}
}

func TestStartAudioCaptureAppliesDefaults(t *testing.T) {
	fake := withFakeAudio(t, &extensionstest.FakeAudioCapturer{
		AvailableFlag: true,
		Handle:        extensions.CaptureHandle{ID: "h1", Format: "pcm"},
	})
	// Empty request — handler must apply format=pcm, sampleRate=16000,
	// channels=1 before calling Start.
	res := runExtTool(t, "vulpine_start_audio_capture", map[string]interface{}{})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	req := fake.LastStartRequest()
	if req.Format != "pcm" {
		t.Errorf("format = %q, want pcm", req.Format)
	}
	if req.SampleRate != 16000 {
		t.Errorf("sampleRate = %d, want 16000", req.SampleRate)
	}
	if req.Channels != 1 {
		t.Errorf("channels = %d, want 1", req.Channels)
	}
}

func TestListMobileDevicesReturnsList(t *testing.T) {
	withFakeMobile(t, &extensionstest.FakeMobileBridge{
		AvailableFlag: true,
		Devices: []extensions.MobileDevice{
			{UDID: "ABC123", Name: "Test Phone", Platform: "android", Model: "Pixel 8"},
		},
	})
	res := runExtTool(t, "vulpine_list_mobile_devices", map[string]interface{}{})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	body := res.Content[0].Text
	if !strings.Contains(body, "ABC123") {
		t.Errorf("expected body to contain UDID, got %q", body)
	}
	if !strings.Contains(body, "Pixel 8") {
		t.Errorf("expected body to contain model, got %q", body)
	}
}

func TestConnectMobileDeviceReturnsSession(t *testing.T) {
	withFakeMobile(t, &extensionstest.FakeMobileBridge{
		AvailableFlag: true,
		Session: extensions.MobileSession{
			ID:          "mobile-session-1",
			UDID:        "ABC123",
			CDPEndpoint: "http://127.0.0.1:9222",
			Protocol:    "cdp",
		},
	})
	res := runExtTool(t, "vulpine_connect_mobile_device", map[string]interface{}{
		"udid": "ABC123",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	body := res.Content[0].Text
	if !strings.Contains(body, "mobile-session-1") || !strings.Contains(body, "http://127.0.0.1:9222") {
		t.Errorf("expected body to contain session details, got %q", body)
	}
}

func TestDisconnectMobileDeviceReturnsOK(t *testing.T) {
	fake := withFakeMobile(t, &extensionstest.FakeMobileBridge{
		AvailableFlag: true,
	})
	res := runExtTool(t, "vulpine_disconnect_mobile_device", map[string]interface{}{
		"session_id": "mobile-session-1",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	if got := res.Content[0].Text; got != `{"ok":true}` {
		t.Fatalf("response = %q", got)
	}
	if len(fake.Disconnected) != 1 || fake.Disconnected[0] != "mobile-session-1" {
		t.Fatalf("disconnected = %+v", fake.Disconnected)
	}
}
