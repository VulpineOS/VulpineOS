import React, { useEffect, useState } from 'react'

const EXAMPLE_SCRIPT = `{
  "steps": [
    {"action": "navigate", "target": "https://example.com"},
    {"action": "wait", "target": "h1"},
    {"action": "extract", "target": "h1", "store": "heading"},
    {"action": "screenshot", "store": "result.png"}
  ]
}`

export default function Scripts({ ws }) {
  const [contexts, setContexts] = useState([])
  const [selectedContext, setSelectedContext] = useState('')
  const [script, setScript] = useState(EXAMPLE_SCRIPT)
  const [output, setOutput] = useState('')
  const [vars, setVars] = useState({})
  const [running, setRunning] = useState(false)

  useEffect(() => {
    if (!ws.connected) return
    ws.call('contexts.list')
      .then(result => {
        const nextContexts = result?.contexts || []
        setContexts(nextContexts)
        if (!selectedContext && nextContexts.length > 0) {
          setSelectedContext(nextContexts[0].id)
        }
      })
      .catch(() => {})
  }, [ws.connected])

  const runScript = async () => {
    setRunning(true)
    setOutput('Running...\n')
    setVars({})
    try {
      const result = await ws.call('scripts.run', { script, contextId: selectedContext || '' })
      const lines = (result?.results || []).map(step => {
        const suffix = step.output ? ` -> ${step.output}` : ''
        return `[${step.status}] ${step.action} ${step.target || step.value || ''}${suffix}`
      })
      setOutput(lines.join('\n') + `\n\n${result?.ok ? 'Done.' : `Error: ${result?.error || 'script failed'}`}\n`)
      setVars(result?.vars || {})
      if (result?.contextId && !selectedContext) {
        setSelectedContext(result.contextId)
      }
    } catch (e) {
      setOutput(`Error: ${e.message}\n`)
    }
    setRunning(false)
  }

  return (
    <div>
      <div className="page-header">
        <h1>Scripts</h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <select className="input" value={selectedContext} onChange={e => setSelectedContext(e.target.value)} style={{ minWidth: 220 }}>
            <option value="">Auto context</option>
            {contexts.map(context => (
              <option key={context.id} value={context.id}>{context.id}</option>
            ))}
          </select>
          <button className="btn btn-primary" onClick={runScript} disabled={running}>
            {running ? 'Running...' : 'Run Script'}
          </button>
        </div>
      </div>

      <div className="grid grid-2">
        <div className="card">
          <h3>Script Editor</h3>
          <textarea
            className="input"
            rows={20}
            value={script}
            onChange={e => setScript(e.target.value)}
            style={{ fontFamily: "'SF Mono', 'Fira Code', monospace", fontSize: 12, resize: 'vertical', lineHeight: 1.6 }}
          />
          <p style={{ fontSize: 11, color: '#555', marginTop: 8 }}>
            Actions: navigate, click, type, wait, extract, screenshot, set, if
          </p>
        </div>
        <div className="card">
          <h3>Execution Output</h3>
          <pre style={{ fontFamily: "'SF Mono', 'Fira Code', monospace", fontSize: 12, color: '#aaa', whiteSpace: 'pre-wrap', minHeight: 240 }}>
            {output || 'Click "Run Script" to execute against a real browser context.'}
          </pre>
          <h3 style={{ marginTop: 16 }}>Variables</h3>
          <pre style={{ fontFamily: "'SF Mono', 'Fira Code', monospace", fontSize: 12, color: '#aaa', whiteSpace: 'pre-wrap', minHeight: 120 }}>
            {Object.keys(vars).length > 0 ? JSON.stringify(vars, null, 2) : 'No variables captured yet.'}
          </pre>
        </div>
      </div>
    </div>
  )
}
