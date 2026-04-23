import React, { useEffect, useState } from 'react'

export default function Settings({ ws }) {
  const [cfg, setCfg] = useState({})
  const [providers, setProviders] = useState([])
  const [status, setStatus] = useState({})
  const [sentinel, setSentinel] = useState({ variantBundles: [], trustRecipes: [], maturityMetrics: [], assignmentRules: [], outcomeLabels: [], outcomeSummary: [], probeSummary: [], trustActivity: [], trustEffectiveness: [], trustAssets: [], maturityEvidence: [], transportEvidence: [], coherenceDiff: [], stageSummary: [], assignmentRecommendations: [], canarySummary: [], variantCompareSummary: [], siteIntelligenceSummary: [], probeSequenceSummary: [], vendorIntelligenceSummary: [], vendorEffectiveness: [], vendorUplift: [], vendorRollout: [], sitePressure: [], patchQueue: [], experimentSummary: [] })
  const [sentinelTimeline, setSentinelTimeline] = useState([])
  const [defaultBudgetCost, setDefaultBudgetCost] = useState('0')
  const [defaultBudgetTokens, setDefaultBudgetTokens] = useState('0')
  const [saved, setSaved] = useState('')

  useEffect(() => {
    if (!ws.connected) return
    ws.call('config.providers').then(r => setProviders(r?.providers || [])).catch(() => {})
    ws.call('config.get').then(r => {
      setCfg(r || {})
      setDefaultBudgetCost(String(r?.defaultBudgetMaxCostUsd ?? 0))
      setDefaultBudgetTokens(String(r?.defaultBudgetMaxTokens ?? 0))
    }).catch(() => {})
    ws.call('status.get').then(r => setStatus(r || {})).catch(() => {})
    ws.call('sentinel.get').then(r => setSentinel(r || { variantBundles: [], trustRecipes: [], maturityMetrics: [], assignmentRules: [], outcomeLabels: [], outcomeSummary: [], probeSummary: [], trustActivity: [], trustEffectiveness: [], trustAssets: [], maturityEvidence: [], transportEvidence: [], coherenceDiff: [], stageSummary: [], assignmentRecommendations: [], canarySummary: [], variantCompareSummary: [], siteIntelligenceSummary: [], probeSequenceSummary: [], vendorIntelligenceSummary: [], vendorEffectiveness: [], vendorUplift: [], vendorRollout: [], sitePressure: [], patchQueue: [], experimentSummary: [] })).catch(() => {})
    ws.call('sentinel.timeline', { limit: 4 }).then(r => setSentinelTimeline(r?.sessions || [])).catch(() => {})
  }, [ws.connected])

  const selectedProvider = providers.find(p => p.id === cfg.provider) || null
  const modelOptions = selectedProvider?.models?.length ? selectedProvider.models : (cfg.model ? [cfg.model] : [])
  const kernelMode = status.kernel_running ? (status.kernel_headless ? 'HEADLESS' : 'GUI') : 'DISABLED'
  const sentinelVariants = sentinel.variantBundles || []
  const sentinelTrustRecipes = sentinel.trustRecipes || []
  const sentinelMetrics = sentinel.maturityMetrics || []
  const sentinelRules = sentinel.assignmentRules || []
  const sentinelOutcomeLabels = sentinel.outcomeLabels || []
  const sentinelOutcomeSummary = sentinel.outcomeSummary || []
  const sentinelProbeSummary = sentinel.probeSummary || []
  const sentinelTrustActivity = sentinel.trustActivity || []
  const sentinelTrustEffectiveness = sentinel.trustEffectiveness || []
  const sentinelTrustAssets = sentinel.trustAssets || []
  const sentinelMaturityEvidence = sentinel.maturityEvidence || []
  const sentinelTransportEvidence = sentinel.transportEvidence || []
  const sentinelCoherenceDiff = sentinel.coherenceDiff || []
  const sentinelStageSummary = sentinel.stageSummary || []
  const sentinelAssignmentRecommendations = sentinel.assignmentRecommendations || []
  const sentinelCanarySummary = sentinel.canarySummary || []
  const sentinelVariantCompareSummary = sentinel.variantCompareSummary || []
  const sentinelSiteIntelligenceSummary = sentinel.siteIntelligenceSummary || []
  const sentinelProbeSequenceSummary = sentinel.probeSequenceSummary || []
  const sentinelVendorIntelligenceSummary = sentinel.vendorIntelligenceSummary || []
  const sentinelVendorEffectiveness = sentinel.vendorEffectiveness || []
  const sentinelVendorUplift = sentinel.vendorUplift || []
  const sentinelVendorRollout = sentinel.vendorRollout || []
  const sentinelSitePressure = sentinel.sitePressure || []
  const sentinelPatchQueue = sentinel.patchQueue || []
  const sentinelExperimentSummary = sentinel.experimentSummary || []
  const variantNameFor = (id) => sentinelVariants.find(bundle => bundle.id === id)?.name || id || 'unassigned'
  const trustNameFor = (id) => sentinelTrustRecipes.find(recipe => recipe.id === id)?.name || id || 'unassigned'

  const formatMetricThresholds = (metric) => (metric.thresholds || [])
    .map(threshold => `${threshold.stage} ${threshold.minimum}${metric.unit ? ` ${metric.unit}` : ''}`)
    .join(' · ')

  const formatRuleGate = (rule) => {
    const parts = []
    if (rule.minSessionAgeSeconds) parts.push(`age ${rule.minSessionAgeSeconds}s`)
    if (rule.minSuccessfulVisits) parts.push(`${rule.minSuccessfulVisits} visits`)
    if (rule.minDistinctDays) parts.push(`${rule.minDistinctDays} days`)
    if (rule.minChallengeFreeRuns) parts.push(`${rule.minChallengeFreeRuns} quiet runs`)
    if (rule.maxRecentHardChallenges === 0) parts.push('no recent hard challenges')
    return parts.join(' · ') || 'Always eligible'
  }

  const formatTimelineItem = (item) => {
    const assignment = item.variantBundleId || item.trustRecipeId
      ? `${variantNameFor(item.variantBundleId)} / ${trustNameFor(item.trustRecipeId)}`
      : ''
    if (item.type === 'outcome') {
      return `${item.outcome || 'outcome'}${item.challengeVendor ? ` · ${item.challengeVendor}` : ''}${assignment ? ` · ${assignment}` : ''}`
    }
    const parts = [`${item.kind || 'event'} · ${item.name || 'unnamed'}`]
    if (item.attributes?.detail) parts.push(item.attributes.detail)
    if (item.attributes?.count) parts.push(`x${item.attributes.count}`)
    if (item.attributes?.prior_session_count) parts.push(`seen ${item.attributes.prior_session_count} sessions`)
    if (item.attributes?.distinct_days_seen) parts.push(`${item.attributes.distinct_days_seen} days`)
    if (item.attributes?.hours_since_last_seen) parts.push(`gap ${item.attributes.hours_since_last_seen}h`)
    if (assignment) parts.push(assignment)
    return parts.join(' · ')
  }

  const updateProvider = (providerId) => {
    const provider = providers.find(p => p.id === providerId)
    setCfg(prev => ({
      ...prev,
      provider: providerId,
      model: provider?.defaultModel || prev.model || '',
    }))
  }

  const flashSaved = (message) => {
    setSaved(message)
    setTimeout(() => setSaved(''), 3000)
  }

  const saveProvider = async () => {
    try {
      await ws.call('config.set', { provider: cfg.provider, model: cfg.model, apiKey: cfg.apiKey })
      flashSaved('Provider saved')
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  const saveDefaults = async () => {
    const maxCost = Number(defaultBudgetCost)
    const maxTokens = Number(defaultBudgetTokens)
    try {
      await ws.call('config.set', {
        defaultBudgetMaxCostUsd: Number.isFinite(maxCost) ? maxCost : 0,
        defaultBudgetMaxTokens: Number.isFinite(maxTokens) ? Math.max(0, Math.trunc(maxTokens)) : 0,
      })
      setCfg(prev => ({
        ...prev,
        defaultBudgetMaxCostUsd: Number.isFinite(maxCost) ? maxCost : 0,
        defaultBudgetMaxTokens: Number.isFinite(maxTokens) ? Math.max(0, Math.trunc(maxTokens)) : 0,
      }))
      flashSaved('Defaults saved')
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Settings</h1>
        {saved && <span className="badge badge-green">{saved}</span>}
      </div>

      <div className="grid grid-2">
        <div className="card">
          <h3>LLM Provider</h3>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div>
              <label style={{ fontSize: 12, color: '#666', display: 'block', marginBottom: 4 }}>Provider</label>
              <select className="input" value={cfg.provider || ''} onChange={e => updateProvider(e.target.value)}>
                <option value="">Select provider...</option>
                {providers.map(provider => (
                  <option key={provider.id} value={provider.id}>{provider.name}</option>
                ))}
              </select>
            </div>
            <div>
              <label style={{ fontSize: 12, color: '#666', display: 'block', marginBottom: 4 }}>Model</label>
              <select className="input" value={cfg.model || ''} onChange={e => setCfg({ ...cfg, model: e.target.value })}>
                <option value="">Select model...</option>
                {modelOptions.map(model => (
                  <option key={model} value={model}>{model}</option>
                ))}
              </select>
            </div>
            <div>
              <label style={{ fontSize: 12, color: '#666', display: 'block', marginBottom: 4 }}>
                API Key {selectedProvider?.envVar ? `(${selectedProvider.envVar})` : ''}
              </label>
              <input className="input" type="password" value={cfg.apiKey || ''} onChange={e => setCfg({ ...cfg, apiKey: e.target.value })} placeholder="sk-..." />
              <p style={{ fontSize: 11, color: '#555', marginTop: 4 }}>
                {cfg.apiKey
                  ? 'New key entered; saving will replace the stored key.'
                  : cfg.hasKey
                    ? 'A key is already stored locally. Leave this blank to keep it.'
                    : 'No key stored yet.'}
              </p>
            </div>
            <button className="btn btn-primary" onClick={saveProvider}>Save Provider</button>
          </div>
        </div>

        <div className="card">
          <h3>Default Agent Budget</h3>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div>
              <label style={{ fontSize: 12, color: '#666', display: 'block', marginBottom: 4 }}>Default max cost (USD)</label>
              <input className="input" type="number" step="0.01" value={defaultBudgetCost} onChange={e => setDefaultBudgetCost(e.target.value)} />
              <p style={{ fontSize: 11, color: '#555', marginTop: 4 }}>0 means unlimited. Applies to new agents and any agent still inheriting the default.</p>
            </div>
            <div>
              <label style={{ fontSize: 12, color: '#666', display: 'block', marginBottom: 4 }}>Default max tokens</label>
              <input className="input" type="number" step="1" value={defaultBudgetTokens} onChange={e => setDefaultBudgetTokens(e.target.value)} />
              <p style={{ fontSize: 11, color: '#555', marginTop: 4 }}>0 means unlimited.</p>
            </div>
            <button className="btn btn-primary" onClick={saveDefaults}>Save Defaults</button>
          </div>
        </div>

        <div className="card">
          <h3>Kernel</h3>
          <div style={{ fontSize: 13, color: '#aaa', lineHeight: 2 }}>
            <div>Binary: {cfg.binaryPath || 'auto-detect'}</div>
            <div>Auto-restart on crash: <span className="badge badge-green">Enabled</span> (max 3 restarts)</div>
            <div>Context pool: 10 pre-warmed, 20 max, recycle after 50 uses</div>
            <div>Memory limit per context: runtime default</div>
          </div>
        </div>

        <div className="card">
          <h3>About</h3>
          <div style={{ fontSize: 13, color: '#aaa', lineHeight: 2 }}>
            <div>VulpineOS — Agent Security Runtime</div>
            <div>Browser: Camoufox (Firefox 146.0.1)</div>
            <div>Protocol: Juggler + foxbridge CDP proxy</div>
            <div>Agent model setup: {cfg.setupComplete ? 'Configured' : 'Not configured'}</div>
            <div>OpenClaw profile: {status.openclaw_profile_configured ? 'Configured' : 'Not configured'}</div>
            <div>
              Route: {(status.browser_route || 'unknown').toUpperCase()}
              {status.browser_route_source ? ` (${status.browser_route_source})` : ''}
              {' · '}
              {kernelMode}
            </div>
            <div>Window: {(status.browser_window || 'unknown').toUpperCase()}</div>
            <div>Gateway: {status.gateway_running ? 'RUNNING' : 'STOPPED'}</div>
            <div>
              Sentinel: {status.sentinel_available ? (status.sentinel_mode || 'ON').toUpperCase() : 'OFF'}
              {status.sentinel_provider ? ` · ${status.sentinel_provider}` : ''}
            </div>
            <div>Variant bundles: {status.sentinel_variant_bundles || 0}</div>
            <div>Trust recipes: {status.sentinel_trust_recipes || 0}</div>
            <div>Maturity metrics: {status.sentinel_maturity_metrics || 0}</div>
            <div>Assignment rules: {status.sentinel_assignment_rules || 0}</div>
            <div>Variant names: {sentinelVariants.map(bundle => bundle.name).join(', ') || 'None'}</div>
            <div>Trust names: {sentinelTrustRecipes.map(recipe => recipe.name).join(', ') || 'None'}</div>
          </div>
        </div>

        <div className="card" style={{ gridColumn: '1 / -1' }}>
          <h3>Sentinel Trust Lab</h3>
          {!status.sentinel_available ? (
            <div className="empty-state">Sentinel is not available in this runtime.</div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
              <div className="detail-grid">
                <div className="detail-row"><span>Mode</span><strong>{(status.sentinel_mode || 'unknown').toUpperCase()}</strong></div>
                <div className="detail-row"><span>Provider</span><strong>{status.sentinel_provider || 'Unknown'}</strong></div>
                <div className="detail-row"><span>Variant source</span><strong>{status.sentinel_variant_source || 'Unknown'}</strong></div>
                <div className="detail-row"><span>Event sink</span><strong>{status.sentinel_event_sink || 'Unknown'}</strong></div>
                <div className="detail-row"><span>Outcome sink</span><strong>{status.sentinel_outcome_sink || 'Unknown'}</strong></div>
                <div className="detail-row"><span>Updated</span><strong>{status.sentinel_updated_at ? new Date(status.sentinel_updated_at).toLocaleString() : 'Unknown'}</strong></div>
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Variant bundles</h4>
                <table className="table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Browser</th>
                      <th>Fingerprint</th>
                      <th>Proxy</th>
                      <th>Transport</th>
                      <th>Behavior</th>
                      <th>Trust</th>
                      <th>Weight</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sentinelVariants.map(bundle => (
                      <tr key={bundle.id}>
                        <td>{bundle.name}</td>
                        <td>{bundle.browserVariant || 'baseline'}</td>
                        <td>{bundle.fingerprintVariant || 'baseline'}</td>
                        <td>{bundle.proxyVariant || 'baseline'}</td>
                        <td>{bundle.transportVariant || 'baseline'}</td>
                        <td>{bundle.behaviorVariant || 'baseline'}</td>
                        <td>{bundle.trustVariant || 'baseline'}</td>
                        <td>{bundle.weight || 0}%</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Trust recipes</h4>
                <table className="table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Strategy</th>
                      <th>Minimum age</th>
                      <th>Visits</th>
                      <th>Cadence</th>
                      <th>Notes</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sentinelTrustRecipes.map(recipe => (
                      <tr key={recipe.id}>
                        <td>{recipe.name}</td>
                        <td>{recipe.warmupStrategy || 'n/a'}</td>
                        <td>{recipe.minSessionAgeSeconds ? `${recipe.minSessionAgeSeconds}s` : 'n/a'}</td>
                        <td>{recipe.requiredVisits || 0}</td>
                        <td>{recipe.returnCadence || 'n/a'}</td>
                        <td>{recipe.notes || '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Session maturity metrics</h4>
                <table className="table">
                  <thead>
                    <tr>
                      <th>Metric</th>
                      <th>Thresholds</th>
                      <th>Description</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sentinelMetrics.map(metric => (
                      <tr key={metric.id}>
                        <td>{metric.name}</td>
                        <td>{formatMetricThresholds(metric) || 'None'}</td>
                        <td>{metric.description || '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Assignment rules</h4>
                <table className="table">
                  <thead>
                    <tr>
                      <th>Rule</th>
                      <th>Stage</th>
                      <th>Variant</th>
                      <th>Trust</th>
                      <th>Gates</th>
                      <th>Holdout</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sentinelRules.map(rule => (
                      <tr key={rule.id}>
                        <td>{rule.name}</td>
                        <td>{rule.stage || 'n/a'}</td>
                        <td>{rule.variantBundleId || 'n/a'}</td>
                        <td>{rule.trustRecipeId || 'n/a'}</td>
                        <td>{formatRuleGate(rule)}</td>
                        <td>{rule.holdoutPercent ? `${rule.holdoutPercent}%` : '0%'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Experiment board</h4>
                {sentinelExperimentSummary.length === 0 ? (
                  <div className="empty-state">No experiment outcomes have been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Variant</th>
                        <th>Trust</th>
                        <th>Sessions</th>
                        <th>Domains</th>
                        <th>Success</th>
                        <th>Soft</th>
                        <th>Hard</th>
                        <th>Block</th>
                        <th>Burn</th>
                        <th>Vendors</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelExperimentSummary.map((row, index) => (
                        <tr key={`${row.variantBundleId || 'variant'}-${row.trustRecipeId || 'trust'}-${index}`}>
                          <td>{variantNameFor(row.variantBundleId)}</td>
                          <td>{trustNameFor(row.trustRecipeId)}</td>
                          <td>{row.sessionCount || 0}</td>
                          <td>{row.domainCount || 0}</td>
                          <td>{row.successCount || 0}</td>
                          <td>{row.softChallengeCount || 0}</td>
                          <td>{row.hardChallengeCount || 0}</td>
                          <td>{row.blockCount || 0}</td>
                          <td>{row.burnCount || 0}</td>
                          <td>{(row.challengeVendors || []).join(', ') || 'none'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Variant compare board</h4>
                {sentinelVariantCompareSummary.length === 0 ? (
                  <div className="empty-state">No per-domain assignment comparisons have been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Variant</th>
                        <th>Trust</th>
                        <th>Sessions</th>
                        <th>Success</th>
                        <th>Degraded</th>
                        <th>Soft</th>
                        <th>Hard</th>
                        <th>Block</th>
                        <th>Burn</th>
                        <th>Pressure</th>
                        <th>Canary</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelVariantCompareSummary.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.variantBundleId || 'variant'}-${row.trustRecipeId || 'trust'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{variantNameFor(row.variantBundleId)}</td>
                          <td>{trustNameFor(row.trustRecipeId)}</td>
                          <td>{row.sessionCount || 0}</td>
                          <td>{row.successCount || 0}</td>
                          <td>{row.degradedCount || 0}</td>
                          <td>{row.softCount || 0}</td>
                          <td>{row.hardCount || 0}</td>
                          <td>{row.blockCount || 0}</td>
                          <td>{row.burnCount || 0}</td>
                          <td>{row.pressureScore || 0}</td>
                          <td>{`${(row.canaryStatus || 'unknown').toUpperCase()}${row.latestOutcome ? ` · ${row.latestOutcome.toUpperCase()}` : ''}`}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Site intelligence board</h4>
                {sentinelSiteIntelligenceSummary.length === 0 ? (
                  <div className="empty-state">No site intelligence has been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Top script</th>
                        <th>Top probe</th>
                        <th>Vendor</th>
                        <th>Pressure</th>
                        <th>Recommendations</th>
                        <th>Canary</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelSiteIntelligenceSummary.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td className="mono-cell">{row.topScriptUrl || 'n/a'}</td>
                          <td>{row.topProbeFamily || 'n/a'}</td>
                          <td>{row.dominantChallengeVendor || 'none'}</td>
                          <td>{row.pressureScore || 0}</td>
                          <td>{row.activeRecommendationCount || 0}</td>
                          <td>{(row.latestCanaryStatus || 'unknown').toUpperCase()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Probe sequence board</h4>
                {sentinelProbeSequenceSummary.length === 0 ? (
                  <div className="empty-state">No probe sequences have been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Script</th>
                        <th>Sequence</th>
                        <th>Count</th>
                        <th>Outcome</th>
                        <th>Vendor</th>
                        <th>Canary</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelProbeSequenceSummary.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.scriptUrl || 'script'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td className="mono-cell">{row.scriptUrl || 'n/a'}</td>
                          <td className="mono-cell">{row.sequence || 'n/a'}</td>
                          <td>{row.sequenceCount || 0}</td>
                          <td>{row.latestOutcome || 'n/a'}</td>
                          <td>{row.latestChallengeVendor || 'none'}</td>
                          <td>{(row.latestCanaryStatus || 'unknown').toUpperCase()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Vendor intelligence board</h4>
                {sentinelVendorIntelligenceSummary.length === 0 ? (
                  <div className="empty-state">No vendor clusters have been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Family</th>
                        <th>Script host</th>
                        <th>Vendor</th>
                        <th>Domains</th>
                        <th>Samples</th>
                        <th>Top probe</th>
                        <th>Pressure</th>
                        <th>Outcome</th>
                        <th>Canary</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelVendorIntelligenceSummary.map((row, index) => (
                        <tr key={`${row.scriptHost || 'host'}-${row.challengeVendor || 'vendor'}-${index}`}>
                          <td>{row.vendorFamily || 'unknown'}</td>
                          <td className="mono-cell">{row.scriptHost || 'unknown'}</td>
                          <td>{row.challengeVendor || 'unknown'}</td>
                          <td>{row.domainCount || 0}</td>
                          <td>{(row.domainSamples || []).join(', ') || 'n/a'}</td>
                          <td>{row.topProbeFamily || 'n/a'}</td>
                          <td>{row.pressureScore || 0}</td>
                          <td>{row.latestOutcome || 'n/a'}</td>
                          <td>{(row.latestCanaryStatus || 'unknown').toUpperCase()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Vendor effectiveness board</h4>
                {sentinelVendorEffectiveness.length === 0 ? (
                  <div className="empty-state">No vendor effectiveness has been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Family</th>
                        <th>Variant</th>
                        <th>Trust</th>
                        <th>Domains</th>
                        <th>Success</th>
                        <th>Soft</th>
                        <th>Hard</th>
                        <th>Block</th>
                        <th>Burn</th>
                        <th>Score</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelVendorEffectiveness.map((row, index) => (
                        <tr key={`${row.vendorFamily || 'family'}-${row.variantBundleId || 'variant'}-${index}`}>
                          <td>{row.vendorFamily || 'unknown'}</td>
                          <td>{variantNameFor(row.variantBundleId)}</td>
                          <td>{trustNameFor(row.trustRecipeId)}</td>
                          <td>{row.domainCount || 0}</td>
                          <td>{row.successCount || 0}</td>
                          <td>{row.softChallengeCount || 0}</td>
                          <td>{row.hardChallengeCount || 0}</td>
                          <td>{row.blockCount || 0}</td>
                          <td>{row.burnCount || 0}</td>
                          <td>{row.effectivenessScore || 0}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Vendor uplift board</h4>
                {sentinelVendorUplift.length === 0 ? (
                  <div className="empty-state">No vendor uplift has been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Family</th>
                        <th>Variant</th>
                        <th>Trust</th>
                        <th>Control</th>
                        <th>Success Δ</th>
                        <th>Challenge Δ</th>
                        <th>Score Δ</th>
                        <th>Recommendation</th>
                        <th>Confidence</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelVendorUplift.map((row, index) => (
                        <tr key={`${row.vendorFamily || 'family'}-${row.variantBundleId || 'variant'}-${row.trustRecipeId || 'trust'}-${index}`}>
                          <td>{row.vendorFamily || 'unknown'}</td>
                          <td>{variantNameFor(row.variantBundleId)}</td>
                          <td>{trustNameFor(row.trustRecipeId)}</td>
                          <td>{row.baselineAvailable ? `${variantNameFor(row.controlVariantBundleId)} / ${trustNameFor(row.controlTrustRecipeId)}` : 'n/a'}</td>
                          <td>{row.successRateDeltaPct || 0}%</td>
                          <td>{row.challengeRateDeltaPct || 0}%</td>
                          <td>{row.scoreDelta || 0}</td>
                          <td>{(row.recommendation || 'unknown').toUpperCase()}</td>
                          <td>{(row.confidence || 'unknown').toUpperCase()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Vendor rollout board</h4>
                {sentinelVendorRollout.length === 0 ? (
                  <div className="empty-state">No vendor rollout decisions have been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Family</th>
                        <th>Lead</th>
                        <th>Control</th>
                        <th>Arms</th>
                        <th>Score Δ</th>
                        <th>Success Δ</th>
                        <th>Challenge Δ</th>
                        <th>Decision</th>
                        <th>Confidence</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelVendorRollout.map((row, index) => (
                        <tr key={`${row.vendorFamily || 'family'}-${row.leadingVariantBundleId || 'lead'}-${index}`}>
                          <td>{row.vendorFamily || 'unknown'}</td>
                          <td>{`${variantNameFor(row.leadingVariantBundleId)} / ${trustNameFor(row.leadingTrustRecipeId)}`}</td>
                          <td>{row.baselineAvailable ? `${variantNameFor(row.controlVariantBundleId)} / ${trustNameFor(row.controlTrustRecipeId)}` : 'n/a'}</td>
                          <td>{row.armCount || 0}</td>
                          <td>{row.scoreDelta || 0}</td>
                          <td>{row.successRateDeltaPct || 0}%</td>
                          <td>{row.challengeRateDeltaPct || 0}%</td>
                          <td>{(row.recommendation || 'unknown').toUpperCase()}</td>
                          <td>{(row.confidence || 'unknown').toUpperCase()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Site pressure board</h4>
                {sentinelSitePressure.length === 0 ? (
                  <div className="empty-state">No site pressure has been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Vendor</th>
                        <th>Probes</th>
                        <th>Sessions</th>
                        <th>Success</th>
                        <th>Soft</th>
                        <th>Hard</th>
                        <th>Block</th>
                        <th>Burn</th>
                        <th>Score</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelSitePressure.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.challengeVendor || 'vendor'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{row.challengeVendor || 'none'}</td>
                          <td>{row.probeCount || 0}</td>
                          <td>{row.sessionCount || 0}</td>
                          <td>{row.successCount || 0}</td>
                          <td>{row.softChallengeCount || 0}</td>
                          <td>{row.hardChallengeCount || 0}</td>
                          <td>{row.blockCount || 0}</td>
                          <td>{row.burnCount || 0}</td>
                          <td>{row.pressureScore || 0}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Trust activity board</h4>
                {sentinelTrustActivity.length === 0 ? (
                  <div className="empty-state">No trust activity has been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>State</th>
                        <th>Events</th>
                        <th>Sessions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelTrustActivity.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.state || 'state'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{row.state || 'unknown'}</td>
                          <td>{row.count || 0}</td>
                          <td>{row.sessionCount || 0}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Trust effectiveness</h4>
                {sentinelTrustEffectiveness.length === 0 ? (
                  <div className="empty-state">No trust effectiveness has been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Variant</th>
                        <th>Trust</th>
                        <th>Warm</th>
                        <th>Success</th>
                        <th>Soft</th>
                        <th>Hard</th>
                        <th>Block</th>
                        <th>Burn</th>
                        <th>Score</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelTrustEffectiveness.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.variantBundleId || 'variant'}-${row.trustRecipeId || 'trust'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{variantNameFor(row.variantBundleId)}</td>
                          <td>{trustNameFor(row.trustRecipeId)}</td>
                          <td>{row.warmingCount || 0}</td>
                          <td>{row.successCount || 0}</td>
                          <td>{row.softChallengeCount || 0}</td>
                          <td>{row.hardChallengeCount || 0}</td>
                          <td>{row.blockCount || 0}</td>
                          <td>{row.burnCount || 0}</td>
                          <td>{row.effectivenessScore || 0}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Trust assets</h4>
                {sentinelTrustAssets.length === 0 ? (
                  <div className="empty-state">No trust-asset maturity has been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Variant</th>
                        <th>Trust</th>
                        <th>Snaps</th>
                        <th>Cookie</th>
                        <th>Storage</th>
                        <th>Avg cookies</th>
                        <th>Avg storage</th>
                        <th>Seen</th>
                        <th>Sessions</th>
                        <th>Soft</th>
                        <th>Hard</th>
                        <th>Block</th>
                        <th>Score</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelTrustAssets.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.variantBundleId || 'variant'}-${row.trustRecipeId || 'trust'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{variantNameFor(row.variantBundleId)}</td>
                          <td>{trustNameFor(row.trustRecipeId)}</td>
                          <td>{row.snapshotCount || 0}</td>
                          <td>{row.cookieBackedCount || 0}</td>
                          <td>{row.storageBackedCount || 0}</td>
                          <td>{row.averageCookieCount ? row.averageCookieCount.toFixed(1) : '0.0'}</td>
                          <td>{row.averageStorageEntryCount ? row.averageStorageEntryCount.toFixed(1) : '0.0'}</td>
                          <td>{row.averageHoursSinceLastSeen ? `${row.averageHoursSinceLastSeen.toFixed(1)}h` : '0.0h'}</td>
                          <td>{row.averageTotalSessionsSeen ? row.averageTotalSessionsSeen.toFixed(1) : '0.0'}</td>
                          <td>{row.softChallengeCount || 0}</td>
                          <td>{row.hardChallengeCount || 0}</td>
                          <td>{row.blockCount || 0}</td>
                          <td>{row.assetScore || 0}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Maturity evidence</h4>
                {sentinelMaturityEvidence.length === 0 ? (
                  <div className="empty-state">No maturity evidence has been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Variant</th>
                        <th>Trust</th>
                        <th>Warm</th>
                        <th>Revisits</th>
                        <th>Days</th>
                        <th>Avg gap</th>
                        <th>Success</th>
                        <th>Soft</th>
                        <th>Hard</th>
                        <th>Block</th>
                        <th>Score</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelMaturityEvidence.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.variantBundleId || 'variant'}-${row.trustRecipeId || 'trust'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{variantNameFor(row.variantBundleId)}</td>
                          <td>{trustNameFor(row.trustRecipeId)}</td>
                          <td>{row.warmingCount || 0}</td>
                          <td>{row.revisitCount || 0}</td>
                          <td>{row.distinctDays || 0}</td>
                          <td>{row.averageGapHours ? `${row.averageGapHours.toFixed(1)}h` : '0.0h'}</td>
                          <td>{row.successCount || 0}</td>
                          <td>{row.softChallengeCount || 0}</td>
                          <td>{row.hardChallengeCount || 0}</td>
                          <td>{row.blockCount || 0}</td>
                          <td>{row.maturityScore || 0}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Stage board</h4>
                {sentinelStageSummary.length === 0 ? (
                  <div className="empty-state">No maturity stage summaries have been derived yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Variant</th>
                        <th>Trust</th>
                        <th>Current</th>
                        <th>Rule</th>
                        <th>Aligned</th>
                        <th>Blocker</th>
                        <th>Visits</th>
                        <th>Days</th>
                        <th>Quiet</th>
                        <th>Age</th>
                        <th>Hard</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelStageSummary.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.variantBundleId || 'variant'}-${row.trustRecipeId || 'trust'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{variantNameFor(row.variantBundleId)}</td>
                          <td>{trustNameFor(row.trustRecipeId)}</td>
                          <td>{(row.currentStage || 'cold').toUpperCase()}</td>
                          <td>{row.ruleName ? `${row.ruleName} (${row.ruleStage || 'n/a'})` : (row.ruleStage || 'none')}</td>
                          <td>{row.ruleAligned ? 'YES' : 'NO'}</td>
                          <td>{row.blockingReason || 'none'}</td>
                          <td>{row.successCount || 0}</td>
                          <td>{row.distinctDays || 0}</td>
                          <td>{row.challengeFreeRuns || 0}</td>
                          <td>{row.sessionAgeSeconds ? `${row.sessionAgeSeconds}s` : '0s'}</td>
                          <td>{row.hardChallengeCount || 0}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Assignment recommendations</h4>
                {sentinelAssignmentRecommendations.length === 0 ? (
                  <div className="empty-state">No assignment recommendations have been derived yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Current</th>
                        <th>Stage</th>
                        <th>Action</th>
                        <th>Target</th>
                        <th>Reason</th>
                        <th>Priority</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelAssignmentRecommendations.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.variantBundleId || 'variant'}-${row.trustRecipeId || 'trust'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{`${variantNameFor(row.variantBundleId)} / ${trustNameFor(row.trustRecipeId)}`}</td>
                          <td>{(row.currentStage || 'cold').toUpperCase()}</td>
                          <td>{(row.action || 'hold').toUpperCase()}</td>
                          <td>{row.targetVariantBundleId || row.targetTrustRecipeId ? `${variantNameFor(row.targetVariantBundleId)} / ${trustNameFor(row.targetTrustRecipeId)}` : 'n/a'}</td>
                          <td>{row.reason || 'none'}</td>
                          <td>{(row.priority || 'low').toUpperCase()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Canary board</h4>
                {sentinelCanarySummary.length === 0 ? (
                  <div className="empty-state">No canary regressions have been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Current</th>
                        <th>Sessions</th>
                        <th>Latest</th>
                        <th>Quiet streak</th>
                        <th>Delta</th>
                        <th>Recommendation</th>
                        <th>Status</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelCanarySummary.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.variantBundleId || 'variant'}-${row.trustRecipeId || 'trust'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{`${variantNameFor(row.variantBundleId)} / ${trustNameFor(row.trustRecipeId)}`}</td>
                          <td>{row.canarySessionCount || 0}</td>
                          <td>{(row.latestOutcome || 'none').toUpperCase()}</td>
                          <td>{row.challengeFreeStreak || 0}</td>
                          <td>{row.regressionDelta > 0 ? `+${row.regressionDelta}` : `${row.regressionDelta || 0}`}</td>
                          <td>{(row.latestRecommendationAction || 'hold').toUpperCase()}</td>
                          <td>{row.regressed ? 'REGRESSED' : 'STABLE'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Transport evidence</h4>
                {sentinelTransportEvidence.length === 0 ? (
                  <div className="empty-state">No transport evidence has been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Variant</th>
                        <th>Trust</th>
                        <th>Rotations</th>
                        <th>Reasons</th>
                        <th>Routes</th>
                        <th>Soft</th>
                        <th>Hard</th>
                        <th>Block</th>
                        <th>Score</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelTransportEvidence.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.variantBundleId || 'variant'}-${row.trustRecipeId || 'trust'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{variantNameFor(row.variantBundleId)}</td>
                          <td>{trustNameFor(row.trustRecipeId)}</td>
                          <td>{row.rotationCount || 0}</td>
                          <td>{(row.reasons || []).join(', ') || 'n/a'}</td>
                          <td className="mono-cell">{(row.proxyEndpoints || []).join(', ') || 'n/a'}</td>
                          <td>{row.softChallengeCount || 0}</td>
                          <td>{row.hardChallengeCount || 0}</td>
                          <td>{row.blockCount || 0}</td>
                          <td>{row.transportScore || 0}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Coherence diff</h4>
                {sentinelCoherenceDiff.length === 0 ? (
                  <div className="empty-state">No identity or route invariants have been flagged yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Variant</th>
                        <th>Trust</th>
                        <th>Findings</th>
                        <th>Sessions</th>
                        <th>Soft</th>
                        <th>Hard</th>
                        <th>Block</th>
                        <th>Severity</th>
                        <th>Score</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelCoherenceDiff.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.variantBundleId || 'variant'}-${row.trustRecipeId || 'trust'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{variantNameFor(row.variantBundleId)}</td>
                          <td>{trustNameFor(row.trustRecipeId)}</td>
                          <td>{(row.findings || []).join(' | ') || 'none'}</td>
                          <td>{row.sessionCount || 0}</td>
                          <td>{row.softChallengeCount || 0}</td>
                          <td>{row.hardChallengeCount || 0}</td>
                          <td>{row.blockCount || 0}</td>
                          <td>{(row.severity || 'low').toUpperCase()}</td>
                          <td>{row.score || 0}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Recent capture timeline</h4>
                {sentinelTimeline.length === 0 ? (
                  <div className="empty-state">No Sentinel evidence has been captured yet.</div>
                ) : (
                  <div className="runtime-list">
                    {sentinelTimeline.map(session => (
                      <div key={session.sessionId} className="runtime-item">
                        <div className="runtime-copy" style={{ gap: 8 }}>
                          <strong>
                            {session.domain || session.sessionId || 'Session'} · {session.eventCount || 0} events · {session.outcomeCount || 0} outcomes
                          </strong>
                          <span>{session.url || session.agentId || session.sessionId}</span>
                          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                            {(session.items || []).map((item, index) => (
                              <span key={`${session.sessionId}-${index}`} className="mono-cell" style={{ whiteSpace: 'normal' }}>
                                {formatTimelineItem(item)}
                              </span>
                            ))}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Probe summary</h4>
                {sentinelProbeSummary.length === 0 ? (
                  <div className="empty-state">No browser probe evidence has been summarized yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Probe</th>
                        <th>API</th>
                        <th>Script</th>
                        <th>Count</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelProbeSummary.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.api || 'api'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{row.probeType || 'unknown'}</td>
                          <td>{row.api || 'unknown'}</td>
                          <td className="mono-cell">{row.scriptUrl || row.lastUrl || 'inline/unknown'}</td>
                          <td>{row.count || 0}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Patch queue</h4>
                {sentinelPatchQueue.length === 0 ? (
                  <div className="empty-state">No ranked patch candidates yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Domain</th>
                        <th>Probe</th>
                        <th>API</th>
                        <th>Score</th>
                        <th>Priority</th>
                        <th>Outcomes</th>
                        <th>Recommendation</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelPatchQueue.map((row, index) => (
                        <tr key={`${row.domain || 'domain'}-${row.api || 'api'}-${index}`}>
                          <td>{row.domain || 'unknown'}</td>
                          <td>{row.probeType || 'unknown'}</td>
                          <td>{row.api || 'unknown'}</td>
                          <td>{row.score || 0}</td>
                          <td>{(row.priority || 'low').toUpperCase()}</td>
                          <td>{(row.outcomes || []).join(', ') || 'none'}</td>
                          <td>{row.recommendation || 'Review the captured evidence.'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Outcome taxonomy</h4>
                <table className="table">
                  <thead>
                    <tr>
                      <th>Outcome</th>
                      <th>Category</th>
                      <th>Severity</th>
                      <th>Description</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sentinelOutcomeLabels.map(label => (
                      <tr key={label.id}>
                        <td>{label.name}</td>
                        <td>{label.category || 'n/a'}</td>
                        <td>{label.severity || 'n/a'}</td>
                        <td>{label.description || '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              <div>
                <h4 style={{ margin: '0 0 10px' }}>Captured outcomes</h4>
                {sentinelOutcomeSummary.length === 0 ? (
                  <div className="empty-state">No outcome labels have been recorded yet.</div>
                ) : (
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Outcome</th>
                        <th>Count</th>
                        <th>Vendors</th>
                        <th>Last seen</th>
                      </tr>
                    </thead>
                    <tbody>
                      {sentinelOutcomeSummary.map(row => (
                        <tr key={row.outcome}>
                          <td>{row.outcome}</td>
                          <td>{row.count || 0}</td>
                          <td>{(row.vendors || []).join(', ') || '—'}</td>
                          <td>{row.lastSeenAt ? new Date(row.lastSeenAt).toLocaleString() : 'Unknown'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
