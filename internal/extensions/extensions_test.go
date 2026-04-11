package extensions

import (
	"context"
	"errors"
	"testing"
)

func TestRegistryDefaultsNonNil(t *testing.T) {
	if Registry.Credentials == nil {
		t.Fatal("Registry.Credentials is nil")
	}
	if Registry.Audio == nil {
		t.Fatal("Registry.Audio is nil")
	}
	if Registry.Mobile == nil {
		t.Fatal("Registry.Mobile is nil")
	}
}

func TestDefaultCredentialProviderUnavailable(t *testing.T) {
	p := Registry.Credentials
	if p.Available() {
		t.Fatal("default CredentialProvider should report Available() == false")
	}
	ctx := context.Background()

	if _, err := p.Lookup(ctx, "https://example.com"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Lookup: expected ErrUnavailable, got %v", err)
	}
	if err := p.Fill(ctx, "id", FillTarget{PageID: "p", Selector: "#user", Field: "username"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Fill: expected ErrUnavailable, got %v", err)
	}
	if _, err := p.GenerateCode(ctx, "id"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("GenerateCode: expected ErrUnavailable, got %v", err)
	}
	if _, err := p.List(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("List: expected ErrUnavailable, got %v", err)
	}
}

func TestDefaultAudioCapturerUnavailable(t *testing.T) {
	a := Registry.Audio
	if a.Available() {
		t.Fatal("default AudioCapturer should report Available() == false")
	}
	ctx := context.Background()

	if _, err := a.Start(ctx, CaptureRequest{Format: "pcm", SampleRate: 48000, Channels: 2}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Start: expected ErrUnavailable, got %v", err)
	}
	if err := a.Stop(ctx, "handle"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Stop: expected ErrUnavailable, got %v", err)
	}
	if _, _, err := a.Read(ctx, "handle", 1024); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Read: expected ErrUnavailable, got %v", err)
	}
}

func TestDefaultMobileBridgeUnavailable(t *testing.T) {
	m := Registry.Mobile
	if m.Available() {
		t.Fatal("default MobileBridge should report Available() == false")
	}
	ctx := context.Background()

	if _, err := m.ListDevices(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListDevices: expected ErrUnavailable, got %v", err)
	}
	if _, err := m.Connect(ctx, "udid"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Connect: expected ErrUnavailable, got %v", err)
	}
}
