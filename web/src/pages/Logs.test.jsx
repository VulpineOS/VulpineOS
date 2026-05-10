import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Logs from './Logs'

describe('Logs page', () => {
  it('saves retention and exports runtime audit data', async () => {
    const calls = vi.fn(async (method, params) => {
      if (method === 'runtime.list') {
        return {
          events: [{
            id: 'evt-1',
            component: 'gateway',
            event: 'started',
            level: 'info',
            message: 'Gateway started',
            timestamp: '2026-04-22T11:00:00Z',
          }],
          settings: { retention: 200 },
        }
      }
      if (method === 'runtime.setRetention') {
        return { settings: { retention: params.retention } }
      }
      if (method === 'runtime.export') {
        return {
          content: '{"ok":true}',
          contentType: 'application/json',
          fileName: 'runtime-audit.json',
        }
      }
      return {}
    })

    render(
      <Logs
        ws={{
          connected: true,
          call: calls,
          events: [{
            method: 'Vulpine.runtimeEvent',
            params: {
              token: 'panel-token',
              id: 'evt-2',
              component: 'gateway',
              event: 'profile_repair_failed',
              level: 'warn',
              message: 'Gateway profile repair failed',
              panelUrl: 'http://127.0.0.1:8443/?token=panel-token',
              timestamp: '2026-04-22T11:02:00Z',
            },
          }],
        }}
      />,
    )

    expect(await screen.findByText('gateway.started')).toBeInTheDocument()
    expect(screen.getByText(/profile_repair_failed/)).toBeInTheDocument()
    expect(screen.queryByText(/panel-token/)).not.toBeInTheDocument()
    expect(screen.getByText(/"token":"\[redacted\]"/)).toBeInTheDocument()

    fireEvent.change(screen.getByDisplayValue('200'), { target: { value: '150' } })
    fireEvent.click(screen.getByText('Save'))
    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('runtime.setRetention', { retention: 150 })
    })
    expect(screen.getByText('Stored retention: 150 events')).toBeInTheDocument()

    fireEvent.click(screen.getByText('Export JSON'))
    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('runtime.export', expect.objectContaining({ format: 'json' }))
    })
    expect(window.URL.createObjectURL).toHaveBeenCalled()
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalled()
  })

  it('does not refetch runtime audit only because the ws object identity changed', async () => {
    const calls = vi.fn(async (method) => {
      if (method === 'runtime.list') return { events: [], settings: { retention: 200 } }
      return {}
    })
    const { rerender } = render(<Logs ws={{ connected: true, call: calls, events: [] }} />)

    await waitFor(() => {
      expect(calls).toHaveBeenCalledTimes(1)
    })

    rerender(<Logs ws={{ connected: true, call: calls, events: [{ method: 'Browser.telemetryUpdate', params: {} }] }} />)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(calls).toHaveBeenCalledTimes(1)
  })

  it('ingests every runtime audit event appended in one websocket batch', async () => {
    const calls = vi.fn(async (method) => {
      if (method === 'runtime.list') return { events: [], settings: { retention: 200 } }
      return {}
    })
    const { rerender } = render(<Logs ws={{ connected: true, call: calls, events: [] }} />)

    await waitFor(() => {
      expect(calls).toHaveBeenCalledTimes(1)
    })

    rerender(
      <Logs
        ws={{
          connected: true,
          call: calls,
          events: [
            {
              method: 'Vulpine.runtimeEvent',
              params: {
                id: 'evt-1',
                component: 'gateway',
                event: 'started',
                level: 'info',
                message: 'Gateway started',
                timestamp: '2026-04-22T11:00:00Z',
              },
            },
            {
              method: 'Vulpine.runtimeEvent',
              params: {
                id: 'evt-2',
                component: 'gateway',
                event: 'profile_repair_failed',
                level: 'warn',
                message: 'Profile repair failed',
                timestamp: '2026-04-22T11:02:00Z',
              },
            },
          ],
        }}
      />,
    )

    expect(await screen.findByText('gateway.profile_repair_failed')).toBeInTheDocument()
    expect(screen.getByText('gateway.started')).toBeInTheDocument()
  })
})
