import React from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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
    expect(screen.getByText('Resume Selected')).toBeDisabled()
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

  it('runs selected bulk pause and resume actions only for eligible statuses', async () => {
    const calls = vi.fn(async (method) => {
      if (method === 'agents.list') {
        return {
          agents: [
            { id: 'agent-1', name: 'Agent One', status: 'running', contextId: '', fingerprintSummary: '', totalTokens: 0, budgetSource: 'agent', budgetMaxCostUsd: 1.5, budgetMaxTokens: 5000 },
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
      expect(calls).toHaveBeenCalledWith('agents.pauseMany', { agentIds: ['agent-1'] })
    })
    expect(screen.getByText('Paused 1 agents')).toBeInTheDocument()

    checkboxes = screen.getAllByRole('checkbox')
    fireEvent.click(checkboxes[1])
    fireEvent.click(checkboxes[2])
    fireEvent.click(screen.getByText('Resume Selected'))
    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('agents.resumeMany', { agentIds: ['agent-2'] })
    })
    expect(screen.getByText('Resumed 1 agents')).toBeInTheDocument()

    checkboxes = screen.getAllByRole('checkbox')
    fireEvent.click(checkboxes[1])
    fireEvent.click(checkboxes[2])
    fireEvent.click(screen.getByText('Kill Selected'))
    expect(screen.getByText('Confirm kill')).toBeInTheDocument()
    const confirmBanner = screen.getByText('Confirm kill').closest('.panel-banner')
    fireEvent.click(within(confirmBanner).getByText('Kill'))
    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('agents.killMany', { agentIds: ['agent-1', 'agent-2'] })
    })
    expect(screen.getByText('Killed 2 agents')).toBeInTheDocument()
  })

  it('renders empty browser contexts without crashing', async () => {
    const calls = vi.fn(async (method) => {
      if (method === 'agents.list') return { agents: [] }
      if (method === 'costs.getAll') return { usage: [], defaults: {} }
      if (method === 'costs.total') return { totalCostUsd: 0 }
      if (method === 'contexts.list') {
        return {
          contexts: [
            { id: 'ctx-empty-url' },
          ],
        }
      }
      return {}
    })

    renderPage({ connected: true, events: [], call: calls })

    expect(await screen.findByText('ctx-empty-ur · about:blank')).toBeInTheDocument()
  })

  it('preserves token totals when status events omit usage', async () => {
    const calls = vi.fn(async (method) => {
      if (method === 'agents.list') {
        return {
          agents: [
            { id: 'agent-1', name: 'Agent One', status: 'active', contextId: '', fingerprintSummary: '', totalTokens: 42 },
          ],
        }
      }
      if (method === 'costs.getAll') return { usage: [], defaults: {} }
      if (method === 'costs.total') return { totalCostUsd: 0 }
      if (method === 'contexts.list') return { contexts: [] }
      return { status: 'ok' }
    })
    const ws = { connected: true, events: [], call: calls }
    const { rerender } = renderPage(ws)

    expect(await screen.findByText('Agent One')).toBeInTheDocument()
    rerender(
      <MemoryRouter>
        <Agents
          ws={{
            ...ws,
            events: [
              { method: 'Vulpine.agentStatus', params: { agentId: 'agent-1', status: 'paused', tokens: 0 } },
            ],
          }}
        />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('paused')).toBeInTheDocument())
    expect(screen.getByText('42')).toBeInTheDocument()
  })

  it('ignores repeated websocket status events that would roll state backwards', async () => {
    const calls = vi.fn(async (method) => {
      if (method === 'agents.list') {
        return {
          agents: [
            { id: 'agent-1', name: 'Agent One', status: 'active', contextId: '', fingerprintSummary: '', totalTokens: 90 },
          ],
        }
      }
      if (method === 'costs.getAll') return { usage: [], defaults: {} }
      if (method === 'costs.total') return { totalCostUsd: 0 }
      if (method === 'contexts.list') return { contexts: [] }
      return { status: 'ok' }
    })
    const ws = { connected: true, events: [], call: calls }
    const { rerender } = renderPage(ws)

    expect(await screen.findByText('Agent One')).toBeInTheDocument()
    rerender(
      <MemoryRouter>
        <Agents
          ws={{
            ...ws,
            events: [
              { seq: 1, method: 'Vulpine.agentStatus', params: { agentId: 'agent-1', status: 'running', tokens: 120 } },
            ],
          }}
        />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('running')).toBeInTheDocument())
    expect(screen.getByText('120')).toBeInTheDocument()

    rerender(
      <MemoryRouter>
        <Agents
          ws={{
            ...ws,
            events: [
              { seq: 1, method: 'Vulpine.agentStatus', params: { agentId: 'agent-1', status: 'paused', tokens: 10 } },
            ],
          }}
        />
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('running')).toBeInTheDocument())
    expect(screen.queryByText('paused')).not.toBeInTheDocument()
    expect(screen.getByText('120')).toBeInTheDocument()
  })

  it('does not offer kill actions for terminal agents', async () => {
    const calls = vi.fn(async (method) => {
      if (method === 'agents.list') {
        return {
          agents: [
            { id: 'agent-active', name: 'Active Agent', status: 'active', contextId: '', fingerprintSummary: '', totalTokens: 0 },
            { id: 'agent-done', name: 'Done Agent', status: 'interrupted', contextId: '', fingerprintSummary: '', totalTokens: 0 },
          ],
        }
      }
      if (method === 'costs.getAll') return { usage: [], defaults: {} }
      if (method === 'costs.total') return { totalCostUsd: 0 }
      if (method === 'contexts.list') return { contexts: [] }
      return {}
    })

    renderPage({ connected: true, events: [], call: calls })

    const doneRow = (await screen.findByText('Done Agent')).closest('tr')
    expect(within(doneRow).queryByText('Kill')).not.toBeInTheDocument()

    const checkboxes = screen.getAllByRole('checkbox')
    fireEvent.click(checkboxes[2])
    expect(screen.getByText('Kill Selected')).toBeDisabled()

    fireEvent.click(checkboxes[1])
    expect(screen.getByText('Kill Selected')).not.toBeDisabled()
    fireEvent.click(screen.getByText('Kill Selected'))
    expect(screen.getByText('Confirm kill')).toBeInTheDocument()
    const confirmBanner = screen.getByText('Confirm kill').closest('.panel-banner')
    fireEvent.click(within(confirmBanner).getByText('Kill'))
    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('agents.killMany', { agentIds: ['agent-active'] })
    })
  })

  it('shows pause controls for live non-active statuses', async () => {
    const calls = vi.fn(async (method) => {
      if (method === 'agents.list') {
        return {
          agents: [
            { id: 'agent-thinking', name: 'Thinking Agent', status: 'thinking', contextId: '', fingerprintSummary: '', totalTokens: 0 },
          ],
        }
      }
      if (method === 'costs.getAll') return { usage: [], defaults: {} }
      if (method === 'costs.total') return { totalCostUsd: 0 }
      if (method === 'contexts.list') return { contexts: [] }
      return {}
    })

    renderPage({ connected: true, events: [], call: calls })

    const row = (await screen.findByText('Thinking Agent')).closest('tr')
    expect(within(row).getByText('thinking')).toBeInTheDocument()
    expect(within(row).getByText('Pause')).toBeInTheDocument()
    expect(within(row).queryByText('Resume')).not.toBeInTheDocument()
  })
})
