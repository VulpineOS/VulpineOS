package remote

import (
	"fmt"
	"time"

	"vulpineos/internal/proxy"
	"vulpineos/internal/vault"
)

type rotationConfigPayload struct {
	Enabled               bool     `json:"enabled"`
	RotateOnRateLimit     bool     `json:"rotateOnRateLimit"`
	RotateOnBlock         bool     `json:"rotateOnBlock"`
	RotateIntervalSeconds int64    `json:"rotateIntervalSeconds"`
	SyncFingerprint       bool     `json:"syncFingerprint"`
	ProxyPool             []string `json:"proxyPool"`
	CurrentIndex          int      `json:"currentIndex"`
}

func (api *PanelAPI) SyncPersistentState() error {
	if api.Vault == nil {
		return nil
	}
	agents, err := api.Vault.ListAgents()
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}
	for _, agent := range agents {
		if err := api.syncAgentState(agent); err != nil {
			return err
		}
	}
	return nil
}

func (api *PanelAPI) syncAgentState(agent vault.Agent) error {
	meta, err := vault.ParseAgentMetadata(agent.Metadata)
	if err != nil {
		return fmt.Errorf("parse metadata for %s: %w", agent.ID, err)
	}
	api.applyBudget(agent.ID, meta)
	api.applyRotation(agent.ID, meta)
	return nil
}

func (api *PanelAPI) applyBudget(agentID string, meta vault.AgentMetadata) {
	if api.Costs == nil {
		return
	}
	if meta.Budget != nil && meta.Budget.Override {
		api.Costs.SetBudget(agentID, meta.Budget.MaxCostUSD, meta.Budget.MaxTokens)
		return
	}
	if api.Config != nil && (api.Config.DefaultBudgetMaxCostUSD > 0 || api.Config.DefaultBudgetMaxTokens > 0) {
		api.Costs.SetBudget(agentID, api.Config.DefaultBudgetMaxCostUSD, api.Config.DefaultBudgetMaxTokens)
		return
	}
	api.Costs.ClearBudget(agentID)
}

func (api *PanelAPI) effectiveBudget(meta vault.AgentMetadata) (float64, int64, string) {
	if meta.Budget != nil && meta.Budget.Override {
		return meta.Budget.MaxCostUSD, meta.Budget.MaxTokens, "agent"
	}
	if api.Config != nil && (api.Config.DefaultBudgetMaxCostUSD > 0 || api.Config.DefaultBudgetMaxTokens > 0) {
		return api.Config.DefaultBudgetMaxCostUSD, api.Config.DefaultBudgetMaxTokens, "default"
	}
	return 0, 0, "none"
}

func (api *PanelAPI) applyRotation(agentID string, meta vault.AgentMetadata) {
	if api.Rotator == nil {
		return
	}
	if meta.ProxyRotation == nil {
		api.Rotator.SetConfig(agentID, proxy.DefaultRotationConfig())
		return
	}
	cfg := rotationConfigFromMetadata(meta.ProxyRotation)
	api.Rotator.SetConfig(agentID, &cfg)
}

func (api *PanelAPI) effectiveRotation(agentID string, meta vault.AgentMetadata) (proxy.RotationConfig, string) {
	if meta.ProxyRotation != nil {
		return rotationConfigFromMetadata(meta.ProxyRotation), "agent"
	}
	if api.Rotator != nil {
		if cfg := api.Rotator.GetConfig(agentID); cfg != nil {
			return cloneRotationConfig(*cfg), "runtime"
		}
	}
	return *proxy.DefaultRotationConfig(), "default"
}

func rotationConfigFromMetadata(meta *vault.AgentRotationMetadata) proxy.RotationConfig {
	if meta == nil {
		return *proxy.DefaultRotationConfig()
	}
	cfg := proxy.RotationConfig{
		Enabled:           meta.Enabled,
		RotateOnRateLimit: meta.RotateOnRateLimit,
		RotateOnBlock:     meta.RotateOnBlock,
		RotateInterval:    time.Duration(meta.RotateIntervalSeconds) * time.Second,
		SyncFingerprint:   meta.SyncFingerprint,
		ProxyPool:         append([]string(nil), meta.ProxyPool...),
		CurrentIndex:      meta.CurrentIndex,
	}
	if meta.SyncFingerprint == false {
		cfg.SyncFingerprint = false
	}
	return cfg
}

func rotationMetadataFromConfig(cfg proxy.RotationConfig) *vault.AgentRotationMetadata {
	return &vault.AgentRotationMetadata{
		Enabled:               cfg.Enabled,
		RotateOnRateLimit:     cfg.RotateOnRateLimit,
		RotateOnBlock:         cfg.RotateOnBlock,
		RotateIntervalSeconds: int64(cfg.RotateInterval / time.Second),
		SyncFingerprint:       cfg.SyncFingerprint,
		ProxyPool:             append([]string(nil), cfg.ProxyPool...),
		CurrentIndex:          cfg.CurrentIndex,
	}
}

func cloneRotationConfig(cfg proxy.RotationConfig) proxy.RotationConfig {
	cfg.ProxyPool = append([]string(nil), cfg.ProxyPool...)
	return cfg
}

func rotationPayloadFromConfig(cfg proxy.RotationConfig) rotationConfigPayload {
	return rotationConfigPayload{
		Enabled:               cfg.Enabled,
		RotateOnRateLimit:     cfg.RotateOnRateLimit,
		RotateOnBlock:         cfg.RotateOnBlock,
		RotateIntervalSeconds: int64(cfg.RotateInterval / time.Second),
		SyncFingerprint:       cfg.SyncFingerprint,
		ProxyPool:             append([]string(nil), cfg.ProxyPool...),
		CurrentIndex:          cfg.CurrentIndex,
	}
}

func rotationConfigFromPayload(p rotationConfigPayload) proxy.RotationConfig {
	cfg := *proxy.DefaultRotationConfig()
	cfg.Enabled = p.Enabled
	cfg.RotateOnRateLimit = p.RotateOnRateLimit
	cfg.RotateOnBlock = p.RotateOnBlock
	cfg.RotateInterval = time.Duration(p.RotateIntervalSeconds) * time.Second
	cfg.SyncFingerprint = p.SyncFingerprint
	cfg.ProxyPool = append([]string(nil), p.ProxyPool...)
	cfg.CurrentIndex = p.CurrentIndex
	return cfg
}
