package remote

import (
	"encoding/json"
	"strings"
	"testing"

	"vulpineos/internal/runtimeaudit"
	"vulpineos/internal/vault"
)

func newRuntimeAuditAPI(t *testing.T) *PanelAPI {
	t.Helper()
	db, err := vault.OpenPath(t.TempDir() + "/vault.db")
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	manager := runtimeaudit.New(db)
	if _, err := manager.Log("kernel", "info", "started", "kernel booted", map[string]string{"pid": "1"}); err != nil {
		t.Fatalf("Log 1: %v", err)
	}
	if _, err := manager.Log("openclaw", "warn", "restart_failed", "agent restart failed", map[string]string{"agent": "a-1"}); err != nil {
		t.Fatalf("Log 2: %v", err)
	}
	return &PanelAPI{RuntimeAudit: manager}
}

func TestRuntimeExportJSON(t *testing.T) {
	api := newRuntimeAuditAPI(t)

	payload, err := api.HandleMessage("runtime.export", json.RawMessage(`{"level":"warn","format":"json"}`))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result struct {
		Content     string `json:"content"`
		ContentType string `json:"contentType"`
		FileName    string `json:"fileName"`
		Format      string `json:"format"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal result: %v", err)
	}
	if result.Format != "json" || result.ContentType != "application/json" {
		t.Fatalf("unexpected export metadata: %#v", result)
	}
	if !strings.Contains(result.FileName, "runtime-audit-") {
		t.Fatalf("fileName = %q", result.FileName)
	}
	if !strings.Contains(result.Content, "\"restart_failed\"") {
		t.Fatalf("content = %s", result.Content)
	}
	if strings.Contains(result.Content, "\"kernel\"") {
		t.Fatalf("expected warn filter to exclude kernel event: %s", result.Content)
	}
}

func TestRuntimeExportNDJSON(t *testing.T) {
	api := newRuntimeAuditAPI(t)

	payload, err := api.HandleMessage("runtime.export", json.RawMessage(`{"format":"ndjson"}`))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result struct {
		Content     string `json:"content"`
		ContentType string `json:"contentType"`
		FileName    string `json:"fileName"`
		Format      string `json:"format"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal result: %v", err)
	}
	if result.Format != "ndjson" || result.ContentType != "application/x-ndjson" {
		t.Fatalf("unexpected export metadata: %#v", result)
	}
	lines := strings.Split(strings.TrimSpace(result.Content), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %#v", lines)
	}
	if !strings.Contains(lines[0], "\"runtime_audit_export\"") {
		t.Fatalf("header = %s", lines[0])
	}
}
