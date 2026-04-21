import React, { useEffect, useRef, useState } from 'react'
import { useParams, Link } from 'react-router-dom'

function downloadTextFile(content, fileName, contentType = 'application/json') {
  const blob = new Blob([content || ''], { type: contentType })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = fileName
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

export default function AgentDetail({ ws }) {
  const { id } = useParams()
  const [agent, setAgent] = useState(null)
  const [messages, setMessages] = useState([])
  const [timeline, setTimeline] = useState([])
  const [fingerprint, setFingerprint] = useState(null)
  const [sessionLog, setSessionLog] = useState('')
  const [input, setInput] = useState('')
  const [tab, setTab] = useState('conversation')
  const [budgetCost, setBudgetCost] = useState('0')
  const [budgetTokens, setBudgetTokens] = useState('0')
  const [inheritDefaultBudget, setInheritDefaultBudget] = useState(true)
  const [fingerprintSeed, setFingerprintSeed] = useState('')
  const lastEventCountRef = useRef(0)

  const conversationMessages = messages.filter(m => m.role !== 'system')
  const traceMessages = messages.filter(m => m.role === 'system')

  function traceMeta(content) {
    if (!content) return { label: 'TRACE', tone: '#9ca3af', bg: '#111827' }
    if (content.startsWith('Tool failed:')) return { label: 'FAIL', tone: '#f87171', bg: '#2a1416' }
    if (content.startsWith('Tool timed out:')) return { label: 'TIMEOUT', tone: '#fb923c', bg: '#2b1a12' }
    if (content.startsWith('Tool incomplete:')) return { label: 'PARTIAL', tone: '#f59e0b', bg: '#2b2111' }
    if (content.startsWith('Tool completed:')) return { label: 'OK', tone: '#34d399', bg: '#10231c' }
    if (content.startsWith('Running ')) return { label: 'RUN', tone: '#fbbf24', bg: '#2a2111' }
    if (content.startsWith('Thinking:')) return { label: 'THINK', tone: '#a78bfa', bg: '#1d1630' }
    if (content.startsWith('Warning:')) return { label: 'WARN', tone: '#facc15', bg: '#2a240d' }
    return { label: 'TRACE', tone: '#9ca3af', bg: '#111827' }
  }

  function formatTimestamp(value) {
    if (!value) return null
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return null
    return date.toLocaleTimeString()
  }

  const refresh = () => {
    if (!ws.connected || !id) return
    ws.call('agents.list').then(r => {
      const nextAgent = (r?.agents || []).find(a => a.id === id) || null
      setAgent(nextAgent)
    }).catch(() => {})
    ws.call('agents.getMessages', { agentId: id }).then(r => setMessages(r?.messages || [])).catch(() => {})
    ws.call('recording.getTimeline', { agentId: id }).then(r => setTimeline(r?.actions || [])).catch(() => {})
    ws.call('fingerprints.get', { agentId: id }).then(r => setFingerprint(r)).catch(() => {})
  }

  const loadSessionLog = async () => {
    try {
      const result = await ws.call('agents.getSessionLog', { agentId: id })
      setSessionLog(result?.content || '')
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  useEffect(() => {
    refresh()
  }, [ws.connected, id])

  useEffect(() => {
    if (!agent) return
    setBudgetCost(String(agent.budgetMaxCostUsd ?? 0))
    setBudgetTokens(String(agent.budgetMaxTokens ?? 0))
    setInheritDefaultBudget((agent.budgetSource || 'none') !== 'agent')
    setFingerprintSeed(current => current || `${agent.name || id}-${Date.now()}`)
  }, [agent?.id, agent?.budgetMaxCostUsd, agent?.budgetMaxTokens, agent?.budgetSource])

  useEffect(() => {
    lastEventCountRef.current = 0
  }, [id])

  useEffect(() => {
    if (tab !== 'raw' || !ws.connected || !id) return
    loadSessionLog()
    const interval = setInterval(() => {
      loadSessionLog()
    }, 2000)
    return () => clearInterval(interval)
  }, [tab, ws.connected, id])

  useEffect(() => {
    if (!id) return
    if (ws.events.length < lastEventCountRef.current) {
      lastEventCountRef.current = 0
    }
    const nextEvents = ws.events.slice(lastEventCountRef.current)
    lastEventCountRef.current = ws.events.length
    for (const event of nextEvents) {
      if (event.method === 'Vulpine.agentStatus' && event.params?.agentId === id) {
        setAgent(prev => ({
          ...(prev || { id }),
          ...(prev || {}),
          id,
          status: event.params.status || prev?.status,
          contextId: event.params.contextId || prev?.contextId,
          task: event.params.objective || prev?.task,
          totalTokens: event.params.tokens ?? prev?.totalTokens ?? 0,
        }))
      }
      if (event.method === 'Vulpine.conversation' && event.params?.agentId === id) {
        const nextMsg = {
          role: event.params.role,
          content: event.params.content,
          tokens: event.params.tokens || 0,
        }
        setMessages(prev => {
          const last = prev[prev.length - 1]
          if (last && last.role === nextMsg.role && last.content === nextMsg.content && (last.tokens || 0) === nextMsg.tokens) {
            return prev
          }
          return [...prev, nextMsg]
        })
      }
    }
  }, [ws.events, id])

  const pause = async () => {
    try {
      await ws.call('agents.pause', { agentId: id })
      setAgent(prev => prev ? { ...prev, status: 'paused' } : prev)
    } catch (e) { ws.notify?.(e.message) }
  }

  const resume = async () => {
    try {
      await ws.call('agents.resume', { agentId: id })
      setAgent(prev => prev ? { ...prev, status: 'active' } : prev)
    } catch (e) { ws.notify?.(e.message) }
  }

  const kill = async () => {
    if (!confirm('Kill agent ' + id.substring(0, 8) + '?')) return
    try {
      await ws.call('agents.kill', { agentId: id })
      setAgent(prev => prev ? { ...prev, status: 'interrupted' } : prev)
    } catch (e) { ws.notify?.(e.message) }
  }

  const sendMessage = async () => {
    const text = input.trim()
    if (!text) return
    try {
      await ws.call('agents.resume', { agentId: id, message: text })
      setMessages(prev => [...prev, { role: 'user', content: text, tokens: 0 }])
      setAgent(prev => prev ? { ...prev, status: 'active' } : prev)
      setInput('')
    } catch (e) { ws.notify?.(e.message) }
  }

  const saveBudget = async () => {
    try {
      const result = await ws.call('costs.setBudget', inheritDefaultBudget
        ? { agentId: id, inheritDefault: true }
        : {
            agentId: id,
            maxCostUsd: Number(budgetCost) || 0,
            maxTokens: Number(budgetTokens) || 0,
          })
      setAgent(prev => prev ? {
        ...prev,
        budgetMaxCostUsd: result?.maxCostUsd ?? 0,
        budgetMaxTokens: result?.maxTokens ?? 0,
        budgetSource: result?.budgetSource || (inheritDefaultBudget ? 'default' : 'agent'),
      } : prev)
      ws.notify?.('Budget saved', 'success')
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  const exportRecording = async () => {
    try {
      const result = await ws.call('recording.export', { agentId: id })
      downloadTextFile(result?.content || '', result?.fileName || `agent-${id}-recording.json`, result?.contentType || 'application/json')
      ws.notify?.('Recording exported', 'success')
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  const regenerateFingerprint = async () => {
    try {
      await ws.call('fingerprints.generate', { agentId: id, seed: fingerprintSeed || `${id}-${Date.now()}` })
      await ws.call('fingerprints.get', { agentId: id }).then(r => setFingerprint(r))
      ws.notify?.('Fingerprint updated', 'success')
      setFingerprintSeed(`${agent?.name || id}-${Date.now()}`)
      refresh()
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  return (
    <div>
      <div className="page-header">
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <Link to="/agents" style={{ color: '#666', textDecoration: 'none' }}>Back to Agents</Link>
          <h1>Agent {id.substring(0, 12)}</h1>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <span className={`badge badge-${agent?.status === 'active' ? 'green' : agent?.status === 'paused' ? 'yellow' : agent?.status === 'completed' ? 'blue' : 'gray'}`}>
            {agent?.status || 'unknown'}
          </span>
          {agent?.status === 'active' && <button className="btn btn-ghost" onClick={pause}>Pause</button>}
          {agent?.status === 'paused' && <button className="btn btn-ghost" onClick={resume}>Resume</button>}
          {agent?.status !== 'completed' && <button className="btn btn-danger" onClick={kill}>Kill</button>}
          <button className="btn btn-ghost" onClick={refresh}>Refresh</button>
        </div>
      </div>

      <div className="card" style={{ marginBottom: 16 }}>
        <div className="page-header" style={{ marginBottom: 12 }}>
          <h3 style={{ margin: 0 }}>Controls</h3>
          <span className="badge badge-gray">budget: {agent?.budgetSource || 'none'}</span>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr 1fr auto', gap: 12, alignItems: 'end' }}>
          <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: '#aaa' }}>
            <input type="checkbox" checked={inheritDefaultBudget} onChange={e => setInheritDefaultBudget(e.target.checked)} />
            Inherit default budget
          </label>
          <div>
            <label style={{ fontSize: 12, color: '#666', display: 'block', marginBottom: 4 }}>Max cost (USD)</label>
            <input className="input" type="number" step="0.01" value={budgetCost} disabled={inheritDefaultBudget} onChange={e => setBudgetCost(e.target.value)} />
          </div>
          <div>
            <label style={{ fontSize: 12, color: '#666', display: 'block', marginBottom: 4 }}>Max tokens</label>
            <input className="input" type="number" step="1" value={budgetTokens} disabled={inheritDefaultBudget} onChange={e => setBudgetTokens(e.target.value)} />
          </div>
          <button className="btn btn-primary" onClick={saveBudget}>Save Budget</button>
        </div>
      </div>

      <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
        {['conversation', 'trace', 'raw', 'recording', 'fingerprint'].map(t => (
          <button key={t} className={`btn ${tab === t ? 'btn-primary' : 'btn-ghost'}`} onClick={() => setTab(t)}>
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>

      {tab === 'conversation' && (
        <div className="card">
          <h3>Conversation</h3>
          <div style={{ maxHeight: 500, overflowY: 'auto', marginBottom: 12 }}>
            {conversationMessages.length === 0 && <p style={{ color: '#666' }}>No messages yet.</p>}
            {conversationMessages.map((m, i) => (
              <div key={i} style={{ padding: '8px 0', borderBottom: '1px solid #1e1e2e' }}>
                <span style={{ color: m.role === 'user' ? '#60a5fa' : m.role === 'assistant' ? '#a78bfa' : '#666', fontWeight: 600, fontSize: 12 }}>
                  {m.role?.toUpperCase()}
                </span>
                {formatTimestamp(m.timestamp) && (
                  <span style={{ color: '#666', fontSize: 11, marginLeft: 8 }}>{formatTimestamp(m.timestamp)}</span>
                )}
                <div style={{ fontSize: 13, color: '#ccc', marginTop: 4, whiteSpace: 'pre-wrap' }}>{m.content}</div>
              </div>
            ))}
          </div>
          <div style={{ display: 'flex', gap: 8 }}>
            <input className="input" value={input} onChange={e => setInput(e.target.value)}
              placeholder="Send message to agent..." onKeyDown={e => e.key === 'Enter' && sendMessage()} />
            <button className="btn btn-primary" onClick={sendMessage}>Send</button>
          </div>
        </div>
      )}

      {tab === 'trace' && (
        <div className="card">
          <h3>Action Trace</h3>
          <div style={{ maxHeight: 500, overflowY: 'auto', marginBottom: 12 }}>
            {traceMessages.length === 0 && <p style={{ color: '#666' }}>No action trace yet.</p>}
            {traceMessages.map((m, i) => {
              const meta = traceMeta(m.content)
              return (
                <div key={i} style={{ padding: '8px 0', borderBottom: '1px solid #1e1e2e' }}>
                  <span style={{
                    color: meta.tone,
                    background: meta.bg,
                    border: `1px solid ${meta.tone}33`,
                    fontWeight: 700,
                    fontSize: 11,
                    borderRadius: 6,
                    padding: '2px 6px',
                    display: 'inline-block',
                    minWidth: 56,
                    textAlign: 'center',
                  }}>
                    {meta.label}
                  </span>
                  {formatTimestamp(m.timestamp) && (
                    <span style={{ color: '#666', fontSize: 11, marginLeft: 8 }}>{formatTimestamp(m.timestamp)}</span>
                  )}
                  <div style={{ fontSize: 13, color: '#ccc', marginTop: 4, whiteSpace: 'pre-wrap', fontFamily: 'monospace' }}>
                    {m.content}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}

      {tab === 'raw' && (
        <div className="card">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
            <h3 style={{ margin: 0 }}>Raw Session Log</h3>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <span style={{ fontSize: 11, color: '#666' }}>Auto-refreshing</span>
              <button className="btn btn-ghost" onClick={loadSessionLog}>Refresh</button>
            </div>
          </div>
          {sessionLog === '' ? (
            <p style={{ color: '#666' }}>No raw session log loaded yet.</p>
          ) : (
            <pre style={{ fontSize: 12, color: '#aaa', overflow: 'auto', maxHeight: 500, whiteSpace: 'pre-wrap' }}>
              {sessionLog}
            </pre>
          )}
        </div>
      )}

      {tab === 'recording' && (
        <div className="card">
          <div className="page-header" style={{ marginBottom: 12 }}>
            <h3 style={{ margin: 0 }}>Action Timeline</h3>
            <button className="btn btn-primary" onClick={exportRecording}>Export JSON</button>
          </div>
          <div style={{ fontFamily: 'monospace', fontSize: 12, lineHeight: 1.8 }}>
            {timeline.length === 0 && <p style={{ color: '#666' }}>No recorded actions.</p>}
            {timeline.map((a, i) => (
              <div key={i} style={{ color: '#aaa' }}>
                <span style={{ color: '#666' }}>[{new Date(a.timestamp).toLocaleTimeString()}]</span>{' '}
                <span style={{ color: '#a78bfa' }}>{a.type?.toUpperCase()}</span>{' '}
                {a.data && <span>{JSON.stringify(a.data).substring(0, 60)}</span>}
              </div>
            ))}
          </div>
        </div>
      )}

      {tab === 'fingerprint' && (
        <div className="card">
          <div className="page-header" style={{ marginBottom: 12 }}>
            <h3 style={{ margin: 0 }}>Fingerprint</h3>
            <div style={{ display: 'flex', gap: 8 }}>
              <input
                className="input"
                style={{ width: 280 }}
                value={fingerprintSeed}
                onChange={e => setFingerprintSeed(e.target.value)}
                placeholder="Fingerprint seed"
              />
              <button className="btn btn-primary" onClick={regenerateFingerprint}>Regenerate & Apply</button>
            </div>
          </div>
          {fingerprint ? (
            <pre style={{ fontSize: 12, color: '#aaa', overflow: 'auto', maxHeight: 400 }}>
              {JSON.stringify(fingerprint, null, 2)}
            </pre>
          ) : (
            <p style={{ color: '#666' }}>No fingerprint assigned.</p>
          )}
        </div>
      )}
    </div>
  )
}
