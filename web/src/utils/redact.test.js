import { describe, expect, it } from 'vitest'
import { redactSensitiveText } from './redact'

describe('redactSensitiveText', () => {
  it('redacts JSON fields, token query params, and bearer values', () => {
    const out = redactSensitiveText({
      apiKey: 'sk-secret',
      url: 'http://127.0.0.1:8443/?token=panel-token&view=agents',
      header: 'Authorization: Bearer bearer-token',
      pid: '44',
    })

    expect(out).not.toContain('sk-secret')
    expect(out).not.toContain('panel-token')
    expect(out).not.toContain('bearer-token')
    expect(out).toContain('"apiKey":"[redacted]"')
    expect(out).toContain('token=[redacted]')
    expect(out).toContain('Bearer [redacted]')
    expect(out).toContain('"pid":"44"')
  })
})
