import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Scripts from './Scripts'

describe('Scripts page', () => {
  it('runs scripts through the backend control path and shows captured vars', async () => {
    const ws = {
      connected: true,
      call: vi.fn(async (method, params) => {
        if (method === 'contexts.list') {
          return { contexts: [{ id: 'ctx-1', pages: 1, url: 'about:blank' }] }
        }
        if (method === 'scripts.run') {
          return {
            ok: true,
            contextId: params.contextId || 'ctx-1',
            sessionId: 'sess-1',
            results: [
              { status: 'ok', action: 'navigate', target: 'https://example.com', output: 'ok' },
              { status: 'ok', action: 'extract', target: 'h1', output: 'Welcome' },
            ],
            vars: { heading: 'Welcome' },
          }
        }
        return {}
      }),
    }

    render(<Scripts ws={ws} />)

    fireEvent.click(await screen.findByText('Run Script'))

    await waitFor(() => {
      expect(ws.call).toHaveBeenCalledWith('scripts.run', expect.objectContaining({ contextId: 'ctx-1' }))
    })
    expect(await screen.findByText(/\[ok\] navigate https:\/\/example.com -> ok/)).toBeInTheDocument()
    expect(screen.getByText(/"heading": "Welcome"/)).toBeInTheDocument()
  })
})
