package remote

import (
	"encoding/json"
	"strings"
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

	payload, err := api.HandleMessage("agents.killMany", json.RawMessage(`{"agentIds":[" missing-kill "]}`))
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

func TestAgentControlsRejectUnsafeAgentID(t *testing.T) {
	api := newBulkAgentAPI(t)

	for _, method := range []string{"agents.kill", "agents.pause", "agents.resume"} {
		t.Run(method, func(t *testing.T) {
			_, err := api.HandleMessage(method, json.RawMessage(`{"agentId":"../escape"}`))
			if err == nil || !strings.Contains(err.Error(), "invalid agentId") {
				t.Fatalf("error = %v, want invalid agentId", err)
			}
		})
	}
}

func TestAgentsBulkControlsRejectUnsafeAgentIDs(t *testing.T) {
	api := newBulkAgentAPI(t)

	for _, method := range []string{"agents.pauseMany", "agents.resumeMany", "agents.killMany"} {
		t.Run(method+" path", func(t *testing.T) {
			_, err := api.HandleMessage(method, json.RawMessage(`{"agentIds":["valid-agent","../escape"]}`))
			if err == nil || !strings.Contains(err.Error(), "invalid agentId") {
				t.Fatalf("error = %v, want invalid agentId", err)
			}
		})

		t.Run(method+" blank", func(t *testing.T) {
			_, err := api.HandleMessage(method, json.RawMessage(`{"agentIds":["valid-agent"," "]}`))
			if err == nil || !strings.Contains(err.Error(), "agentId is required") {
				t.Fatalf("error = %v, want required agentId", err)
			}
		})

		t.Run(method+" limit", func(t *testing.T) {
			ids := make([]string, maxPanelBulkAgentIDs+1)
			for i := range ids {
				ids[i] = "agent-" + strings.Repeat("a", 8) + "-" + string(rune('a'+(i%26)))
			}
			params, err := json.Marshal(map[string]interface{}{"agentIds": ids})
			if err != nil {
				t.Fatalf("Marshal params: %v", err)
			}
			_, err = api.HandleMessage(method, params)
			if err == nil || !strings.Contains(err.Error(), "agentIds exceeds") {
				t.Fatalf("error = %v, want bulk limit error", err)
			}
		})
	}
}
