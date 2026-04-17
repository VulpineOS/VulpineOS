package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withTempHome(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
	})
	return tmpHome
}

func TestConfigSaveLoad(t *testing.T) {
	// Create temp dir to act as config dir
	tmpDir, err := os.MkdirTemp("", "vulpine-config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write config directly to temp path
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := &Config{
		Provider:      "anthropic",
		APIKey:        "sk-ant-test-key-12345",
		Model:         "anthropic/claude-sonnet-4-6",
		SetupComplete: true,
		BinaryPath:    "/usr/local/bin/camoufox",
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Load it back
	loadedData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(loadedData, &loaded); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	if loaded.Provider != cfg.Provider {
		t.Errorf("provider = %q, want %q", loaded.Provider, cfg.Provider)
	}
	if loaded.APIKey != cfg.APIKey {
		t.Errorf("apiKey = %q, want %q", loaded.APIKey, cfg.APIKey)
	}
	if loaded.Model != cfg.Model {
		t.Errorf("model = %q, want %q", loaded.Model, cfg.Model)
	}
	if loaded.SetupComplete != cfg.SetupComplete {
		t.Errorf("setupComplete = %v, want %v", loaded.SetupComplete, cfg.SetupComplete)
	}
	if loaded.BinaryPath != cfg.BinaryPath {
		t.Errorf("binaryPath = %q, want %q", loaded.BinaryPath, cfg.BinaryPath)
	}
	if loaded.NeedsSetup() {
		t.Error("loaded config should not need setup")
	}
}

func TestConfigSkillManagement(t *testing.T) {
	cfg := &Config{}

	// AddGlobalSkill
	cfg.AddGlobalSkill("web-search", map[string]string{"SERP_API_KEY": "key123"})
	if len(cfg.GlobalSkills) != 1 {
		t.Fatalf("expected 1 global skill, got %d", len(cfg.GlobalSkills))
	}
	if cfg.GlobalSkills[0].Name != "web-search" {
		t.Errorf("skill name = %q, want 'web-search'", cfg.GlobalSkills[0].Name)
	}
	if !cfg.GlobalSkills[0].Enabled {
		t.Error("skill should be enabled")
	}
	if cfg.GlobalSkills[0].Env["SERP_API_KEY"] != "key123" {
		t.Errorf("env var = %q, want 'key123'", cfg.GlobalSkills[0].Env["SERP_API_KEY"])
	}

	// Add another skill
	cfg.AddGlobalSkill("code-runner", nil)
	if len(cfg.GlobalSkills) != 2 {
		t.Fatalf("expected 2 global skills, got %d", len(cfg.GlobalSkills))
	}

	// RemoveGlobalSkill (disables, doesn't delete)
	cfg.RemoveGlobalSkill("web-search")
	if cfg.GlobalSkills[0].Enabled {
		t.Error("web-search should be disabled after RemoveGlobalSkill")
	}
	if !cfg.GlobalSkills[1].Enabled {
		t.Error("code-runner should still be enabled")
	}

	// Re-add same skill should re-enable
	cfg.AddGlobalSkill("web-search", map[string]string{"SERP_API_KEY": "newkey"})
	if !cfg.GlobalSkills[0].Enabled {
		t.Error("web-search should be re-enabled")
	}
	if cfg.GlobalSkills[0].Env["SERP_API_KEY"] != "newkey" {
		t.Errorf("env var should be updated to 'newkey', got %q", cfg.GlobalSkills[0].Env["SERP_API_KEY"])
	}

	// AddAgentSkill
	cfg.AddAgentSkill("agent-001", "browser-use", nil)
	if cfg.AgentSkills == nil {
		t.Fatal("AgentSkills map should be initialized")
	}
	skills := cfg.AgentSkills["agent-001"]
	if len(skills) != 1 {
		t.Fatalf("expected 1 agent skill, got %d", len(skills))
	}
	if skills[0].Name != "browser-use" {
		t.Errorf("agent skill name = %q, want 'browser-use'", skills[0].Name)
	}

	// Add another skill to same agent
	cfg.AddAgentSkill("agent-001", "file-editor", map[string]string{"SANDBOX": "true"})
	skills = cfg.AgentSkills["agent-001"]
	if len(skills) != 2 {
		t.Fatalf("expected 2 agent skills, got %d", len(skills))
	}

	// Add skill to different agent
	cfg.AddAgentSkill("agent-002", "web-search", nil)
	if len(cfg.AgentSkills["agent-002"]) != 1 {
		t.Errorf("expected 1 skill for agent-002, got %d", len(cfg.AgentSkills["agent-002"]))
	}

	// RemoveGlobalSkill on non-existent should be a no-op
	cfg.RemoveGlobalSkill("nonexistent-skill")
	if len(cfg.GlobalSkills) != 2 {
		t.Errorf("skill count should not change for non-existent removal")
	}
}

func TestGenerateOpenClawConfig(t *testing.T) {
	tmpHome := withTempHome(t)
	skillDir := filepath.Join(tmpHome, ".vulpineos", "skills")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Provider:               "anthropic",
		APIKey:                 "sk-ant-test-key-99999",
		Model:                  "anthropic/claude-sonnet-4-6",
		SetupComplete:          true,
		ResizePanelsWithArrows: true,
		GlobalSkills: []SkillEntry{
			{Name: "web-search", Enabled: true, Env: map[string]string{"KEY": "val"}},
			{Name: "disabled-skill", Enabled: false},
		},
	}

	// GenerateOpenClawConfig writes to Dir() which we can't easily override,
	// so we test the output shape by calling it and checking the file.
	if err := cfg.GenerateOpenClawConfig("/usr/local/bin/vulpineos", "/usr/local/bin/camoufox"); err != nil {
		t.Fatalf("GenerateOpenClawConfig: %v", err)
	}

	// Read the generated file
	ocPath := OpenClawConfigPath()
	data, err := os.ReadFile(ocPath)
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}

	var oc map[string]interface{}
	if err := json.Unmarshal(data, &oc); err != nil {
		t.Fatalf("openclaw.json not valid JSON: %v", err)
	}

	// Verify browser.enabled is false
	browser, ok := oc["browser"].(map[string]interface{})
	if !ok {
		t.Fatal("missing browser section")
	}
	if browser["enabled"] != true {
		t.Errorf("browser.enabled = %v, want true", browser["enabled"])
	}

	// Verify skills section exists with our test skill
	skillsSec, skillsOK := oc["skills"].(map[string]interface{})
	if !skillsOK {
		t.Fatal("missing skills section")
	}
	skillEntries, seOK := skillsSec["entries"].(map[string]interface{})
	if !seOK {
		t.Fatal("missing skills.entries section")
	}
	webSearch, wsOK := skillEntries["web-search"].(map[string]interface{})
	if !wsOK {
		t.Fatal("missing skills.entries.web-search")
	}
	if webSearch["enabled"] != true {
		t.Errorf("web-search enabled = %v, want true", webSearch["enabled"])
	}

	// Verify env contains ANTHROPIC_API_KEY
	env, ok := oc["env"].(map[string]interface{})
	if !ok {
		t.Fatal("missing env section")
	}
	if env["ANTHROPIC_API_KEY"] != "sk-ant-test-key-99999" {
		t.Errorf("env ANTHROPIC_API_KEY = %v, want sk-ant-test-key-99999", env["ANTHROPIC_API_KEY"])
	}

	// Verify model is set
	agents, ok := oc["agents"].(map[string]interface{})
	if !ok {
		t.Fatal("missing agents section")
	}
	defaults, ok := agents["defaults"].(map[string]interface{})
	if !ok {
		t.Fatal("missing agents.defaults")
	}
	model, ok := defaults["model"].(map[string]interface{})
	if !ok {
		t.Fatal("missing agents.defaults.model")
	}
	if model["primary"] != "anthropic/claude-sonnet-4-6" {
		t.Errorf("model.primary = %v, want anthropic/claude-sonnet-4-6", model["primary"])
	}

	defaults, _ = agents["defaults"].(map[string]interface{})
	if defaults["workspace"] != OpenClawWorkspaceDir() {
		t.Errorf("workspace = %v, want %q", defaults["workspace"], OpenClawWorkspaceDir())
	}

	if _, err := os.Stat(filepath.Join(OpenClawWorkspaceDir(), "AGENTS.md")); err != nil {
		t.Fatalf("expected isolated workspace bootstrap files: %v", err)
	}
	skillPath := filepath.Join(OpenClawProfileDir(), "skills", "vulpine-browser", "SKILL.md")
	skillData, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("expected patched vulpine-browser skill: %v", err)
	}
	if !strings.Contains(string(skillData), "Do not call `vulpineos-browser`") {
		t.Fatalf("expected skill to forbid vulpineos-browser helper, got:\n%s", string(skillData))
	}
}

