import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Proxies from './Proxies'

describe('Proxies page', () => {
  it('loads and saves agent rotation config', async () => {
    const ws = {
      connected: true,
      notify: vi.fn(),
      call: vi.fn(async (method, params) => {
        if (method === 'proxies.list') {
          return { proxies: [{ id: 'proxy-1', url: 'http://a:80', latencyMs: 0 }, { id: 'proxy-2', url: 'http://b:80', latencyMs: 0 }] }
        }
        if (method === 'agents.list') {
          return { agents: [{ id: 'agent-1', name: 'Alpha' }] }
        }
        if (method === 'proxies.getRotation') {
          return {
            source: 'agent',
            config: {
              enabled: true,
              rotateOnRateLimit: true,
              rotateOnBlock: false,
              rotateIntervalSeconds: 300,
              syncFingerprint: true,
              proxyPool: ['http://a:80'],
              currentIndex: 0,
            },
          }
        }
        if (method === 'proxies.setRotation') {
          return { status: 'ok', config: params.config }
        }
        return {}
      }),
    }

    render(<Proxies ws={ws} />)

    expect(await screen.findByText('Rotation')).toBeInTheDocument()
    await waitFor(() => {
      expect(ws.call).toHaveBeenCalledWith('proxies.getRotation', { agentId: 'agent-1' })
    })

    fireEvent.click(screen.getByText('Save Rotation'))
    await waitFor(() => {
      expect(ws.call).toHaveBeenCalledWith('proxies.setRotation', {
        agentId: 'agent-1',
        config: {
          enabled: true,
          rotateOnRateLimit: true,
          rotateOnBlock: false,
          rotateIntervalSeconds: 300,
          syncFingerprint: true,
          proxyPool: ['http://a:80'],
          currentIndex: 0,
        },
      })
    })
  })
})
