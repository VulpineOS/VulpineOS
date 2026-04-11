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

// registry holds the active provider implementations. Fields default to
// no-op stubs so that all interface methods are safe to call even when
// a feature has not been enabled in this build.
type registry struct {
	Credentials CredentialProvider
	Audio       AudioCapturer
	Mobile      MobileBridge
}

// SetCredentials registers a credential provider. Intended to be called
// from init() in build-tagged extension files.
func (r *registry) SetCredentials(p CredentialProvider) { r.Credentials = p }

// SetAudio registers an audio capturer. Intended to be called from
// init() in build-tagged extension files.
func (r *registry) SetAudio(c AudioCapturer) { r.Audio = c }

// SetMobile registers a mobile bridge. Intended to be called from
// init() in build-tagged extension files.
func (r *registry) SetMobile(b MobileBridge) { r.Mobile = b }

// Registry is the global provider registry. It is a pointer so that
// alternate builds can mutate it from their own init() functions.
var Registry = &registry{
	Credentials: defaultCredentialProvider,
	Audio:       defaultAudioCapturer,
	Mobile:      defaultMobileBridge,
}

// Init is called once at startup; private extension builds may register
// providers from their own init() functions before this hook fires. The
// public build leaves it as a no-op so the call site is a stable anchor.
func Init() {}
