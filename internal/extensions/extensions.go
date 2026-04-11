// Package extensions defines pluggable provider interfaces that optional
// VulpineOS features depend on. The public build of VulpineOS ships
// nil-safe, no-op default implementations for every provider, so the
// tree always compiles and runs without any extra dependencies.
//
// Alternate builds may replace the defaults at init() time by assigning
// to Registry from a build-tagged file. Callers should always check
// Available() on a provider before invoking it, and must handle the
// "feature unavailable" error returned by the default stubs.
package extensions

// Registry holds the active provider implementations. Fields default to
// no-op stubs so that all interface methods are safe to call even when
// a feature has not been enabled in this build.
var Registry = struct {
	Credentials CredentialProvider
	Audio       AudioCapturer
	Mobile      MobileBridge
}{
	Credentials: defaultCredentialProvider,
	Audio:       defaultAudioCapturer,
	Mobile:      defaultMobileBridge,
}
