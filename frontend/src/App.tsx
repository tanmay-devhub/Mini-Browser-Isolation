import React, { useState } from 'react'
import { useSession } from './hooks/useSession'
import { useWebRTC } from './hooks/useWebRTC'
import SessionView from './components/SessionView'
import StatusBar from './components/StatusBar'

const DEFAULT_URL = 'https://example.com'

export default function App() {
  const [urlInput, setUrlInput] = useState(DEFAULT_URL)
  const { state, session, error, startSession, endSession } = useSession()
  const { connect, disconnect, sendInput, videoRef, streamMode, fallbackSrc } = useWebRTC(
    session?.id ?? null,
  )

  const handleStart = () => {
    if (urlInput.trim()) startSession(urlInput.trim())
  }

  const handleEnd = () => {
    disconnect()
    endSession()
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      {/* Top toolbar */}
      <header style={{
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        padding: '10px 16px',
        background: '#161925',
        borderBottom: '1px solid #2d3148',
        flexShrink: 0,
      }}>
        <div style={{ fontWeight: 700, fontSize: 15, color: '#6366f1', whiteSpace: 'nowrap' }}>
          🔒 Isolated Browser
        </div>

        <input
          type="url"
          value={urlInput}
          onChange={(e) => setUrlInput(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && state === 'idle' && handleStart()}
          placeholder="https://example.com"
          disabled={state !== 'idle' && state !== 'error'}
          style={{
            flex: 1,
            padding: '6px 12px',
            borderRadius: 6,
            border: '1px solid #2d3148',
            background: '#0f1117',
            color: '#e2e8f0',
            fontSize: 13,
          }}
        />

        {(state === 'idle' || state === 'error') && (
          <button
            onClick={handleStart}
            disabled={!urlInput.trim()}
            style={btnStyle('#6366f1')}
          >
            Start Session
          </button>
        )}

        {(state === 'starting' || state === 'ready') && (
          <button onClick={handleEnd} style={btnStyle('#ef4444')}>
            End Session
          </button>
        )}

        {state === 'starting' && (
          <span style={{ fontSize: 12, color: '#f59e0b' }}>Starting…</span>
        )}
      </header>

      {/* Status bar (shown once a session exists) */}
      <StatusBar session={session} streamMode={streamMode} />

      {/* Main content */}
      <main style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        {(state === 'ready' || state === 'starting') && session ? (
          <SessionView
            sessionId={session.id}
            streamMode={streamMode}
            fallbackSrc={fallbackSrc}
            videoRef={videoRef}
            onConnect={connect}
            onDisconnect={disconnect}
            sendInput={sendInput}
          />
        ) : state === 'error' ? (
          <CenteredMessage color="#ef4444">
            Error: {error ?? 'Unknown error'}
          </CenteredMessage>
        ) : (
          <CenteredMessage color="#6366f1">
            Enter a URL and click <strong>Start Session</strong> to begin.
          </CenteredMessage>
        )}
      </main>
    </div>
  )
}

function CenteredMessage({ children, color }: { children: React.ReactNode; color: string }) {
  return (
    <div style={{
      flex: 1,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      color,
      fontSize: 15,
      textAlign: 'center',
      padding: 32,
    }}>
      {children}
    </div>
  )
}

function btnStyle(bg: string): React.CSSProperties {
  return {
    background: bg,
    color: '#fff',
    border: 'none',
    borderRadius: 6,
    padding: '7px 16px',
    fontSize: 13,
    fontWeight: 600,
    cursor: 'pointer',
    whiteSpace: 'nowrap',
  }
}
