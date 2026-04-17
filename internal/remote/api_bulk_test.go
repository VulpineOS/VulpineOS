package remote

import (
	"encoding/json"
	"testing"

	"vulpineos/internal/openclaw"
	"vulpineos/internal/orchestrator"
	"vulpineos/internal/vault"
)

func newBulkAgentAPI(t *testing.T) *PanelAPI {
	t.Helper()
	db, err := vault.OpenPath(t.TempDir() + "/vault.db")
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return &PanelAPI{
		Vault: db,
		Orchestrator: &orchestrator.Orchestrator{
			Vault:  db,
			Agents: openclaw.NewManager("/definitely-not-installed-openclaw"),
		},
	}
}

func TestAgentsPauseManyReportsFailures(t *testing.T) {
	api := newBulkAgentAPI(t)

	payload, err := api.HandleMessage("agents.pauseMany", json.RawMessage(`{"agentIds":["missing-a","missing-b"]}`))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result struct {
		Status   string            `json:"status"`
		Paused   int               `json:"paused"`
		Failures map[string]string `json:"failures"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal result: %v", err)
	}
	if result.Status != "ok" || result.Paused != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Failures) != 2 {
		t.Fatalf("failures = %#v", result.Failures)
	}
}

func TestAgentsResumeManyReportsPerAgentFailure(t *testing.T) {
	api := newBulkAgentAPI(t)

	agent, err := api.Vault.CreateAgent("Resume Me", "resume task", "{}")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if err := api.Vault.UpdateAgentStatus(agent.ID, "paused"); err != nil {
		t.Fatalf("UpdateAgentStatus: %v", err)
	}

	params, err := json.Marshal(map[string]interface{}{"agentIds": []string{agent.ID, "missing-agent"}})
	if err != nil {
		t.Fatalf("Marshal params: %v", err)
	}

	payload, err := api.HandleMessage("agents.resumeMany", params)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result struct {
		Status   string            `json:"status"`
		Resumed  int               `json:"resumed"`
		Failures map[string]string `json:"failures"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal result: %v", err)
	}
	if result.Status != "ok" || result.Resumed != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Failures) != 2 {
		t.Fatalf("failures = %#v", result.Failures)
	}
	if result.Failures[agent.ID] == "" {
		t.Fatalf("expected resume failure for persisted agent: %#v", result.Failures)
	}
}

func TestAgentsKillManyReportsFailures(t *testing.T) {
	api := newBulkAgentAPI(t)

	payload, err := api.HandleMessage("agents.killMany", json.RawMessage(`{"agentIds":["missing-kill"]}`))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var result struct {
		Status   string            `json:"status"`
		Killed   int               `json:"killed"`
		Failures map[string]string `json:"failures"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal result: %v", err)
	}
	if result.Status != "ok" || result.Killed != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Failures["missing-kill"] == "" {
		t.Fatalf("expected failure for missing agent: %#v", result.Failures)
	}
}
