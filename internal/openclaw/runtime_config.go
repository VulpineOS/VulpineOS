package openclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PrepareScopedConfig clones an existing OpenClaw config and, when cdpURL is set,
// points the browser at that CDP endpoint.
func PrepareScopedConfig(baseConfigPath, cdpURL string) (string, func(), error) {
	if baseConfigPath == "" {
		return "", nil, fmt.Errorf("base OpenClaw config path is required")
	}

	data, err := os.ReadFile(baseConfigPath)
	if err != nil {
		return "", nil, fmt.Errorf("read base OpenClaw config: %w", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", nil, fmt.Errorf("parse base OpenClaw config: %w", err)
	}

	browser, ok := cfg["browser"].(map[string]interface{})
	if !ok || browser == nil {
		browser = make(map[string]interface{})
		cfg["browser"] = browser
	}
	browser["enabled"] = true
	browser["headless"] = true
	if cdpURL != "" {
		browser["cdpUrl"] = cdpURL
	}

	tmpDir, err := os.MkdirTemp("", "vulpine-openclaw-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp OpenClaw config dir: %w", err)
	}

	path := filepath.Join(tmpDir, "openclaw.json")
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("marshal scoped OpenClaw config: %w", err)
	}
	if err := os.WriteFile(path, out, 0600); err != nil {
		os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("write scoped OpenClaw config: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}
	return path, cleanup, nil
}

// PrepareRuntimeConfig clones an existing OpenClaw config without mutating the shared
// profile file, allowing OpenClaw to rewrite the per-run config in isolation.
func PrepareRuntimeConfig(baseConfigPath string) (string, func(), error) {
	return PrepareScopedConfig(baseConfigPath, "")
}
