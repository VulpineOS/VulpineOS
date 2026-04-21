import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import Login from './Login'

describe('Login page', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('validates the access key through /auth/check before logging in', async () => {
    const onLogin = vi.fn()
    const fetchMock = vi.fn(async () => ({ ok: true, status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    render(<Login onLogin={onLogin} />)

    fireEvent.change(screen.getByPlaceholderText('Access Key'), { target: { value: 'secret' } })
    fireEvent.click(screen.getByText('Connect'))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/auth/check', {
        headers: { Authorization: 'Bearer secret' },
      })
    })
    expect(onLogin).toHaveBeenCalledWith('secret')
  })

  it('shows an inline error when the access key is rejected', async () => {
    const onLogin = vi.fn()
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 401 })))

    render(<Login onLogin={onLogin} />)

    fireEvent.change(screen.getByPlaceholderText('Access Key'), { target: { value: 'wrong' } })
    fireEvent.click(screen.getByText('Connect'))

    expect(await screen.findByText('Access key rejected')).toBeInTheDocument()
    expect(onLogin).not.toHaveBeenCalled()
  })
})
