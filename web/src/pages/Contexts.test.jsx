import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Contexts from './Contexts'

describe('Contexts page', () => {
  it('creates, selects, and removes contexts', async () => {
    let contexts = [
      { id: 'ctx-existing-1234567890', pages: 1 },
    ]
    const calls = vi.fn(async (method, params) => {
      if (method === 'contexts.list') return { contexts }
      if (method === 'contexts.create') {
        contexts = [...contexts, { id: 'ctx-new-abcdefghijklmnop', pages: 1 }]
        return { browserContextId: 'ctx-new-abcdefghijklmnop' }
      }
      if (method === 'contexts.remove') {
        contexts = contexts.filter(context => context.id !== params.browserContextId)
        return { status: 'ok' }
      }
      return {}
    })

    render(<Contexts ws={{ connected: true, events: [], call: calls }} />)

    expect(await screen.findByText(/ctx-existing-123/)).toBeInTheDocument()

    fireEvent.click(screen.getByText('New Context'))
    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('contexts.create', { removeOnDetach: true })
    })
    await waitFor(() => {
      expect(window.localStorage.getItem('vulpine_context_id')).toBe('ctx-new-abcdefghijklmnop')
    })
    expect(screen.getByText(/selected/)).toBeInTheDocument()

    fireEvent.click(screen.getAllByText('Remove')[1])
    await waitFor(() => {
      expect(calls).toHaveBeenCalledWith('contexts.remove', { browserContextId: 'ctx-new-abcdefghijklmnop' })
    })
  })
})
