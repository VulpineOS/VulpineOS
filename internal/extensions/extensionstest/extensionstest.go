// Package extensionstest provides reusable fake provider
// implementations for tests that need to exercise the "available"
// code paths in internal/extensions and internal/mcp without pulling
// in a real backend. Using this package from _test.go files avoids
// the drift that comes from every test file rolling its own fakes.
package extensionstest

import (
	"context"
	"sync"

	"vulpineos/internal/extensions"
)

// FakeCredentialProvider is a CredentialProvider that returns a single
// canned credential. FillCalls records every Fill invocation so tests
// can assert on the exact sequence of username/password writes. If
// FillFn is non-nil it is called in place of the default stub.
type FakeCredentialProvider struct {
	Cred          extensions.Credential
	TOTP          string
	AvailableFlag bool
	FillFn        func(ctx context.Context, credID string, target extensions.FillTarget) error

	mu        sync.Mutex
	FillCalls []FillCall
}

// FillCall captures the arguments passed to Fill.
type FillCall struct {
	CredID string
	Target extensions.FillTarget
}

// Lookup always returns the canned credential as a copy.
func (f *FakeCredentialProvider) Lookup(ctx context.Context, siteURL string) (*extensions.Credential, error) {
	c := f.Cred
	return &c, nil
}

// Fill records the call and delegates to FillFn if set. Always
// validates the target first, matching the contract documented on
// FillTarget.Validate.
func (f *FakeCredentialProvider) Fill(ctx context.Context, credID string, target extensions.FillTarget) error {
	if err := target.Validate(); err != nil {
		return err
	}
	f.mu.Lock()
	f.FillCalls = append(f.FillCalls, FillCall{CredID: credID, Target: target})
	f.mu.Unlock()
	if f.FillFn != nil {
		return f.FillFn(ctx, credID, target)
	}
	return nil
}

// GenerateCode returns TOTP if non-empty, otherwise "000000".
func (f *FakeCredentialProvider) GenerateCode(ctx context.Context, credID string) (string, error) {
	if f.TOTP != "" {
		return f.TOTP, nil
	}
	return "000000", nil
}

// List returns a single-element slice containing the canned credential.
func (f *FakeCredentialProvider) List(ctx context.Context) ([]extensions.Credential, error) {
	return []extensions.Credential{f.Cred}, nil
}

// Available reports AvailableFlag.
func (f *FakeCredentialProvider) Available() bool { return f.AvailableFlag }

// RecordedFills returns a snapshot copy of FillCalls, safe for assertion
// from the test goroutine.
func (f *FakeCredentialProvider) RecordedFills() []FillCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]FillCall, len(f.FillCalls))
	copy(out, f.FillCalls)
	return out
}

// FakeAudioCapturer is an AudioCapturer that records Start requests
// and returns canned handles. StartReq is updated on every Start call
// so tests can assert that defaults were applied.
type FakeAudioCapturer struct {
	AvailableFlag bool
	Handle        extensions.CaptureHandle
	Chunk         []byte
	EOF           bool

	mu       sync.Mutex
	StartReq extensions.CaptureRequest
	StopID   string
	ReadID   string
}

// Start records the request and returns the canned handle.
func (f *FakeAudioCapturer) Start(ctx context.Context, req extensions.CaptureRequest) (*extensions.CaptureHandle, error) {
	f.mu.Lock()
	f.StartReq = req
	f.mu.Unlock()
	h := f.Handle
	return &h, nil
}

// Stop records the stop handle ID.
func (f *FakeAudioCapturer) Stop(ctx context.Context, handleID string) error {
	f.mu.Lock()
	f.StopID = handleID
	f.mu.Unlock()
	return nil
}

// Read records the read handle ID and returns the canned chunk.
func (f *FakeAudioCapturer) Read(ctx context.Context, handleID string, maxBytes int) ([]byte, bool, error) {
	f.mu.Lock()
	f.ReadID = handleID
	f.mu.Unlock()
	return f.Chunk, f.EOF, nil
}

// Available reports AvailableFlag.
func (f *FakeAudioCapturer) Available() bool { return f.AvailableFlag }

// LastStartRequest returns the most recent CaptureRequest seen by Start.
func (f *FakeAudioCapturer) LastStartRequest() extensions.CaptureRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.StartReq
}

// FakeMobileBridge is a MobileBridge that returns canned devices.
type FakeMobileBridge struct {
	AvailableFlag bool
	Devices       []extensions.MobileDevice
	Session       extensions.MobileSession
}

// ListDevices returns the canned device slice.
func (f *FakeMobileBridge) ListDevices(ctx context.Context) ([]extensions.MobileDevice, error) {
	return f.Devices, nil
}

// Connect returns the canned session.
func (f *FakeMobileBridge) Connect(ctx context.Context, udid string) (*extensions.MobileSession, error) {
	s := f.Session
	return &s, nil
}

// Available reports AvailableFlag.
func (f *FakeMobileBridge) Available() bool { return f.AvailableFlag }
