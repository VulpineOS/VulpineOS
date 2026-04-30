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

func TestAgentsGetMessagesCapsPanelLimit(t *testing.T) {
	api, db := newPanelAPITestFixture(t)
	agent, err := db.CreateAgent("Message Cap", "task", "{}")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	for i := 0; i < maxPanelAgentMessages+5; i++ {
		if err := db.AppendMessage(agent.ID, "assistant", "message", i); err != nil {
			t.Fatalf("AppendMessage %d: %v", i, err)
		}
	}

	payload, err := api.HandleMessage("agents.getMessages", json.RawMessage(`{"agentId":"`+agent.ID+`","limit":100000}`))
	if err != nil {
		t.Fatalf("HandleMessage agents.getMessages: %v", err)
	}
	var result struct {
		Messages  []vault.AgentMessage `json:"messages"`
		Limit     int                  `json:"limit"`
		Truncated bool                 `json:"truncated"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("Unmarshal messages: %v", err)
	}
	if len(result.Messages) != maxPanelAgentMessages || result.Limit != maxPanelAgentMessages || !result.Truncated {
		t.Fatalf("unexpected cap result: len=%d limit=%d truncated=%v", len(result.Messages), result.Limit, result.Truncated)
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

func TestProxyPanelRedactsCredentialsAndUsesIDsForRotation(t *testing.T) {
	api, db := newPanelAPITestFixture(t)
	agent, err := db.CreateAgent("Agent Two", "task", "{}")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	addPayload, err := api.HandleMessage("proxies.add", json.RawMessage(`{"url":"http://user:pass@example.com:8080"}`))
	if err != nil {
		t.Fatalf("HandleMessage proxies.add: %v", err)
	}
	var added struct {
		ID     string `json:"id"`
		URL    string `json:"url"`
		Config string `json:"config"`
		Label  string `json:"label"`
	}
	if err := json.Unmarshal(addPayload, &added); err != nil {
		t.Fatalf("Unmarshal added proxy: %v", err)
	}
	if added.ID == "" {
		t.Fatal("expected added proxy id")
	}
	if added.Config != "" {
		t.Fatalf("proxies.add leaked raw config: %q", added.Config)
	}
	if strings.Contains(added.URL, "user") || strings.Contains(added.URL, "pass") {
		t.Fatalf("proxies.add leaked credentials in URL: %q", added.URL)
	}
	if strings.Contains(added.Label, "user") || strings.Contains(added.Label, "pass") {
		t.Fatalf("proxies.add leaked credentials in label: %q", added.Label)
	}

	listPayload, err := api.HandleMessage("proxies.list", nil)
	if err != nil {
		t.Fatalf("HandleMessage proxies.list: %v", err)
	}
	var listed struct {
		Proxies []struct {
			ID    string `json:"id"`
			URL   string `json:"url"`
			Label string `json:"label"`
		} `json:"proxies"`
	}
	if err := json.Unmarshal(listPayload, &listed); err != nil {
		t.Fatalf("Unmarshal listed proxies: %v", err)
	}
	if len(listed.Proxies) != 1 {
		t.Fatalf("listed proxies = %d, want 1", len(listed.Proxies))
	}
	if strings.Contains(listed.Proxies[0].URL, "user") || strings.Contains(listed.Proxies[0].URL, "pass") {
		t.Fatalf("proxy list leaked credentials: %q", listed.Proxies[0].URL)
	}
	if strings.Contains(listed.Proxies[0].Label, "user") || strings.Contains(listed.Proxies[0].Label, "pass") {
		t.Fatalf("proxy label leaked credentials: %q", listed.Proxies[0].Label)
	}

	setPayload := json.RawMessage(`{"agentId":"` + agent.ID + `","config":{"enabled":true,"proxyPool":["` + added.ID + `"]}}`)
	if _, err := api.HandleMessage("proxies.setRotation", setPayload); err != nil {
		t.Fatalf("HandleMessage proxies.setRotation: %v", err)
	}
	cfg := api.Rotator.GetConfig(agent.ID)
	if cfg == nil || len(cfg.ProxyPool) != 1 || cfg.ProxyPool[0] != "http://user:pass@example.com:8080" {
		t.Fatalf("rotator proxy pool = %#v, want resolved proxy URL", cfg)
	}

	getPayload, err := api.HandleMessage("proxies.getRotation", json.RawMessage(`{"agentId":"`+agent.ID+`"}`))
	if err != nil {
		t.Fatalf("HandleMessage proxies.getRotation: %v", err)
	}
	var got struct {
		Config rotationConfigPayload `json:"config"`
	}
	if err := json.Unmarshal(getPayload, &got); err != nil {
		t.Fatalf("Unmarshal rotation: %v", err)
	}
	if len(got.Config.ProxyPool) != 1 || got.Config.ProxyPool[0] != added.ID {
		t.Fatalf("panel proxy pool = %#v, want proxy id", got.Config.ProxyPool)
	}
}

func TestProxyPanelErrorsDoNotLeakCredentials(t *testing.T) {
	api, db := newPanelAPITestFixture(t)
	agent, err := db.CreateAgent("Agent Two", "task", "{}")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	_, err = api.HandleMessage("proxies.add", json.RawMessage(`{"url":"http://user:add-secret@%zz"}`))
	if err == nil {
		t.Fatal("expected invalid proxy add error")
	}
	if strings.Contains(err.Error(), "add-secret") || strings.Contains(err.Error(), "user:") {
		t.Fatalf("proxy add error leaked credentials: %v", err)
	}

	rotationPayload := json.RawMessage(`{"agentId":"` + agent.ID + `","config":{"enabled":true,"proxyPool":["http://user:pool-secret@%zz"]}}`)
	_, err = api.HandleMessage("proxies.setRotation", rotationPayload)
	if err == nil {
		t.Fatal("expected invalid rotation proxy error")
	}
	if strings.Contains(err.Error(), "pool-secret") || strings.Contains(err.Error(), "user:") {
		t.Fatalf("proxy rotation error leaked credentials: %v", err)
	}
}

func TestProxiesDeleteRejectsBlankID(t *testing.T) {
	api, db := newPanelAPITestFixture(t)
	if _, err := db.AddProxy(`{"host":"example.com","port":8080}`, "", "example"); err != nil {
		t.Fatalf("AddProxy: %v", err)
	}

	if _, err := api.HandleMessage("proxies.delete", json.RawMessage(`{"proxyId":"   "}`)); err == nil {
		t.Fatal("expected blank proxy id error")
	}
	proxies, err := db.ListProxies()
	if err != nil {
		t.Fatalf("ListProxies: %v", err)
	}
	if len(proxies) != 1 {
		t.Fatalf("blank delete removed proxies; len = %d", len(proxies))
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

func TestBusPolicyControlsNormalizeAndRejectUnsafeIDs(t *testing.T) {
	api, _ := newPanelAPITestFixture(t)

	addParams, err := json.Marshal(map[string]interface{}{
		"fromAgent":   " alpha ",
		"toAgent":     " * ",
		"autoApprove": true,
	})
	if err != nil {
		t.Fatalf("Marshal add policy: %v", err)
	}
	if _, err := api.HandleMessage("bus.addPolicy", addParams); err != nil {
		t.Fatalf("HandleMessage bus.addPolicy: %v", err)
	}
	policies := api.AgentBus.Policies()
	if len(policies) != 1 || policies[0].FromAgent != "alpha" || policies[0].ToAgent != "*" {
		t.Fatalf("policies = %#v, want normalized alpha -> *", policies)
	}

	for _, tc := range []struct {
		name string
		from string
		to   string
	}{
		{name: "blank from", from: " ", to: "*"},
		{name: "path from", from: "../escape", to: "*"},
		{name: "space from", from: "agent one", to: "*"},
		{name: "long to", from: "*", to: strings.Repeat("a", maxPanelBusEndpointBytes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params, err := json.Marshal(map[string]interface{}{
				"fromAgent":   tc.from,
				"toAgent":     tc.to,
				"autoApprove": true,
			})
			if err != nil {
				t.Fatalf("Marshal policy: %v", err)
			}
			if _, err := api.HandleMessage("bus.addPolicy", params); err == nil {
				t.Fatal("expected invalid policy error")
			}
		})
	}
}

func TestBusApproveRejectValidatesMessageIDWithoutEchoingInput(t *testing.T) {
	api, _ := newPanelAPITestFixture(t)
	if _, err := api.AgentBus.Send(agentbus.Notify, "agent-a", "agent-b", "hello", ""); err != nil {
		t.Fatalf("AgentBus.Send: %v", err)
	}

	if _, err := api.HandleMessage("bus.approve", json.RawMessage(`{"messageId":"   "}`)); err == nil {
		t.Fatal("expected blank message id error")
	}

	params, err := json.Marshal(map[string]string{"messageId": "secret-token-value"})
	if err != nil {
		t.Fatalf("Marshal missing message id: %v", err)
	}
	_, err = api.HandleMessage("bus.reject", params)
	if err == nil {
		t.Fatal("expected missing message id error")
	}
	if strings.Contains(err.Error(), "secret-token-value") {
		t.Fatalf("message id echoed in error: %v", err)
	}

	pending := api.AgentBus.PendingMessages()
	if len(pending) != 1 {
		t.Fatalf("pending messages = %d, want 1", len(pending))
	}
	params, err = json.Marshal(map[string]string{"messageId": " " + pending[0].ID + " "})
	if err != nil {
		t.Fatalf("Marshal approve message id: %v", err)
	}
	if _, err := api.HandleMessage("bus.approve", params); err != nil {
		t.Fatalf("HandleMessage bus.approve: %v", err)
	}
	if pending := api.AgentBus.PendingMessages(); len(pending) != 0 {
		t.Fatalf("pending messages = %d, want 0", len(pending))
	}
}

func TestBusPendingRedactsAndLimitsContent(t *testing.T) {
	api, _ := newPanelAPITestFixture(t)
	content := "token=secret-value " + strings.Repeat("a", maxPanelBusContentBytes+128)
	if _, err := api.AgentBus.Send(agentbus.Notify, "agent-a", "agent-b", content, ""); err != nil {
		t.Fatalf("AgentBus.Send: %v", err)
	}

	payload, err := api.HandleMessage("bus.pending", nil)
	if err != nil {
		t.Fatalf("HandleMessage bus.pending: %v", err)
	}
	var pending []agentbus.Message
	if err := json.Unmarshal(payload, &pending); err != nil {
		t.Fatalf("Unmarshal pending messages: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending messages = %d, want 1", len(pending))
	}
	if strings.Contains(pending[0].Content, "secret-value") {
		t.Fatalf("panel pending content leaked secret: %q", pending[0].Content)
	}
	if !strings.Contains(pending[0].Content, "[truncated]") {
		t.Fatalf("panel pending content was not marked truncated")
	}
	if len(pending[0].Content) > maxPanelBusContentBytes+len("\n[truncated]") {
		t.Fatalf("panel pending content length = %d, want <= cap", len(pending[0].Content))
	}
	if raw := api.AgentBus.PendingMessages()[0].Content; !strings.Contains(raw, "secret-value") {
		t.Fatalf("raw bus message was unexpectedly altered: %q", raw)
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

func TestRecordingControlsRejectUnsafeAgentID(t *testing.T) {
	api, _ := newPanelAPITestFixture(t)

	for _, method := range []string{"recording.getTimeline", "recording.export"} {
		t.Run(method, func(t *testing.T) {
			_, err := api.HandleMessage(method, json.RawMessage(`{"agentId":"../escape"}`))
			if err == nil || !strings.Contains(err.Error(), "invalid agentId") {
				t.Fatalf("error = %v, want invalid agentId", err)
			}
		})
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
