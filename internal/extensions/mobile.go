package extensions

import "context"

// MobileBridge is the generic interface for discovering and attaching
// to mobile devices that expose a remote debugging endpoint. The
// returned MobileSession can then be driven by the same automation
// layer as a desktop browser context.
type MobileBridge interface {
	ListDevices(ctx context.Context) ([]MobileDevice, error)
	Connect(ctx context.Context, udid string) (*MobileSession, error)
	Disconnect(ctx context.Context, sessionID string) error
	Available() bool
}

// MobileDevice describes a device visible to the bridge.
type MobileDevice struct {
	UDID     string
	Name     string
	Platform string // "android" | "ios"
	Model    string
}

// MobileSession is the result of connecting to a device: an identifier
// plus a remote debugging endpoint callers can dial.
type MobileSession struct {
	ID          string
	UDID        string
	CDPEndpoint string
}

// defaultMobileBridge is the no-op stub used when no mobile bridge has
// been registered. All methods return ErrUnavailable.
var defaultMobileBridge MobileBridge = noopMobileBridge{}

type noopMobileBridge struct{}

func (noopMobileBridge) ListDevices(ctx context.Context) ([]MobileDevice, error) {
	return nil, ErrUnavailable
}

func (noopMobileBridge) Connect(ctx context.Context, udid string) (*MobileSession, error) {
	return nil, ErrUnavailable
}

func (noopMobileBridge) Disconnect(ctx context.Context, sessionID string) error {
	return ErrUnavailable
}

func (noopMobileBridge) Available() bool { return false }
