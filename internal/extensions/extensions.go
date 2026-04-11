// Package extensions defines pluggable provider interfaces that optional
// VulpineOS features depend on. The public build of VulpineOS ships
// nil-safe, no-op default implementations for every provider, so the
// tree always compiles and runs without any extra dependencies.
//
// Alternate builds may replace the defaults at init() time by calling
// the setter methods on Registry from a build-tagged file. Callers
// should always check Available() on a provider before invoking it,
// and must handle the "feature unavailable" error returned by the
// default stubs.
package extensions

import "sync"

// registry holds the active provider implementations. Reads and writes
// are guarded by an embedded RWMutex so that setters running from
// build-tagged init files do not race with MCP tool handlers reading
// providers on worker goroutines.
//
// Access the providers via the Credentials/Audio/Mobile accessor
// methods rather than touching fields directly; this keeps the
// read-path lock-safe under -race.
type registry struct {
	mu          sync.RWMutex
	credentials CredentialProvider
	audio       AudioCapturer
	mobile      MobileBridge
}

// Credentials returns the currently-registered credential provider.
// Always non-nil; returns the no-op default when nothing is registered.
func (r *registry) Credentials() CredentialProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.credentials
}

// Audio returns the currently-registered audio capturer. Always
// non-nil; returns the no-op default when nothing is registered.
func (r *registry) Audio() AudioCapturer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.audio
}

// Mobile returns the currently-registered mobile bridge. Always
// non-nil; returns the no-op default when nothing is registered.
func (r *registry) Mobile() MobileBridge {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mobile
}

// SetCredentials registers a credential provider. Intended to be called
// from init() in build-tagged extension files. Safe to call from tests
// that swap in a fake provider.
func (r *registry) SetCredentials(p CredentialProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p == nil {
		p = defaultCredentialProvider
	}
	r.credentials = p
}

// SetAudio registers an audio capturer. Intended to be called from
// init() in build-tagged extension files.
func (r *registry) SetAudio(c AudioCapturer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c == nil {
		c = defaultAudioCapturer
	}
	r.audio = c
}

// SetMobile registers a mobile bridge. Intended to be called from
// init() in build-tagged extension files.
func (r *registry) SetMobile(b MobileBridge) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b == nil {
		b = defaultMobileBridge
	}
	r.mobile = b
}

// Registry is the global provider registry. It is a pointer so that
// alternate builds can mutate it from their own init() functions.
var Registry = &registry{
	credentials: defaultCredentialProvider,
	audio:       defaultAudioCapturer,
	mobile:      defaultMobileBridge,
}

// Init is called once at startup; private extension builds may register
// providers from their own init() functions before this hook fires. The
// public build leaves it as a no-op so the call site is a stable anchor.
func Init() {}
