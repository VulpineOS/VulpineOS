package nanoclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// VulpineOSConfig generates an openclaw.json that routes browser ops through VulpineOS MCP.
type VulpineOSConfig struct {
	Plugins PluginsConfig `json:"plugins"`
	Browser BrowserConfig `json:"browser"`
}

type PluginsConfig struct {
	MCP MCPConfig `json:"mcp"`
}

type MCPConfig struct {
	Servers map[string]MCPServerConfig `json:"servers"`
}

type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

type BrowserConfig struct {
	Enabled bool `json:"enabled"`
}

// GenerateConfig creates an openclaw.json config file that disables the built-in
// Chromium browser and routes all browser operations through the VulpineOS MCP server.
func GenerateConfig(vulpineosBinary string, wsURL string) (*VulpineOSConfig, error) {
	if vulpineosBinary == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("get executable path: %w", err)
		}
		vulpineosBinary = exe
	}

	args := []string{"mcp"}
	if wsURL != "" {
		args = append(args, "--connect", wsURL)
	}

	return &VulpineOSConfig{
		Plugins: PluginsConfig{
			MCP: MCPConfig{
				Servers: map[string]MCPServerConfig{
					"vulpineos": {
						Command: vulpineosBinary,
						Args:    args,
						Env:     map[string]string{},
					},
				},
			},
		},
		Browser: BrowserConfig{
			Enabled: false, // Disable Chromium — use VulpineOS instead
		},
	}, nil
}

// WriteConfig writes the OpenClaw config to a file.
func WriteConfig(config *VulpineOSConfig, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}

	path := filepath.Join(dir, "openclaw.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return "", fmt.Errorf("chmod config: %w", err)
	}

	return path, nil
}

// FindBundledNanoClaw checks if NanoClaw is bundled alongside the vulpineos binary.
func FindBundledNanoClaw() (nodePath, nanoclawPath string, found bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", false
	}
	dir := filepath.Dir(exe)

	// Check for bundled nanoclaw directory
	nanoclawDir := filepath.Join(dir, "nanoclaw")
	nodeBin := filepath.Join(nanoclawDir, "node")
	nanoclawMain := filepath.Join(nanoclawDir, "node_modules", "nanoclaw", "bin", "nanoclaw")

	if _, err := os.Stat(nodeBin); err != nil {
		nodeBin = "node"
	}

	if _, err := os.Stat(nanoclawMain); err == nil {
		return nodeBin, nanoclawMain, true
	}

	// Check if nanoclaw is globally installed
	if path, err := filepath.Glob("/usr/local/lib/node_modules/nanoclaw"); err == nil && len(path) > 0 {
		nanoclawBin := filepath.Join(path[0], "bin", "nanoclaw")
		if _, err := os.Stat(nanoclawBin); err == nil {
			return "node", nanoclawBin, true
		}
		return "node", path[0], true
	}

	return "", "", false
}
