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
//
// All fields that might be mutated during a test are guarded by mu.
// Prefer the Set* helpers when changing fields from a goroutine that
// is not the one constructing the fake; the helpers take the write
// lock on your behalf. All read methods acquire a read lock.
type FakeCredentialProvider struct {
	mu            sync.RWMutex
	Cred          extensions.Credential
	TOTP          string
	AvailableFlag bool
	FillFn        func(ctx context.Context, credID string, target extensions.FillTarget) error

	FillCalls []FillCall
}

// FillCall captures the arguments passed to Fill.
type FillCall struct {
	CredID string
	Target extensions.FillTarget
}

// SetCred swaps the canned credential under the write lock. This is
// the recommended way to mutate the fake from a test goroutine that
// races with Lookup/List/Fill.
func (f *FakeCredentialProvider) SetCred(c extensions.Credential) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Cred = c
}

// SetAvailable toggles AvailableFlag under the write lock.
func (f *FakeCredentialProvider) SetAvailable(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.AvailableFlag = v
}

// SetTOTP overwrites the canned TOTP under the write lock.
func (f *FakeCredentialProvider) SetTOTP(v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.TOTP = v
}

// Lookup always returns the canned credential as a copy.
func (f *FakeCredentialProvider) Lookup(ctx context.Context, siteURL string) (*extensions.Credential, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
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
	fn := f.FillFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, credID, target)
	}
	return nil
}

// GenerateCode returns TOTP if non-empty, otherwise "000000".
func (f *FakeCredentialProvider) GenerateCode(ctx context.Context, credID string) (string, error) {
	f.mu.RLock()
	totp := f.TOTP
	f.mu.RUnlock()
	if totp != "" {
		return totp, nil
	}
	return "000000", nil
}

// List returns a single-element slice containing the canned credential.
func (f *FakeCredentialProvider) List(ctx context.Context) ([]extensions.Credential, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return []extensions.Credential{f.Cred}, nil
}

// Available reports AvailableFlag.
func (f *FakeCredentialProvider) Available() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.AvailableFlag
}

// RecordedFills returns a snapshot copy of FillCalls, safe for assertion
// from the test goroutine.
func (f *FakeCredentialProvider) RecordedFills() []FillCall {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]FillCall, len(f.FillCalls))
	copy(out, f.FillCalls)
	return out
}

// FakeAudioCapturer is an AudioCapturer that records Start requests
// and returns canned handles. StartReq is updated on every Start call
// so tests can assert that defaults were applied.
type FakeAudioCapturer struct {
	mu            sync.RWMutex
	AvailableFlag bool
	Handle        extensions.CaptureHandle
	Chunk         []byte
	EOF           bool

	StartReq extensions.CaptureRequest
	StopID   string
	ReadID   string
}

// SetAvailable toggles AvailableFlag under the write lock.
func (f *FakeAudioCapturer) SetAvailable(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.AvailableFlag = v
}

// SetHandle swaps the canned handle under the write lock.
func (f *FakeAudioCapturer) SetHandle(h extensions.CaptureHandle) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Handle = h
}

// SetChunk swaps the canned chunk/EOF pair under the write lock.
func (f *FakeAudioCapturer) SetChunk(chunk []byte, eof bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Chunk = chunk
	f.EOF = eof
}

// Start records the request and returns the canned handle.
func (f *FakeAudioCapturer) Start(ctx context.Context, req extensions.CaptureRequest) (*extensions.CaptureHandle, error) {
	f.mu.Lock()
	f.StartReq = req
	h := f.Handle
	f.mu.Unlock()
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
	chunk := f.Chunk
	eof := f.EOF
	f.mu.Unlock()
	return chunk, eof, nil
}

// Available reports AvailableFlag.
func (f *FakeAudioCapturer) Available() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.AvailableFlag
}

// LastStartRequest returns the most recent CaptureRequest seen by Start.
func (f *FakeAudioCapturer) LastStartRequest() extensions.CaptureRequest {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.StartReq
}

// FakeMobileBridge is a MobileBridge that returns canned devices. All
// fields are guarded by mu so tests can mutate the fake from
// concurrent goroutines without data races under -race.
type FakeMobileBridge struct {
	mu            sync.RWMutex
	AvailableFlag bool
	Devices       []extensions.MobileDevice
	Session       extensions.MobileSession
	Disconnected  []string
}

// SetAvailable toggles AvailableFlag under the write lock.
func (f *FakeMobileBridge) SetAvailable(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.AvailableFlag = v
}

// SetDevices replaces the canned device slice under the write lock.
func (f *FakeMobileBridge) SetDevices(d []extensions.MobileDevice) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Devices = d
}

// SetSession replaces the canned session under the write lock.
func (f *FakeMobileBridge) SetSession(s extensions.MobileSession) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Session = s
}

// ListDevices returns a copy of the canned device slice.
func (f *FakeMobileBridge) ListDevices(ctx context.Context) ([]extensions.MobileDevice, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]extensions.MobileDevice, len(f.Devices))
	copy(out, f.Devices)
	return out, nil
}

