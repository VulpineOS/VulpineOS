//go:build windows

package kernel

import "os/exec"

func configureKernelProcess(cmd *exec.Cmd) {}

func killKernelProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
