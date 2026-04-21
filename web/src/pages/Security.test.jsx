import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Security from './Security'

describe('Security page', () => {
  it('renders runtime-backed protection states instead of hard-coded active badges', async () => {
    const ws = {
      connected: true,
      events: [
        { method: 'Browser.injectionAttemptDetected', params: { url: 'https://example.com', blocked: true }, ts: Date.now() },
      ],
      call: vi.fn(async (method) => {
        if (method === 'security.status') {
          return {
            browserActive: false,
            securityEnabled: true,
            signaturePatternCount: 13,
            sandboxBlockedAPIs: ['fetch', 'WebSocket'],
            protections: [
              { key: 'ax_filter', name: 'Injection-Proof AX Filter', description: 'desc', status: 'disabled', details: 'No browser session.' },
              { key: 'signatures', name: 'Injection Signature Scanner', description: 'desc', status: 'available', details: '13 signatures loaded.' },
            ],
          }
        }
        return {}
      }),
    }

    render(<Security ws={ws} />)

    expect(await screen.findByText('Protection Status')).toBeInTheDocument()
    expect(screen.getByText('suite: enabled')).toBeInTheDocument()
    expect(screen.getByText('disabled')).toBeInTheDocument()
    expect(screen.getByText('available')).toBeInTheDocument()
    expect(screen.getByText('Browser active: No')).toBeInTheDocument()
    expect(screen.getByText('Signature patterns loaded: 13')).toBeInTheDocument()
    expect(screen.getByText(/INJECTION/)).toBeInTheDocument()
  })
})