// Connect returns the canned session.
func (f *FakeMobileBridge) Connect(ctx context.Context, udid string) (*extensions.MobileSession, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	s := f.Session
	if s.UDID == "" {
		s.UDID = udid
	}
	return &s, nil
}

// Disconnect records the closed session ID.
func (f *FakeMobileBridge) Disconnect(ctx context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Disconnected = append(f.Disconnected, sessionID)
	return nil
}

// Available reports AvailableFlag.
func (f *FakeMobileBridge) Available() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.AvailableFlag
}

// FakeSentinelProvider records events and outcomes while returning
// canned status and variant bundles.
type FakeSentinelProvider struct {
	mu              sync.RWMutex
	AvailableFlag   bool
	StatusValue     extensions.SentinelStatus
	VariantBundles  []extensions.SentinelVariantBundle
	TrustRecipes    []extensions.SentinelTrustRecipe
	MaturityMetrics []extensions.SentinelMaturityMetric
	AssignmentRules []extensions.SentinelAssignmentRule
	Events          []extensions.SentinelEvent
	Outcomes        []extensions.SentinelOutcome
}

// SetAvailable toggles AvailableFlag under the write lock.
func (f *FakeSentinelProvider) SetAvailable(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.AvailableFlag = v
}

// SetStatus replaces the canned status under the write lock.
func (f *FakeSentinelProvider) SetStatus(s extensions.SentinelStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.StatusValue = s
}

// SetVariantBundles replaces the canned variant bundles under the write lock.
func (f *FakeSentinelProvider) SetVariantBundles(v []extensions.SentinelVariantBundle) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.VariantBundles = append([]extensions.SentinelVariantBundle(nil), v...)
}

// SetTrustRecipes replaces the canned trust recipes under the write lock.
func (f *FakeSentinelProvider) SetTrustRecipes(v []extensions.SentinelTrustRecipe) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.TrustRecipes = append([]extensions.SentinelTrustRecipe(nil), v...)
}

// SetMaturityMetrics replaces the canned maturity metrics under the write lock.
func (f *FakeSentinelProvider) SetMaturityMetrics(v []extensions.SentinelMaturityMetric) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.MaturityMetrics = append([]extensions.SentinelMaturityMetric(nil), v...)
}

// SetAssignmentRules replaces the canned assignment rules under the write lock.
func (f *FakeSentinelProvider) SetAssignmentRules(v []extensions.SentinelAssignmentRule) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.AssignmentRules = append([]extensions.SentinelAssignmentRule(nil), v...)
}

// Status returns the canned status.
func (f *FakeSentinelProvider) Status(ctx context.Context) (*extensions.SentinelStatus, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	status := f.StatusValue
	return &status, nil
}

// RecordEvent appends the event under the write lock.
func (f *FakeSentinelProvider) RecordEvent(ctx context.Context, event extensions.SentinelEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Events = append(f.Events, event)
	return nil
}

// RecordOutcome appends the outcome under the write lock.
func (f *FakeSentinelProvider) RecordOutcome(ctx context.Context, outcome extensions.SentinelOutcome) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Outcomes = append(f.Outcomes, outcome)
	return nil
}

// ListVariantBundles returns a copy of the canned variant bundles.
func (f *FakeSentinelProvider) ListVariantBundles(ctx context.Context) ([]extensions.SentinelVariantBundle, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]extensions.SentinelVariantBundle, len(f.VariantBundles))
	copy(out, f.VariantBundles)
	return out, nil
}

// ListTrustRecipes returns a copy of the canned trust recipes.
func (f *FakeSentinelProvider) ListTrustRecipes(ctx context.Context) ([]extensions.SentinelTrustRecipe, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]extensions.SentinelTrustRecipe, len(f.TrustRecipes))
	copy(out, f.TrustRecipes)
	return out, nil
}

// ListMaturityMetrics returns a copy of the canned maturity metrics.
func (f *FakeSentinelProvider) ListMaturityMetrics(ctx context.Context) ([]extensions.SentinelMaturityMetric, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]extensions.SentinelMaturityMetric, len(f.MaturityMetrics))
	copy(out, f.MaturityMetrics)
	return out, nil
}

// ListAssignmentRules returns a copy of the canned assignment rules.
func (f *FakeSentinelProvider) ListAssignmentRules(ctx context.Context) ([]extensions.SentinelAssignmentRule, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]extensions.SentinelAssignmentRule, len(f.AssignmentRules))
	copy(out, f.AssignmentRules)
	return out, nil
}

// Available reports AvailableFlag.
func (f *FakeSentinelProvider) Available() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.AvailableFlag
}

// RecordedEvents returns a snapshot copy of the captured events.
func (f *FakeSentinelProvider) RecordedEvents() []extensions.SentinelEvent {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]extensions.SentinelEvent, len(f.Events))
	copy(out, f.Events)
	return out
}

// RecordedOutcomes returns a snapshot copy of the captured outcomes.
func (f *FakeSentinelProvider) RecordedOutcomes() []extensions.SentinelOutcome {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]extensions.SentinelOutcome, len(f.Outcomes))
	copy(out, f.Outcomes)
	return out
}
