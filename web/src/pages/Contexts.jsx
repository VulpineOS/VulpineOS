import React, { useState, useEffect } from 'react'

export default function Contexts({ ws }) {
  const [contexts, setContexts] = useState([])
  const [selectedContext, setSelectedContext] = useState(localStorage.getItem('vulpine_context_id') || '')

  useEffect(() => {
    const active = new Map()
    ;(ws.events || []).forEach(ev => {
      if (ev.method === 'Browser.attachedToTarget') {
        const contextId = ev.params?.targetInfo?.browserContextId
        if (!contextId) return
        const current = active.get(contextId) || { id: contextId, pages: 0, url: 'about:blank' }
        current.pages += 1
        current.url = ev.params?.targetInfo?.url || current.url
        active.set(contextId, current)
      }
      if (ev.method === 'Browser.detachedFromTarget') {
        // detach events do not include browserContextId, so keep the latest known snapshot
      }
    })
    const known = JSON.parse(localStorage.getItem('vulpine_known_contexts') || '[]')
    known.forEach(id => {
      if (!active.has(id)) active.set(id, { id, pages: 0, url: 'about:blank' })
    })
    setContexts(Array.from(active.values()))
  }, [ws.connected, ws.events])

  const createContext = async () => {
    try {
      const r = await ws.juggler('Browser.createBrowserContext', { removeOnDetach: true })
      if (r?.result?.browserContextId) {
        const id = r.result.browserContextId
        const next = Array.from(new Set([...contexts.map(c => c.id), id]))
        localStorage.setItem('vulpine_known_contexts', JSON.stringify(next))
        localStorage.setItem('vulpine_context_id', id)
        setSelectedContext(id)
        setContexts(prev => [...prev, { id, pages: 0, url: 'about:blank' }])
      }
    } catch (e) {
      alert('Failed: ' + e.message)
    }
  }

  const removeContext = async (id) => {
    try {
      await ws.juggler('Browser.removeBrowserContext', { browserContextId: id })
      const next = contexts.filter(c => c.id !== id).map(c => c.id)
      localStorage.setItem('vulpine_known_contexts', JSON.stringify(next))
      if (localStorage.getItem('vulpine_context_id') === id) {
        localStorage.removeItem('vulpine_context_id')
        setSelectedContext('')
      }
      setContexts(prev => prev.filter(c => c.id !== id))
    } catch (e) {
      alert('Failed: ' + e.message)
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Browser Contexts</h1>
        <button className="btn btn-primary" onClick={createContext}>New Context</button>
      </div>

      <div className="card">
        <h3>Active Contexts</h3>
        <table className="table">
          <thead>
            <tr>
              <th>Context ID</th>
              <th>Pages</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {contexts.length === 0 && (
              <tr><td colSpan="3" style={{ textAlign: 'center', color: '#666', padding: 32 }}>
                No active browser contexts. The shared context is always available.
              </td></tr>
            )}
            {contexts.map(c => (
              <tr key={c.id}>
                <td style={{ fontFamily: 'monospace' }}>{c.id.substring(0, 16)}...{selectedContext === c.id ? ' selected' : ''}</td>
                <td>{c.pages}</td>
                <td>
                  <button className="btn btn-ghost btn-sm" onClick={() => {
                    localStorage.setItem('vulpine_context_id', c.id)
                    setSelectedContext(c.id)
                  }} style={{ marginRight: 4 }}>Select</button>
                  <button className="btn btn-ghost btn-sm" onClick={() => removeContext(c.id)}>Remove</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
