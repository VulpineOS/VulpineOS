package extensions

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"sync"

	mb "github.com/VulpineOS/mobilebridge/pkg/mobilebridge"
	"github.com/google/uuid"
)

var mbListDevices = mb.ListDevices
var mbLookPath = exec.LookPath
var mbConnectSession = startMobilebridgeSession

type mobileSessionCleanup func() error

var mobileSessions = struct {
	sync.Mutex
	cleanup map[string]mobileSessionCleanup
}{cleanup: make(map[string]mobileSessionCleanup)}

type mobilebridgeAdapter struct{}

func (mobilebridgeAdapter) Available() bool {
	_, err := mbLookPath("adb")
	return err == nil
}

func (mobilebridgeAdapter) ListDevices(ctx context.Context) ([]MobileDevice, error) {
	devices, err := mbListDevices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MobileDevice, 0, len(devices))
	for _, d := range devices {
		name := d.Model
		if name == "" {
			name = d.Serial
		}
		out = append(out, MobileDevice{
			UDID:     d.Serial,
			Name:     name,
			Platform: "android",
			Model:    d.Model,
		})
	}
	return out, nil
}

func (mobilebridgeAdapter) Connect(ctx context.Context, udid string) (*MobileSession, error) {
	if udid == "" {
		return nil, errors.New("mobilebridge: empty udid")
	}
	session, cleanup, err := mbConnectSession(ctx, udid)
	if err != nil {
		return nil, err
	}
	if session.ID == "" {
		session.ID = uuid.NewString()
	}
	if session.UDID == "" {
		session.UDID = udid
	}
	mobileSessions.Lock()
	mobileSessions.cleanup[session.ID] = cleanup
	mobileSessions.Unlock()
	return session, nil
}

func (mobilebridgeAdapter) Disconnect(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("mobilebridge: empty session id")
	}
	mobileSessions.Lock()
	cleanup, ok := mobileSessions.cleanup[sessionID]
	if ok {
		delete(mobileSessions.cleanup, sessionID)
	}
	mobileSessions.Unlock()
	if !ok {
		return fmt.Errorf("mobilebridge: session not found: %s", sessionID)
	}
	if cleanup == nil {
		return nil
	}
	return cleanup()
}

func init() {
	Registry.SetMobile(mobilebridgeAdapter{})
}

func startMobilebridgeSession(ctx context.Context, udid string) (*MobileSession, mobileSessionCleanup, error) {
	serverPort, err := freeTCPPort()
	if err != nil {
		return nil, nil, err
	}

	serverAddr := fmt.Sprintf("127.0.0.1:%d", serverPort)
	session, err := mb.StartAttachedServer(ctx, udid, serverAddr)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() error {
		return session.Close()
	}
	return &MobileSession{
		UDID:        udid,
		CDPEndpoint: session.Endpoint,
		Protocol:    "cdp",
	}, cleanup, nil
}

func freeTCPPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)
	return addr.Port, nil
}
