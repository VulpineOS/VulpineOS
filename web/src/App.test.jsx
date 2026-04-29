import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'

class FakeWebSocket {
  constructor(url) {
    this.url = url
  }

  close = vi.fn()
}

describe('App shell', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
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
})
