import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Settings from './Settings'

describe('Settings page', () => {
  it('shows browser route and mode in the about card', async () => {
    const ws = {
      connected: true,
      call: vi.fn(async (method) => {
        if (method === 'config.get') {
          return {
            provider: 'anthropic',
            model: 'claude-sonnet-4-6',
            setupComplete: true,
          }
        }
        if (method === 'status.get') {
          return {
            browser_route: 'camoufox',
            browser_route_source: 'runtime',
            browser_window: 'hidden',
            kernel_headless: false,
          }
        }
        return {}
      }),
    }

    render(<Settings ws={ws} />)

    expect(await screen.findByText('Route: CAMOUFOX (runtime) · GUI')).toBeInTheDocument()
    expect(screen.getByText('Window: HIDDEN')).toBeInTheDocument()
    expect(screen.getByText('OpenClaw integration: Configured')).toBeInTheDocument()
  })
})
