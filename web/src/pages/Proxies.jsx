import React, { useEffect, useState } from 'react'

const defaultRotation = {
  enabled: false,
  rotateOnRateLimit: false,
  rotateOnBlock: false,
  rotateIntervalSeconds: 0,
  syncFingerprint: true,
  proxyPool: [],
  currentIndex: 0,
}

export default function Proxies({ ws }) {
  const [proxies, setProxies] = useState([])
  const [agents, setAgents] = useState([])
  const [selectedAgent, setSelectedAgent] = useState('')
  const [rotation, setRotation] = useState(defaultRotation)
  const [rotationSource, setRotationSource] = useState('default')
  const [importText, setImportText] = useState('')
  const [showImport, setShowImport] = useState(false)

  const refreshProxies = async () => {
    try {
      const result = await ws.call('proxies.list')
      setProxies(result?.proxies || [])
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  const refreshAgents = async () => {
    try {
      const result = await ws.call('agents.list')
      const nextAgents = result?.agents || []
      setAgents(nextAgents)
      if (!selectedAgent && nextAgents.length > 0) {
        setSelectedAgent(nextAgents[0].id)
      }
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  const loadRotation = async (agentId) => {
    if (!agentId) {
      setRotation(defaultRotation)
      setRotationSource('default')
      return
    }
    try {
      const result = await ws.call('proxies.getRotation', { agentId })
      setRotation({ ...defaultRotation, ...(result?.config || {}) })
      setRotationSource(result?.source || 'default')
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  useEffect(() => {
    if (!ws.connected) return
    refreshProxies()
    refreshAgents()
  }, [ws.connected])

  useEffect(() => {
    if (ws.connected && selectedAgent) loadRotation(selectedAgent)
  }, [ws.connected, selectedAgent])

  const importProxies = async () => {
    const urls = importText.split('\n').filter(l => l.trim())
    for (const url of urls) {
      try {
        await ws.call('proxies.add', { url: url.trim() })
      } catch {}
    }
    setImportText('')
    setShowImport(false)
    refreshProxies()
  }

  const testProxy = async (id) => {
    try {
      const r = await ws.call('proxies.test', { proxyId: id })
      ws.notify?.(`Latency: ${r?.latencyMs || '?'}ms`, 'success')
      refreshProxies()
    } catch (e) {
      ws.notify?.(`Test failed: ${e.message}`)
    }
  }

  const deleteProxy = async (id) => {
    try {
      await ws.call('proxies.delete', { proxyId: id })
      refreshProxies()
      setRotation(current => {
        const nextPool = current.proxyPool.filter(url => proxies.find(proxy => proxy.id === id)?.url !== url)
        return {
          ...current,
          proxyPool: nextPool,
          currentIndex: nextPool.length === 0 ? 0 : Math.min(current.currentIndex, nextPool.length - 1),
        }
      })
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  const togglePoolMember = (url) => {
    setRotation(current => {
      const exists = current.proxyPool.includes(url)
      const nextPool = exists ? current.proxyPool.filter(entry => entry !== url) : [...current.proxyPool, url]
      return {
        ...current,
        proxyPool: nextPool,
        currentIndex: nextPool.length === 0 ? 0 : Math.min(current.currentIndex, nextPool.length - 1),
      }
    })
  }

  const saveRotation = async () => {
    if (!selectedAgent) return
    try {
      const payload = {
        ...rotation,
        rotateIntervalSeconds: Number(rotation.rotateIntervalSeconds) || 0,
        currentIndex: Number(rotation.currentIndex) || 0,
      }
      const result = await ws.call('proxies.setRotation', { agentId: selectedAgent, config: payload })
      setRotation({ ...defaultRotation, ...(result?.config || payload) })
      setRotationSource('agent')
      ws.notify?.('Rotation saved', 'success')
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Proxies ({proxies.length})</h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button className="btn btn-ghost" onClick={() => setShowImport(!showImport)}>{showImport ? 'Cancel' : 'Import'}</button>
          <button className="btn btn-ghost" onClick={() => { refreshProxies(); refreshAgents() }}>Refresh</button>
        </div>
      </div>

      {showImport && (
        <div className="card">
          <h3>Import Proxies</h3>
          <textarea
            className="input"
            rows={5}
            value={importText}
            onChange={e => setImportText(e.target.value)}
            placeholder="http://user:pass@proxy1:8080&#10;socks5://proxy2:1080"
            style={{ resize: 'vertical', fontFamily: 'monospace', fontSize: 12 }}
          />
          <button className="btn btn-primary" style={{ marginTop: 12 }} onClick={importProxies}>
            Import {importText.split('\n').filter(l => l.trim()).length} Proxies
          </button>
        </div>
      )}

      <div className="grid grid-2">
        <div className="card">
          <div className="page-header" style={{ marginBottom: 12 }}>
            <h3 style={{ margin: 0 }}>Rotation</h3>
            <span className="badge badge-gray">source: {rotationSource}</span>
          </div>
          {agents.length === 0 ? (
            <p style={{ color: '#666' }}>Create an agent to configure rotation.</p>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div>
                <label style={{ fontSize: 12, color: '#666', display: 'block', marginBottom: 4 }}>Agent</label>
                <select className="input" value={selectedAgent} onChange={e => setSelectedAgent(e.target.value)}>
                  {agents.map(agent => (
                    <option key={agent.id} value={agent.id}>{agent.name || agent.id}</option>
                  ))}
                </select>
              </div>

              <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: '#aaa' }}>
                <input type="checkbox" checked={rotation.enabled} onChange={e => setRotation(current => ({ ...current, enabled: e.target.checked }))} />
                Enable rotation
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: '#aaa' }}>
                <input type="checkbox" checked={rotation.rotateOnRateLimit} onChange={e => setRotation(current => ({ ...current, rotateOnRateLimit: e.target.checked }))} />
                Rotate on rate limit
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: '#aaa' }}>
                <input type="checkbox" checked={rotation.rotateOnBlock} onChange={e => setRotation(current => ({ ...current, rotateOnBlock: e.target.checked }))} />
                Rotate on block
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: '#aaa' }}>
                <input type="checkbox" checked={rotation.syncFingerprint} onChange={e => setRotation(current => ({ ...current, syncFingerprint: e.target.checked }))} />
                Sync fingerprint on rotate
              </label>

              <div>
                <label style={{ fontSize: 12, color: '#666', display: 'block', marginBottom: 4 }}>Rotate interval (seconds)</label>
                <input
                  className="input"
                  type="number"
                  min="0"
                  value={rotation.rotateIntervalSeconds}
                  onChange={e => setRotation(current => ({ ...current, rotateIntervalSeconds: e.target.value }))}
                />
              </div>

              <div>
                <label style={{ fontSize: 12, color: '#666', display: 'block', marginBottom: 8 }}>Proxy pool</label>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8, maxHeight: 220, overflowY: 'auto' }}>
                  {proxies.length === 0 && <span style={{ color: '#666', fontSize: 12 }}>Import proxies first.</span>}
                  {proxies.map(proxy => (
                    <label key={proxy.id} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: '#aaa' }}>
                      <input
                        type="checkbox"
                        checked={rotation.proxyPool.includes(proxy.url)}
                        onChange={() => togglePoolMember(proxy.url)}
                      />
                      <span style={{ fontFamily: 'monospace' }}>{proxy.url}</span>
                    </label>
                  ))}
                </div>
              </div>

              <button className="btn btn-primary" onClick={saveRotation}>Save Rotation</button>
            </div>
          )}
        </div>

        <div className="card">
          <table className="table">
            <thead><tr><th>URL</th><th>Status</th><th>Latency</th><th>Country</th><th>Actions</th></tr></thead>
            <tbody>
              {proxies.length === 0 && <tr><td colSpan="5" style={{ textAlign: 'center', color: '#666', padding: 32 }}>No proxies.</td></tr>}
              {proxies.map(p => (
                <tr key={p.id || p.url}>
                  <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{(p.url || '').substring(0, 45)}</td>
                  <td><span className={`badge ${p.latencyMs > 0 ? 'badge-green' : 'badge-gray'}`}>{p.latencyMs > 0 ? 'tested' : 'untested'}</span></td>
                  <td>{p.latencyMs > 0 ? `${p.latencyMs}ms` : '—'}</td>
                  <td>{p.country || '—'}</td>
                  <td>
                    <button className="btn btn-ghost btn-sm" onClick={() => testProxy(p.id)} style={{ marginRight: 4 }}>Test</button>
                    <button className="btn btn-ghost btn-sm" onClick={() => deleteProxy(p.id)}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
