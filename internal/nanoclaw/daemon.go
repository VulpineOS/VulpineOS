package nanoclaw

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"vulpineos/internal/config"
)

// Daemon manages the NanoClaw daemon process.
type Daemon struct {
	cmd        *exec.Cmd
	binary     string
	socketPath string
	mu         sync.Mutex
	exited     bool
	stopped    bool
	exitCh     chan error
}

// NewDaemon creates a new daemon manager.
func NewDaemon(binary string) *Daemon {
	return &Daemon{binary: binary}
}

// Start launches the NanoClaw daemon and waits for the socket to be ready.
func (d *Daemon) Start() error {
	d.mu.Lock()
	if d.cmd != nil && !d.exited {
		d.mu.Unlock()
		return nil // already running
	}
	d.mu.Unlock()

	nanoclawBin := d.binary
	if nanoclawBin == "" {
		mgr := NewManager("")
		nanoclawBin = mgr.findNanoClaw()
	}
	if nanoclawBin == "" {
		return fmt.Errorf("NanoClaw binary not found")
	}

	// Build command: run the daemon with vulpine profile
	args := []string{
		"--profile", "vulpine",
	}

	cmd := exec.Command(nanoclawBin, args...)

	// Inject OpenRouter env vars if configured
	if cfg, err := config.Load(); err == nil && cfg.Provider == "openrouter" {
		cmd.Env = append(os.Environ(),
			"OPENCODE_PROVIDER=openrouter",
			"OPENCODE_MODEL="+cfg.Model,
		)
	}

	// Log to temp file
	logPath := os.TempDir() + "/vulpineos-nanoclaw.log"
	if logFile, err := os.Create(logPath); err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start nanoclaw daemon: %w", err)
	}

	d.mu.Lock()
	d.cmd = cmd
	d.exited = false
	d.exitCh = make(chan error, 1)
	exitCh := d.exitCh
	d.mu.Unlock()

	go func() {
		err := cmd.Wait()
		d.mu.Lock()
		if d.cmd == cmd {
			d.exited = true
		}
		d.mu.Unlock()
		exitCh <- err
		close(exitCh)
	}()

	// Wait for socket to appear
	nanoclawDir := findNanoclawDirForDaemon(nanoclawBin)
	if nanoclawDir == "" {
		return fmt.Errorf("NanoClaw directory not found")
	}
	d.socketPath = filepath.Join(nanoclawDir, "data", "cli.sock")

	log.Printf("NanoClaw daemon starting, waiting for socket at %s", d.socketPath)

	for i := 0; i < 30; i++ { // 15 seconds max
		if _, err := os.Stat(d.socketPath); err == nil {
			log.Printf("NanoClaw daemon ready (socket found)")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("NanoClaw daemon did not create socket within 15 seconds")
}

// Stop gracefully terminates the daemon.
func (d *Daemon) Stop() error {
	d.mu.Lock()
	if d.cmd == nil || d.exited || d.stopped {
		d.mu.Unlock()
		return nil
	}
	d.stopped = true
	cmd := d.cmd
	exitCh := d.exitCh
	d.mu.Unlock()

	if cmd.Process != nil {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			cmd.Process.Kill()
		}
	}

	select {
	case <-exitCh:
		return nil
	case <-time.After(5 * time.Second):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return fmt.Errorf("daemon did not exit within 5 seconds")
	}
}

// Running returns true if the daemon process is alive.
func (d *Daemon) Running() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cmd != nil && !d.exited
}

// findNanoclawDirForDaemon locates the nanoclaw directory without requiring
// the socket to already exist (unlike GetNanoclawDir).
func findNanoclawDirForDaemon(binary string) string {
	// Try deriving from binary path first
	binDir := filepath.Dir(binary)
	if filepath.Base(binDir) == "nanoclaw" {
		if _, err := os.Stat(filepath.Join(binDir, "data")); err == nil {
			return binDir
		}
	}

	// Fall back to walking up from cwd looking for nanoclaw/data
	cwd, _ := os.Getwd()
	dir := cwd
	for i := 0; i < 5; i++ {
		nanoclawDir := filepath.Join(dir, "nanoclaw")
		if _, err := os.Stat(filepath.Join(nanoclawDir, "data")); err == nil {
			return nanoclawDir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
