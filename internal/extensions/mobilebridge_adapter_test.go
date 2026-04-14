package extensions

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	mb "github.com/VulpineOS/mobilebridge/pkg/mobilebridge"
)

func TestMobilebridgeAdapterAvailable(t *testing.T) {
	prevLook := mbLookPath
	t.Cleanup(func() { mbLookPath = prevLook })

	mbLookPath = func(string) (string, error) { return "/fake/adb", nil }
	if !(mobilebridgeAdapter{}).Available() {
		t.Fatal("Available() = false with adb on PATH")
	}

	mbLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	if (mobilebridgeAdapter{}).Available() {
		t.Fatal("Available() = true without adb on PATH")
	}
}

func TestMobilebridgeAdapterListDevices(t *testing.T) {
	prevList := mbListDevices
	t.Cleanup(func() { mbListDevices = prevList })

	mbListDevices = func(ctx context.Context) ([]mb.Device, error) {
		return []mb.Device{
			{Serial: "R58N12ABCDE", State: "device", Model: "SM_G960U"},
			{Serial: "emulator-5554", State: "device"},
		}, nil
	}

	devices, err := mobilebridgeAdapter{}.ListDevices(context.Background())
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
	if devices[0].UDID != "R58N12ABCDE" || devices[0].Name != "SM_G960U" || devices[0].Platform != "android" || devices[0].Model != "SM_G960U" {
		t.Fatalf("devices[0] = %+v", devices[0])
	}
	if devices[1].UDID != "emulator-5554" || devices[1].Name != "emulator-5554" || devices[1].Platform != "android" {
		t.Fatalf("devices[1] = %+v", devices[1])
	}
}

func TestMobilebridgeAdapterConnectUnavailable(t *testing.T) {
	prevConnect := mbConnectSession
	t.Cleanup(func() { mbConnectSession = prevConnect })

	mbConnectSession = func(ctx context.Context, udid string) (*MobileSession, mobileSessionCleanup, error) {
		return nil, nil, ErrUnavailable
	}

	_, err := mobilebridgeAdapter{}.Connect(context.Background(), "R58N12ABCDE")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Connect error = %v, want ErrUnavailable", err)
	}
}

func TestMobilebridgeAdapterConnectDisconnect(t *testing.T) {
	prevConnect := mbConnectSession
	t.Cleanup(func() {
		mbConnectSession = prevConnect
		mobileSessions.Lock()
		mobileSessions.cleanup = make(map[string]mobileSessionCleanup)
		mobileSessions.Unlock()
	})

	cleaned := false
	mbConnectSession = func(ctx context.Context, udid string) (*MobileSession, mobileSessionCleanup, error) {
		return &MobileSession{
				ID:          "session-1",
				UDID:        udid,
				CDPEndpoint: "http://127.0.0.1:9222",
			}, func() error {
				cleaned = true
				return nil
			}, nil
	}

	session, err := mobilebridgeAdapter{}.Connect(context.Background(), "R58N12ABCDE")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if session.ID != "session-1" || session.UDID != "R58N12ABCDE" || session.CDPEndpoint == "" {
		t.Fatalf("session = %+v", session)
	}
	if err := (mobilebridgeAdapter{}).Disconnect(context.Background(), "session-1"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if !cleaned {
		t.Fatal("cleanup was not called")
	}
}

func TestRegistryMobileUsesMobilebridgeAdapter(t *testing.T) {
	if _, ok := Registry.Mobile().(mobilebridgeAdapter); !ok {
		t.Fatalf("Registry.Mobile() = %T, want mobilebridgeAdapter", Registry.Mobile())
	}
}
