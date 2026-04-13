import React, { useState, useEffect } from 'react'

export default function Contexts({ ws }) {
  const [contexts, setContexts] = useState([])
  const [selectedContext, setSelectedContext] = useState(localStorage.getItem('vulpine_context_id') || '')

  const refresh = () => {
    ws.call('contexts.list').then(r => setContexts(r?.contexts || [])).catch(() => {})
  }

  useEffect(() => {
    if (ws.connected) refresh()
  }, [ws.connected])

  useEffect(() => {
    if (!ws.connected) return
    const latest = ws.events?.[ws.events.length - 1]
    if (!latest) return
    if (latest.method === 'Browser.attachedToTarget' || latest.method === 'Browser.detachedFromTarget') {
      refresh()
    }
  }, [ws.connected, ws.events])

  const createContext = async () => {
    try {
      const r = await ws.call('contexts.create', { removeOnDetach: true })
      if (r?.browserContextId) {
        const id = r.browserContextId
        localStorage.setItem('vulpine_context_id', id)
        setSelectedContext(id)
        refresh()
      }
    } catch (e) {
      alert('Failed: ' + e.message)
    }
  }

  const removeContext = async (id) => {
    try {
      await ws.call('contexts.remove', { browserContextId: id })
      if (localStorage.getItem('vulpine_context_id') === id) {
        localStorage.removeItem('vulpine_context_id')
        setSelectedContext('')
      }
      refresh()
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
