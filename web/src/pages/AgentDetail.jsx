import React, { useState, useEffect, useMemo } from 'react'
import { useParams, Link } from 'react-router-dom'

export default function AgentDetail({ ws }) {
  const { id } = useParams()
  const [agent, setAgent] = useState(null)
  const [messages, setMessages] = useState([])
  const [timeline, setTimeline] = useState([])
  const [fingerprint, setFingerprint] = useState(null)
  const [input, setInput] = useState('')
  const [tab, setTab] = useState('conversation')
  const recentEvents = useMemo(() => ws.events.slice(-200), [ws.events])

  const refresh = () => {
    if (!ws.connected || !id) return
    ws.call('agents.list').then(r => setAgent((r?.agents || []).find(a => a.id === id) || null)).catch(() => {})
    ws.call('agents.getMessages', { agentId: id }).then(r => setMessages(r?.messages || [])).catch(() => {})
    ws.call('recording.getTimeline', { agentId: id }).then(r => setTimeline(r?.actions || [])).catch(() => {})
    ws.call('fingerprints.get', { agentId: id }).then(r => setFingerprint(r)).catch(() => {})
  }

  useEffect(() => {
    refresh()
  }, [ws.connected, id])

  useEffect(() => {
    if (!id) return
    for (const event of recentEvents) {
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
          const last = prev[prev.length-1]
          if (last && last.role === nextMsg.role && last.content === nextMsg.content && (last.tokens || 0) === nextMsg.tokens) {
            return prev
          }
          return [...prev, nextMsg]
        })
      }
    }
  }, [recentEvents, id])

  const pause = async () => {
    try {
      await ws.call('agents.pause', { agentId: id })
      setAgent(prev => prev ? { ...prev, status: 'paused' } : prev)
    } catch (e) { alert(e.message) }
  }

  const resume = async () => {
    try {
      await ws.call('agents.resume', { agentId: id })
      setAgent(prev => prev ? { ...prev, status: 'active' } : prev)
    } catch (e) { alert(e.message) }
  }

  const kill = async () => {
    if (!confirm('Kill agent ' + id.substring(0, 8) + '?')) return
    try {
      await ws.call('agents.kill', { agentId: id })
      setAgent(prev => prev ? { ...prev, status: 'interrupted' } : prev)
    } catch (e) { alert(e.message) }
  }

  const sendMessage = async () => {
    const text = input.trim()
    if (!text) return
    try {
      await ws.call('agents.resume', { agentId: id, message: text })
      setMessages(prev => [...prev, { role: 'user', content: text, tokens: 0 }])
      setAgent(prev => prev ? { ...prev, status: 'active' } : prev)
      setInput('')
    } catch (e) { alert(e.message) }
  }

  return (
    <div>
      <div className="page-header">
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <Link to="/agents" style={{ color: '#666', textDecoration: 'none' }}>← Agents</Link>
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

      <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
        {['conversation', 'recording', 'fingerprint'].map(t => (
          <button key={t} className={`btn ${tab === t ? 'btn-primary' : 'btn-ghost'}`} onClick={() => setTab(t)}>
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>

      {tab === 'conversation' && (
        <div className="card">
          <h3>Conversation</h3>
          <div style={{ maxHeight: 500, overflowY: 'auto', marginBottom: 12 }}>
            {messages.length === 0 && <p style={{ color: '#666' }}>No messages yet.</p>}
            {messages.map((m, i) => (
              <div key={i} style={{ padding: '8px 0', borderBottom: '1px solid #1e1e2e' }}>
                <span style={{ color: m.role === 'user' ? '#60a5fa' : m.role === 'assistant' ? '#a78bfa' : '#666', fontWeight: 600, fontSize: 12 }}>
                  {m.role?.toUpperCase()}
                </span>
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

      {tab === 'recording' && (
        <div className="card">
          <h3>Action Timeline</h3>
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
          <h3>Fingerprint</h3>
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
