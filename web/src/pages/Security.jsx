import React, { useEffect, useState } from 'react'

export default function Security({ ws }) {
  const [status, setStatus] = useState({ protections: [], sandboxBlockedAPIs: [], signaturePatternCount: 0 })
  const injectionEvents = ws.events.filter(e => e.method === 'Browser.injectionAttemptDetected').slice(-20).reverse()

  useEffect(() => {
    if (!ws.connected) return
    ws.call('security.status').then(result => setStatus(result || {})).catch(() => {})
  }, [ws.connected])

  const badgeClass = (protectionStatus) => {
    switch (protectionStatus) {
      case 'active':
        return 'badge-green'
      case 'available':
        return 'badge-blue'
      default:
        return 'badge-gray'
    }
  }

  return (
    <div>
      <div className="page-header"><h1>Security</h1></div>

      <div className="grid grid-2">
        <div className="card">
          <div className="page-header" style={{ marginBottom: 12 }}>
            <h3 style={{ margin: 0 }}>Protection Status</h3>
            <span className={`badge ${status.securityEnabled ? 'badge-green' : 'badge-gray'}`}>
              suite: {status.securityEnabled ? 'enabled' : 'disabled'}
            </span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            {(status.protections || []).map(protection => (
              <div key={protection.key} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '8px 0', borderBottom: '1px solid #1e1e2e' }}>
                <div>
                  <div style={{ fontSize: 14, color: '#e0e0e8' }}>{protection.name}</div>
                  <div style={{ fontSize: 12, color: '#666' }}>{protection.description}</div>
                  {protection.details && <div style={{ fontSize: 11, color: '#777', marginTop: 4 }}>{protection.details}</div>}
                </div>
                <span className={`badge ${badgeClass(protection.status)}`}>{protection.status}</span>
              </div>
            ))}
            {(status.protections || []).length === 0 && <p style={{ color: '#666' }}>Security status unavailable.</p>}
          </div>
        </div>

        <div className="card">
          <h3>Runtime Summary</h3>
          <div style={{ fontSize: 13, color: '#aaa', lineHeight: 2, marginBottom: 24 }}>
            <div>Browser active: {status.browserActive ? 'Yes' : 'No'}</div>
            <div>Signature patterns loaded: {status.signaturePatternCount || 0}</div>
            <div>Sandbox blocked APIs: {(status.sandboxBlockedAPIs || []).join(', ') || 'none'}</div>
          </div>

          <h3>Injection Attempts ({injectionEvents.length})</h3>
          <div className="event-log" style={{ maxHeight: 400 }}>
            {injectionEvents.length === 0 && <p style={{ color: '#666' }}>No injection attempts detected.</p>}
            {injectionEvents.map((ev, i) => (
              <div key={i} className="event">
                <span className="event-time">{new Date(ev.ts).toLocaleTimeString()} </span>
                <span style={{ color: '#ef4444' }}>INJECTION </span>
                <span style={{ color: '#888', fontSize: 12 }}>{JSON.stringify(ev.params).substring(0, 120)}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
