import React from 'react'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import Dashboard from './Dashboard'

describe('Dashboard page', () => {
  it('shows browser route, window state, and active work', async () => {
    const ws = {
      connected: true,
      connectionState: 'connected',
      telemetry: { memoryMB: 512, activePages: 2, activeContexts: 1, detectionRiskScore: 7 },
      events: [],
      call: vi.fn(async (method) => {
        if (method === 'status.get') {
          return {
            kernel_running: true,
            kernel_pid: 1234,
            browser_route: 'camoufox',
            browser_route_source: 'runtime',
            browser_window: 'hidden',
            gateway_running: true,
            sentinel_available: true,
            sentinel_mode: 'private_scaffold',
            sentinel_trust_recipes: 1,
            sentinel_maturity_metrics: 5,
            sentinel_assignment_rules: 3,
            kernel_headless: false,
            openclaw_profile_configured: true,
            active_agents: 1,
            pool_active: 1,
            pool_total: 4,
            pool_available: 3,
            total_cost_usd: 1.25,
            total_citizens: 2,
            total_templates: 3,
          }
        }
        if (method === 'costs.total') {
          return { totalCostUsd: 1.25 }
        }
        if (method === 'costs.getAll') {
          return {
            usage: [
              { agentId: 'agent-1', totalTokens: 4200, estimatedCost: 1.25 },
            ],
            defaults: { maxCostUsd: 5, maxTokens: 10000 },
          }
        }
        if (method === 'agents.list') {
          return {
            agents: [
              { id: 'agent-1', name: 'Scraper', status: 'active', contextId: 'ctx-1234567890', totalTokens: 4200, budgetSource: 'agent' },
            ],
          }
        }
        if (method === 'runtime.list') {
          return {
            events: [
              {
                id: 1,
                level: 'warn',
                component: 'gateway',
                event: 'profile_repair_failed',
                message: 'Gateway profile repair failed',
                timestamp: '2026-04-21T04:00:00Z',
              },
            ],
          }
        }
        return {}
      }),
    }

    render(
      <MemoryRouter>
        <Dashboard ws={ws} />
      </MemoryRouter>,
    )

    expect(await screen.findByText('CAMOUFOX')).toBeInTheDocument()
    expect(screen.getByText('runtime source · GUI · 512MB')).toBeInTheDocument()
    expect(screen.getAllByText('HIDDEN').length).toBeGreaterThan(0)
    expect(screen.getByText('Scraper')).toBeInTheDocument()
    expect(screen.getByText('4,200 tokens · 1 tracked usage records')).toBeInTheDocument()
    expect(screen.getByText('1 agent override · $5.00 · 10,000 tok')).toBeInTheDocument()
    expect(screen.getByText('RUNNING')).toBeInTheDocument()
    expect(screen.getByText('PRIVATE_SCAFFOLD')).toBeInTheDocument()
    expect(screen.getByText('Maturity metrics')).toBeInTheDocument()
    expect(screen.getByText('Assignment rules')).toBeInTheDocument()
    expect(screen.getAllByText('Gateway profile repair failed').length).toBeGreaterThan(0)
    expect(screen.getByText('Review agents')).toBeInTheDocument()
  })
})
