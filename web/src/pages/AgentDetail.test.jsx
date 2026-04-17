import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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
        return { agents: [{ id: 'agent-1', name: 'Agent One', status: 'paused', contextId: '', totalTokens: 0 }] }
      }
      if (method === 'agents.getMessages') {
        return {
          messages: [
            { role: 'user', content: 'hello' },
            { role: 'system', content: 'Running browser action: open https://example.com' },
          ],
        }
      }
      if (method === 'recording.getTimeline') {
        return { actions: [] }
      }
      if (method === 'fingerprints.get') {
        return {}
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

    fireEvent.click(screen.getByText('Trace'))
    expect(screen.getByText('Action Trace')).toBeInTheDocument()
    expect(screen.getByText('Running browser action: open https://example.com')).toBeInTheDocument()

    fireEvent.click(screen.getByText('Conversation'))
    fireEvent.change(screen.getByPlaceholderText('Send message to agent...'), { target: { value: 'continue' } })
    fireEvent.click(screen.getByText('Send'))

    await waitFor(() => {
      expect(call).toHaveBeenCalledWith('agents.resume', { agentId: 'agent-1', message: 'continue' })
    })
    expect(screen.getByText('continue')).toBeInTheDocument()
  })
})
