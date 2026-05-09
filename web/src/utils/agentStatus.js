export const LIVE_AGENT_STATUSES = new Set(['active', 'running', 'thinking', 'starting'])
export const TERMINAL_AGENT_STATUSES = new Set(['completed', 'error', 'failed', 'interrupted'])

export function isLiveAgentStatus(status) {
  return LIVE_AGENT_STATUSES.has(status)
}

export function isPausedAgentStatus(status) {
  return status === 'paused'
}

export function isTerminalAgentStatus(status) {
  return TERMINAL_AGENT_STATUSES.has(status)
}

export function agentStatusBadgeClass(status) {
  if (isLiveAgentStatus(status)) return 'green'
  if (isPausedAgentStatus(status)) return 'yellow'
  if (status === 'completed') return 'blue'
  if (status === 'error' || status === 'failed') return 'red'
  return 'gray'
}
