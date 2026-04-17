import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Settings from './Settings'

describe('Settings page', () => {
  it('shows browser route and mode in the about card', async () => {
    const ws = {
      connected: true,
      call: vi.fn(async (method) => {
        if (method === 'config.providers') {
          return {
            providers: [
              {
                id: 'anthropic',
                name: 'Anthropic (Claude)',
                envVar: 'ANTHROPIC_API_KEY',
                defaultModel: 'anthropic/claude-sonnet-4-6',
                models: ['anthropic/claude-sonnet-4-6'],
                needsKey: true,
              },
            ],
          }
        }
        if (method === 'config.get') {
          return {
            provider: 'anthropic',
            model: 'claude-sonnet-4-6',
            hasKey: true,
            setupComplete: true,
          }
        }
        if (method === 'status.get') {
          return {
            browser_route: 'camoufox',
            browser_route_source: 'runtime',
            browser_window: 'hidden',
            gateway_running: true,
            kernel_headless: false,
          }
        }
        return {}
      }),
    }

    render(<Settings ws={ws} />)

    expect(await screen.findByText('Route: CAMOUFOX (runtime) · GUI')).toBeInTheDocument()
    expect(screen.getByText('Window: HIDDEN')).toBeInTheDocument()
    expect(screen.getByText('Gateway: RUNNING')).toBeInTheDocument()
    expect(screen.getByText('OpenClaw integration: Configured')).toBeInTheDocument()
    expect(screen.getByText('API Key (ANTHROPIC_API_KEY)')).toBeInTheDocument()
    expect(screen.getByText('A key is already stored locally. Leave this blank to keep it.')).toBeInTheDocument()
  })
})
