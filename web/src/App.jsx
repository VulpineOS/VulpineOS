import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { Routes, Route, Link, useLocation } from 'react-router-dom'
import { useWebSocket } from './hooks/useWebSocket'
import Dashboard from './pages/Dashboard'
import Agents from './pages/Agents'
import AgentDetail from './pages/AgentDetail'
import Bus from './pages/Bus'
import Contexts from './pages/Contexts'
import Proxies from './pages/Proxies'
import Settings from './pages/Settings'
import Logs from './pages/Logs'
import Security from './pages/Security'
import Webhooks from './pages/Webhooks'
import Scripts from './pages/Scripts'
import Login from './pages/Login'
import './App.css'

function bootstrapPanelKey() {
  const params = new URLSearchParams(window.location.search)
  const token = params.get('token')
  if (token) {
    sessionStorage.setItem('vulpine_key', token)
    params.delete('token')
    const next = `${window.location.pathname}${params.toString() ? `?${params.toString()}` : ''}${window.location.hash}`
    window.history.replaceState({}, document.title, next)
    return token
  }
  return sessionStorage.getItem('vulpine_key') || ''
}

function connectionMeta(ws) {
  switch (ws.connectionState) {
    case 'connecting':
      return { tone: 'blue', title: 'Connecting', message: 'Connecting to the panel server.' }
    case 'reconnecting':
      return {
        tone: 'yellow',
        title: 'Reconnecting',
        message: `Connection lost. Retrying in ${Math.max(1, Math.ceil((ws.retryDelayMs || 0) / 1000))}s${ws.reconnectAttempt ? ` (attempt ${ws.reconnectAttempt}/8)` : ''}.`,
      }
    case 'failed':
      return { tone: 'red', title: 'Connection failed', message: ws.lastError || 'Unable to establish a panel session.' }
    default:
      return null
  }
}

export default function App() {
  const [apiKey, setApiKey] = useState(() => bootstrapPanelKey())
  const [notice, setNotice] = useState(null)
  const ws = useWebSocket(apiKey)
  const location = useLocation()
  const conn = connectionMeta(ws)

  const notify = useCallback((message, level = 'error') => {
    if (!message) return
    setNotice({ id: Date.now(), message, level })
  }, [])

  useEffect(() => {
    if (!notice) return undefined
    const timeout = setTimeout(() => setNotice(current => current?.id === notice.id ? null : current), notice.level === 'error' ? 8000 : 5000)
    return () => clearTimeout(timeout)
  }, [notice])

  const clearSession = useCallback(() => {
    sessionStorage.removeItem('vulpine_key')
    setApiKey('')
    setNotice(null)
  }, [])

  const panelWS = useMemo(() => ({
    ...ws,
    notify,
    clearNotice: () => setNotice(null),
  }), [notify, ws])

  if (!apiKey) {
    return <Login onLogin={(key) => { sessionStorage.setItem('vulpine_key', key); setApiKey(key) }} />
  }

  const nav = [
    { path: '/', label: 'Dashboard' },
    { path: '/agents', label: 'Agents' },
    { path: '/bus', label: 'Bus' },
    { path: '/contexts', label: 'Contexts' },
    { path: '/proxies', label: 'Proxies' },
    { path: '/security', label: 'Security' },
    { path: '/webhooks', label: 'Webhooks' },
    { path: '/scripts', label: 'Scripts' },
    { path: '/logs', label: 'Logs' },
    { path: '/settings', label: 'Settings' },
  ]

  return (
    <div className="app">
      <nav className="sidebar">
        <div className="logo">
          <h2>VulpineOS</h2>
          <span className={`status-dot ${ws.connectionState}`} />
          <span className="status-label">{ws.connectionState === 'connected' ? 'Connected' : ws.connectionState}</span>
        </div>
        {nav.map(n => (
          <Link key={n.path} to={n.path} className={`nav-item ${location.pathname === n.path ? 'active' : ''}`}>
            <span>{n.label}</span>
          </Link>
        ))}
        <div className="nav-spacer" />
        <button className="nav-item logout" onClick={clearSession}>
          <span>Logout</span>
        </button>
      </nav>
      <main className="content">
        {(conn || notice) && (
          <div className="panel-banner-stack">
            {conn && (
              <div className={`panel-banner panel-banner-${conn.tone}`}>
                <div>
                  <strong>{conn.title}</strong>
                  <span>{conn.message}</span>
                </div>
                <div className="panel-banner-actions">
                  <button className="btn btn-ghost btn-sm" onClick={ws.retry}>Retry now</button>
                  {ws.connectionState === 'failed' && (
                    <button className="btn btn-ghost btn-sm" onClick={clearSession}>Reset access key</button>
                  )}
                </div>
              </div>
            )}
            {notice && (
              <div className={`panel-banner panel-banner-${notice.level}`}>
                <div>
                  <strong>{notice.level === 'error' ? 'Error' : notice.level === 'success' ? 'Done' : 'Notice'}</strong>
                  <span>{notice.message}</span>
                </div>
                <div className="panel-banner-actions">
                  <button className="btn btn-ghost btn-sm" onClick={() => setNotice(null)}>Dismiss</button>
                </div>
              </div>
            )}
          </div>
        )}
        <Routes>
          <Route path="/" element={<Dashboard ws={panelWS} />} />
          <Route path="/agents" element={<Agents ws={panelWS} />} />
          <Route path="/agents/:id" element={<AgentDetail ws={panelWS} />} />
          <Route path="/bus" element={<Bus ws={panelWS} />} />
          <Route path="/contexts" element={<Contexts ws={panelWS} />} />
          <Route path="/proxies" element={<Proxies ws={panelWS} />} />
          <Route path="/security" element={<Security ws={panelWS} />} />
          <Route path="/webhooks" element={<Webhooks ws={panelWS} />} />
          <Route path="/scripts" element={<Scripts ws={panelWS} />} />
          <Route path="/logs" element={<Logs ws={panelWS} />} />
          <Route path="/settings" element={<Settings ws={panelWS} />} />
        </Routes>
      </main>
    </div>
  )
}
