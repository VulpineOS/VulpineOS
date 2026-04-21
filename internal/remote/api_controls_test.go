package remote

import (
	"encoding/json"
	"strings"
	"testing"

	"vulpineos/internal/agentbus"
	"vulpineos/internal/config"
	"vulpineos/internal/costtrack"
	"vulpineos/internal/proxy"
	"vulpineos/internal/recording"
	"vulpineos/internal/vault"
)

func newPanelAPITestFixture(t *testing.T) (*PanelAPI, *vault.DB) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	db, err := vault.Open()
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	api := &PanelAPI{
		Config:   &config.Config{},
		Vault:    db,
		Costs:    costtrack.New("openai/gpt-5.4"),
		Rotator:  proxy.NewRotator(),
		AgentBus: agentbus.New(),
		Recorder: recording.NewRecorder(),
	}
	return api, db
}

func TestCostsSetBudgetPersistsOverrideAndCanRevertToDefault(t *testing.T) {
	api, db := newPanelAPITestFixture(t)
	api.Config.DefaultBudgetMaxCostUSD = 1.5
	api.Config.DefaultBudgetMaxTokens = 5000

	agent, err := db.CreateAgent("Agent One", "task", "{}")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if err := api.SyncPersistentState(); err != nil {
		t.Fatalf("SyncPersistentState: %v", err)
	}
	if budget := api.Costs.GetBudget(agent.ID); budget == nil || budget.MaxCostUSD != 1.5 || budget.MaxTokens != 5000 {
		t.Fatalf("default budget = %#v, want 1.5 / 5000", budget)
	}

	payload, err := api.HandleMessage("costs.setBudget", json.RawMessage(`{"agentId":"`+agent.ID+`","maxCostUsd":2.75,"maxTokens":9000}`))
	if err != nil {
		t.Fatalf("HandleMessage costs.setBudget: %v", err)
	}
	var result struct {
		MaxCostUSD   float64 `json:"maxCostUsd"`
		MaxTokens    int64   `json:"maxTokens"`
		BudgetSource string  `json:"budgetSource"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal budget result: %v", err)
	}
	if result.MaxCostUSD != 2.75 || result.MaxTokens != 9000 || result.BudgetSource != "agent" {
		t.Fatalf("unexpected budget result: %#v", result)
	}

	stored, err := db.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	meta, err := vault.ParseAgentMetadata(stored.Metadata)
	if err != nil {
		t.Fatalf("ParseAgentMetadata: %v", err)
	}
	if meta.Budget == nil || !meta.Budget.Override || meta.Budget.MaxCostUSD != 2.75 || meta.Budget.MaxTokens != 9000 {
		t.Fatalf("unexpected stored budget metadata: %#v", meta.Budget)
	}

	if _, err := api.HandleMessage("costs.setBudget", json.RawMessage(`{"agentId":"`+agent.ID+`","inheritDefault":true}`)); err != nil {
		t.Fatalf("HandleMessage costs.setBudget inheritDefault: %v", err)
	}
	stored, err = db.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent after inheritDefault: %v", err)
	}
	meta, err = vault.ParseAgentMetadata(stored.Metadata)
	if err != nil {
		t.Fatalf("ParseAgentMetadata after inheritDefault: %v", err)
	}
	if meta.Budget != nil {
		t.Fatalf("budget metadata = %#v, want nil", meta.Budget)
	}
	if budget := api.Costs.GetBudget(agent.ID); budget == nil || budget.MaxCostUSD != 1.5 || budget.MaxTokens != 5000 {
		t.Fatalf("restored default budget = %#v, want 1.5 / 5000", budget)
	}
}

func TestProxyRotationPersistsAcrossPanelAPIInstances(t *testing.T) {
	api, db := newPanelAPITestFixture(t)

	agent, err := db.CreateAgent("Agent Two", "task", "{}")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	params := json.RawMessage(`{"agentId":"` + agent.ID + `","config":{"enabled":true,"rotateOnRateLimit":true,"rotateOnBlock":true,"rotateIntervalSeconds":600,"syncFingerprint":true,"proxyPool":["http://a:80","http://b:80"],"currentIndex":1}}`)
	if _, err := api.HandleMessage("proxies.setRotation", params); err != nil {
		t.Fatalf("HandleMessage proxies.setRotation: %v", err)
	}

	api2 := &PanelAPI{
		Config:  &config.Config{},
		Vault:   db,
		Rotator: proxy.NewRotator(),
	}
	if err := api2.SyncPersistentState(); err != nil {
		t.Fatalf("api2.SyncPersistentState: %v", err)
	}

	payload, err := api2.HandleMessage("proxies.getRotation", json.RawMessage(`{"agentId":"`+agent.ID+`"}`))
	if err != nil {
		t.Fatalf("HandleMessage proxies.getRotation: %v", err)
	}
	var result struct {
		Config rotationConfigPayload `json:"config"`
		Source string                `json:"source"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal rotation result: %v", err)
	}
	if result.Source != "agent" {
		t.Fatalf("source = %q, want agent", result.Source)
	}
	if !result.Config.Enabled || !result.Config.RotateOnRateLimit || !result.Config.RotateOnBlock || result.Config.RotateIntervalSeconds != 600 {
		t.Fatalf("unexpected rotation config: %#v", result.Config)
	}
	if len(result.Config.ProxyPool) != 2 || result.Config.CurrentIndex != 1 {
		t.Fatalf("unexpected proxy pool state: %#v", result.Config)
	}
	if cfg := api2.Rotator.GetConfig(agent.ID); cfg == nil || cfg.RotateInterval.Seconds() != 600 || cfg.CurrentIndex != 1 {
		t.Fatalf("rotator config = %#v, want restored config", cfg)
	}
}

