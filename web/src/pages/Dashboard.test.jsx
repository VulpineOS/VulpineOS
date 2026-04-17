import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Dashboard from './Dashboard'

describe('Dashboard page', () => {
  it('shows browser route and mode in the kernel card', async () => {
    const ws = {
      connected: true,
      telemetry: { memoryMB: 512 },
      events: [],
      call: vi.fn(async (method) => {
        if (method === 'status.get') {
          return {
            kernel_running: true,
            kernel_pid: 1234,
            browser_route: 'camoufox',
            browser_route_source: 'runtime',
            browser_window: 'hidden',
            kernel_headless: false,
          }
        }
        return {}
      }),
    }

    render(<Dashboard ws={ws} />)

    expect(await screen.findByText('CAMOUFOX (runtime) · GUI')).toBeInTheDocument()
    expect(screen.getByText('Window: HIDDEN')).toBeInTheDocument()
    expect(screen.getByText('PID 1234 · 512MB')).toBeInTheDocument()
  })
})
