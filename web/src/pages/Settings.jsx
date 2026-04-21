import React, { useEffect, useState } from 'react'

export default function Settings({ ws }) {
  const [cfg, setCfg] = useState({})
  const [providers, setProviders] = useState([])
  const [status, setStatus] = useState({})
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
  }, [ws.connected])

  const selectedProvider = providers.find(p => p.id === cfg.provider) || null
  const modelOptions = selectedProvider?.models?.length ? selectedProvider.models : (cfg.model ? [cfg.model] : [])
  const kernelMode = status.kernel_running ? (status.kernel_headless ? 'HEADLESS' : 'GUI') : 'DISABLED'

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
          </div>
        </div>
      </div>
    </div>
  )
}
