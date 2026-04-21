import React, { useState } from 'react'

export default function Login({ onLogin }) {
  const [key, setKey] = useState('')
  const [error, setError] = useState('')

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!key.trim()) { setError('Access key required'); return }

    try {
      const res = await fetch('/auth/check', {
        headers: {
          Authorization: `Bearer ${key.trim()}`,
        },
      })
      if (res.ok) {
        onLogin(key.trim())
        return
      }
      if (res.status === 401) {
        setError('Access key rejected')
        return
      }
      setError('Server not reachable')
    } catch {
      setError('Server not reachable')
    }
  }

  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: '100vh' }}>
      <div className="card" style={{ width: 400 }}>
        <h2 style={{ color: '#a78bfa', marginBottom: 8 }}>VulpineOS</h2>
        <p style={{ color: '#666', fontSize: 14, marginBottom: 24 }}>
          Enter the server access key configured at startup. If the server was started with
          <code style={{ marginLeft: 4 }}>--api-key</code>, use that value here. The key is stored only for this browser session.
        </p>
        <form onSubmit={handleSubmit}>
          <input
            className="input"
            type="password"
            placeholder="Access Key"
            value={key}
            onChange={e => { setKey(e.target.value); setError('') }}
            autoFocus
          />
          {error && <p style={{ color: '#ef4444', fontSize: 13, marginTop: 8 }}>{error}</p>}
          <button className="btn btn-primary" type="submit" style={{ width: '100%', marginTop: 16 }}>
            Connect
          </button>
        </form>
      </div>
    </div>
  )
}
