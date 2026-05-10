import React, { useEffect, useMemo, useRef, useState } from 'react'
import { redactSensitiveText } from '../utils/redact'

export default function Logs({ ws }) {
  const [filter, setFilter] = useState('')
  const [runtimeEvents, setRuntimeEvents] = useState([])
  const [runtimeSettings, setRuntimeSettings] = useState({ retention: 200 })
  const [runtimeFilter, setRuntimeFilter] = useState({ query: '', component: '', level: '', event: '', limit: 50 })
  const [retentionInput, setRetentionInput] = useState('200')
  const processedEventCount = useRef(0)
  const { connected, call } = ws
  const events = ws.events || []

  const filtered = filter
    ? events.filter(e => e.method.toLowerCase().includes(filter.toLowerCase()))
    : events

  useEffect(() => {
    if (!connected) return
    call('runtime.list', runtimeFilter)
      .then(result => {
        setRuntimeEvents(result?.events || [])
        setRuntimeSettings(result?.settings || { retention: 200 })
        setRetentionInput(String(result?.settings?.retention || 200))
      })
      .catch(() => {})
  }, [call, connected, runtimeFilter])

  useEffect(() => {
    if (events.length < processedEventCount.current) {
      processedEventCount.current = 0
    }
    const appended = events.slice(processedEventCount.current)
    processedEventCount.current = events.length
    const incoming = appended
      .filter(event => event.method === 'Vulpine.runtimeEvent' && event.params)
      .map(event => event.params)
    if (incoming.length === 0) return

    const query = runtimeFilter.query.trim().toLowerCase()
    const matchesFilter = event => {
      const haystack = [
        event.component,
        event.event,
        event.level,
        event.message,
        JSON.stringify(event.metadata || {}),
      ].join(' ').toLowerCase()
      if (runtimeFilter.component && event.component !== runtimeFilter.component) return false
      if (runtimeFilter.event && event.event !== runtimeFilter.event) return false
      if (runtimeFilter.level && event.level !== runtimeFilter.level) return false
      if (query && !haystack.includes(query)) return false
      return true
    }
    const matching = incoming.filter(matchesFilter)
    if (matching.length === 0) return

    setRuntimeEvents(prev => {
      const seen = new Set()
      const next = []
      for (const event of [...matching].reverse()) {
        const key = event.id || `${event.component}-${event.event}-${event.timestamp}`
        if (seen.has(key)) continue
        seen.add(key)
        next.push(event)
      }
      for (const event of prev) {
        const key = event.id || `${event.component}-${event.event}-${event.timestamp}`
        if (seen.has(key)) continue
        seen.add(key)
        next.push(event)
      }
      return next.slice(0, runtimeFilter.limit || 50)
    })
  }, [events, runtimeFilter])

  const runtimeComponents = useMemo(() => (
    [...new Set(runtimeEvents.map(event => event.component).filter(Boolean))].sort()
  ), [runtimeEvents])

  const runtimeEventNames = useMemo(() => (
    [...new Set(runtimeEvents.map(event => event.event).filter(Boolean))].sort()
  ), [runtimeEvents])

  async function saveRetention() {
    const retention = Number(retentionInput)
    if (!Number.isFinite(retention) || retention <= 0) return
    const result = await call('runtime.setRetention', { retention })
    setRuntimeSettings(result?.settings || { retention })
    setRetentionInput(String(result?.settings?.retention || retention))
    setRuntimeFilter(current => ({ ...current }))
  }

  async function downloadRuntimeExport(format) {
    const result = await call('runtime.export', { ...runtimeFilter, format })
    const blob = new Blob([result?.content || ''], { type: result?.contentType || 'application/json' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = result?.fileName || `runtime-audit.${format === 'ndjson' ? 'ndjson' : 'json'}`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  }

  return (
    <div>
      <div className="page-header">
        <h1>Event Log</h1>
        <input
          className="input"
          style={{ width: 250 }}
          placeholder="Filter events..."
          value={filter}
          onChange={e => setFilter(e.target.value)}
        />
      </div>

      <div className="card">
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center', marginBottom: 12 }}>
          <h3 style={{ margin: 0 }}>Runtime Audit ({runtimeEvents.length})</h3>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <span style={{ fontSize: 12, color: '#666' }}>Retention</span>
            <input
              className="input"
              style={{ width: 90 }}
              value={retentionInput}
              onChange={e => setRetentionInput(e.target.value)}
            />
            <button className="btn btn-secondary" onClick={saveRetention}>Save</button>
            <button className="btn btn-secondary" onClick={() => downloadRuntimeExport('json')}>Export JSON</button>
            <button className="btn btn-secondary" onClick={() => downloadRuntimeExport('ndjson')}>Export NDJSON</button>
          </div>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '1.3fr 1fr 1fr 120px', gap: 12, marginBottom: 16 }}>
          <input
            className="input"
            placeholder="Search runtime audit..."
            value={runtimeFilter.query}
            onChange={e => setRuntimeFilter(current => ({ ...current, query: e.target.value }))}
          />
          <select
            className="input"
            value={runtimeFilter.component}
            onChange={e => setRuntimeFilter(current => ({ ...current, component: e.target.value }))}
          >
            <option value="">All components</option>
            {runtimeComponents.map(component => <option key={component} value={component}>{component}</option>)}
          </select>
          <select
            className="input"
            value={runtimeFilter.event}
            onChange={e => setRuntimeFilter(current => ({ ...current, event: e.target.value }))}
          >
            <option value="">All events</option>
            {runtimeEventNames.map(event => <option key={event} value={event}>{event}</option>)}
          </select>
          <select
            className="input"
            value={runtimeFilter.level}
            onChange={e => setRuntimeFilter(current => ({ ...current, level: e.target.value }))}
          >
            <option value="">All levels</option>
            <option value="info">Info</option>
            <option value="warn">Warn</option>
            <option value="error">Error</option>
          </select>
        </div>
        <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
          <button className="btn btn-secondary" onClick={() => setRuntimeFilter(current => ({ ...current, level: '', query: '', event: '', component: '' }))}>All</button>
          <button className="btn btn-secondary" onClick={() => setRuntimeFilter(current => ({ ...current, level: 'warn', query: '', event: '', component: '' }))}>Warn</button>
          <button className="btn btn-secondary" onClick={() => setRuntimeFilter(current => ({ ...current, level: 'error', query: '', event: '', component: '' }))}>Errors</button>
        </div>
        <div className="event-log" style={{ maxHeight: 320, marginBottom: 24 }}>
          {runtimeEvents.length === 0 && <p style={{ color: '#666' }}>No persisted runtime events yet.</p>}
          {runtimeEvents.map((ev) => (
            <div key={ev.id || `${ev.component}-${ev.event}-${ev.timestamp}`} className="event" style={{ display: 'flex', gap: 12 }}>
              <span className="event-time" style={{ minWidth: 80 }}>
                {ev.timestamp ? new Date(ev.timestamp).toLocaleTimeString() : ''}
              </span>
              <span className="event-method" style={{ minWidth: 180 }}>{ev.component}.{ev.event}</span>
              <span style={{ minWidth: 64, color: ev.level === 'error' ? '#b42318' : ev.level === 'warn' ? '#b54708' : '#555', fontSize: 12 }}>
                {ev.level}
              </span>
              <span style={{ color: '#555', fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {ev.message}
              </span>
            </div>
          ))}
        </div>
        <div style={{ fontSize: 12, color: '#666' }}>
          Stored retention: {runtimeSettings.retention} events
        </div>
      </div>

      <div className="card">
        <h3>Events ({filtered.length})</h3>
        <div className="event-log" style={{ maxHeight: 600 }}>
          {filtered.length === 0 && <p style={{ color: '#666' }}>No events. Events appear as the kernel operates.</p>}
          {[...filtered].reverse().map((ev, i) => (
            <div key={i} className="event" style={{ display: 'flex', gap: 12 }}>
              <span className="event-time" style={{ minWidth: 80 }}>
                {new Date(ev.ts).toLocaleTimeString()}
              </span>
              <span className="event-method" style={{ minWidth: 200 }}>{ev.method}</span>
              <span style={{ color: '#555', fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {ev.params ? redactSensitiveText(ev.params).substring(0, 100) : ''}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