func TestBusRemovePolicyRemovesConfiguredRule(t *testing.T) {
	api, _ := newPanelAPITestFixture(t)
	api.AgentBus.AddPolicy("alpha", "beta", true)

	if _, err := api.HandleMessage("bus.removePolicy", json.RawMessage(`{"fromAgent":"alpha","toAgent":"beta"}`)); err != nil {
		t.Fatalf("HandleMessage bus.removePolicy: %v", err)
	}
	if policies := api.AgentBus.Policies(); len(policies) != 0 {
		t.Fatalf("policies = %#v, want empty", policies)
	}
}

func TestRecordingExportWrapsDownloadPayload(t *testing.T) {
	api, _ := newPanelAPITestFixture(t)
	api.Recorder.Record("agent-1", recording.ActionNavigate, json.RawMessage(`{"url":"https://example.com"}`))

	payload, err := api.HandleMessage("recording.export", json.RawMessage(`{"agentId":"agent-1"}`))
	if err != nil {
		t.Fatalf("HandleMessage recording.export: %v", err)
	}
	var result struct {
		Content     string `json:"content"`
		ContentType string `json:"contentType"`
		FileName    string `json:"fileName"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal recording export: %v", err)
	}
	if result.ContentType != "application/json" || result.FileName != "agent-agent-1-recording.json" {
		t.Fatalf("unexpected recording export metadata: %#v", result)
	}
	if !strings.Contains(result.Content, `"type":"navigate"`) {
		t.Fatalf("recording export content = %q, want navigate action", result.Content)
	}
}

func TestFingerprintsGeneratePersistsAgentFingerprint(t *testing.T) {
	api, db := newPanelAPITestFixture(t)

	agent, err := db.CreateAgent("Agent Three", "task", "{}")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	payload, err := api.HandleMessage("fingerprints.generate", json.RawMessage(`{"agentId":"`+agent.ID+`","seed":"alpha"}`))
	if err != nil {
		t.Fatalf("HandleMessage fingerprints.generate: %v", err)
	}
	var result struct {
		Fingerprint string `json:"fingerprint"`
		Applied     bool   `json:"applied"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal fingerprint result: %v", err)
	}
	if !result.Applied || result.Fingerprint == "" || result.Fingerprint == "{}" {
		t.Fatalf("unexpected fingerprint result: %#v", result)
	}
	stored, err := db.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if stored.Fingerprint != result.Fingerprint {
		t.Fatalf("stored fingerprint mismatch")
	}
}
