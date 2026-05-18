package nanoclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PrepareScopedConfig clones an existing NanoClaw config and, when cdpURL is set,
// points the browser at that CDP endpoint.
func PrepareScopedConfig(baseConfigPath, cdpURL string) (string, func(), error) {
	if baseConfigPath == "" {
		return "", nil, fmt.Errorf("base NanoClaw config path is required")
	}

	data, err := os.ReadFile(baseConfigPath)
	if err != nil {
		return "", nil, fmt.Errorf("read base NanoClaw config: %w", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", nil, fmt.Errorf("parse base NanoClaw config: %w", err)
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

	tmpDir, err := os.MkdirTemp("", "vulpine-nanoclaw-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp NanoClaw config dir: %w", err)
	}

	path := filepath.Join(tmpDir, "nanoclaw.json")
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("marshal scoped NanoClaw config: %w", err)
	}
	if err := os.WriteFile(path, out, 0600); err != nil {
		os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("write scoped NanoClaw config: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}
	return path, cleanup, nil
}

// PrepareRuntimeConfig clones an existing NanoClaw config without mutating the shared
// profile file, allowing NanoClaw to rewrite the per-run config in isolation.
func PrepareRuntimeConfig(baseConfigPath string) (string, func(), error) {
	return PrepareScopedConfig(baseConfigPath, "")
}
