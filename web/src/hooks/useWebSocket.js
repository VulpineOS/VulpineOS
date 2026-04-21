import { useState, useEffect, useRef, useCallback } from 'react'

const MAX_RECONNECT_ATTEMPTS = 8
const RECONNECT_BASE_DELAY_MS = 1000
const RECONNECT_MAX_DELAY_MS = 10000
const REQUEST_TIMEOUT_MS = 30000

function connectionErrorMessage(state, lastError) {
  if (lastError) return lastError
  switch (state) {
    case 'connecting':
      return 'Still connecting to the panel server'
    case 'reconnecting':
      return 'Connection lost; reconnecting'
    case 'failed':
      return 'Panel session failed'
    default:
      return 'Not connected'
  }
}

export function useWebSocket(apiKey) {
  const [connected, setConnected] = useState(false)
  const [connectionState, setConnectionState] = useState('disconnected')
  const [events, setEvents] = useState([])
  const [telemetry, setTelemetry] = useState({})
  const [lastError, setLastError] = useState('')
  const [reconnectAttempt, setReconnectAttempt] = useState(0)
  const [retryDelayMs, setRetryDelayMs] = useState(0)
  const wsRef = useRef(null)
  const idRef = useRef(1)
  const pendingRef = useRef({})
  const reconnectTimerRef = useRef(null)
  const manualCloseRef = useRef(false)
  const hasConnectedRef = useRef(false)

  const rejectPending = useCallback((message) => {
    const err = new Error(message)
    const pending = pendingRef.current
    pendingRef.current = {}
    for (const key of Object.keys(pending)) {
      pending[key]?.reject?.(err)
      if (pending[key]?.timer) clearTimeout(pending[key].timer)
    }
  }, [])

  const clearReconnectTimer = useCallback(() => {
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current)
      reconnectTimerRef.current = null
    }
  }, [])

  const connect = useCallback((attempt = 0) => {
    if (!apiKey) return
    clearReconnectTimer()

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${protocol}//${window.location.host}/ws?token=${encodeURIComponent(apiKey)}`)
    wsRef.current = ws
    setReconnectAttempt(attempt)
    setRetryDelayMs(0)
    setConnectionState(attempt > 0 || hasConnectedRef.current ? 'reconnecting' : 'connecting')

    let opened = false

    ws.onopen = () => {
      opened = true
      hasConnectedRef.current = true
      setConnected(true)
      setConnectionState('connected')
      setLastError('')
      setReconnectAttempt(0)
      setRetryDelayMs(0)
    }

    ws.onerror = () => {
      setLastError((current) => current || 'WebSocket connection error')
    }

    ws.onclose = (event) => {
      if (wsRef.current === ws) wsRef.current = null
      setConnected(false)
      rejectPending('Connection lost while waiting for a response')

      if (manualCloseRef.current) {
        setConnectionState(apiKey ? 'disconnected' : 'idle')
        return
      }

      const nextAttempt = attempt + 1
      const reason = event?.reason?.trim()
      const code = event?.code
      const fallback = opened ? 'Connection closed by the panel server' : 'Unable to establish a panel session'
      setLastError(reason || (code === 1008 ? 'Access key rejected by panel server' : fallback))

      if (nextAttempt > MAX_RECONNECT_ATTEMPTS) {
        setConnectionState('failed')
        setRetryDelayMs(0)
        return
      }

      const delay = Math.min(RECONNECT_BASE_DELAY_MS * (2 ** (nextAttempt - 1)), RECONNECT_MAX_DELAY_MS)
      setConnectionState('reconnecting')
      setReconnectAttempt(nextAttempt)
      setRetryDelayMs(delay)
      reconnectTimerRef.current = setTimeout(() => {
        reconnectTimerRef.current = null
        connect(nextAttempt)
      }, delay)
    }

    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        if (msg.type === 'control') {
          const payload = msg.payload || {}
          const params = payload.params || payload
          if (params.id && pendingRef.current[params.id]) {
            const pending = pendingRef.current[params.id]
            delete pendingRef.current[params.id]
            if (pending.timer) clearTimeout(pending.timer)
            pending.resolve(params)
          }
          return
        }

        if (msg.type === 'juggler' || msg.method) {
          const payload = msg.payload || msg
          if (payload.id && pendingRef.current[payload.id]) {
            const pending = pendingRef.current[payload.id]
            delete pendingRef.current[payload.id]
            if (pending.timer) clearTimeout(pending.timer)
            pending.resolve(payload)
            return
          }
          const method = payload.method || ''
          const params = payload.params || {}
          setEvents(prev => [...prev.slice(-199), { method, params, ts: Date.now() }])
          if (method === 'Browser.telemetryUpdate') setTelemetry(params)
        }
      } catch {}
    }
  }, [apiKey, clearReconnectTimer, rejectPending])

  useEffect(() => {
    manualCloseRef.current = false
    clearReconnectTimer()

    if (!apiKey) {
      if (wsRef.current) {
        manualCloseRef.current = true
        wsRef.current.close()
        wsRef.current = null
      }
      rejectPending('Panel session ended')
      setConnected(false)
      setConnectionState('disconnected')
      setLastError('')
      hasConnectedRef.current = false
      return () => {}
    }

    connect(0)

    return () => {
      manualCloseRef.current = true
      clearReconnectTimer()
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
      rejectPending('Panel session ended')
      setConnected(false)
    }
  }, [apiKey, clearReconnectTimer, connect, rejectPending])

  const retry = useCallback(() => {
    if (!apiKey) return
    if (wsRef.current) {
      manualCloseRef.current = true
      wsRef.current.close()
      wsRef.current = null
    }
    manualCloseRef.current = false
    setLastError('')
    connect(0)
  }, [apiKey, connect])

  const call = useCallback((method, params = {}) => {
    return new Promise((resolve, reject) => {
      if (!wsRef.current || wsRef.current.readyState !== 1) {
        reject(new Error(connectionErrorMessage(connectionState, lastError)))
        return
      }

      const id = idRef.current++
      const envelope = JSON.stringify({ type: 'control', payload: { command: method, params, id } })
      const timer = setTimeout(() => {
        if (!pendingRef.current[id]) return
        delete pendingRef.current[id]
        reject(new Error('Request timed out'))
      }, REQUEST_TIMEOUT_MS)

      pendingRef.current[id] = {
        timer,
        resolve: (resp) => {
          if (resp.error) reject(new Error(resp.error))
          else resolve(resp.result)
        },
        reject,
      }
      wsRef.current.send(envelope)
    })
  }, [connectionState, lastError])

  const juggler = useCallback((method, params = {}) => {
    return new Promise((resolve, reject) => {
      if (!wsRef.current || wsRef.current.readyState !== 1) {
        reject(new Error(connectionErrorMessage(connectionState, lastError)))
        return
      }

      const id = idRef.current++
      const timer = setTimeout(() => {
        if (!pendingRef.current[id]) return
        delete pendingRef.current[id]
        reject(new Error('Request timed out'))
      }, REQUEST_TIMEOUT_MS)

      pendingRef.current[id] = {
        timer,
        resolve: (resp) => {
          if (resp.error) reject(new Error(resp.error.message || 'Request failed'))
          else resolve(resp)
        },
        reject,
      }
      wsRef.current.send(JSON.stringify({ type: 'juggler', payload: { id, method, params } }))
    })
  }, [connectionState, lastError])

  return {
    connected,
    connectionState,
    telemetry,
    events,
    call,
    juggler,
    send: juggler,
    retry,
    lastError,
    reconnectAttempt,
    retryDelayMs,
  }
}
