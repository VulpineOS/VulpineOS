//go:build windows

package foxbridge

import "os/exec"

func configureProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
