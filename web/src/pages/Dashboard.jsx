import React, { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'

function levelTone(level) {
  if (level === 'error') return 'red'
  if (level === 'warn' || level === 'warning') return 'yellow'
  if (level === 'info') return 'blue'
  return 'gray'
}

function formatRuntimeMeta(event) {
  const parts = []
  if (event?.component) parts.push(event.component)
  if (event?.event) parts.push(event.event.replaceAll('_', ' '))
  return parts.join(' · ')
}

export default function Dashboard({ ws }) {
  const [status, setStatus] = useState(null)
  const [agents, setAgents] = useState([])
  const [runtimeEvents, setRuntimeEvents] = useState([])
  const [costSummary, setCostSummary] = useState({
    totalCostUsd: 0,
    totalTokens: 0,
    trackedUsage: 0,
    overrideCount: 0,
    defaultMaxCostUsd: 0,
    defaultMaxTokens: 0,
  })

  useEffect(() => {
    if (!ws.connected) return undefined
    let cancelled = false
    const refresh = async () => {
      try {
        const [nextStatus, nextAgents, nextRuntime, nextTotal, nextCosts] = await Promise.all([
          ws.call('status.get'),
          ws.call('agents.list'),
          ws.call('runtime.list', { limit: 12 }),
          ws.call('costs.total'),
          ws.call('costs.getAll'),
        ])
        if (cancelled) return
        const agentList = nextAgents?.agents || []
        const usage = nextCosts?.usage || []
        const defaults = nextCosts?.defaults || {}
        setStatus(nextStatus || {})
        setAgents(agentList)
        setRuntimeEvents(nextRuntime?.events || [])
        setCostSummary({
          totalCostUsd: nextTotal?.totalCostUsd ?? nextStatus?.total_cost_usd ?? 0,
          totalTokens: usage.reduce((sum, entry) => sum + (entry.totalTokens || 0), 0),
          trackedUsage: usage.length,
          overrideCount: agentList.filter(agent => agent.budgetSource === 'agent').length,
          defaultMaxCostUsd: defaults.maxCostUsd ?? 0,
          defaultMaxTokens: defaults.maxTokens ?? 0,
        })
      } catch {
        if (!cancelled) {
          setStatus(current => current || {})
        }
      }
    }
    refresh()
    const interval = setInterval(refresh, 5000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [ws.connected])

  const t = ws.telemetry || {}
  const s = status || {}
  const kernelMode = s.kernel_running ? (s.kernel_headless ? 'HEADLESS' : 'GUI') : 'DISABLED'
  const statusBadge = ws.connectionState === 'connected'
    ? ['badge-green', 'Connected']
    : ws.connectionState === 'reconnecting'
      ? ['badge-yellow', 'Reconnecting']
      : ws.connectionState === 'connecting'
        ? ['badge-blue', 'Connecting']
        : ['badge-red', 'Failed']

  const alertEvents = useMemo(
    () => runtimeEvents.filter(event => event.level === 'warn' || event.level === 'warning' || event.level === 'error'),
    [runtimeEvents],
  )

  const activeAgents = useMemo(() => {
    const order = { active: 0, paused: 1, error: 2, completed: 3 }
    return [...agents]
      .sort((a, b) => {
        const rankDelta = (order[a.status] ?? 99) - (order[b.status] ?? 99)
        if (rankDelta !== 0) return rankDelta
        return (b.totalTokens || 0) - (a.totalTokens || 0)
      })
      .slice(0, 6)
  }, [agents])

  const defaultBudgetLabel = useMemo(() => {
    const parts = []
    if (costSummary.defaultMaxCostUsd > 0) parts.push(`$${costSummary.defaultMaxCostUsd.toFixed(2)}`)
    if (costSummary.defaultMaxTokens > 0) parts.push(`${costSummary.defaultMaxTokens.toLocaleString()} tok`)
    return parts.length > 0 ? parts.join(' · ') : 'Unlimited by default'
  }, [costSummary.defaultMaxCostUsd, costSummary.defaultMaxTokens])

  const quickActions = [
    { to: '/agents', title: 'Review agents', detail: `${s.active_agents || 0} active · ${agents.length || 0} tracked` },
    { to: '/bus', title: 'Check approvals', detail: alertEvents.length ? `${alertEvents.length} runtime alerts need triage` : 'No recent runtime warnings' },
    { to: '/logs', title: 'Inspect runtime log', detail: `${runtimeEvents.length} recent retained events` },
    { to: '/settings', title: 'Check configuration', detail: `${(s.browser_route || 'unknown').toUpperCase()} · ${kernelMode}` },
  ]

  return (
    <div>
      <div className="page-header">
        <div>
          <h1>Dashboard</h1>
          <p className="page-subtitle">Runtime state, active work, and recent operator signals.</p>
        </div>
        <span className={`badge ${statusBadge[0]}`}>
          {statusBadge[1]}
        </span>
      </div>

      <div className="grid grid-4">
        <div className="card metric-card">
          <div className="metric-kicker">Fleet</div>
          <div className="stat-value">{s.active_agents || 0}</div>
          <div className="metric-caption">{agents.length || 0} tracked agents · {s.pool_active || 0}/{s.pool_total || 0} contexts active</div>
        </div>
        <div className="card metric-card">
          <div className="metric-kicker">Browser route</div>
          <div className="stat-value">{(s.browser_route || 'unknown').toUpperCase()}</div>
          <div className="metric-caption">
            {s.browser_route_source ? `${s.browser_route_source} source` : 'source unknown'} · {kernelMode} · {t.memoryMB ? `${t.memoryMB}MB` : 'memory n/a'}
          </div>
        </div>
        <div className="card metric-card">
          <div className="metric-kicker">Spend</div>
          <div className="stat-value">${(costSummary.totalCostUsd || 0).toFixed(4)}</div>
          <div className="metric-caption">
            {(costSummary.totalTokens || 0).toLocaleString()} tokens · {costSummary.trackedUsage} tracked usage records
          </div>
        </div>
        <div className="card metric-card">
          <div className="metric-kicker">Budget posture</div>
          <div className="stat-value">{costSummary.overrideCount}</div>
          <div className="metric-caption">
            {costSummary.overrideCount === 1 ? '1 agent override' : `${costSummary.overrideCount} agent overrides`} · {defaultBudgetLabel}
          </div>
        </div>
      </div>

      <div className="card">
        <div className="card-header">
          <div>
            <h3>Quick actions</h3>
            <p className="card-subtitle">Shortcuts into the current runtime state.</p>
          </div>
        </div>
        <div className="quick-actions">
          {quickActions.map(action => (
            <Link key={action.to} to={action.to} className="quick-action-card">
              <strong>{action.title}</strong>
              <span>{action.detail}</span>
            </Link>
          ))}
        </div>
      </div>

      <div className="grid grid-2 dashboard-columns">
        <div className="stack">
          <div className="card">
            <div className="card-header">
              <div>
                <h3>Active work</h3>
                <p className="card-subtitle">Most relevant agents by current status and token activity.</p>
              </div>
              <Link className="btn btn-ghost btn-sm" to="/agents">Open agents</Link>
            </div>
            {activeAgents.length === 0 ? (
              <div className="empty-state">No agents are running yet.</div>
            ) : (
              <table className="table">
                <thead>
                  <tr>
                    <th>Agent</th>
                    <th>Status</th>
                    <th>Context</th>
                    <th>Tokens</th>
                  </tr>
                </thead>
                <tbody>
                  {activeAgents.map(agent => (
                    <tr key={agent.id}>
                      <td>
                        <Link to={`/agents/${agent.id}`} className="table-link">
                          {agent.name || agent.id.slice(0, 12)}
                        </Link>
                      </td>
                      <td>
                        <span className={`badge badge-${agent.status === 'active' ? 'green' : agent.status === 'paused' ? 'yellow' : agent.status === 'error' ? 'red' : 'gray'}`}>
                          {agent.status}
                        </span>
                      </td>
                      <td className="mono-cell">{agent.contextId ? agent.contextId.slice(0, 12) : 'shared'}</td>
                      <td>{(agent.totalTokens || 0).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          <div className="card">
            <div className="card-header">
              <div>
                <h3>Environment</h3>
                <p className="card-subtitle">Current browser route, pool, and telemetry summary.</p>
              </div>
            </div>
            <div className="detail-grid">
              <div className="detail-row"><span>Kernel PID</span><strong>{s.kernel_pid || 'N/A'}</strong></div>
              <div className="detail-row"><span>Window</span><strong>{(s.browser_window || 'unknown').toUpperCase()}</strong></div>
              <div className="detail-row"><span>Gateway</span><strong>{s.gateway_running ? 'RUNNING' : 'STOPPED'}</strong></div>
              <div className="detail-row"><span>Sentinel</span><strong>{s.sentinel_available ? (s.sentinel_mode || 'ON').toUpperCase() : 'OFF'}</strong></div>
              <div className="detail-row"><span>Contexts</span><strong>{t.activeContexts || 0}</strong></div>
              <div className="detail-row"><span>Pages</span><strong>{t.activePages || 0}</strong></div>
              <div className="detail-row"><span>Detection risk</span><strong>{t.detectionRiskScore || 0}%</strong></div>
              <div className="detail-row"><span>Pool available</span><strong>{s.pool_available || 0}</strong></div>
              <div className="detail-row"><span>Pool total</span><strong>{s.pool_total || 0}</strong></div>
              <div className="detail-row"><span>Citizens</span><strong>{s.total_citizens || 0}</strong></div>
              <div className="detail-row"><span>Templates</span><strong>{s.total_templates || 0}</strong></div>
              <div className="detail-row"><span>Trust recipes</span><strong>{s.sentinel_trust_recipes || 0}</strong></div>
            </div>
          </div>
        </div>

        <div className="stack">
          <div className="card">
            <div className="card-header">
              <div>
                <h3>Runtime alerts</h3>
                <p className="card-subtitle">Warnings and errors from the retained audit stream.</p>
              </div>
              <span className={`badge ${alertEvents.length > 0 ? 'badge-yellow' : 'badge-green'}`}>
                {alertEvents.length > 0 ? `${alertEvents.length} alerts` : 'Clear'}
              </span>
            </div>
            {alertEvents.length === 0 ? (
              <div className="empty-state">No warnings or errors in the retained runtime audit.</div>
            ) : (
              <div className="runtime-list">
                {alertEvents.slice(0, 6).map(event => (
                  <div key={event.id} className="runtime-item">
                    <span className={`runtime-level runtime-level-${levelTone(event.level)}`}>{event.level}</span>
                    <div className="runtime-copy">
                      <strong>{event.message}</strong>
                      <span>{formatRuntimeMeta(event)}</span>
                    </div>
                    <time>{new Date(event.timestamp).toLocaleTimeString()}</time>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="card">
            <div className="card-header">
              <div>
                <h3>Recent runtime events</h3>
                <p className="card-subtitle">Latest retained lifecycle activity across the runtime.</p>
              </div>
            </div>
            {runtimeEvents.length === 0 ? (
              <div className="empty-state">Waiting for runtime audit events.</div>
            ) : (
              <div className="runtime-list">
                {runtimeEvents.slice(0, 8).map(event => (
                  <div key={event.id} className="runtime-item">
                    <span className={`runtime-level runtime-level-${levelTone(event.level)}`}>{event.level}</span>
                    <div className="runtime-copy">
                      <strong>{event.message}</strong>
                      <span>{formatRuntimeMeta(event)}</span>
                    </div>
                    <time>{new Date(event.timestamp).toLocaleTimeString()}</time>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
