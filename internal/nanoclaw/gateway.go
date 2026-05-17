package nanoclaw

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"vulpineos/internal/config"
)

// Gateway manages the NanoClaw gateway daemon process.
type Gateway struct {
	cmd           *exec.Cmd
	binary        string
	logFile       *os.File
	mu            sync.Mutex
	exited        bool
	exitCh        chan error
	waitReadyFunc func(string) error
}

// NewGateway creates a gateway manager.
func NewGateway(binary string) *Gateway {
	return &Gateway{binary: binary}
}

// Start launches the NanoClaw gateway in the background.
// Stops any stale gateway from a previous session first.
func (g *Gateway) Start() error {
	g.mu.Lock()
	if g.cmd != nil && !g.exited {
		g.mu.Unlock()
		return nil // already running
	}
	g.mu.Unlock()

	nanoclawBin := g.binary
	if nanoclawBin == "" {
		mgr := NewManager("")
		nanoclawBin = mgr.findNanoClaw()
	}
	if nanoclawBin == "" {
		return fmt.Errorf("NanoClaw binary not found")
	}

	// Kill any stale gateway from a previous VulpineOS session
	stopCmd := exec.Command(nanoclawBin, "gateway", "stop")
	stopCmd.Run() // ignore errors — may not be running
	time.Sleep(500 * time.Millisecond)

	args := []string{
		"--profile", "vulpine",
		"gateway",
		"run",
		"--bind", "loopback",
		"--allow-unconfigured",
	}

	cmd := exec.Command(nanoclawBin, args...)

	if cfg, err := config.Load(); err == nil && cfg.Provider == "openrouter" {
		cmd.Env = append(os.Environ(),
			"OPENCODE_PROVIDER=openrouter",
			"OPENCODE_MODEL="+cfg.Model,
		)
	}

	logPath := os.TempDir() + "/vulpineos-gateway.log"
	if logFile, err := os.Create(logPath); err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		g.mu.Lock()
		g.logFile = logFile
		g.mu.Unlock()
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start gateway: %w", err)
	}

	g.mu.Lock()
	g.cmd = cmd
	g.exited = false
	g.exitCh = make(chan error, 1)
	exitCh := g.exitCh
	g.mu.Unlock()

	go func() {
		err := cmd.Wait()
		g.mu.Lock()
		if g.cmd == cmd {
			g.exited = true
		}
		g.mu.Unlock()
		exitCh <- err
		close(exitCh)
	}()

	log.Printf("NanoClaw gateway started (PID %d), log: %s", cmd.Process.Pid, logPath)
	waitReady := g.waitReady
	if g.waitReadyFunc != nil {
		waitReady = g.waitReadyFunc
	}
	if err := waitReady(nanoclawBin); err != nil {
		g.Stop()
		return err
	}
	return nil
}

// Stop kills the gateway process.
func (g *Gateway) Stop() {
	g.mu.Lock()
	cmd := g.cmd
	exitCh := g.exitCh
	exited := g.exited
	logFile := g.logFile
	g.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		if !exited {
			_ = cmd.Process.Kill()
		}
		if exitCh != nil {
			<-exitCh
		}
		g.mu.Lock()
		if g.cmd == cmd {
			g.cmd = nil
			g.exitCh = nil
			g.exited = true
			g.logFile = nil
		}
		g.mu.Unlock()
		if logFile != nil {
			_ = logFile.Close()
		}
		log.Println("NanoClaw gateway stopped")
	}
}

// Running returns true if the gateway process is alive.
func (g *Gateway) Running() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.cmd != nil && !g.exited
}

func (g *Gateway) waitReady(nanoclawBin string) error {
	deadline := time.Now().Add(15 * time.Second)
	var lastErr error

	for time.Now().Before(deadline) {
		// The OpenClaw health CLI regularly takes ~2.5s wall time on this host
		// even when the gateway probe itself reports "OK (0ms)". Give the probe
		// enough headroom so Gateway.Start does not fail on a false timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		cmd := exec.CommandContext(ctx, nanoclawBin, "--profile", "vulpine", "gateway", "health")
		if err := cmd.Run(); err == nil {
			cancel()
			return nil
		} else {
			lastErr = err
		}
		cancel()
		time.Sleep(500 * time.Millisecond)
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("gateway health probe timed out")
	}
	return fmt.Errorf("wait for gateway readiness: %w", lastErr)
}
