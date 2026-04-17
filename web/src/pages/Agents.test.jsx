import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import Agents from './Agents'

function renderPage(ws) {
  return render(
    <MemoryRouter>
      <Agents ws={ws} />
    </MemoryRouter>,
  )
}

describe('Agents page', () => {
  it('updates live status from websocket events and shows bulk controls', async () => {
    const calls = vi.fn(async (method) => {
      if (method === 'agents.list') {
        return {
          agents: [
            { id: 'agent-1', name: 'Agent One', status: 'active', contextId: '', fingerprintSummary: '', totalTokens: 0 },
            { id: 'agent-2', name: 'Agent Two', status: 'paused', contextId: '', fingerprintSummary: '', totalTokens: 0 },
          ],
        }
      }
      if (method === 'costs.getAll') return { usage: [] }
      if (method === 'contexts.list') return { contexts: [] }
      return { status: 'ok' }
    })

    const ws = {
      connected: true,
      events: [],
      call: calls,
    }

    const { rerender } = renderPage(ws)

    expect(await screen.findByText('Agent One')).toBeInTheDocument()
    expect(screen.getByText('Pause Selected')).toBeDisabled()
    expect(screen.getByText('Resume Selected')).toBeDisabled()
    expect(screen.getByText('Kill Selected')).toBeDisabled()

    fireEvent.click(screen.getAllByRole('checkbox')[1])
    expect(screen.getByText('Pause Selected')).not.toBeDisabled()
    expect(screen.getByText('Resume Selected')).not.toBeDisabled()
    expect(screen.getByText('Kill Selected')).not.toBeDisabled()

    rerender(
      <MemoryRouter>
        <Agents
          ws={{
            ...ws,
            events: [
              {
                method: 'Vulpine.agentStatus',
                params: { agentId: 'agent-2', status: 'active', contextId: 'ctx-2', objective: 'Scrape pricing', tokens: 17 },
              },
            ],
          }}
        />
      </MemoryRouter>,
    )

    await waitFor(() => {
      const statusBadges = screen.getAllByText('active')
      expect(statusBadges.length).toBeGreaterThanOrEqual(2)
    })
    expect(screen.getByText('ctx-2')).toBeInTheDocument()
  })

  it('runs selected bulk pause and resume actions', async () => {
    const calls = vi.fn(async (method) => {
      if (method === 'agents.list') {
        return {
          agents: [
            { id: 'agent-1', name: 'Agent One', status: 'active', contextId: '', fingerprintSummary: '', totalTokens: 0 },
            { id: 'agent-2', name: 'Agent Two', status: 'paused', contextId: '', fingerprintSummary: '', totalTokens: 0 },
          ],
        }
      }
      if (method === 'costs.getAll') return { usage: [] }
      if (method === 'contexts.list') return { contexts: [] }
      return { status: 'ok' }
    })

    const ws = {
      connected: true,
      events: [],
      call: calls,
    }

    renderPage(ws)

    expect(await screen.findByText('Agent One')).toBeInTheDocument()

    const checkboxes = screen.getAllByRole('checkbox')
    fireEvent.click(checkboxes[1])
    fireEvent.click(checkboxes[2])

    fireEvent.click(screen.getByText('Pause Selected'))
    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('agents.pause', { agentId: 'agent-1' })
      expect(calls).toHaveBeenCalledWith('agents.pause', { agentId: 'agent-2' })
    })

    fireEvent.click(checkboxes[1])
    fireEvent.click(checkboxes[2])
    fireEvent.click(screen.getByText('Resume Selected'))
    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('agents.resume', { agentId: 'agent-1' })
      expect(calls).toHaveBeenCalledWith('agents.resume', { agentId: 'agent-2' })
    })
  })
})
