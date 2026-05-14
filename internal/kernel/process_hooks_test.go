package kernel

import (
	"os/exec"
	"testing"
)

func TestKernelProcessHooksApplyInRegistrationOrder(t *testing.T) {
	clearKernelProcessHooksForTest()
	t.Cleanup(clearKernelProcessHooksForTest)

	registerKernelProcessHook(func(cmd *exec.Cmd) {
		cmd.Env = append(cmd.Env, "FIRST=1")
	})
	registerKernelProcessHook(func(cmd *exec.Cmd) {
		cmd.Env = append(cmd.Env, "SECOND=2")
	})

	cmd := exec.Command("test")
	applyKernelProcessHooks(cmd)

	if got := cmd.Env; len(got) != 2 || got[0] != "FIRST=1" || got[1] != "SECOND=2" {
		t.Fatalf("cmd.Env = %#v", got)
	}
}
