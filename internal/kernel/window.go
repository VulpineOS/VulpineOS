package kernel

import (
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WindowController manages browser window visibility.
type WindowController struct {
	visible   bool
	pid       int
	targetPID int
	mu        sync.Mutex
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
	for _, pid := range w.candidatePIDs() {
		if err := exec.Command("osascript", "-e",
			`tell application "System Events" to set visible of first process whose unix id is `+strconv.Itoa(pid)+` to true`,
		).Run(); err != nil {
			continue
		}
		if err := exec.Command("osascript", "-e",
			`tell application "System Events" to set frontmost of first process whose unix id is `+strconv.Itoa(pid)+` to true`,
		).Run(); err != nil {
			continue
		}
		w.targetPID = pid
		return nil
	}
	return fmt.Errorf("show browser process tree rooted at %d", w.pid)
}

func (w *WindowController) hide() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	for _, pid := range w.candidatePIDs() {
		if err := exec.Command("osascript", "-e",
			`tell application "System Events" to set visible of first process whose unix id is `+strconv.Itoa(pid)+` to false`,
		).Run(); err != nil {
			continue
		}
		w.targetPID = pid
		return nil
	}
	return fmt.Errorf("hide browser process tree rooted at %d", w.pid)
}

func (w *WindowController) candidatePIDs() []int {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,comm=").Output()
	if err != nil {
		if w.targetPID != 0 {
			return []int{w.targetPID, w.pid}
		}
		return []int{w.pid}
	}

	children := make(map[int][]int)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		children[ppid] = append(children[ppid], pid)
	}

	seen := map[int]struct{}{w.pid: {}}
	queue := []int{w.pid}
	var ordered []int
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		ordered = append(ordered, pid)
		next := children[pid]
		sort.Ints(next)
		for _, child := range next {
			if _, ok := seen[child]; ok {
				continue
			}
			seen[child] = struct{}{}
			queue = append(queue, child)
		}
	}

	// Prefer descendants before the launcher PID on macOS app bundles.
	if len(ordered) <= 1 {
		if w.targetPID != 0 && (len(ordered) == 0 || ordered[0] != w.targetPID) {
			return append([]int{w.targetPID}, ordered...)
		}
		return ordered
	}
	ordered = append(ordered[1:], ordered[0])
	if w.targetPID != 0 {
		filtered := []int{w.targetPID}
		for _, pid := range ordered {
			if pid != w.targetPID {
				filtered = append(filtered, pid)
			}
		}
		return filtered
	}
	return ordered
}
