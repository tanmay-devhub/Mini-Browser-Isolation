import React from 'react'
import { SessionInfo } from '../hooks/useSession'
import { StreamMode } from '../hooks/useWebRTC'

interface Props {
  session: SessionInfo | null
  streamMode: StreamMode
}

const pill = (color: string, text: string) => (
  <span style={{
    background: color,
    color: '#fff',
    borderRadius: 4,
    padding: '2px 8px',
    fontSize: 11,
    fontWeight: 600,
    letterSpacing: '0.04em',
    textTransform: 'uppercase',
  }}>{text}</span>
)

export default function StatusBar({ session, streamMode }: Props) {
  if (!session) return null

  const statusColor: Record<string, string> = {
    starting: '#f59e0b',
    ready: '#10b981',
    error: '#ef4444',
    terminated: '#6b7280',
  }

  const modeColor: Record<StreamMode, string> = {
    webrtc: '#6366f1',
    websocket: '#f59e0b',
    none: '#6b7280',
  }

  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      padding: '6px 14px',
      background: '#1e2130',
      borderBottom: '1px solid #2d3148',
      fontSize: 12,
      color: '#94a3b8',
    }}>
      <span>Session: <code style={{ color: '#e2e8f0' }}>{session.id.slice(0, 8)}…</code></span>
      {pill(statusColor[session.status] ?? '#6b7280', session.status)}
      {pill(modeColor[streamMode], streamMode === 'none' ? 'no stream' : streamMode)}
      {session.metrics && (
        <>
          <span>CPU {session.metrics.cpuPercent.toFixed(1)}%</span>
          <span>RAM {session.metrics.memMB.toFixed(0)} MB</span>
          <span>Up {session.metrics.uptimeSec.toFixed(0)}s</span>
        </>
      )}
      {session.error && (
        <span style={{ color: '#ef4444' }}>Error: {session.error}</span>
      )}
    </div>
  )
}
