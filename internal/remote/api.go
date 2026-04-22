package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vulpineos/internal/agentbus"
	"vulpineos/internal/config"
	"vulpineos/internal/costtrack"
	"vulpineos/internal/extensions"
	"vulpineos/internal/juggler"
	"vulpineos/internal/kernel"
	"vulpineos/internal/openclaw"
	"vulpineos/internal/orchestrator"
	"vulpineos/internal/proxy"
	"vulpineos/internal/recording"
	"vulpineos/internal/runtimeaudit"
	"vulpineos/internal/vault"
	"vulpineos/internal/webhooks"
)

// PanelAPI handles control messages from the web panel, dispatching to subsystems.
type gatewayStatus interface {
	Running() bool
}

type PanelAPI struct {
	Orchestrator *orchestrator.Orchestrator
	Config       *config.Config
	Vault        *vault.DB
	AgentBus     *agentbus.Bus
	Costs        *costtrack.Tracker
	Webhooks     *webhooks.Manager
	Recorder     *recording.Recorder
	Rotator      *proxy.Rotator
	Kernel       *kernel.Kernel
	Gateway      gatewayStatus
	Client       *juggler.Client
	Contexts     *ContextRegistry
	RuntimeAudit *runtimeaudit.Manager
}

// HandleMessage dispatches a control message to the appropriate handler.
// Returns the JSON result or an error.
func (api *PanelAPI) HandleMessage(method string, params json.RawMessage) (json.RawMessage, error) {
	switch method {
	// --- Agent management ---
	case "agents.list":
		return api.agentsList()
	case "agents.spawn":
		return api.agentsSpawn(params)
	case "agents.kill":
		return api.agentsKill(params)
	case "agents.pause":
		return api.agentsPause(params)
	case "agents.pauseMany":
		return api.agentsPauseMany(params)
	case "agents.resume":
		return api.agentsResume(params)
	case "agents.resumeMany":
		return api.agentsResumeMany(params)
	case "agents.killMany":
		return api.agentsKillMany(params)
	case "agents.getMessages":
		return api.agentsGetMessages(params)
	case "agents.getSessionLog":
		return api.agentsGetSessionLog(params)

	// --- Config ---
	case "config.get":
		return api.configGet()
	case "config.providers":
		return api.configProviders()
	case "config.set":
		return api.configSet(params)

	// --- Cost tracking ---
	case "costs.getAll":
		return api.costsGetAll()
	case "costs.setBudget":
		return api.costsSetBudget(params)
	case "costs.total":
		return api.costsTotal()

	// --- Webhooks ---
	case "webhooks.list":
		return api.webhooksList()
	case "webhooks.add":
		return api.webhooksAdd(params)
	case "webhooks.remove":
		return api.webhooksRemove(params)

	// --- Proxies ---
	case "proxies.list":
		return api.proxiesList()
	case "proxies.add":
		return api.proxiesAdd(params)
	case "proxies.delete":
		return api.proxiesDelete(params)
	case "proxies.test":
		return api.proxiesTest(params)
	case "proxies.getRotation":
		return api.proxiesGetRotation(params)
	case "proxies.setRotation":
		return api.proxiesSetRotation(params)

	// --- Agent Bus ---
	case "bus.pending":
		return api.busPending()
	case "bus.approve":
		return api.busApprove(params)
	case "bus.reject":
		return api.busReject(params)
	case "bus.policies":
		return api.busPolicies()
	case "bus.addPolicy":
		return api.busAddPolicy(params)
	case "bus.removePolicy":
		return api.busRemovePolicy(params)

	// --- Recording ---
	case "recording.getTimeline":
		return api.recordingGetTimeline(params)
	case "recording.export":
		return api.recordingExport(params)

	// --- Fingerprints ---
	case "fingerprints.get":
		return api.fingerprintsGet(params)
	case "fingerprints.generate":
		return api.fingerprintsGenerate(params)

	// --- Scripts ---
	case "scripts.run":
		return api.scriptsRun(params)

	// --- Security ---
	case "security.status":
		return api.securityStatus()

	// --- Status ---
	case "status.get":
		return api.statusGet()
	case "sentinel.get":
		return api.sentinelGet()
	case "sentinel.timeline":
		return api.sentinelTimeline(params)

	// --- Runtime audit ---
	case "runtime.list":
		return api.runtimeList(params)
	case "runtime.export":
		return api.runtimeExport(params)
	case "runtime.setRetention":
		return api.runtimeSetRetention(params)

	// --- Contexts ---
	case "contexts.list":
		return api.contextsList()
	case "contexts.create":
		return api.contextsCreate(params)
	case "contexts.remove":
		return api.contextsRemove(params)

	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// ---------------------------------------------------------------------------
// Agent management
// ---------------------------------------------------------------------------

func (api *PanelAPI) agentsList() (json.RawMessage, error) {
	if api.Vault == nil {
		return nil, fmt.Errorf("vault not available")
	}
	agents, err := api.Vault.ListAgents()
	if err != nil {
		return nil, err
	}
	type agentSummary struct {
		ID                 string  `json:"id"`
		Name               string  `json:"name"`
		Status             string  `json:"status"`
		Task               string  `json:"task"`
		TotalTokens        int     `json:"totalTokens"`
		Fingerprint        string  `json:"fingerprint"`
		FingerprintSummary string  `json:"fingerprintSummary"`
		ContextID          string  `json:"contextId,omitempty"`
		BudgetMaxCostUSD   float64 `json:"budgetMaxCostUsd,omitempty"`
		BudgetMaxTokens    int64   `json:"budgetMaxTokens,omitempty"`
		BudgetSource       string  `json:"budgetSource,omitempty"`
	}
	out := make([]agentSummary, len(agents))
	for i, a := range agents {
		meta, _ := vault.ParseAgentMetadata(a.Metadata)
		budgetCost, budgetTokens, budgetSource := api.effectiveBudget(meta)
		out[i] = agentSummary{
			ID:                 a.ID,
			Name:               a.Name,
			Status:             a.Status,
			Task:               a.Task,
			TotalTokens:        a.TotalTokens,
			Fingerprint:        a.Fingerprint,
			FingerprintSummary: vault.FingerprintSummary(a.Fingerprint),
			ContextID:          meta.ContextID,
			BudgetMaxCostUSD:   budgetCost,
			BudgetMaxTokens:    budgetTokens,
			BudgetSource:       budgetSource,
		}
	}
	return json.Marshal(map[string]interface{}{"agents": out})
}

func (api *PanelAPI) agentsSpawn(params json.RawMessage) (json.RawMessage, error) {
	if api.Orchestrator == nil {
		return nil, fmt.Errorf("orchestrator not available")
	}
	var p struct {
		TemplateID string `json:"templateId"`
		Name       string `json:"name"`
		Task       string `json:"task"`
		ContextID  string `json:"contextId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.TemplateID != "" {
		agentID, err := api.Orchestrator.SpawnNomad(p.TemplateID)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"agentId": agentID})
	}
	if api.Vault == nil {
		return nil, fmt.Errorf("vault not available")
	}
	task := strings.TrimSpace(p.Task)
	if task == "" {
		return nil, fmt.Errorf("task is required")
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = task
		if len(name) > 48 {
			name = name[:48]
		}
	}
	fp, err := vault.GenerateFingerprint(name)
	if err != nil {
		fp = "{}"
	}
	agent, err := api.Vault.CreateAgent(name, task, fp)
	if err != nil {
		return nil, err
	}
	if p.ContextID != "" {
		metadata := vault.MarshalAgentMetadata(vault.AgentMetadata{ContextID: p.ContextID})
		if err := api.Vault.UpdateAgentMetadata(agent.ID, metadata); err == nil {
			agent.Metadata = metadata
		}
	}
	_ = api.syncAgentState(*agent)
	initialPrompt := openclaw.IntroMessage(name, task)
	sessionName := "vulpine-" + agent.ID
	configPath, cleanup, err := api.agentRuntimeConfig(agent)
	if err != nil {
		_ = api.Vault.UpdateAgentStatus(agent.ID, "error")
		_ = api.Vault.AppendMessage(agent.ID, "system", "Failed to prepare runtime: "+err.Error(), 0)
		return nil, err
	}
	_, err = api.Orchestrator.Agents.SpawnWithSessionIsolated(agent.ID, initialPrompt, sessionName, configPath, cleanup)
	if err != nil {
		_ = api.Vault.UpdateAgentStatus(agent.ID, "error")
		_ = api.Vault.AppendMessage(agent.ID, "system", "Failed to start: "+err.Error(), 0)
		return nil, err
	}
	_ = api.Vault.UpdateAgentStatus(agent.ID, "active")
	_ = api.Vault.AppendMessage(agent.ID, "system", "Agent starting...", 0)
	return json.Marshal(map[string]string{"agentId": agent.ID})
}

func (api *PanelAPI) agentsKill(params json.RawMessage) (json.RawMessage, error) {
	if api.Orchestrator == nil {
		return nil, fmt.Errorf("orchestrator not available")
	}
	var p struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if err := api.Orchestrator.KillAgent(p.AgentID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func (api *PanelAPI) agentsPause(params json.RawMessage) (json.RawMessage, error) {
	if api.Orchestrator == nil {
		return nil, fmt.Errorf("orchestrator not available")
	}
	var p struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if err := api.Orchestrator.Agents.PauseAgent(p.AgentID); err != nil {
		return nil, err
	}
	if api.Vault != nil {
		_ = api.Vault.UpdateAgentStatus(p.AgentID, "paused")
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func (api *PanelAPI) agentsPauseMany(params json.RawMessage) (json.RawMessage, error) {
	if api.Orchestrator == nil || api.Vault == nil {
		return nil, fmt.Errorf("orchestrator not available")
	}
	var p struct {
		AgentIDs []string `json:"agentIds"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	paused := 0
	failures := map[string]string{}
	for _, agentID := range p.AgentIDs {
		if strings.TrimSpace(agentID) == "" {
			continue
		}
		if err := api.Orchestrator.Agents.PauseAgent(agentID); err != nil {
			failures[agentID] = err.Error()
			continue
		}
		_ = api.Vault.UpdateAgentStatus(agentID, "paused")
		paused++
	}
	return json.Marshal(map[string]interface{}{
		"status":   "ok",
		"paused":   paused,
		"failures": failures,
	})
}

func (api *PanelAPI) agentsResume(params json.RawMessage) (json.RawMessage, error) {
	if api.Orchestrator == nil {
		return nil, fmt.Errorf("orchestrator not available")
	}
	var p struct {
		AgentID     string `json:"agentId"`
		SessionName string `json:"sessionName"`
		Message     string `json:"message"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.SessionName == "" {
		p.SessionName = "vulpine-" + p.AgentID
	}
	if strings.TrimSpace(p.Message) != "" {
		if api.Vault == nil {
			return nil, fmt.Errorf("vault not available")
		}
		agent, err := api.Vault.GetAgent(p.AgentID)
		if err != nil {
			return nil, err
		}
		_ = api.Vault.AppendMessage(p.AgentID, "user", p.Message, 0)
		configPath, cleanup, err := api.agentRuntimeConfig(agent)
		if err != nil {
			return nil, err
		}
		id, err := api.Orchestrator.Agents.SpawnWithSessionIsolated(p.AgentID, p.Message, p.SessionName, configPath, cleanup)
		if err != nil {
			return nil, err
		}
		_ = api.Vault.UpdateAgentStatus(p.AgentID, "active")
		return json.Marshal(map[string]string{"agentId": id})
	}
	if api.Vault == nil {
		return nil, fmt.Errorf("vault not available")
	}
	agent, err := api.Vault.GetAgent(p.AgentID)
	if err != nil {
		return nil, err
	}
	configPath, cleanup, err := api.agentRuntimeConfig(agent)
	if err != nil {
		return nil, err
	}
	id, err := api.Orchestrator.Agents.ResumeWithSessionIsolated(p.AgentID, p.SessionName, configPath, cleanup)
	if err != nil {
		return nil, err
	}
	_ = api.Vault.UpdateAgentStatus(p.AgentID, "active")
	return json.Marshal(map[string]string{"agentId": id})
}

func (api *PanelAPI) agentsResumeMany(params json.RawMessage) (json.RawMessage, error) {
	if api.Orchestrator == nil || api.Vault == nil {
		return nil, fmt.Errorf("orchestrator not available")
	}
	var p struct {
		AgentIDs []string `json:"agentIds"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	resumed := 0
	failures := map[string]string{}
	for _, agentID := range p.AgentIDs {
		if strings.TrimSpace(agentID) == "" {
			continue
		}
		agent, err := api.Vault.GetAgent(agentID)
		if err != nil {
			failures[agentID] = err.Error()
			continue
		}
		configPath, cleanup, err := api.agentRuntimeConfig(agent)
		if err != nil {
			failures[agentID] = err.Error()
			continue
		}
		sessionName := "vulpine-" + agentID
		if _, err := api.Orchestrator.Agents.ResumeWithSessionIsolated(agentID, sessionName, configPath, cleanup); err != nil {
			failures[agentID] = err.Error()
			continue
		}
		_ = api.Vault.UpdateAgentStatus(agentID, "active")
		resumed++
	}
	return json.Marshal(map[string]interface{}{
		"status":   "ok",
		"resumed":  resumed,
		"failures": failures,
	})
}

func (api *PanelAPI) agentsKillMany(params json.RawMessage) (json.RawMessage, error) {
	if api.Orchestrator == nil || api.Vault == nil {
		return nil, fmt.Errorf("orchestrator not available")
	}
	var p struct {
		AgentIDs []string `json:"agentIds"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	killed := 0
	failures := map[string]string{}
	for _, agentID := range p.AgentIDs {
		if strings.TrimSpace(agentID) == "" {
			continue
		}
		if err := api.Orchestrator.KillAgent(agentID); err != nil {
			failures[agentID] = err.Error()
			continue
		}
		killed++
	}
	return json.Marshal(map[string]interface{}{
		"status":   "ok",
		"killed":   killed,
		"failures": failures,
	})
}

func (api *PanelAPI) agentsGetMessages(params json.RawMessage) (json.RawMessage, error) {
	if api.Vault == nil {
		return nil, fmt.Errorf("vault not available")
	}
	var p struct {
		AgentID string `json:"agentId"`
		Limit   int    `json:"limit,omitempty"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	var msgs []vault.AgentMessage
	var err error
	if p.Limit > 0 {
		msgs, err = api.Vault.GetRecentMessages(p.AgentID, p.Limit)
	} else {
		msgs, err = api.Vault.GetMessages(p.AgentID)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"messages": msgs})
}

func (api *PanelAPI) agentsGetSessionLog(params json.RawMessage) (json.RawMessage, error) {
	var p struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if strings.TrimSpace(p.AgentID) == "" {
		return nil, fmt.Errorf("agentId is required")
	}
	path := filepath.Join(config.OpenClawProfileDir(), "agents", "main", "sessions", "vulpine-"+p.AgentID+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session log not found")
		}
		return nil, err
	}
	return json.Marshal(map[string]interface{}{
		"path":    path,
		"content": sanitizeSessionLog(string(data)),
	})
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

func (api *PanelAPI) configGet() (json.RawMessage, error) {
	if api.Config == nil {
		return nil, fmt.Errorf("config not available")
	}
	// Return a safe view (mask the API key)
	out := struct {
		Provider                string  `json:"provider"`
		Model                   string  `json:"model"`
		HasKey                  bool    `json:"hasKey"`
		SetupComplete           bool    `json:"setupComplete"`
		BinaryPath              string  `json:"binaryPath,omitempty"`
		DefaultBudgetMaxCostUSD float64 `json:"defaultBudgetMaxCostUsd,omitempty"`
		DefaultBudgetMaxTokens  int64   `json:"defaultBudgetMaxTokens,omitempty"`
	}{
		Provider:                api.Config.Provider,
		Model:                   api.Config.Model,
		HasKey:                  api.Config.APIKey != "",
		SetupComplete:           api.Config.SetupComplete,
		BinaryPath:              api.Config.BinaryPath,
		DefaultBudgetMaxCostUSD: api.Config.DefaultBudgetMaxCostUSD,
		DefaultBudgetMaxTokens:  api.Config.DefaultBudgetMaxTokens,
	}
	return json.Marshal(out)
}

func (api *PanelAPI) configProviders() (json.RawMessage, error) {
	type providerInfo struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		EnvVar       string   `json:"envVar"`
		DefaultModel string   `json:"defaultModel"`
		Models       []string `json:"models"`
		NeedsKey     bool     `json:"needsKey"`
	}
	out := make([]providerInfo, 0, len(config.Providers))
	for _, provider := range config.Providers {
		out = append(out, providerInfo{
			ID:           provider.ID,
			Name:         provider.Name,
			EnvVar:       provider.EnvVar,
			DefaultModel: provider.DefaultModel,
			Models:       provider.Models,
			NeedsKey:     provider.NeedsKey,
		})
	}
	return json.Marshal(map[string]interface{}{"providers": out})
}

func (api *PanelAPI) configSet(params json.RawMessage) (json.RawMessage, error) {
	if api.Config == nil {
		return nil, fmt.Errorf("config not available")
	}
	var p struct {
		Provider                string   `json:"provider,omitempty"`
		Model                   string   `json:"model,omitempty"`
		APIKey                  string   `json:"apiKey,omitempty"`
		DefaultBudgetMaxCostUSD *float64 `json:"defaultBudgetMaxCostUsd,omitempty"`
		DefaultBudgetMaxTokens  *int64   `json:"defaultBudgetMaxTokens,omitempty"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.Provider != "" {
		api.Config.Provider = p.Provider
	}
	if p.Model != "" {
		api.Config.Model = p.Model
	}
	if p.APIKey != "" {
		api.Config.APIKey = p.APIKey
	}
	if p.DefaultBudgetMaxCostUSD != nil {
		api.Config.DefaultBudgetMaxCostUSD = *p.DefaultBudgetMaxCostUSD
	}
	if p.DefaultBudgetMaxTokens != nil {
		api.Config.DefaultBudgetMaxTokens = *p.DefaultBudgetMaxTokens
	}
	api.Config.RefreshSetupComplete()
	if err := api.Config.Save(); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}
	if api.Config.SetupComplete {
		exe, _ := os.Executable()
		if err := api.Config.GenerateOpenClawConfig(exe, api.Config.BinaryPath); err != nil {
			return nil, fmt.Errorf("generate openclaw config: %w", err)
		}
		if err := config.RepairOpenClawProfile(api.Config.FoxbridgeCDPURL); err != nil {
			return nil, fmt.Errorf("repair openclaw profile: %w", err)
		}
	}
	if api.Costs != nil {
		api.Costs.SetModel(api.Config.Model)
	}
	if err := api.SyncPersistentState(); err != nil {
		return nil, fmt.Errorf("sync persistent state: %w", err)
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Cost tracking
// ---------------------------------------------------------------------------

func (api *PanelAPI) costsGetAll() (json.RawMessage, error) {
	if api.Costs == nil {
		return nil, fmt.Errorf("cost tracker not available")
	}
	defaultCost := 0.0
	defaultTokens := int64(0)
	if api.Config != nil {
		defaultCost = api.Config.DefaultBudgetMaxCostUSD
		defaultTokens = api.Config.DefaultBudgetMaxTokens
	}
	return json.Marshal(map[string]interface{}{
		"usage":    api.Costs.AllUsage(),
		"budgets":  api.Costs.AllBudgets(),
		"defaults": map[string]interface{}{"maxCostUsd": defaultCost, "maxTokens": defaultTokens},
	})
}

func (api *PanelAPI) costsSetBudget(params json.RawMessage) (json.RawMessage, error) {
	if api.Costs == nil {
		return nil, fmt.Errorf("cost tracker not available")
	}
	var p struct {
		AgentID        string  `json:"agentId"`
		MaxCost        float64 `json:"maxCostUsd"`
		MaxTokens      int64   `json:"maxTokens"`
		InheritDefault bool    `json:"inheritDefault"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if strings.TrimSpace(p.AgentID) == "" {
		return nil, fmt.Errorf("agentId is required")
	}
	if api.Vault == nil {
		return nil, fmt.Errorf("vault not available")
	}
	agent, err := api.Vault.GetAgent(p.AgentID)
	if err != nil {
		return nil, err
	}
	meta, err := vault.ParseAgentMetadata(agent.Metadata)
	if err != nil {
		return nil, fmt.Errorf("parse agent metadata: %w", err)
	}
	if p.InheritDefault {
		meta.Budget = nil
	} else {
		meta.Budget = &vault.AgentBudgetMetadata{
			Override:   true,
			MaxCostUSD: p.MaxCost,
			MaxTokens:  p.MaxTokens,
		}
	}
	if err := api.Vault.UpdateAgentMetadata(p.AgentID, vault.MarshalAgentMetadata(meta)); err != nil {
		return nil, err
	}
	if err := api.syncAgentState(vault.Agent{ID: agent.ID, Metadata: vault.MarshalAgentMetadata(meta)}); err != nil {
		return nil, err
	}
	effectiveCost, effectiveTokens, source := api.effectiveBudget(meta)
	return json.Marshal(map[string]interface{}{
		"status":         "ok",
		"maxCostUsd":     effectiveCost,
		"maxTokens":      effectiveTokens,
		"budgetSource":   source,
		"inheritDefault": p.InheritDefault,
	})
}

func (api *PanelAPI) costsTotal() (json.RawMessage, error) {
	if api.Costs == nil {
		return nil, fmt.Errorf("cost tracker not available")
	}
	return json.Marshal(map[string]float64{"totalCostUsd": api.Costs.TotalCost()})
}

// ---------------------------------------------------------------------------
// Webhooks
// ---------------------------------------------------------------------------

func (api *PanelAPI) webhooksList() (json.RawMessage, error) {
	if api.Webhooks == nil {
		return nil, fmt.Errorf("webhook manager not available")
	}
	return json.Marshal(map[string]interface{}{"webhooks": api.Webhooks.List()})
}

func (api *PanelAPI) webhooksAdd(params json.RawMessage) (json.RawMessage, error) {
	if api.Webhooks == nil {
		return nil, fmt.Errorf("webhook manager not available")
	}
	var p struct {
		URL    string               `json:"url"`
		Events []webhooks.EventType `json:"events"`
		Secret string               `json:"secret"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	id := api.Webhooks.Register(p.URL, p.Events, p.Secret)
	return json.Marshal(map[string]string{"id": id})
}

func (api *PanelAPI) webhooksRemove(params json.RawMessage) (json.RawMessage, error) {
	if api.Webhooks == nil {
		return nil, fmt.Errorf("webhook manager not available")
	}
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	api.Webhooks.Unregister(p.ID)
	return json.Marshal(map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Proxies
// ---------------------------------------------------------------------------

func (api *PanelAPI) proxiesList() (json.RawMessage, error) {
	if api.Vault == nil {
		return nil, fmt.Errorf("vault not available")
	}
	proxies, err := api.Vault.ListProxies()
	if err != nil {
		return nil, err
	}
	type proxySummary struct {
		ID        string `json:"id"`
		URL       string `json:"url"`
		Country   string `json:"country,omitempty"`
		Label     string `json:"label"`
		LatencyMS int64  `json:"latencyMs"`
	}
	out := make([]proxySummary, 0, len(proxies))
	for _, stored := range proxies {
		summary := proxySummary{ID: stored.ID, Label: stored.Label}
		var cfg proxy.ProxyConfig
		if err := json.Unmarshal([]byte(stored.Config), &cfg); err == nil {
			summary.URL = cfg.URL()
		}
		var geo proxy.GeoInfo
		if err := json.Unmarshal([]byte(stored.Geo), &geo); err == nil {
			summary.Country = geo.Country
		}
		out = append(out, summary)
	}
	return json.Marshal(map[string]interface{}{"proxies": out})
}

func (api *PanelAPI) proxiesAdd(params json.RawMessage) (json.RawMessage, error) {
	if api.Vault == nil {
		return nil, fmt.Errorf("vault not available")
	}
	var p struct {
		Config string `json:"config"` // JSON proxy config or proxy URL
		URL    string `json:"url"`
		Geo    string `json:"geo"`
		Label  string `json:"label"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.Config == "" && p.URL != "" {
		pc, err := proxy.ParseProxyURL(p.URL)
		if err != nil {
			return nil, err
		}
		data, _ := json.Marshal(pc)
		p.Config = string(data)
		if p.Label == "" {
			p.Label = p.URL
		}
	}
	stored, err := api.Vault.AddProxy(p.Config, p.Geo, p.Label)
	if err != nil {
		return nil, err
	}
	return json.Marshal(stored)
}

func (api *PanelAPI) proxiesDelete(params json.RawMessage) (json.RawMessage, error) {
	if api.Vault == nil {
		return nil, fmt.Errorf("vault not available")
	}
	var p struct {
		ID      string `json:"id"`
		ProxyID string `json:"proxyId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.ID == "" {
		p.ID = p.ProxyID
	}
	if err := api.Vault.DeleteProxy(p.ID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func (api *PanelAPI) proxiesTest(params json.RawMessage) (json.RawMessage, error) {
	var p struct {
		URL     string `json:"url"`
		ProxyID string `json:"proxyId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	var pc *proxy.ProxyConfig
	if p.URL != "" {
		parsed, err := proxy.ParseProxyURL(p.URL)
		if err != nil {
			return nil, fmt.Errorf("parse proxy URL: %w", err)
		}
		pc = parsed
	} else if p.ProxyID != "" && api.Vault != nil {
		stored, err := api.Vault.GetProxy(p.ProxyID)
		if err != nil {
			return nil, err
		}
		var cfg proxy.ProxyConfig
		if err := json.Unmarshal([]byte(stored.Config), &cfg); err != nil {
			return nil, fmt.Errorf("parse stored proxy config: %w", err)
		}
		pc = &cfg
	} else {
		return nil, fmt.Errorf("proxy url or proxy id is required")
	}
	latency, err := proxy.TestProxy(*pc)
	if err != nil {
		return nil, fmt.Errorf("proxy test failed: %w", err)
	}
	return json.Marshal(map[string]int64{"latencyMs": latency, "latency": latency})
}

func (api *PanelAPI) proxiesSetRotation(params json.RawMessage) (json.RawMessage, error) {
	if api.Rotator == nil {
		return nil, fmt.Errorf("proxy rotator not available")
	}
	var p struct {
		AgentID string                `json:"agentId"`
		Config  rotationConfigPayload `json:"config"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if strings.TrimSpace(p.AgentID) == "" {
		return nil, fmt.Errorf("agentId is required")
	}
	if api.Vault == nil {
		return nil, fmt.Errorf("vault not available")
	}
	agent, err := api.Vault.GetAgent(p.AgentID)
	if err != nil {
		return nil, err
	}
	meta, err := vault.ParseAgentMetadata(agent.Metadata)
	if err != nil {
		return nil, fmt.Errorf("parse agent metadata: %w", err)
	}
	cfg := rotationConfigFromPayload(p.Config)
	meta.ProxyRotation = rotationMetadataFromConfig(cfg)
	metadata := vault.MarshalAgentMetadata(meta)
	if err := api.Vault.UpdateAgentMetadata(p.AgentID, metadata); err != nil {
		return nil, err
	}
	api.Rotator.SetConfig(p.AgentID, &cfg)
	return json.Marshal(map[string]interface{}{"status": "ok", "config": rotationPayloadFromConfig(cfg)})
}

func (api *PanelAPI) proxiesGetRotation(params json.RawMessage) (json.RawMessage, error) {
	var p struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if strings.TrimSpace(p.AgentID) == "" {
		return nil, fmt.Errorf("agentId is required")
	}
	if api.Vault == nil {
		return nil, fmt.Errorf("vault not available")
	}
	agent, err := api.Vault.GetAgent(p.AgentID)
	if err != nil {
		return nil, err
	}
	meta, err := vault.ParseAgentMetadata(agent.Metadata)
	if err != nil {
		return nil, fmt.Errorf("parse agent metadata: %w", err)
	}
	cfg, source := api.effectiveRotation(p.AgentID, meta)
	return json.Marshal(map[string]interface{}{"config": rotationPayloadFromConfig(cfg), "source": source})
}

// ---------------------------------------------------------------------------
// Agent Bus
// ---------------------------------------------------------------------------

func (api *PanelAPI) busPending() (json.RawMessage, error) {
	if api.AgentBus == nil {
		return nil, fmt.Errorf("agent bus not available")
	}
	return json.Marshal(api.AgentBus.PendingMessages())
}

func (api *PanelAPI) busApprove(params json.RawMessage) (json.RawMessage, error) {
	if api.AgentBus == nil {
		return nil, fmt.Errorf("agent bus not available")
	}
	var p struct {
		MessageID string `json:"messageId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if err := api.AgentBus.Approve(p.MessageID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func (api *PanelAPI) busReject(params json.RawMessage) (json.RawMessage, error) {
	if api.AgentBus == nil {
		return nil, fmt.Errorf("agent bus not available")
	}
	var p struct {
		MessageID string `json:"messageId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if err := api.AgentBus.Reject(p.MessageID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"status": "ok"})
}

func (api *PanelAPI) busPolicies() (json.RawMessage, error) {
	if api.AgentBus == nil {
		return nil, fmt.Errorf("agent bus not available")
	}
	return json.Marshal(api.AgentBus.Policies())
}

func (api *PanelAPI) busAddPolicy(params json.RawMessage) (json.RawMessage, error) {
	if api.AgentBus == nil {
		return nil, fmt.Errorf("agent bus not available")
	}
	var p struct {
		FromAgent   string `json:"fromAgent"`
		ToAgent     string `json:"toAgent"`
		AutoApprove bool   `json:"autoApprove"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	api.AgentBus.AddPolicy(p.FromAgent, p.ToAgent, p.AutoApprove)
	return json.Marshal(map[string]string{"status": "ok"})
}

func (api *PanelAPI) busRemovePolicy(params json.RawMessage) (json.RawMessage, error) {
	if api.AgentBus == nil {
		return nil, fmt.Errorf("agent bus not available")
	}
	var p struct {
		FromAgent string `json:"fromAgent"`
		ToAgent   string `json:"toAgent"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	api.AgentBus.RemovePolicy(p.FromAgent, p.ToAgent)
	return json.Marshal(map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Recording
// ---------------------------------------------------------------------------

func (api *PanelAPI) recordingGetTimeline(params json.RawMessage) (json.RawMessage, error) {
	if api.Recorder == nil {
		return nil, fmt.Errorf("recorder not available")
	}
	var p struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	timeline := api.Recorder.GetTimeline(p.AgentID)
	if timeline == nil {
		timeline = []recording.Action{}
	}
	return json.Marshal(map[string]interface{}{"actions": timeline})
}

func (api *PanelAPI) recordingExport(params json.RawMessage) (json.RawMessage, error) {
	if api.Recorder == nil {
		return nil, fmt.Errorf("recorder not available")
	}
	var p struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	data, err := api.Recorder.Export(p.AgentID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{
		"content":     string(data),
		"contentType": "application/json",
		"fileName":    fmt.Sprintf("agent-%s-recording.json", p.AgentID),
	})
}

// ---------------------------------------------------------------------------
// Fingerprints
// ---------------------------------------------------------------------------

func (api *PanelAPI) fingerprintsGet(params json.RawMessage) (json.RawMessage, error) {
	if api.Vault == nil {
		return nil, fmt.Errorf("vault not available")
	}
	var p struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	agent, err := api.Vault.GetAgent(p.AgentID)
	if err != nil {
		return nil, err
	}
	// Return parsed fingerprint data plus the summary
	fp, _ := vault.ParseFingerprint(agent.Fingerprint)
	out := struct {
		Raw     string                 `json:"raw"`
		Parsed  *vault.FingerprintData `json:"parsed,omitempty"`
		Summary string                 `json:"summary"`
	}{
		Raw:     agent.Fingerprint,
		Parsed:  fp,
		Summary: vault.FingerprintSummary(agent.Fingerprint),
	}
	return json.Marshal(out)
}

func (api *PanelAPI) fingerprintsGenerate(params json.RawMessage) (json.RawMessage, error) {
	var p struct {
		Seed    string `json:"seed"`
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.Seed == "" {
		p.Seed = "default"
	}
	fp, err := vault.GenerateFingerprint(p.Seed)
	if err != nil {
		return nil, err
	}
	applied := false
	if strings.TrimSpace(p.AgentID) != "" {
		if api.Vault == nil {
			return nil, fmt.Errorf("vault not available")
		}
		if err := api.Vault.UpdateAgentFingerprint(p.AgentID, fp); err != nil {
			return nil, err
		}
		applied = true
	}
	return json.Marshal(map[string]interface{}{
		"fingerprint": fp,
		"summary":     vault.FingerprintSummary(fp),
		"applied":     applied,
	})
}

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

func (api *PanelAPI) statusGet() (json.RawMessage, error) {
	route, source := api.browserRoute()
	out := map[string]interface{}{
		"kernelUp":                    false,
		"kernelPid":                   0,
		"kernel_running":              false,
		"kernel_pid":                  0,
		"kernel_headless":             false,
		"browser_route":               route,
		"browser_route_source":        source,
		"browser_window":              api.browserWindow(),
		"gateway_running":             false,
		"openclaw_profile_configured": config.OpenClawProfileBrowserRoute() != "",
		"sentinel_available":          false,
		"sentinel_mode":               extensions.SentinelModePublicNoop,
		"sentinel_maturity_metrics":   0,
		"sentinel_assignment_rules":   0,
	}

	if api.Kernel != nil {
		out["kernelUp"] = api.Kernel.Running()
		out["kernelPid"] = api.Kernel.PID()
		out["kernel_running"] = api.Kernel.Running()
		out["kernel_pid"] = api.Kernel.PID()
		out["kernel_headless"] = api.Kernel.IsHeadless()
	}
	if api.Gateway != nil {
		out["gateway_running"] = api.Gateway.Running()
	}
	if api.Orchestrator != nil {
		status := api.Orchestrator.Status()
		out["orchestrator"] = &status
		out["kernel_running"] = status.KernelRunning
		out["kernel_pid"] = status.KernelPID
		out["pool_available"] = status.PoolAvailable
		out["pool_active"] = status.PoolActive
		out["pool_total"] = status.PoolTotal
		out["active_agents"] = status.ActiveAgents
		out["total_citizens"] = status.TotalCitizens
		out["total_templates"] = status.TotalTemplates
		out["total_cost_usd"] = status.TotalCostUSD
	}
	if status, available, err := extensions.SentinelSnapshot(context.Background()); err == nil {
		out["sentinel_available"] = available
		out["sentinel_provider"] = status.Provider
		out["sentinel_mode"] = status.Mode
		out["sentinel_event_sink"] = status.EventSink
		out["sentinel_outcome_sink"] = status.OutcomeSink
		out["sentinel_variant_source"] = status.VariantSource
		out["sentinel_variant_bundles"] = status.VariantBundles
		out["sentinel_trust_recipes"] = status.TrustRecipes
		out["sentinel_maturity_metrics"] = status.MaturityMetrics
		out["sentinel_assignment_rules"] = status.AssignmentRules
		if !status.UpdatedAt.IsZero() {
			out["sentinel_updated_at"] = status.UpdatedAt
		}
	} else if err != nil {
		out["sentinel_error"] = err.Error()
	}

	return json.Marshal(out)
}

func (api *PanelAPI) sentinelGet() (json.RawMessage, error) {
	status, available, err := extensions.SentinelSnapshot(context.Background())
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{
		"available":         available,
		"status":            status,
		"variantBundles":    []extensions.SentinelVariantBundle{},
		"trustRecipes":      []extensions.SentinelTrustRecipe{},
		"maturityMetrics":   []extensions.SentinelMaturityMetric{},
		"assignmentRules":   []extensions.SentinelAssignmentRule{},
		"outcomeLabels":     []extensions.SentinelOutcomeLabel{},
		"outcomeSummary":    []extensions.SentinelOutcomeSummary{},
		"probeSummary":      []extensions.SentinelProbeSummary{},
		"trustActivity":     []extensions.SentinelTrustActivitySummary{},
		"sitePressure":      []extensions.SentinelSitePressureSummary{},
		"patchQueue":        []extensions.SentinelPatchCandidate{},
		"experimentSummary": []extensions.SentinelExperimentSummary{},
	}
	if !available {
		return json.Marshal(out)
	}
	provider := extensions.Registry.Sentinel()
	if provider == nil {
		return json.Marshal(out)
	}
	bundles, err := provider.ListVariantBundles(context.Background())
	if err != nil {
		return nil, err
	}
	trustRecipes, err := provider.ListTrustRecipes(context.Background())
	if err != nil {
		return nil, err
	}
	maturityMetrics, err := provider.ListMaturityMetrics(context.Background())
	if err != nil {
		return nil, err
	}
	assignmentRules, err := provider.ListAssignmentRules(context.Background())
	if err != nil {
		return nil, err
	}
	outcomeLabels, err := provider.ListOutcomeLabels(context.Background())
	if err != nil {
		return nil, err
	}
	outcomeSummary, err := provider.SummarizeOutcomes(context.Background())
	if err != nil {
		return nil, err
	}
	probeSummary, err := provider.SummarizeProbes(context.Background())
	if err != nil {
		return nil, err
	}
	trustActivity, err := provider.SummarizeTrustActivity(context.Background())
	if err != nil {
		return nil, err
	}
	sitePressure, err := provider.SummarizeSitePressure(context.Background())
	if err != nil {
		return nil, err
	}
	patchQueue, err := provider.SummarizePatchQueue(context.Background())
	if err != nil {
		return nil, err
	}
	experimentSummary, err := provider.SummarizeExperiments(context.Background())
	if err != nil {
		return nil, err
	}
	out["variantBundles"] = bundles
	out["trustRecipes"] = trustRecipes
	out["maturityMetrics"] = maturityMetrics
	out["assignmentRules"] = assignmentRules
	out["outcomeLabels"] = outcomeLabels
	out["outcomeSummary"] = outcomeSummary
	out["probeSummary"] = probeSummary
	out["trustActivity"] = trustActivity
	out["sitePressure"] = sitePressure
	out["patchQueue"] = patchQueue
	out["experimentSummary"] = experimentSummary
	return json.Marshal(out)
}

func (api *PanelAPI) sentinelTimeline(params json.RawMessage) (json.RawMessage, error) {
	var filter extensions.SentinelTimelineFilter
	if len(params) > 0 {
		if err := json.Unmarshal(params, &filter); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
	}
	if filter.Limit <= 0 {
		filter.Limit = 5
	}
	provider := extensions.Registry.Sentinel()
	out := map[string]interface{}{
		"sessions": []extensions.SentinelSessionTimeline{},
	}
	if provider == nil || !provider.Available() {
		return json.Marshal(out)
	}
	timelines, err := provider.ListSessionTimelines(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	out["sessions"] = timelines
	return json.Marshal(out)
}

func (api *PanelAPI) browserRoute() (string, string) {
	switch {
	case api.Config != nil && strings.TrimSpace(api.Config.FoxbridgeCDPURL) != "":
		return "camoufox", "runtime"
	case config.OpenClawProfileBrowserRoute() != "":
		return config.OpenClawProfileBrowserRoute(), "profile"
	case api.Kernel == nil:
		return "disabled", "server"
	case api.Kernel != nil && api.Kernel.IsHeadless():
		return "headless", "kernel"
	default:
		return "direct", "kernel"
	}
}

func (api *PanelAPI) browserWindow() string {
	if api.Kernel == nil {
		return "n/a"
	}
	if api.Kernel.IsHeadless() {
		return "headless"
	}
	w := api.Kernel.Window()
	if w == nil {
		return "unavailable"
	}
	visible, found := w.Status()
	if !found {
		return "unavailable"
	}
	if visible {
		return "visible"
	}
	return "hidden"
}

func (api *PanelAPI) runtimeList(params json.RawMessage) (json.RawMessage, error) {
	if api.RuntimeAudit == nil {
		return json.Marshal(map[string]interface{}{
			"events":   []vault.RuntimeEvent{},
			"settings": vault.RuntimeAuditSettings{},
			"applied":  map[string]interface{}{},
		})
	}
	filter, _, err := decodeRuntimeAuditParams(params)
	if err != nil {
		return nil, err
	}
	settings, events, applied, err := api.runtimeAuditSnapshot(filter)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{
		"events":   events,
		"settings": settings,
		"applied":  applied,
	})
}

func (api *PanelAPI) runtimeExport(params json.RawMessage) (json.RawMessage, error) {
	if api.RuntimeAudit == nil {
		return json.Marshal(map[string]interface{}{
			"content":     "",
			"contentType": "application/json",
			"fileName":    "runtime-audit.json",
			"format":      "json",
		})
	}
	filter, format, err := decodeRuntimeAuditParams(params)
	if err != nil {
		return nil, err
	}
	settings, events, applied, err := api.runtimeAuditSnapshot(filter)
	if err != nil {
		return nil, err
	}
	if format == "" {
		format = "json"
	}

	exportedAt := time.Now().UTC()
	switch format {
	case "json":
		payload, err := json.MarshalIndent(map[string]interface{}{
			"exportedAt": exportedAt.Format(time.RFC3339),
			"settings":   settings,
			"applied":    applied,
			"events":     events,
		}, "", "  ")
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]interface{}{
			"content":     string(payload),
			"contentType": "application/json",
			"fileName":    "runtime-audit-" + exportedAt.Format("20060102-150405") + ".json",
			"format":      "json",
		})
	case "ndjson":
		lines := make([]string, 0, len(events)+1)
		header, err := json.Marshal(map[string]interface{}{
			"type":       "runtime_audit_export",
			"exportedAt": exportedAt.Format(time.RFC3339),
			"settings":   settings,
			"applied":    applied,
		})
		if err != nil {
			return nil, err
		}
		lines = append(lines, string(header))
		for _, event := range events {
			encoded, err := json.Marshal(event)
			if err != nil {
				return nil, err
			}
			lines = append(lines, string(encoded))
		}
		return json.Marshal(map[string]interface{}{
			"content":     strings.Join(lines, "\n") + "\n",
			"contentType": "application/x-ndjson",
			"fileName":    "runtime-audit-" + exportedAt.Format("20060102-150405") + ".ndjson",
			"format":      "ndjson",
		})
	default:
		return nil, fmt.Errorf("invalid format: %s", format)
	}
}

func (api *PanelAPI) runtimeAuditSnapshot(filter vault.RuntimeEventFilter) (vault.RuntimeAuditSettings, []vault.RuntimeEvent, map[string]interface{}, error) {
	settings, err := api.RuntimeAudit.Settings()
	if err != nil {
		return vault.RuntimeAuditSettings{}, nil, nil, err
	}
	events, err := api.RuntimeAudit.List(filter)
	if err != nil {
		return vault.RuntimeAuditSettings{}, nil, nil, err
	}
	return settings, events, map[string]interface{}{
		"limit":     filter.Limit,
		"component": filter.Component,
		"level":     filter.Level,
		"event":     filter.Event,
		"query":     filter.Query,
	}, nil
}

func decodeRuntimeAuditParams(params json.RawMessage) (vault.RuntimeEventFilter, string, error) {
	var p struct {
		Limit     int    `json:"limit"`
		Component string `json:"component"`
		Level     string `json:"level"`
		Event     string `json:"event"`
		Query     string `json:"query"`
		Format    string `json:"format"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return vault.RuntimeEventFilter{}, "", fmt.Errorf("invalid params: %w", err)
		}
	}
	return vault.RuntimeEventFilter{
		Limit:     p.Limit,
		Component: p.Component,
		Level:     p.Level,
		Event:     p.Event,
		Query:     p.Query,
	}, p.Format, nil
}

func (api *PanelAPI) runtimeSetRetention(params json.RawMessage) (json.RawMessage, error) {
	if api.RuntimeAudit == nil {
		return json.Marshal(map[string]interface{}{
			"settings": vault.RuntimeAuditSettings{},
		})
	}
	var p struct {
		Retention int `json:"retention"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	settings, err := api.RuntimeAudit.SetRetention(p.Retention)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{
		"settings": settings,
	})
}

func (api *PanelAPI) agentRuntimeConfig(agent *vault.Agent) (string, func(), error) {
	if agent == nil {
		return "", nil, fmt.Errorf("agent not found")
	}
	if api.Config != nil {
		if err := config.RepairOpenClawProfile(api.Config.FoxbridgeCDPURL); err != nil {
			return "", nil, fmt.Errorf("repair openclaw profile: %w", err)
		}
	}
	meta, err := vault.ParseAgentMetadata(agent.Metadata)
	if err != nil {
		return "", nil, fmt.Errorf("parse agent metadata: %w", err)
	}
	if meta.ContextID == "" {
		return openclaw.PrepareRuntimeConfig(config.OpenClawConfigPath())
	}
	if api.Orchestrator == nil {
		return "", nil, fmt.Errorf("orchestrator not available")
	}
	return api.Orchestrator.PrepareScopedOpenClawConfig(meta.ContextID)
}

func (api *PanelAPI) contextsList() (json.RawMessage, error) {
	if api.Contexts == nil {
		return json.Marshal(map[string]interface{}{"contexts": []ContextInfo{}})
	}
	return json.Marshal(map[string]interface{}{"contexts": api.Contexts.List()})
}

func (api *PanelAPI) contextsCreate(params json.RawMessage) (json.RawMessage, error) {
	if api.Client == nil {
		return nil, fmt.Errorf("juggler client not available")
	}
	var p struct {
		RemoveOnDetach bool `json:"removeOnDetach"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
	}
	result, err := api.Client.Call("", "Browser.createBrowserContext", map[string]interface{}{
		"removeOnDetach": p.RemoveOnDetach,
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		return nil, fmt.Errorf("parse createBrowserContext result: %w", err)
	}
	if api.Contexts != nil {
		api.Contexts.Created(out.BrowserContextID)
	}
	return json.Marshal(map[string]string{"browserContextId": out.BrowserContextID})
}

func (api *PanelAPI) contextsRemove(params json.RawMessage) (json.RawMessage, error) {
	if api.Client == nil {
		return nil, fmt.Errorf("juggler client not available")
	}
	var p struct {
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.BrowserContextID == "" {
		return nil, fmt.Errorf("browserContextId is required")
	}
	if _, err := api.Client.Call("", "Browser.removeBrowserContext", map[string]interface{}{
		"browserContextId": p.BrowserContextID,
	}); err != nil {
		return nil, err
	}
	if api.Contexts != nil {
		api.Contexts.Removed(p.BrowserContextID)
	}
	return json.Marshal(map[string]string{"status": "ok"})
}
