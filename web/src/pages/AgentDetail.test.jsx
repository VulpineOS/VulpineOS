import React from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import AgentDetail from './AgentDetail'

function renderDetail(ws) {
  return render(
    <MemoryRouter initialEntries={['/agents/agent-1']}>
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail ws={ws} />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('AgentDetail page', () => {
  it('appends live conversation events, exposes trace entries, and exposes resume controls', async () => {
    const call = vi.fn(async (method) => {
      if (method === 'agents.list') {
        return { agents: [{ id: 'agent-1', name: 'Agent One', status: 'paused', contextId: '', totalTokens: 0, budgetMaxCostUsd: 1.5, budgetMaxTokens: 5000, budgetSource: 'default' }] }
      }
      if (method === 'agents.getMessages') {
        return {
          messages: [
            { role: 'user', content: 'hello' },
            { role: 'system', content: 'Running browser open https://example.com' },
            { role: 'system', content: 'Thinking: Inspecting the loaded page state' },
            { role: 'system', content: 'Tool incomplete: browser click button.buy — target became detached before click completed' },
          ],
        }
      }
      if (method === 'recording.getTimeline') {
        return { actions: [] }
      }
      if (method === 'agents.getSessionLog') {
        return { content: '{"type":"message","message":{"role":"assistant"}}\n', truncated: true, bytes: 2048, totalBytes: 4096 }
      }
      if (method === 'fingerprints.get') {
        return {}
      }
      if (method === 'recording.export') {
        return { content: '{"events":[]}', fileName: 'agent-agent-1-recording.json', contentType: 'application/json' }
      }
      return { status: 'ok' }
    })

    const ws = { connected: true, events: [], call }
    const { rerender } = renderDetail(ws)

    expect(await screen.findByText('hello')).toBeInTheDocument()
    expect(screen.getByText('paused')).toBeInTheDocument()
    expect(screen.getByText('Resume')).toBeInTheDocument()

    rerender(
      <MemoryRouter initialEntries={['/agents/agent-1']}>
        <Routes>
          <Route
            path="/agents/:id"
            element={
              <AgentDetail
                ws={{
                  ...ws,
                  events: [
                    { method: 'Vulpine.agentStatus', params: { agentId: 'agent-1', status: 'active', tokens: 12 } },
                    { method: 'Vulpine.conversation', params: { agentId: 'agent-1', role: 'assistant', content: 'done', tokens: 4 } },
                  ],
                }}
              />
            }
          />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('done')).toBeInTheDocument())
    expect(screen.getByText('active')).toBeInTheDocument()
    expect(screen.queryByText('Running browser action: open https://example.com')).not.toBeInTheDocument()
    expect(screen.getAllByText('done')).toHaveLength(1)

    rerender(
      <MemoryRouter initialEntries={['/agents/agent-1']}>
        <Routes>
          <Route
            path="/agents/:id"
            element={
              <AgentDetail
                ws={{
                  ...ws,
                  events: [
                    { method: 'Vulpine.agentStatus', params: { agentId: 'agent-1', status: 'active', tokens: 12 } },
                    { method: 'Vulpine.conversation', params: { agentId: 'agent-1', role: 'assistant', content: 'done', tokens: 4 } },
                  ],
                }}
              />
            }
          />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getAllByText('done')).toHaveLength(1))

    fireEvent.click(screen.getByText('Trace'))
    expect(screen.getByText('Action Trace')).toBeInTheDocument()
    expect(screen.getByText('Running browser open https://example.com')).toBeInTheDocument()
    expect(screen.getByText('Thinking: Inspecting the loaded page state')).toBeInTheDocument()
    expect(screen.getByText('Tool incomplete: browser click button.buy — target became detached before click completed')).toBeInTheDocument()
    expect(screen.getByText('RUN')).toBeInTheDocument()
    expect(screen.getByText('THINK')).toBeInTheDocument()
    expect(screen.getByText('PARTIAL')).toBeInTheDocument()

    fireEvent.click(screen.getByText('Raw'))
    expect(await screen.findByText('Raw Session Log')).toBeInTheDocument()
    expect(screen.getByText('Auto-refreshing')).toBeInTheDocument()
    expect(screen.getByText(/Showing the latest 2.0 KB of a 4.0 KB sanitized log/)).toBeInTheDocument()
    expect(screen.getByText('{"type":"message","message":{"role":"assistant"}}')).toBeInTheDocument()

    fireEvent.click(screen.getByText('Conversation'))
    fireEvent.click(screen.getByText('Save Budget'))
    await waitFor(() => {
      expect(call).toHaveBeenCalledWith('costs.setBudget', { agentId: 'agent-1', inheritDefault: true })
    })
    fireEvent.change(screen.getByPlaceholderText('Send message to agent...'), { target: { value: 'continue' } })
    fireEvent.click(screen.getByText('Send'))

    await waitFor(() => {
      expect(call).toHaveBeenCalledWith('agents.resume', { agentId: 'agent-1', message: 'continue' })
    })
    await waitFor(() => {
      expect(screen.getByText('continue')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByText('Recording'))
    fireEvent.click(screen.getByText('Export JSON'))
    await waitFor(() => {
      expect(call).toHaveBeenCalledWith('recording.export', { agentId: 'agent-1' })
    })

    fireEvent.click(screen.getByText('Fingerprint'))
    fireEvent.click(screen.getByText('Regenerate & Apply'))
    await waitFor(() => {
      expect(call).toHaveBeenCalledWith('fingerprints.generate', expect.objectContaining({ agentId: 'agent-1' }))
    })
  })

  it('continues consuming websocket events after the event buffer is capped', async () => {
    const call = vi.fn(async (method) => {
      if (method === 'agents.list') {
        return { agents: [{ id: 'agent-1', name: 'Agent One', status: 'paused', contextId: '', totalTokens: 0 }] }
      }
      if (method === 'agents.getMessages') return { messages: [] }
      if (method === 'recording.getTimeline') return { actions: [] }
      if (method === 'fingerprints.get') return {}
      return { status: 'ok' }
    })

    const baseEvents = Array.from({ length: 200 }, (_, index) => ({
      seq: index + 1,
      method: 'Browser.telemetryUpdate',
      params: { activePages: index },
    }))
    const ws = { connected: true, events: baseEvents, call }
    const { rerender } = renderDetail(ws)

    expect(await screen.findByText('Agent agent-1')).toBeInTheDocument()

    const cappedEvents = [
      ...baseEvents.slice(1),
      {
        seq: 201,
        method: 'Vulpine.conversation',
        params: { agentId: 'agent-1', role: 'assistant', content: 'after cap', tokens: 3 },
      },
    ]

    rerender(
      <MemoryRouter initialEntries={['/agents/agent-1']}>
        <Routes>
          <Route path="/agents/:id" element={<AgentDetail ws={{ ...ws, events: cappedEvents }} />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('after cap')).toBeInTheDocument())
  })

  it('uses inline confirmation before killing an agent', async () => {
    const call = vi.fn(async (method) => {
      if (method === 'agents.list') {
        return { agents: [{ id: 'agent-1', name: 'Agent One', status: 'active', contextId: '', totalTokens: 0 }] }
      }
      if (method === 'agents.getMessages') return { messages: [] }
      if (method === 'recording.getTimeline') return { actions: [] }
      if (method === 'fingerprints.get') return {}
      return { status: 'ok' }
    })

    renderDetail({ connected: true, events: [], call })

    expect(await screen.findByText('active')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Kill'))
    expect(screen.getByText('Confirm kill')).toBeInTheDocument()
    expect(call).not.toHaveBeenCalledWith('agents.kill', { agentId: 'agent-1' })

    const confirmBanner = screen.getByText('Confirm kill').closest('.panel-banner')
    fireEvent.click(within(confirmBanner).getByText('Kill'))
    await waitFor(() => {
      expect(call).toHaveBeenCalledWith('agents.kill', { agentId: 'agent-1' })
    })
    expect(screen.getByText('interrupted')).toBeInTheDocument()
    expect(screen.queryByText('Kill')).not.toBeInTheDocument()
  })
})
