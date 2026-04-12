package extensions

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"sync"

	mb "github.com/PopcornDev1/mobilebridge/pkg/mobilebridge"
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
	adbPort, err := freeTCPPort()
	if err != nil {
		return nil, nil, err
	}
	serverPort, err := freeTCPPort()
	if err != nil {
		return nil, nil, err
	}

	proxy, err := mb.NewProxy(ctx, udid, adbPort)
	if err != nil {
		return nil, nil, err
	}
	serverAddr := fmt.Sprintf("127.0.0.1:%d", serverPort)
	server := mb.NewServer(udid, serverAddr)
	if err := server.Start(); err != nil {
		_ = proxy.Close()
		return nil, nil, err
	}
	if err := server.RunWithProxy(proxy); err != nil {
		_ = server.Stop()
		_ = proxy.Close()
		return nil, nil, err
	}

	cleanup := func() error {
		var err error
		if serverErr := server.Stop(); serverErr != nil {
			err = serverErr
		}
		if proxyErr := proxy.Close(); proxyErr != nil && err == nil {
			err = proxyErr
		}
		return err
	}
	return &MobileSession{
		UDID:        udid,
		CDPEndpoint: "http://" + serverAddr,
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
