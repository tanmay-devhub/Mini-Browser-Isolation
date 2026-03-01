import { useState, useCallback, useRef } from 'react'
import { api, SessionStatusResponse } from '../api/client'

export type SessionState = 'idle' | 'starting' | 'ready' | 'error' | 'terminated'

export interface SessionInfo {
  id: string
  status: SessionStatusResponse['status']
  metrics?: SessionStatusResponse['metrics']
  error?: string
}

export function useSession() {
  const [state, setState] = useState<SessionState>('idle')
  const [session, setSession] = useState<SessionInfo | null>(null)
  const [error, setError] = useState<string | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const stopPolling = useCallback(() => {
    if (pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
  }, [])

  const startSession = useCallback(async (url: string) => {
    try {
      setState('starting')
      setError(null)

      const created = await api.createSession(url)
      setSession({ id: created.sessionId, status: 'starting' })

      // Poll until the runner is ready (or errors out).
      pollRef.current = setInterval(async () => {
        try {
          const status = await api.getSession(created.sessionId)
          setSession({
            id: status.sessionId,
            status: status.status,
            metrics: status.metrics,
            error: status.error,
          })

          if (status.status === 'ready') {
            setState('ready')
            stopPolling()
          } else if (status.status === 'error' || status.status === 'terminated') {
            setState(status.status as SessionState)
            setError(status.error ?? 'Unknown error')
            stopPolling()
          }
        } catch (e) {
          // Transient poll error – keep retrying.
          console.warn('poll error', e)
        }
      }, 1500)
    } catch (e) {
      setState('error')
      setError(String(e))
    }
  }, [stopPolling])

  const endSession = useCallback(async () => {
    stopPolling()
    if (session?.id) {
      try {
        await api.deleteSession(session.id)
      } catch {
        // Best-effort cleanup.
      }
    }
    setState('idle')
    setSession(null)
  }, [session, stopPolling])

  return { state, session, error, startSession, endSession }
}
