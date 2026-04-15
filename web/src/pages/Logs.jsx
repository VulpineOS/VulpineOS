import React, { useEffect, useMemo, useState } from 'react'

export default function Logs({ ws }) {
  const [filter, setFilter] = useState('')
  const [runtimeEvents, setRuntimeEvents] = useState([])
  const events = ws.events || []

  const filtered = filter
    ? events.filter(e => e.method.toLowerCase().includes(filter.toLowerCase()))
    : events

  useEffect(() => {
    if (!ws.connected) return
    ws.call('runtime.list', { limit: 50 })
      .then(result => setRuntimeEvents(result?.events || []))
      .catch(() => {})
  }, [ws.connected])

  useEffect(() => {
    const latest = events[events.length - 1]
    if (!latest || latest.method !== 'Vulpine.runtimeEvent' || !latest.params) return
    setRuntimeEvents(prev => {
      const next = [latest.params, ...prev.filter(event => event.id !== latest.params.id)]
      return next.slice(0, 50)
    })
  }, [events])

  const filteredRuntimeEvents = useMemo(() => (
    filter
      ? runtimeEvents.filter(event => {
          const haystack = [
            event.component,
            event.event,
            event.level,
            event.message,
            JSON.stringify(event.metadata || {}),
          ].join(' ').toLowerCase()
          return haystack.includes(filter.toLowerCase())
        })
      : runtimeEvents
  ), [filter, runtimeEvents])

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
        <h3>Runtime Audit ({filteredRuntimeEvents.length})</h3>
        <div className="event-log" style={{ maxHeight: 320, marginBottom: 24 }}>
          {filteredRuntimeEvents.length === 0 && <p style={{ color: '#666' }}>No persisted runtime events yet.</p>}
          {filteredRuntimeEvents.map((ev) => (
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
                {ev.params ? JSON.stringify(ev.params).substring(0, 100) : ''}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
