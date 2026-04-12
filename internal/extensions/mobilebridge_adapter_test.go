package extensions

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	mb "github.com/PopcornDev1/mobilebridge/pkg/mobilebridge"
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
	_, err := mobilebridgeAdapter{}.Connect(context.Background(), "R58N12ABCDE")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Connect error = %v, want ErrUnavailable", err)
	}
}

func TestRegistryMobileUsesMobilebridgeAdapter(t *testing.T) {
	if _, ok := Registry.Mobile().(mobilebridgeAdapter); !ok {
		t.Fatalf("Registry.Mobile() = %T, want mobilebridgeAdapter", Registry.Mobile())
	}
}
