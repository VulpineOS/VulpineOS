package extensions

import (
	"context"
	"time"
)

// AudioCapturer is the generic interface for starting, stopping, and
// draining audio capture streams from a browser context. The concrete
// source (tab audio, microphone, synthesized tone, etc.) is left to
// the implementation.
type AudioCapturer interface {
	Start(ctx context.Context, req CaptureRequest) (*CaptureHandle, error)
	Stop(ctx context.Context, handleID string) error
	Read(ctx context.Context, handleID string, maxBytes int) (chunk []byte, eof bool, err error)
	Available() bool
}

// CaptureRequest configures a new audio capture session.
type CaptureRequest struct {
	Format     string // "pcm" | "opus" | "wav"
	SampleRate int
	Channels   int
}

// CaptureHandle identifies an active capture session.
type CaptureHandle struct {
	ID        string
	Format    string
	StartedAt time.Time
}

// defaultAudioCapturer is the no-op stub used when no audio capturer
// has been registered. All methods return ErrUnavailable.
var defaultAudioCapturer AudioCapturer = noopAudioCapturer{}

type noopAudioCapturer struct{}

func (noopAudioCapturer) Start(ctx context.Context, req CaptureRequest) (*CaptureHandle, error) {
	return nil, ErrUnavailable
}

func (noopAudioCapturer) Stop(ctx context.Context, handleID string) error {
	return ErrUnavailable
}

func (noopAudioCapturer) Read(ctx context.Context, handleID string, maxBytes int) ([]byte, bool, error) {
	return nil, false, ErrUnavailable
}

func (noopAudioCapturer) Available() bool { return false }
