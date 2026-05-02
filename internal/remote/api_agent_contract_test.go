package remote

import (
	"encoding/json"
	"testing"

	"vulpineos/internal/vault"
)

func TestPanelAPIAgentsGetMessagesContract(t *testing.T) {
	api, db := newPanelAPITestFixture(t)

	agent, err := db.CreateAgent("TestBot", "task", "{}")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	if err := db.AppendMessage(agent.ID, "user", "hello", 5); err != nil {
		t.Fatalf("AppendMessage user: %v", err)
	}
	if err := db.AppendMessage(agent.ID, "assistant", "world", 5); err != nil {
		t.Fatalf("AppendMessage assistant: %v", err)
	}

	params := json.RawMessage(`{"agentId":"` + agent.ID + `","limit":1}`)
	payload, err := api.HandleMessage("agents.getMessages", params)
	if err != nil {
		t.Fatalf("HandleMessage agents.getMessages: %v", err)
	}

	var result struct {
		Messages  []vault.AgentMessage `json:"messages"`
		Limit    int               `json:"limit"`
		Truncated bool             `json:"truncated"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal messages: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Role != "assistant" {
		t.Fatalf("Messages[0].Role = %q, want assistant", result.Messages[0].Role)
	}
	if result.Messages[0].Content != "world" {
		t.Fatalf("Messages[0].Content = %q, want world", result.Messages[0].Content)
	}
	if result.Limit != 1 {
		t.Fatalf("Limit = %d, want 1", result.Limit)
	}
	if !result.Truncated {
		t.Fatal("Truncated = false, want true")
	}
}

func TestPanelAPIAgentsGetMessagesRejectsInvalidAgentID(t *testing.T) {
	api, _ := newPanelAPITestFixture(t)

	params := json.RawMessage(`{"agentId":"../bad"}`)
	_, err := api.HandleMessage("agents.getMessages", params)
	if err == nil {
		t.Fatal("expected error for invalid agentId, got nil")
	}
	if err.Error() != "invalid agentId" {
		t.Fatalf("error = %q, want %q", err.Error(), "invalid agentId")
	}
}