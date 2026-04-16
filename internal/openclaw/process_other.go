//go:build !darwin && !linux

package openclaw

import "os/exec"

func configureAgentProcess(cmd *exec.Cmd) {}

func killAgentProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
