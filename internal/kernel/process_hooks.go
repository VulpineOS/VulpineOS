package kernel

import (
	"os/exec"
	"sync"
)

var kernelProcessHooks = struct {
	sync.RWMutex
	hooks []func(*exec.Cmd)
}{}

func registerKernelProcessHook(hook func(*exec.Cmd)) {
	if hook == nil {
		return
	}
	kernelProcessHooks.Lock()
	defer kernelProcessHooks.Unlock()
	kernelProcessHooks.hooks = append(kernelProcessHooks.hooks, hook)
}

func applyKernelProcessHooks(cmd *exec.Cmd) {
	kernelProcessHooks.RLock()
	hooks := append([]func(*exec.Cmd){}, kernelProcessHooks.hooks...)
	kernelProcessHooks.RUnlock()
	for _, hook := range hooks {
		hook(cmd)
	}
}

func clearKernelProcessHooksForTest() {
	kernelProcessHooks.Lock()
	defer kernelProcessHooks.Unlock()
	kernelProcessHooks.hooks = nil
}
