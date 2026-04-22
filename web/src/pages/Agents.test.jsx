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
            { id: 'agent-1', name: 'Agent One', status: 'active', contextId: '', fingerprintSummary: '', totalTokens: 0, budgetSource: 'agent', budgetMaxCostUsd: 1.5, budgetMaxTokens: 5000 },
            { id: 'agent-2', name: 'Agent Two', status: 'paused', contextId: '', fingerprintSummary: '', totalTokens: 0, budgetSource: 'default', budgetMaxCostUsd: 2.5, budgetMaxTokens: 10000 },
          ],
        }
      }
      if (method === 'costs.getAll') {
        return {
          usage: [
            { agentId: 'agent-1', totalTokens: 1200, estimatedCost: 0.4 },
            { agentId: 'agent-2', totalTokens: 800, estimatedCost: 0.2 },
          ],
          defaults: { maxCostUsd: 2.5, maxTokens: 10000 },
        }
      }
      if (method === 'costs.total') return { totalCostUsd: 0.6 }
      if (method === 'contexts.list') return { contexts: [] }
      if (method === 'agents.pauseMany') return { status: 'ok', paused: 2, failures: {} }
      if (method === 'agents.resumeMany') return { status: 'ok', resumed: 2, failures: {} }
      return { status: 'ok' }
    })

    const ws = {
      connected: true,
      events: [],
      call: calls,
    }

    const { rerender } = renderPage(ws)

    expect(await screen.findByText('Agent One')).toBeInTheDocument()
    expect(screen.getByText('$0.6000')).toBeInTheDocument()
    expect(screen.getByText('2,000')).toBeInTheDocument()
    expect(screen.getByText('Override · $1.50 · 5,000 tok')).toBeInTheDocument()
    expect(screen.getByText('Default · $2.50 · 10,000 tok')).toBeInTheDocument()
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
            { id: 'agent-1', name: 'Agent One', status: 'active', contextId: '', fingerprintSummary: '', totalTokens: 0, budgetSource: 'agent', budgetMaxCostUsd: 1.5, budgetMaxTokens: 5000 },
            { id: 'agent-2', name: 'Agent Two', status: 'paused', contextId: '', fingerprintSummary: '', totalTokens: 0, budgetSource: 'default', budgetMaxCostUsd: 2.5, budgetMaxTokens: 10000 },
          ],
        }
      }
      if (method === 'costs.getAll') return { usage: [], defaults: { maxCostUsd: 2.5, maxTokens: 10000 } }
      if (method === 'costs.total') return { totalCostUsd: 0.0 }
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

    let checkboxes = screen.getAllByRole('checkbox')
    fireEvent.click(checkboxes[1])
    fireEvent.click(checkboxes[2])

    fireEvent.click(screen.getByText('Pause Selected'))
    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('agents.pauseMany', { agentIds: ['agent-1', 'agent-2'] })
    })
    expect(screen.getByText('Paused 2 agents')).toBeInTheDocument()

    checkboxes = screen.getAllByRole('checkbox')
    fireEvent.click(checkboxes[1])
    fireEvent.click(checkboxes[2])
    fireEvent.click(screen.getByText('Resume Selected'))
    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('agents.resumeMany', { agentIds: ['agent-1', 'agent-2'] })
    })
    expect(screen.getByText('Resumed 2 agents')).toBeInTheDocument()
  })
})
