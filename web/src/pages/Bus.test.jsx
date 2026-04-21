import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Bus from './Bus'

describe('Bus page', () => {
  it('loads pending messages and policies and can approve and remove rules', async () => {
    const ws = {
      connected: true,
      notify: vi.fn(),
      call: vi.fn(async (method) => {
        if (method === 'bus.pending') {
          return [
            {
              id: 'msg-1',
              type: 'delegate',
              fromAgent: 'agent-1',
              toAgent: 'agent-2',
              content: 'Check the checkout flow',
              createdAt: '2026-04-21T00:00:00Z',
            },
          ]
        }
        if (method === 'bus.policies') {
          return [{ fromAgent: 'agent-1', toAgent: 'agent-2', autoApprove: true }]
        }
        if (method === 'agents.list') {
          return { agents: [{ id: 'agent-1', name: 'Alpha' }, { id: 'agent-2', name: 'Beta' }] }
        }
        return { status: 'ok' }
      }),
    }

    render(<Bus ws={ws} />)

    expect(await screen.findByText('Check the checkout flow')).toBeInTheDocument()
    expect(screen.getAllByText('Auto approve').length).toBeGreaterThan(0)

    fireEvent.click(screen.getByText('Approve'))
    await waitFor(() => {
      expect(ws.call).toHaveBeenCalledWith('bus.approve', { messageId: 'msg-1' })
    })

    fireEvent.click(screen.getByText('Remove'))
    await waitFor(() => {
      expect(ws.call).toHaveBeenCalledWith('bus.removePolicy', { fromAgent: 'agent-1', toAgent: 'agent-2' })
    })
  })
})
