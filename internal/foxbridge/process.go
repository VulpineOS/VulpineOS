package foxbridge

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"vulpineos/internal/juggler"
	"vulpineos/internal/runtimeaudit"
)

// Process manages the foxbridge CDP-to-Juggler proxy process.
// Supports two modes: external subprocess (Start) or embedded in-process (StartEmbeddedMode).
type Process struct {
	cmd      *exec.Cmd
	port     int
	binary   string
	embedded *EmbeddedServer // non-nil when running in embedded mode
	audit    *runtimeaudit.Manager
}

// Config holds foxbridge startup configuration.
type Config struct {
	CamoufoxBinary string // path to camoufox/firefox binary
	Port           int    // CDP port (default 9222)
	Headless       bool
	ProfileDir     string
}

// New creates a new foxbridge process manager.
func New() *Process {
	return &Process{port: 9222}
}

// SetRuntimeAudit attaches a runtime audit manager.
func (p *Process) SetRuntimeAudit(audit *runtimeaudit.Manager) {
	p.audit = audit
}

// Start launches foxbridge, which in turn launches Camoufox with Juggler pipe.
func (p *Process) Start(cfg Config) error {
	if p.cmd != nil {
		return nil // already running
	}

	bin := findFoxbridge()
	if bin == "" {
		err := fmt.Errorf("foxbridge binary not found (install with: go install github.com/VulpineOS/foxbridge/cmd/foxbridge@latest)")
		p.logRuntimeEvent("error", "start_failed", "foxbridge binary not found", nil)
		return err
	}
	p.binary = bin

	port := cfg.Port
	if port == 0 {
		port = 9222
	}
	p.port = port

	args := []string{
		"--port", fmt.Sprintf("%d", port),
	}
	if cfg.CamoufoxBinary != "" {
		args = append(args, "--binary", cfg.CamoufoxBinary)
	}
	if cfg.Headless {
		args = append(args, "--headless")
	}
	if cfg.ProfileDir != "" {
		args = append(args, "--profile", cfg.ProfileDir)
	}

	p.cmd = exec.Command(bin, args...)

	// Log foxbridge output
	logPath := filepath.Join(os.TempDir(), "vulpineos-foxbridge.log")
	if logFile, err := os.Create(logPath); err == nil {
		p.cmd.Stdout = logFile
		p.cmd.Stderr = logFile
	}

	if err := p.cmd.Start(); err != nil {
		p.logRuntimeEvent("error", "start_failed", "foxbridge failed to start", map[string]string{
			"error": err.Error(),
			"mode":  "external",
		})
		return fmt.Errorf("start foxbridge: %w", err)
	}

	log.Printf("foxbridge started (PID %d, port %d), log: %s", p.cmd.Process.Pid, port, logPath)

	// Wait for the CDP port to become available
	if err := waitForPort(port, 15*time.Second); err != nil {
		p.Stop()
		p.logRuntimeEvent("error", "start_failed", "foxbridge port did not become ready", map[string]string{
			"error": err.Error(),
			"mode":  "external",
			"port":  fmt.Sprintf("%d", port),
		})
		return fmt.Errorf("foxbridge failed to start: %w", err)
	}

	log.Printf("foxbridge CDP proxy ready on ws://127.0.0.1:%d", port)
	p.logRuntimeEvent("info", "started", "foxbridge external proxy started", map[string]string{
		"mode": "external",
		"port": fmt.Sprintf("%d", port),
	})
	return nil
}

// StartEmbeddedMode starts foxbridge as an in-process CDP server wrapping an existing Juggler client.
// This avoids launching a second Firefox process — the CDP server shares the kernel's connection.
func (p *Process) StartEmbeddedMode(client *juggler.Client, port int) error {
	if p.cmd != nil || p.embedded != nil {
		return nil // already running
	}
	if port == 0 {
		port = 9222
	}
	p.port = port

	es, err := StartEmbedded(client, port)
	if err != nil {
		p.logRuntimeEvent("error", "start_failed", "embedded foxbridge failed to start", map[string]string{
			"error": err.Error(),
			"mode":  "embedded",
		})
		return fmt.Errorf("start embedded foxbridge: %w", err)
	}
	p.embedded = es

	// Wait briefly for the HTTP server to bind.
	if err := waitForPort(port, 5*time.Second); err != nil {
		p.embedded = nil
		p.logRuntimeEvent("error", "start_failed", "embedded foxbridge port not ready", map[string]string{
			"error": err.Error(),
			"mode":  "embedded",
			"port":  fmt.Sprintf("%d", port),
		})
		return fmt.Errorf("embedded foxbridge port not ready: %w", err)
	}

	log.Printf("foxbridge embedded CDP proxy ready on ws://127.0.0.1:%d", port)
	p.logRuntimeEvent("info", "started", "foxbridge embedded proxy started", map[string]string{
		"mode": "embedded",
		"port": fmt.Sprintf("%d", port),
	})
	return nil
}

// Stop kills the foxbridge process or stops the embedded server.
func (p *Process) Stop() {
	if p.embedded != nil {
		p.embedded.Stop()
		p.embedded = nil
		p.logRuntimeEvent("info", "stopped", "foxbridge embedded proxy stopped", map[string]string{
			"mode": "embedded",
			"port": fmt.Sprintf("%d", p.port),
		})
		return
	}
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
		p.cmd = nil
		log.Println("foxbridge stopped")
		p.logRuntimeEvent("info", "stopped", "foxbridge external proxy stopped", map[string]string{
			"mode": "external",
			"port": fmt.Sprintf("%d", p.port),
		})
	}
}

// Running returns true if foxbridge is alive.
func (p *Process) Running() bool {
	if p.embedded != nil {
		return true
	}
	return p.cmd != nil && p.cmd.ProcessState == nil
}

// CDPURL returns the CDP WebSocket URL for connecting clients.
func (p *Process) CDPURL() string {
	return fmt.Sprintf("ws://127.0.0.1:%d", p.port)
}

// Port returns the CDP port.
func (p *Process) Port() int {
	return p.port
}

// PID returns the process ID.
func (p *Process) PID() int {
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// findFoxbridge searches for the foxbridge binary in common locations.
func findFoxbridge() string {
	// 1. Next to our own executable
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, "foxbridge")
		if runtime.GOOS == "windows" {
			candidate += ".exe"
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 2. In current working directory
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, "foxbridge")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 3. GOPATH/bin
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		candidate := filepath.Join(gopath, "bin", "foxbridge")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 4. ~/go/bin (default GOPATH)
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, "go", "bin", "foxbridge")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 5. System PATH
	if path, err := exec.LookPath("foxbridge"); err == nil {
		return path
	}

	return ""
}

// waitForPort polls until a TCP port is accepting connections.
func waitForPort(port int, timeout time.Duration) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("port %d not ready after %v", port, timeout)
}

func (p *Process) logRuntimeEvent(level, event, message string, metadata map[string]string) {
	if p.audit == nil {
		return
	}
	if _, err := p.audit.Log("foxbridge", level, event, message, metadata); err != nil {
		log.Printf("runtime audit foxbridge %s: %v", event, err)
	}
}
