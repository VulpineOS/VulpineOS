package openclaw

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareScopedConfig(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "openclaw.json")
	base := []byte(`{
		"env":{"ZAI_API_KEY":"test"},
		"browser":{"enabled":true,"headless":false,"cdpUrl":"ws://127.0.0.1:9222"}
	}`)
	if err := os.WriteFile(basePath, base, 0600); err != nil {
		t.Fatalf("write base config: %v", err)
	}

	path, cleanup, err := PrepareScopedConfig(basePath, "ws://127.0.0.1:45555")
	if err != nil {
		t.Fatalf("PrepareScopedConfig returned error: %v", err)
	}
	defer cleanup()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scoped config: %v", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse scoped config: %v", err)
	}

	browser := cfg["browser"].(map[string]interface{})
	if browser["cdpUrl"] != "ws://127.0.0.1:45555" {
		t.Fatalf("browser.cdpUrl = %v, want ws://127.0.0.1:45555", browser["cdpUrl"])
	}
	if browser["headless"] != true {
		t.Fatalf("browser.headless = %v, want true", browser["headless"])
	}
}