func TestGenerateOpenClawConfigPreservesGatewayAndCommands(t *testing.T) {
	withTempHome(t)

	profileDir := OpenClawProfileDir()
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}

	existing := map[string]interface{}{
		"gateway": map[string]interface{}{
			"mode":      "remote",
			"authToken": "keep-me",
			"url":       "https://gateway.example.com",
		},
		"commands": map[string]interface{}{
			"native":       "manual",
			"nativeSkills": "manual",
			"restart":      false,
		},
		"meta": map[string]interface{}{
			"profile": "vulpine",
		},
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		t.Fatalf("marshal existing config: %v", err)
	}
	if err := os.WriteFile(OpenClawConfigPath(), data, 0600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	cfg := &Config{
		Provider:      "anthropic",
		APIKey:        "sk-ant-test-key-99999",
		Model:         "anthropic/claude-sonnet-4-6",
		SetupComplete: true,
	}
	if err := cfg.GenerateOpenClawConfig("/usr/local/bin/vulpineos", "/usr/local/bin/camoufox"); err != nil {
		t.Fatalf("GenerateOpenClawConfig: %v", err)
	}

	written, err := os.ReadFile(OpenClawConfigPath())
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}

	var oc map[string]interface{}
	if err := json.Unmarshal(written, &oc); err != nil {
		t.Fatalf("openclaw.json not valid JSON: %v", err)
	}

	gateway, ok := oc["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("missing gateway section")
	}
	if gateway["authToken"] != "keep-me" {
		t.Fatalf("gateway authToken = %v, want keep-me", gateway["authToken"])
	}
	if gateway["url"] != "https://gateway.example.com" {
		t.Fatalf("gateway url = %v, want preserved remote URL", gateway["url"])
	}
	if gateway["mode"] != "local" {
		t.Fatalf("gateway mode = %v, want local override", gateway["mode"])
	}

	commands, ok := oc["commands"].(map[string]interface{})
	if !ok {
		t.Fatal("missing commands section")
	}
	if commands["native"] != "manual" {
		t.Fatalf("commands.native = %v, want manual", commands["native"])
	}
	if commands["restart"] != false {
		t.Fatalf("commands.restart = %v, want false", commands["restart"])
	}

	meta, ok := oc["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("missing meta section")
	}
	if meta["profile"] != "vulpine" {
		t.Fatalf("meta.profile = %v, want vulpine", meta["profile"])
	}
}

