import React from 'react'
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useWebSocket } from './useWebSocket'

class FakeWebSocket {
  static instances = []

  constructor(url) {
    this.url = url
    this.readyState = 0
    FakeWebSocket.instances.push(this)
  }

  send = vi.fn()

  close = vi.fn(() => {
    this.readyState = 3
  })

  triggerOpen() {
    this.readyState = 1
    this.onopen?.()
  }

  triggerClose(event = { code: 1006, reason: '' }) {
    this.readyState = 3
    this.onclose?.(event)
  }
}

describe('useWebSocket', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    FakeWebSocket.instances = []
    vi.stubGlobal('WebSocket', FakeWebSocket)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('reconnects with backoff after the connection drops', async () => {
    const { result } = renderHook(() => useWebSocket('secret'))

    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(FakeWebSocket.instances[0].url).toContain('/ws?token=secret')

    act(() => {
      FakeWebSocket.instances[0].triggerOpen()
    })

    expect(result.current.connectionState).toBe('connected')

    act(() => {
      FakeWebSocket.instances[0].triggerClose({ code: 1006, reason: 'network down' })
    })

    expect(result.current.connectionState).toBe('reconnecting')
    expect(result.current.lastError).toBe('network down')
    expect(result.current.retryDelayMs).toBe(1000)

    act(() => {
      vi.advanceTimersByTime(1000)
    })

    expect(FakeWebSocket.instances).toHaveLength(2)

    act(() => {
      FakeWebSocket.instances[1].triggerOpen()
    })

    expect(result.current.connectionState).toBe('connected')
    expect(result.current.connected).toBe(true)
  })

  it('enters failed state after exhausting reconnect attempts', async () => {
    const { result } = renderHook(() => useWebSocket('secret'))
    const delays = [1000, 2000, 4000, 8000, 10000, 10000, 10000, 10000]

    for (let attempt = 0; attempt < 9; attempt += 1) {
      const current = FakeWebSocket.instances[attempt]
      act(() => {
        current.triggerClose({ code: 1006, reason: '' })
      })

      if (attempt < 8) {
        expect(result.current.connectionState).toBe('reconnecting')
        act(() => {
          vi.advanceTimersByTime(delays[attempt])
        })
      }
    }

    expect(result.current.connectionState).toBe('failed')
    expect(result.current.connected).toBe(false)
  })
})
