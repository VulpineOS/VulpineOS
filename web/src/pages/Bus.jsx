import React, { useEffect, useMemo, useState } from 'react'

export default function Bus({ ws }) {
  const [pending, setPending] = useState([])
  const [policies, setPolicies] = useState([])
  const [agents, setAgents] = useState([])
  const [draft, setDraft] = useState({ fromAgent: '*', toAgent: '*', autoApprove: true })

  const agentNames = useMemo(() => {
    const out = {}
    for (const agent of agents) out[agent.id] = agent.name || agent.id
    return out
  }, [agents])

  const load = async () => {
    if (!ws.connected) return
    try {
      const [pendingResult, policyResult, agentResult] = await Promise.all([
        ws.call('bus.pending'),
        ws.call('bus.policies'),
        ws.call('agents.list'),
      ])
      setPending(pendingResult || [])
      setPolicies(policyResult || [])
      setAgents(agentResult?.agents || [])
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  useEffect(() => {
    if (ws.connected) load()
  }, [ws.connected])

  const labelFor = (agentID) => {
    if (!agentID || agentID === '*') return 'Any agent'
    return agentNames[agentID] || agentID
  }

  const approve = async (messageId) => {
    try {
      await ws.call('bus.approve', { messageId })
      ws.notify?.('Message approved', 'success')
      load()
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  const reject = async (messageId) => {
    try {
      await ws.call('bus.reject', { messageId })
      ws.notify?.('Message rejected', 'success')
      load()
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  const savePolicy = async () => {
    try {
      await ws.call('bus.addPolicy', draft)
      ws.notify?.('Policy saved', 'success')
      load()
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  const removePolicy = async (policy) => {
    try {
      await ws.call('bus.removePolicy', { fromAgent: policy.fromAgent, toAgent: policy.toAgent })
      ws.notify?.('Policy removed', 'success')
      load()
    } catch (e) {
      ws.notify?.(e.message)
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Agent Bus</h1>
        <button className="btn btn-ghost" onClick={load}>Refresh</button>
      </div>

      <div className="grid grid-2">
        <div className="card">
          <div className="page-header" style={{ marginBottom: 12 }}>
            <h3 style={{ margin: 0 }}>Pending approvals ({pending.length})</h3>
          </div>
          {pending.length === 0 && <p style={{ color: '#666' }}>No pending agent-to-agent messages.</p>}
          {pending.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {pending.map(message => (
                <div key={message.id} style={{ border: '1px solid #2a2a2a', borderRadius: 8, padding: 12 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, marginBottom: 8 }}>
                    <div style={{ fontSize: 12, color: '#aaa' }}>
                      <strong>{message.type}</strong>{' '}
                      {labelFor(message.fromAgent)} {'->'} {labelFor(message.toAgent)}
                    </div>
                    <div style={{ fontSize: 11, color: '#666' }}>
                      {message.createdAt ? new Date(message.createdAt).toLocaleString() : ''}
                    </div>
                  </div>
                  <div style={{ whiteSpace: 'pre-wrap', color: '#ccc', fontSize: 13, marginBottom: 12 }}>{message.content}</div>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <button className="btn btn-primary" onClick={() => approve(message.id)}>Approve</button>
                    <button className="btn btn-ghost" onClick={() => reject(message.id)}>Reject</button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="card">
          <div className="page-header" style={{ marginBottom: 12 }}>
            <h3 style={{ margin: 0 }}>Policies ({policies.length})</h3>
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr auto auto', gap: 8, marginBottom: 16 }}>
            <select className="input" value={draft.fromAgent} onChange={e => setDraft(current => ({ ...current, fromAgent: e.target.value }))}>
              <option value="*">Any sender</option>
              {agents.map(agent => <option key={agent.id} value={agent.id}>{agent.name || agent.id}</option>)}
            </select>
            <select className="input" value={draft.toAgent} onChange={e => setDraft(current => ({ ...current, toAgent: e.target.value }))}>
              <option value="*">Any receiver</option>
              {agents.map(agent => <option key={agent.id} value={agent.id}>{agent.name || agent.id}</option>)}
            </select>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: '#aaa' }}>
              <input
                type="checkbox"
                checked={draft.autoApprove}
                onChange={e => setDraft(current => ({ ...current, autoApprove: e.target.checked }))}
              />
              Auto approve
            </label>
            <button className="btn btn-primary" onClick={savePolicy}>Save</button>
          </div>

          <table className="table">
            <thead>
              <tr>
                <th>From</th>
                <th>To</th>
                <th>Mode</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {policies.length === 0 && (
                <tr>
                  <td colSpan="4" style={{ textAlign: 'center', color: '#666', padding: 24 }}>No policies configured.</td>
                </tr>
              )}
              {policies.map((policy, index) => (
                <tr key={`${policy.fromAgent}-${policy.toAgent}-${index}`}>
                  <td>{labelFor(policy.fromAgent)}</td>
                  <td>{labelFor(policy.toAgent)}</td>
                  <td>{policy.autoApprove ? 'Auto approve' : 'Manual approval'}</td>
                  <td>
                    <button className="btn btn-ghost btn-sm" onClick={() => removePolicy(policy)}>Remove</button>
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
