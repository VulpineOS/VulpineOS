import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'

class FakeWebSocket {
  static instances = []
  static controlResults = {}
  static autoOpen = true

  constructor(url) {
    this.url = url
    this.readyState = 0
    FakeWebSocket.instances.push(this)
    if (FakeWebSocket.autoOpen) setTimeout(() => this.triggerOpen(), 0)
  }

  send = vi.fn((raw) => {
    const msg = JSON.parse(raw)
    if (msg.type !== 'control') return
    const payload = msg.payload || {}
    const result = FakeWebSocket.controlResults[payload.command] || {}
    queueMicrotask(() => {
      this.onmessage?.({
        data: JSON.stringify({ type: 'control', payload: { id: payload.id, result } }),
      })
    })
  })

  close = vi.fn(() => {
    this.readyState = 3
  })

  triggerOpen() {
    if (this.readyState === 3) return
    this.readyState = 1
    this.onopen?.()
  }

  triggerClose(event = { code: 1006, reason: '' }) {
    this.readyState = 3
    this.onclose?.(event)
  }
}

function makeFetch(status = {}) {
  return vi.fn(async (url) => {
    if (url === '/auth/check') return { ok: true, status: 200 }
    return {
      ok: true,
      json: async () => status,
    }
  })
}

describe('App shell', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    FakeWebSocket.instances = []
    FakeWebSocket.controlResults = {}
    FakeWebSocket.autoOpen = true
    sessionStorage.clear()
    window.history.replaceState({}, '', '/')
  })

  it('revalidates a stored access key and returns to login when rejected', async () => {
    sessionStorage.setItem('vulpine_key', 'stale')
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 401 })))
    vi.stubGlobal('WebSocket', FakeWebSocket)

    render(
      <MemoryRouter>
        <App />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith('/auth/check', {
        headers: { Authorization: 'Bearer stale' },
      })
    })
    expect(await screen.findByPlaceholderText('Access Key')).toBeInTheDocument()
    expect(sessionStorage.getItem('vulpine_key')).toBeNull()
  })

  it('renders an inline not found state for unknown panel routes', async () => {
    sessionStorage.setItem('vulpine_key', 'dev')
    vi.stubGlobal('fetch', makeFetch())
    vi.stubGlobal('WebSocket', FakeWebSocket)

    render(
      <MemoryRouter initialEntries={['/missing']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByText('Page not found')).toBeInTheDocument()
    expect(screen.getByText('Open dashboard')).toBeInTheDocument()
  })

  it('ingests token query params and strips them from browser history', async () => {
    window.history.replaceState({}, '', '/?token=from-query')
    vi.stubGlobal('fetch', makeFetch())
    vi.stubGlobal('WebSocket', FakeWebSocket)
    const replaceState = vi.fn()
    const previousReplaceState = window.history.replaceState
    window.history.replaceState = replaceState

    try {
      render(
        <MemoryRouter initialEntries={['/?token=from-query']}>
          <App />
        </MemoryRouter>,
      )
    } finally {
      window.history.replaceState = previousReplaceState
    }

    expect(sessionStorage.getItem('vulpine_key')).toBe('from-query')
    expect(replaceState).toHaveBeenCalledWith({}, document.title, '/')
    expect(await screen.findByText('VulpineOS')).toBeInTheDocument()
  })

  it('renders runtime state in the shell sidebar', async () => {
    sessionStorage.setItem('vulpine_key', 'dev')
    FakeWebSocket.controlResults = {
      'status.get': {
        browser_route: 'camoufox',
        browser_window: 'visible',
        gateway_running: true,
        kernel_running: true,
        kernel_headless: false,
        active_agents: 3,
      },
    }
    vi.stubGlobal('fetch', makeFetch())
    vi.stubGlobal('WebSocket', FakeWebSocket)

    render(
      <MemoryRouter>
        <App />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getAllByText('CAMOUFOX').length).toBeGreaterThan(0)
    })
    expect(screen.getAllByText('GUI').length).toBeGreaterThan(0)
    expect(screen.getAllByText('VISIBLE').length).toBeGreaterThan(0)
    expect(screen.getAllByText('RUNNING').length).toBeGreaterThan(0)
  })

  it('renders a degraded runtime banner from status.get', async () => {
    sessionStorage.setItem('vulpine_key', 'dev')
    FakeWebSocket.controlResults = {
      'status.get': {
        degraded: true,
        degraded_reasons: [{
          component: 'vault',
          level: 'error',
          message: 'vault unavailable: permission denied',
        }],
        vault_available: false,
      },
    }
    vi.stubGlobal('fetch', makeFetch())
    vi.stubGlobal('WebSocket', FakeWebSocket)

    render(
      <MemoryRouter>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByText('Runtime degraded')).toBeInTheDocument()
    expect(screen.getByText('vault unavailable: permission denied')).toBeInTheDocument()
  })

  it('clears stored access keys when the websocket rejects auth', async () => {
    sessionStorage.setItem('vulpine_key', 'stale')
    vi.stubGlobal('fetch', makeFetch())
    vi.stubGlobal('WebSocket', FakeWebSocket)

    render(
      <MemoryRouter>
        <App />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(FakeWebSocket.instances).toHaveLength(1)
    })

    FakeWebSocket.instances[0].triggerClose({ code: 1008, reason: '' })

    expect(await screen.findByPlaceholderText('Access Key')).toBeInTheDocument()
    expect(sessionStorage.getItem('vulpine_key')).toBeNull()
  })

  it('revalidates the access key after websocket handshake failure', async () => {
    sessionStorage.setItem('vulpine_key', 'stale')
    FakeWebSocket.autoOpen = false
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: true, status: 200 })
      .mockResolvedValueOnce({ ok: false, status: 401 })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('WebSocket', FakeWebSocket)

    render(
      <MemoryRouter>
        <App />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(FakeWebSocket.instances).toHaveLength(1)
    })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })
    FakeWebSocket.instances[0].triggerClose({ code: 1006, reason: '' })

    expect(await screen.findByPlaceholderText('Access Key')).toBeInTheDocument()
    expect(sessionStorage.getItem('vulpine_key')).toBeNull()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })
  })
})