func TestGenerateOpenClawConfigRewritesStaleVulpineBrowserSkill(t *testing.T) {
	withTempHome(t)

	staleSkillDir := filepath.Join(OpenClawProfileDir(), "skills", "vulpine-browser")
	if err := os.MkdirAll(staleSkillDir, 0700); err != nil {
		t.Fatalf("mkdir stale skill dir: %v", err)
	}
	stalePath := filepath.Join(staleSkillDir, "SKILL.md")
	stale := "Use vulpineos-browser shell command for everything."
	if err := os.WriteFile(stalePath, []byte(stale), 0600); err != nil {
		t.Fatalf("write stale skill: %v", err)
	}

	cfg := &Config{
		Provider:      "anthropic",
		APIKey:        "sk-ant-test-key-99999",
		Model:         "anthropic/claude-sonnet-4-6",
		SetupComplete: true,
	}
	if err := cfg.GenerateOpenClawConfig("/usr/local/bin/vulpineos", "/usr/local/bin/camoufox"); err != nil {
		t.Fatalf("GenerateOpenClawConfig: %v", err)
	}

	skillData, err := os.ReadFile(stalePath)
	if err != nil {
		t.Fatalf("read rewritten skill: %v", err)
	}
	body := string(skillData)
	if strings.Contains(body, "shell command for everything") {
		t.Fatalf("expected stale skill body to be replaced, got:\n%s", body)
	}
	if !strings.Contains(body, "Use the `browser` tool directly") {
		t.Fatalf("expected rewritten skill to instruct built-in browser tool, got:\n%s", body)
	}
	if !strings.Contains(body, "Do not call `vulpineos-browser`") {
		t.Fatalf("expected rewritten skill to forbid helper command, got:\n%s", body)
	}
}

