import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Settings from './Settings'

describe('Settings page', () => {
  it('shows browser route and mode in the about card', async () => {
    const ws = {
      connected: true,
      call: vi.fn(async (method) => {
        if (method === 'config.providers') {
          return {
            providers: [
              {
                id: 'anthropic',
                name: 'Anthropic (Claude)',
                envVar: 'ANTHROPIC_API_KEY',
                defaultModel: 'anthropic/claude-sonnet-4-6',
                models: ['anthropic/claude-sonnet-4-6'],
                needsKey: true,
              },
            ],
          }
        }
        if (method === 'config.get') {
          return {
            provider: 'anthropic',
            model: 'claude-sonnet-4-6',
            hasKey: true,
            setupComplete: false,
            defaultBudgetMaxCostUsd: 1.5,
            defaultBudgetMaxTokens: 6000,
          }
        }
        if (method === 'status.get') {
          return {
            browser_route: 'camoufox',
            browser_route_source: 'runtime',
            browser_window: 'hidden',
            gateway_running: true,
            sentinel_available: true,
            sentinel_mode: 'private_scaffold',
            sentinel_provider: 'sentinel-private',
            sentinel_variant_bundles: 1,
            sentinel_trust_recipes: 1,
            sentinel_maturity_metrics: 1,
            sentinel_assignment_rules: 1,
            kernel_headless: false,
            kernel_running: true,
            openclaw_profile_configured: true,
          }
        }
        if (method === 'sentinel.get') {
          return {
            available: true,
            variantBundles: [{ id: 'control', name: 'Control', enabled: true, weight: 100 }],
            trustRecipes: [{ id: 'baseline-warmup', name: 'Baseline warmup', warmupStrategy: 'generic_revisit' }],
            maturityMetrics: [{
              id: 'session_age_seconds',
              name: 'Session age',
              unit: 'seconds',
              thresholds: [{ stage: 'warm', minimum: 1800 }],
              description: 'How long the identity has existed before higher-trust variants are allowed.',
            }],
            assignmentRules: [{
              id: 'cold-holdout',
              name: 'Cold holdout',
              stage: 'cold',
              variantBundleId: 'control',
              trustRecipeId: 'baseline-warmup',
              holdoutPercent: 100,
            }],
          }
        }
        if (method === 'sentinel.timeline') {
          return {
            sessions: [{
              sessionId: 'session-1',
              agentId: 'agent-1',
              domain: 'example.com',
              url: 'https://example.com',
              eventCount: 1,
              outcomeCount: 1,
              items: [
                { type: 'event', kind: 'browser_probe', name: 'canvas.toDataURL' },
                { type: 'outcome', outcome: 'soft_challenge', challengeVendor: 'cloudflare' },
              ],
            }],
          }
        }
        return {}
      }),
    }

    render(<Settings ws={ws} />)

    expect(await screen.findByText('Route: CAMOUFOX (runtime) · GUI')).toBeInTheDocument()
    expect(screen.getByText('Window: HIDDEN')).toBeInTheDocument()
    expect(screen.getByText('Gateway: RUNNING')).toBeInTheDocument()
    expect(screen.getByText('Sentinel: PRIVATE_SCAFFOLD · sentinel-private')).toBeInTheDocument()
    expect(screen.getByText('Variant bundles: 1')).toBeInTheDocument()
    expect(screen.getByText('Trust recipes: 1')).toBeInTheDocument()
    expect(screen.getByText('Maturity metrics: 1')).toBeInTheDocument()
    expect(screen.getByText('Assignment rules: 1')).toBeInTheDocument()
    expect(screen.getByText('Variant names: Control')).toBeInTheDocument()
    expect(screen.getByText('Trust names: Baseline warmup')).toBeInTheDocument()
    expect(screen.getByText('Session age')).toBeInTheDocument()
    expect(screen.getByText('Cold holdout')).toBeInTheDocument()
    expect(screen.getByText('warm 1800 seconds')).toBeInTheDocument()
    expect(screen.getByText('Recent capture timeline')).toBeInTheDocument()
    expect(screen.getByText('example.com · 1 events · 1 outcomes')).toBeInTheDocument()
    expect(screen.getByText('browser_probe · canvas.toDataURL')).toBeInTheDocument()
    expect(screen.getByText('soft_challenge · cloudflare')).toBeInTheDocument()
    expect(screen.getByText('Agent model setup: Not configured')).toBeInTheDocument()
    expect(screen.getByText('OpenClaw profile: Configured')).toBeInTheDocument()
    expect(screen.getByText('API Key (ANTHROPIC_API_KEY)')).toBeInTheDocument()
    expect(screen.getByText('A key is already stored locally. Leave this blank to keep it.')).toBeInTheDocument()
    expect(screen.getByDisplayValue('1.5')).toBeInTheDocument()
    expect(screen.getByDisplayValue('6000')).toBeInTheDocument()

    fireEvent.change(screen.getByDisplayValue('1.5'), { target: { value: '2.25' } })
    fireEvent.change(screen.getByDisplayValue('6000'), { target: { value: '9000' } })
    fireEvent.click(screen.getByText('Save Defaults'))

    await waitFor(() => {
      expect(ws.call).toHaveBeenCalledWith('config.set', { defaultBudgetMaxCostUsd: 2.25, defaultBudgetMaxTokens: 9000 })
    })
  })
})
