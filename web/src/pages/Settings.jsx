import React, { useEffect, useState } from 'react'

export default function Settings({ ws }) {
  const [cfg, setCfg] = useState({})
  const [providers, setProviders] = useState([])
  const [status, setStatus] = useState({})
  const [sentinel, setSentinel] = useState({ variantBundles: [], trustRecipes: [], maturityMetrics: [], assignmentRules: [], outcomeLabels: [], outcomeSummary: [], probeSummary: [], patchQueue: [], experimentSummary: [] })
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
    ws.call('sentinel.get').then(r => setSentinel(r || { variantBundles: [], trustRecipes: [], maturityMetrics: [], assignmentRules: [], outcomeLabels: [], outcomeSummary: [], probeSummary: [], patchQueue: [], experimentSummary: [] })).catch(() => {})
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
    if (item.type === 'outcome') {
      return `${item.outcome || 'outcome'}${item.challengeVendor ? ` · ${item.challengeVendor}` : ''}`
    }
    const parts = [`${item.kind || 'event'} · ${item.name || 'unnamed'}`]
    if (item.attributes?.detail) parts.push(item.attributes.detail)
    if (item.attributes?.count) parts.push(`x${item.attributes.count}`)
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
