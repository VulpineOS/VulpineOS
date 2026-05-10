import React, { useState, useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import {
  agentStatusBadgeClass,
  isLiveAgentStatus,
  isPausedAgentStatus,
  isTerminalAgentStatus,
} from '../utils/agentStatus'

export default function Agents({ ws }) {
  const [agents, setAgents] = useState([])
  const [selected, setSelected] = useState({})
  const [task, setTask] = useState('')
  const [costs, setCosts] = useState([])
  const [totalCost, setTotalCost] = useState(0)
  const [defaultBudget, setDefaultBudget] = useState({ maxCostUsd: 0, maxTokens: 0 })
  const [contexts, setContexts] = useState([])
  const [contextsLoaded, setContextsLoaded] = useState(false)
  const [selectedContext, setSelectedContext] = useState(localStorage.getItem('vulpine_context_id') || '')
  const [notice, setNotice] = useState('')
  const [pendingKill, setPendingKill] = useState(null)
  const eventStreamInitializedRef = useRef(false)
  const lastEventSeqRef = useRef(0)
  const lastEventIndexRef = useRef(0)

  const refresh = () => {
    ws.call('agents.list').then(r => setAgents(r?.agents || [])).catch(() => {})
    ws.call('costs.getAll').then(r => {
      setCosts(r?.usage || [])
      setDefaultBudget(r?.defaults || { maxCostUsd: 0, maxTokens: 0 })
    }).catch(() => {})
    ws.call('costs.total').then(r => setTotalCost(r?.totalCostUsd || 0)).catch(() => {})
    ws.call('contexts.list').then(r => {
      setContexts(r?.contexts || [])
      setContextsLoaded(true)
    }).catch(() => {})
  }

  useEffect(() => { if (ws.connected) refresh() }, [ws.connected])
  useEffect(() => {
    if (!contextsLoaded || !selectedContext) return
    if (!contexts.some(context => context.id === selectedContext)) {
      localStorage.removeItem('vulpine_context_id')
      setSelectedContext('')
    }
  }, [contexts, contextsLoaded, selectedContext])

  useEffect(() => {
    const events = ws.events || []
    const sequencedEvents = events.filter(event => Number.isFinite(event.seq))
    let nextEvents
    if (!eventStreamInitializedRef.current) {
      eventStreamInitializedRef.current = true
      lastEventIndexRef.current = events.length
      lastEventSeqRef.current = sequencedEvents.length > 0
        ? Math.max(...sequencedEvents.map(event => event.seq))
        : 0
      return
    }
    if (sequencedEvents.length > 0) {
      nextEvents = sequencedEvents.filter(event => event.seq > lastEventSeqRef.current)
      if (nextEvents.length > 0) {
        lastEventSeqRef.current = Math.max(...nextEvents.map(event => event.seq))
      }
    } else {
      if (events.length < lastEventIndexRef.current) {
        lastEventIndexRef.current = 0
      }
      nextEvents = events.slice(lastEventIndexRef.current)
      lastEventIndexRef.current = events.length
    }
    for (const event of nextEvents) {
      if (event.method === 'Vulpine.agentStatus' && event.params?.agentId) {
        setAgents(prev => prev.map(agent => {
          if (agent.id !== event.params.agentId) return agent
          const previousTokens = Number(agent.totalTokens || 0)
          const eventTokens = Number(event.params.tokens || 0)
          return {
            ...agent,
            status: event.params.status || agent.status,
            contextId: event.params.contextId || agent.contextId,
            task: event.params.objective || agent.task,
            totalTokens: eventTokens > 0 ? Math.max(previousTokens, eventTokens) : agent.totalTokens,
          }
        }))
      }
    }
  }, [ws.events])

  const spawn = async () => {
    if (!task.trim()) return
    try {
      await ws.call('agents.spawn', { task, contextId: selectedContext || undefined })
      setTask('')
      setTimeout(refresh, 1000)
    } catch (e) { ws.notify?.(`Spawn failed: ${e.message}`) }
  }

  const requestKill = (id) => {
    const agent = agents.find(a => a.id === id)
    if (!agent || isTerminalAgentStatus(agent.status)) return
    setPendingKill({ type: 'single', ids: [id], label: `agent ${id.substring(0, 8)}` })
  }

  const pause = async (id) => {
    try { await ws.call('agents.pause', { agentId: id }); refresh() }
    catch (e) { ws.notify?.(e.message) }
  }

  const resume = async (id) => {
    try { await ws.call('agents.resume', { agentId: id }); refresh() }
    catch (e) { ws.notify?.(e.message) }
  }

  const toggleSelected = (id) => {
    setSelected(prev => ({ ...prev, [id]: !prev[id] }))
  }

  const toggleAll = () => {
    const visible = agents.map(a => a.id)
    const allSelected = visible.length > 0 && visible.every(id => selected[id])
    if (allSelected) {
      setSelected({})
      return
    }
    const next = {}
    for (const id of visible) next[id] = true
    setSelected(next)
  }

  const selectedIDs = agents.filter(a => selected[a.id]).map(a => a.id)
  const selectedPauseIDs = agents
    .filter(a => selected[a.id] && isLiveAgentStatus(a.status))
    .map(a => a.id)
  const selectedResumeIDs = agents
    .filter(a => selected[a.id] && isPausedAgentStatus(a.status))
    .map(a => a.id)
  const selectedKillIDs = agents
    .filter(a => selected[a.id] && !isTerminalAgentStatus(a.status))
    .map(a => a.id)

  const runBulk = async (action) => {
    const targetIDs = action === 'pause' ? selectedPauseIDs : action === 'resume' ? selectedResumeIDs : selectedIDs
    if (targetIDs.length === 0) return
    if (action === 'kill') {
      if (selectedKillIDs.length === 0) return
      setPendingKill({ type: 'bulk', ids: selectedKillIDs, label: `${selectedKillIDs.length} selected agents` })
      return
    }
    try {
      const method = action === 'pause' ? 'agents.pauseMany' : action === 'resume' ? 'agents.resumeMany' : 'agents.killMany'
      const result = await ws.call(method, { agentIds: targetIDs })
      const actionKey = action === 'pause' ? 'paused' : action === 'resume' ? 'resumed' : 'killed'
      const failures = Object.keys(result?.failures || {}).length
      const completed = result?.[actionKey] ?? (failures === 0 ? targetIDs.length : 0)
      if (failures > 0) {
        setNotice(`${actionKey.charAt(0).toUpperCase() + actionKey.slice(1)} ${completed} agents, ${failures} failed`)
      } else {
        setNotice(`${actionKey.charAt(0).toUpperCase() + actionKey.slice(1)} ${completed} agents`)
      }
      setSelected({})
      refresh()
    } catch (e) { ws.notify?.(e.message) }
  }

  const confirmKill = async () => {
    if (!pendingKill || pendingKill.ids.length === 0) return
    try {
      if (pendingKill.type === 'single') {
        await ws.call('agents.kill', { agentId: pendingKill.ids[0] })
        setNotice(`Killed agent ${pendingKill.ids[0].substring(0, 8)}`)
      } else {
        const result = await ws.call('agents.killMany', { agentIds: pendingKill.ids })
        const failures = Object.keys(result?.failures || {}).length
        const killed = result?.killed ?? (failures === 0 ? pendingKill.ids.length : 0)
        setNotice(failures > 0 ? `Killed ${killed} agents, ${failures} failed` : `Killed ${killed} agents`)
        setSelected({})
      }
      setPendingKill(null)
      refresh()
    } catch (e) { ws.notify?.(e.message) }
  }

  const getCost = (id) => {
    const c = costs.find(u => u.agentId === id)
    return c ? `$${c.estimatedCost.toFixed(4)}` : '—'
  }

  const getTokens = (id) => {
    const c = costs.find(u => u.agentId === id)
    if (c) return c.totalTokens.toLocaleString()
    const agent = agents.find(a => a.id === id)
    return (agent?.totalTokens || 0) > 0 ? agent.totalTokens.toLocaleString() : '—'
  }

  const totalTokens = costs.reduce((sum, usage) => sum + (usage.totalTokens || 0), 0)
  const overrideCount = agents.filter(agent => agent.budgetSource === 'agent').length
  const inheritedCount = agents.filter(agent => agent.budgetSource === 'default').length
  const defaultBudgetLabel = [
    defaultBudget.maxCostUsd > 0 ? `$${Number(defaultBudget.maxCostUsd).toFixed(2)}` : null,
    defaultBudget.maxTokens > 0 ? `${Number(defaultBudget.maxTokens).toLocaleString()} tok` : null,
  ].filter(Boolean).join(' · ') || 'Unlimited'

  const formatBudget = (agent) => {
    if (!agent || !agent.budgetSource || agent.budgetSource === 'none') return 'No budget'
    const parts = []
    if ((agent.budgetMaxCostUsd || 0) > 0) parts.push(`$${Number(agent.budgetMaxCostUsd).toFixed(2)}`)
    if ((agent.budgetMaxTokens || 0) > 0) parts.push(`${Number(agent.budgetMaxTokens).toLocaleString()} tok`)
    const limit = parts.length > 0 ? parts.join(' · ') : 'Unlimited'
    return agent.budgetSource === 'agent' ? `Override · ${limit}` : `Default · ${limit}`
  }

  return (
    <div>
      <div className="page-header">
        <h1>Agents</h1>
        <div className="page-actions">
          <button className="btn btn-ghost" disabled={selectedPauseIDs.length === 0} onClick={() => runBulk('pause')}>Pause Selected</button>
          <button className="btn btn-ghost" disabled={selectedResumeIDs.length === 0} onClick={() => runBulk('resume')}>Resume Selected</button>
          <button className="btn btn-danger" disabled={selectedKillIDs.length === 0} onClick={() => runBulk('kill')}>Kill Selected</button>
          <select
            className="input"
            style={{ width: 220, maxWidth: '100%' }}
            value={selectedContext}
            onChange={e => {
              setSelectedContext(e.target.value)
              localStorage.setItem('vulpine_context_id', e.target.value)
            }}
          >
            <option value="">Shared browser</option>
            {contexts.map(ctx => {
              const contextURL = ctx.url || 'about:blank'
              return (
                <option key={ctx.id} value={ctx.id}>{ctx.id.slice(0, 12)} · {contextURL.slice(0, 32)}</option>
              )
            })}
          </select>
          <input className="input" style={{ width: 300, maxWidth: '100%' }} placeholder="Task description..." value={task}
            onChange={e => setTask(e.target.value)} onKeyDown={e => e.key === 'Enter' && spawn()} />
          <button className="btn btn-primary" onClick={spawn}>Spawn</button>
          <button className="btn btn-ghost" onClick={refresh}>Refresh</button>
        </div>
      </div>

      {notice && (
        <div className="card" style={{ marginBottom: 16, padding: 12, color: '#ccc' }}>
          {notice}
        </div>
      )}

      {pendingKill && (
        <div className="panel-banner panel-banner-red" style={{ marginBottom: 16 }}>
          <div>
            <strong>Confirm kill</strong>
            <span>Stop {pendingKill.label} and mark the session interrupted.</span>
          </div>
          <div className="panel-banner-actions">
            <button className="btn btn-danger btn-sm" onClick={confirmKill}>Kill</button>
            <button className="btn btn-ghost btn-sm" onClick={() => setPendingKill(null)}>Cancel</button>
          </div>
        </div>
      )}

      <div className="grid grid-4" style={{ marginBottom: 16 }}>
        <div className="card metric-card">
          <div className="metric-kicker">Spend</div>
          <div className="stat-value">${totalCost.toFixed(4)}</div>
          <div className="metric-caption">{costs.length} tracked agents with usage</div>
        </div>
        <div className="card metric-card">
          <div className="metric-kicker">Tokens</div>
          <div className="stat-value">{totalTokens.toLocaleString()}</div>
          <div className="metric-caption">Aggregate tracked token usage</div>
        </div>
        <div className="card metric-card">
          <div className="metric-kicker">Default budget</div>
          <div className="stat-value">{defaultBudget.maxCostUsd > 0 ? `$${Number(defaultBudget.maxCostUsd).toFixed(2)}` : 'Open'}</div>
          <div className="metric-caption">{defaultBudgetLabel}</div>
        </div>
        <div className="card metric-card">
          <div className="metric-kicker">Overrides</div>
          <div className="stat-value">{overrideCount}</div>
          <div className="metric-caption">{inheritedCount} inheriting the default budget</div>
        </div>
      </div>

      <div className="card">
        <table className="table">
          <thead><tr><th><input type="checkbox" checked={agents.length > 0 && agents.every(a => selected[a.id])} onChange={toggleAll} /></th><th>Agent</th><th>Status</th><th>Context</th><th>Fingerprint</th><th>Tokens</th><th>Cost</th><th>Budget</th><th>Actions</th></tr></thead>
          <tbody>
            {agents.length === 0 && (
              <tr><td colSpan="9" style={{ textAlign: 'center', color: '#666', padding: 32 }}>No agents. Spawn one above.</td></tr>
            )}
            {agents.map(a => (
              <tr key={a.id}>
                <td><input type="checkbox" checked={!!selected[a.id]} onChange={() => toggleSelected(a.id)} /></td>
                <td><Link to={`/agents/${a.id}`} style={{ color: '#a78bfa', textDecoration: 'none' }}>{a.name || a.id.substring(0, 12)}</Link></td>
                <td><span className={`badge badge-${agentStatusBadgeClass(a.status)}`}>{a.status}</span></td>
                <td style={{ fontFamily: 'monospace', fontSize: 12 }}>{a.contextId ? a.contextId.slice(0, 12) : 'shared'}</td>
                <td style={{ fontSize: 12, color: '#888' }}>{a.fingerprintSummary || '—'}</td>
                <td>{getTokens(a.id)}</td>
                <td>{getCost(a.id)}</td>
                <td style={{ fontSize: 12, color: '#888' }}>{formatBudget(a)}</td>
                <td>
                  {isLiveAgentStatus(a.status) && <button className="btn btn-ghost btn-sm" onClick={() => pause(a.id)} style={{ marginRight: 4 }}>Pause</button>}
                  {isPausedAgentStatus(a.status) && <button className="btn btn-ghost btn-sm" onClick={() => resume(a.id)} style={{ marginRight: 4 }}>Resume</button>}
                  {!isTerminalAgentStatus(a.status) && <button className="btn btn-danger btn-sm" onClick={() => requestKill(a.id)}>Kill</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
