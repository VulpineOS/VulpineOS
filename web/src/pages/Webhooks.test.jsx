import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Webhooks from './Webhooks'

describe('Webhooks page', () => {
  it('adds and removes webhook registrations', async () => {
    let hooks = []
    const calls = vi.fn(async (method, params) => {
      if (method === 'webhooks.list') return { webhooks: hooks }
      if (method === 'webhooks.add') {
        hooks = [{
          id: 'hook-1',
          url: params.url,
          events: params.events,
          secret: params.secret,
        }]
        return { id: 'hook-1' }
      }
      if (method === 'webhooks.remove') {
        hooks = hooks.filter(hook => hook.id !== params.id)
        return { status: 'ok' }
      }
      return {}
    })

    render(<Webhooks ws={{ connected: true, call: calls }} />)

    expect(await screen.findByText('No webhooks registered.')).toBeInTheDocument()

    fireEvent.change(screen.getByPlaceholderText('https://your-server.com/webhook'), { target: { value: 'https://example.com/hook' } })
    fireEvent.change(screen.getByPlaceholderText('Secret (optional)'), { target: { value: 'secret-token' } })
    fireEvent.change(screen.getByPlaceholderText('Events (comma-separated, empty=all)'), { target: { value: 'agent.completed, budget.alert' } })
    fireEvent.click(screen.getByText('Add'))

    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('webhooks.add', {
        url: 'https://example.com/hook',
        events: ['agent.completed', 'budget.alert'],
        secret: 'secret-token',
      })
    })
    expect(await screen.findByText('https://example.com/hook')).toBeInTheDocument()
    expect(screen.getByText('agent.completed, budget.alert')).toBeInTheDocument()
    expect(screen.getByText('••••••')).toBeInTheDocument()

    fireEvent.click(screen.getByText('Remove'))
    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('webhooks.remove', { id: 'hook-1' })
    })
  })
})
