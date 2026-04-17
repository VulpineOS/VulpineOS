package openclaw

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"
)

// Gateway manages the OpenClaw gateway daemon process.
type Gateway struct {
	cmd    *exec.Cmd
	binary string
}

// NewGateway creates a gateway manager.
func NewGateway(binary string) *Gateway {
	return &Gateway{binary: binary}
}

// Start launches the OpenClaw gateway in the background.
// Stops any stale gateway from a previous session first.
func (g *Gateway) Start() error {
	if g.cmd != nil {
		return nil // already running
	}

	openclawBin := g.binary
	if openclawBin == "" {
		mgr := NewManager("")
		openclawBin = mgr.findOpenClaw()
	}
	if openclawBin == "" {
		return fmt.Errorf("OpenClaw binary not found")
	}

	// Kill any stale gateway from a previous VulpineOS session
	stopCmd := exec.Command(openclawBin, "--profile", "vulpine", "gateway", "stop")
	stopCmd.Run() // ignore errors — may not be running
	time.Sleep(500 * time.Millisecond)

	args := []string{
		"--profile", "vulpine",
		"gateway",
		"run",
		"--bind", "loopback",
		"--allow-unconfigured",
	}

	g.cmd = exec.Command(openclawBin, args...)

	logPath := os.TempDir() + "/vulpineos-gateway.log"
	if logFile, err := os.Create(logPath); err == nil {
		g.cmd.Stdout = logFile
		g.cmd.Stderr = logFile
	}

	if err := g.cmd.Start(); err != nil {
		return fmt.Errorf("start gateway: %w", err)
	}

	log.Printf("OpenClaw gateway started (PID %d), log: %s", g.cmd.Process.Pid, logPath)
	return g.waitReady(openclawBin)
}

// Stop kills the gateway process.
func (g *Gateway) Stop() {
	if g.cmd != nil && g.cmd.Process != nil {
		g.cmd.Process.Kill()
		g.cmd.Wait()
		g.cmd = nil
		log.Println("OpenClaw gateway stopped")
	}
}

// Running returns true if the gateway process is alive.
func (g *Gateway) Running() bool {
	return g.cmd != nil && g.cmd.ProcessState == nil
}

func (g *Gateway) waitReady(openclawBin string) error {
	deadline := time.Now().Add(15 * time.Second)
	var lastErr error

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		cmd := exec.CommandContext(ctx, openclawBin, "--profile", "vulpine", "gateway", "health")
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
