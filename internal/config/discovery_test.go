package config

import (
	"os/exec"
	"strings"
	"testing"
)

func TestFindNanoClawBinary(t *testing.T) {
	path := findNanoClawBinary()
	if path == "" {
		t.Skip("nanoclaw binary not found")
	}
	if !strings.Contains(path, "nanoclaw") {
		t.Errorf("findNanoClawBinary(): got %q, want to contain 'nanoclaw'", path)
	}
}

func TestDiscoverModels(t *testing.T) {
	path := findNanoClawBinary()
	if path == "" {
		t.Skip("nanoclaw binary not found")
	}

	result, err := DiscoverModels()
	if err != nil {
		t.Fatalf("DiscoverModels(): %v", err)
	}
	if len(result.Providers) == 0 {
		t.Fatal("DiscoverModels(): no providers returned")
	}
	var opencodeProvider *DiscoveredProvider
	for _, p := range result.Providers {
		if p.ID == "opencode" {
			opencodeProvider = &p
			break
		}
	}
	if opencodeProvider == nil {
		t.Fatal("DiscoverModels(): no opencode provider found")
	}
	if len(opencodeProvider.Models) < 3 {
		t.Errorf("opencode provider has only %d models, want at least 3", len(opencodeProvider.Models))
	}

	var opencodeGo *DiscoveredProvider
	for _, p := range result.Providers {
		if p.ID == "opencode-go" {
			opencodeGo = &p
			break
		}
	}
	if opencodeGo == nil {
		t.Error("DiscoverModels(): opencode-go not in runtime-discovered providers")
	}

	hasCredentials := false
	for _, m := range opencodeProvider.Models {
		if strings.Contains(m.Key, ":") && strings.Count(m.Key, "/") != 1 {
			hasCredentials = true
		}
		if m.Name == "" {
			t.Errorf("opencode model %q has empty name", m.Key)
		}
	}
	if hasCredentials {
		t.Error("opencode model keys should not contain embedded credentials")
	}
}

func TestMergedProviders(t *testing.T) {
	merged := MergedProviders()
	if len(merged) == 0 {
		t.Fatal("MergedProviders(): no providers")
	}

	ids := make(map[string]bool)
	for _, p := range merged {
		if ids[p.ID] {
			t.Errorf("duplicate provider ID: %s", p.ID)
		}
		ids[p.ID] = true
	}

	var opencodeIdx, opencodeGoIdx int = -1, -1
	for i, p := range merged {
		if p.ID == "opencode" {
			opencodeIdx = i
		}
		if p.ID == "opencode-go" {
			opencodeGoIdx = i
		}
	}
	if opencodeIdx == -1 {
		t.Error("opencode not in merged providers")
	}
	if opencodeGoIdx == -1 {
		t.Error("opencode-go not in merged providers")
	}
	if opencodeGoIdx < opencodeIdx {
		t.Error("opencode-go should come after opencode in merged list")
	}
}

func TestProviderDisplayName(t *testing.T) {
	tests := map[string]string{
		"opencode":          "OpenCode (Zen)",
		"opencode-go":        "OpenCode (Go)",
		"anthropic":          "Anthropic (Claude)",
		"openai":             "OpenAI (GPT)",
		"google":             "Google (Gemini)",
		"ollama":             "Ollama (Local)",
		"vllm":               "vLLM (Local)",
		"unknown-provider":   "unknown-provider",
	}
	for id, want := range tests {
		got := providerDisplayName(id)
		if got != want {
			t.Errorf("providerDisplayName(%q): got %q, want %q", id, got, want)
		}
	}
}

func TestDiscoveryCache(t *testing.T) {
	path := findNanoClawBinary()
	if path == "" {
		t.Skip("nanoclaw binary not found")
	}

	first, err := DiscoverModels()
	if err != nil {
		t.Fatalf("DiscoverModels() first: %v", err)
	}
	second, err := DiscoverModels()
	if err != nil {
		t.Fatalf("DiscoverModels() second: %v", err)
	}
	if first.DiscoveredAt.Unix() != second.DiscoveredAt.Unix() {
		t.Error("cached result should return same DiscoveredAt timestamp")
	}
}

func TestDiscoverProviderModels(t *testing.T) {
	path := findNanoClawBinary()
	if path == "" {
		t.Skip("nanoclaw binary not found")
	}

	models, err := DiscoverProviderModels("opencode")
	if err != nil {
		t.Fatalf("DiscoverProviderModels(opencode): %v", err)
	}
	if len(models) < 3 {
		t.Errorf("opencode has only %d models, want at least 3", len(models))
	}

	_, err = DiscoverProviderModels("nonexistent-provider-xyz")
	if err == nil {
		t.Error("DiscoverProviderModels(unknown): expected error")
	}
}

func TestOpenclawBinaryDetection(t *testing.T) {
	paths := []string{
		"./node_modules/.bin/nanoclaw",
		"node_modules/.bin/nanoclaw",
		"nanoclaw",
	}
	for _, p := range paths {
		cmd := exec.Command(p, "version")
		if cmd.Run() != nil {
			continue
		}
		found := findNanoClawBinary()
		if found == "" {
			t.Errorf("findNanoClawBinary() returned empty despite %s being runnable", p)
		}
		return
	}
	t.Skip("no nanoclaw binary available")
}