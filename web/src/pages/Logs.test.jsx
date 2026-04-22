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
              id: 'evt-2',
              component: 'gateway',
              event: 'profile_repair_failed',
              level: 'warn',
              message: 'Gateway profile repair failed',
              timestamp: '2026-04-22T11:02:00Z',
            },
          }],
        }}
      />,
    )

    expect(await screen.findByText('gateway.started')).toBeInTheDocument()
    expect(screen.getByText(/profile_repair_failed/)).toBeInTheDocument()

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
})
