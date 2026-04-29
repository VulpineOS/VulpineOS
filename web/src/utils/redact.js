const SENSITIVE_KEY = '(?:apiKey|api_key|apikey|token|access_token|access_key|secret|password|credential|authorization)'

const jsonSecretPattern = new RegExp(`("${SENSITIVE_KEY}"\\s*:\\s*)"[^"]*"`, 'gi')
const querySecretPattern = new RegExp(`([?&]${SENSITIVE_KEY}=)[^&#\\s"]+`, 'gi')
const bearerPattern = /(bearer\s+)[^\s,;"]+/gi

export function redactSensitiveText(value) {
  const text = typeof value === 'string' ? value : JSON.stringify(value)
  if (!text) return ''
  return text
    .replace(jsonSecretPattern, '$1"[redacted]"')
    .replace(querySecretPattern, '$1[redacted]')
    .replace(bearerPattern, '$1[redacted]')
}