func TestGenerateOpenClawConfigPreservesGatewayAuthAndCommands(t *testing.T) {
	withTempHome(t)

	if err := os.MkdirAll(OpenClawProfileDir(), 0700); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	existing := map[string]interface{}{
		"gateway": map[string]interface{}{
			"mode": "local",
			"auth": map[string]interface{}{
				"mode":  "token",
				"token": "keep-me",
			},
		},
		"commands": map[string]interface{}{
			"ownerDisplay": "raw",
			"restart":      true,
		},
		"meta": map[string]interface{}{
			"lastTouchedVersion": "2026.3.13",
		},
	}
	data, err := json.Marshal(existing)
	if err != nil {
		t.Fatalf("marshal existing config: %v", err)
	}
	if err := os.WriteFile(OpenClawConfigPath(), data, 0600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	cfg := &Config{
		Provider:      "anthropic",
		APIKey:        "sk-ant-test-key-99999",
		Model:         "anthropic/claude-sonnet-4-6",
		SetupComplete: true,
	}
	if err := cfg.GenerateOpenClawConfig("/usr/local/bin/vulpineos", "/usr/local/bin/camoufox"); err != nil {
		t.Fatalf("GenerateOpenClawConfig: %v", err)
	}

	out, err := os.ReadFile(OpenClawConfigPath())
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}

	var oc map[string]interface{}
	if err := json.Unmarshal(out, &oc); err != nil {
		t.Fatalf("parse generated config: %v", err)
	}
	gateway := oc["gateway"].(map[string]interface{})
	auth := gateway["auth"].(map[string]interface{})
	if auth["token"] != "keep-me" {
		t.Fatalf("gateway auth token = %v, want keep-me", auth["token"])
	}
	commands := oc["commands"].(map[string]interface{})
	if commands["ownerDisplay"] != "raw" {
		t.Fatalf("ownerDisplay = %v, want raw", commands["ownerDisplay"])
	}
	meta := oc["meta"].(map[string]interface{})
	if meta["lastTouchedVersion"] != "2026.3.13" {
		t.Fatalf("meta.lastTouchedVersion = %v, want 2026.3.13", meta["lastTouchedVersion"])
	}
}

func TestGenerateOpenClawConfig_UnknownProvider(t *testing.T) {
	cfg := &Config{
		Provider: "nonexistent-provider",
		APIKey:   "key",
		Model:    "model",
	}
	err := cfg.GenerateOpenClawConfig("/bin/vulpineos", "")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestGetProvider(t *testing.T) {
	p := GetProvider("anthropic")
	if p == nil {
		t.Fatal("expected non-nil provider for 'anthropic'")
	}
	if p.Name != "Anthropic (Claude)" {
		t.Errorf("name = %q, want 'Anthropic (Claude)'", p.Name)
	}
	if p.EnvVar != "ANTHROPIC_API_KEY" {
		t.Errorf("envVar = %q, want ANTHROPIC_API_KEY", p.EnvVar)
	}

	p = GetProvider("nonexistent")
	if p != nil {
		t.Error("expected nil for unknown provider")
	}

	// Ollama doesn't need a key
	p = GetProvider("ollama")
	if p == nil {
		t.Fatal("expected non-nil provider for 'ollama'")
	}
	if p.NeedsKey {
		t.Error("ollama should not need a key")
	}
}

func TestCustomProvider(t *testing.T) {
	cp := CustomProvider("my-provider", "MY_KEY")
	if cp.ID != "my-provider" {
		t.Errorf("id = %q, want 'my-provider'", cp.ID)
	}
	if !cp.NeedsKey {
		t.Error("custom provider with envVar should need key")
	}

	cp2 := CustomProvider("local-llm", "")
	if cp2.NeedsKey {
		t.Error("custom provider without envVar should not need key")
	}
}

func TestNeedsSetup(t *testing.T) {
	cfg := &Config{}
	if !cfg.NeedsSetup() {
		t.Error("empty config should need setup")
	}

	cfg.SetupComplete = true
	if cfg.NeedsSetup() {
		t.Error("config with setupComplete should not need setup")
	}
}
