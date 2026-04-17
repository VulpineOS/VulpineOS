package kernel

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// WindowController manages browser window visibility.
type WindowController struct {
	visible bool
	pid     int
	mu      sync.Mutex
}

// NewWindowController creates a window controller for the given browser PID.
func NewWindowController(pid int) *WindowController {
	return &WindowController{pid: pid}
}

// IsVisible returns whether the browser window is currently shown.
func (w *WindowController) IsVisible() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.visible
}

// Toggle shows the window if hidden, hides if shown.
func (w *WindowController) Toggle() (bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.visible {
		if err := w.hide(); err != nil {
			return w.visible, err
		}
		w.visible = false
	} else {
		if err := w.show(); err != nil {
			return w.visible, err
		}
		w.visible = true
	}
	return w.visible, nil
}

// Show brings the browser window to the front.
func (w *WindowController) Show() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.show(); err != nil {
		return err
	}
	w.visible = true
	return nil
}

// Hide sends the browser window to the background.
func (w *WindowController) Hide() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.hide(); err != nil {
		return err
	}
	w.visible = false
	return nil
}

// HideWhenReady waits for the browser window to appear, then hides it.
func (w *WindowController) HideWhenReady() {
	if runtime.GOOS != "darwin" {
		return
	}

	// Poll until the process has a window, then hide it
	for i := 0; i < 30; i++ { // up to 15 seconds
		time.Sleep(500 * time.Millisecond)
		if err := w.Hide(); err == nil {
			return
		}
	}
}

func (w *WindowController) show() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	pid := strconv.Itoa(w.pid)
	if err := exec.Command("osascript", "-e",
		`tell application "System Events" to set visible of first process whose unix id is `+pid+` to true`,
	).Run(); err != nil {
		return fmt.Errorf("show browser process %d: %w", w.pid, err)
	}
	if err := exec.Command("osascript", "-e",
		`tell application "System Events" to set frontmost of first process whose unix id is `+pid+` to true`,
	).Run(); err != nil {
		return fmt.Errorf("focus browser process %d: %w", w.pid, err)
	}
	return nil
}

func (w *WindowController) hide() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	pid := strconv.Itoa(w.pid)
	if err := exec.Command("osascript", "-e",
		`tell application "System Events" to set visible of first process whose unix id is `+pid+` to false`,
	).Run(); err != nil {
		return fmt.Errorf("hide browser process %d: %w", w.pid, err)
	}
	return nil
}
