import React, { useEffect, useRef } from 'react'
import { StreamMode } from '../hooks/useWebRTC'

interface Props {
  sessionId: string
  streamMode: StreamMode
  fallbackSrc: string | null
  videoRef: React.RefObject<HTMLVideoElement>
  onConnect: (id: string) => void
  onDisconnect: () => void
  sendInput: (event: object) => void
}

export default function SessionView({
  sessionId,
  streamMode,
  fallbackSrc,
  videoRef,
  onConnect,
  onDisconnect,
  sendInput,
}: Props) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    onConnect(sessionId)
    return () => onDisconnect()
  }, [sessionId]) // eslint-disable-line react-hooks/exhaustive-deps

  // Map browser pointer events to input messages.
  const handleMouseMove = (e: React.MouseEvent) => {
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect) return
    sendInput({
      type: 'mousemove',
      x: e.clientX - rect.left,
      y: e.clientY - rect.top,
    })
  }

  const handleMouseDown = (e: React.MouseEvent) => {
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect) return
    sendInput({
      type: 'mousedown',
      x: e.clientX - rect.left,
      y: e.clientY - rect.top,
      button: e.button,
    })
  }

  const handleMouseUp = (e: React.MouseEvent) => {
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect) return
    sendInput({
      type: 'mouseup',
      x: e.clientX - rect.left,
      y: e.clientY - rect.top,
      button: e.button,
    })
  }

  const handleWheel = (e: React.WheelEvent) => {
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect) return
    sendInput({
      type: 'scroll',
      x: e.clientX - rect.left,
      y: e.clientY - rect.top,
      deltaX: e.deltaX,
      deltaY: e.deltaY,
    })
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    e.preventDefault()
    sendInput({ type: 'keydown', key: e.key, code: e.code })
  }

  const handleKeyUp = (e: React.KeyboardEvent) => {
    sendInput({ type: 'keyup', key: e.key, code: e.code })
  }

  return (
    <div
      ref={containerRef}
      tabIndex={0}
      style={{
        flex: 1,
        position: 'relative',
        background: '#000',
        outline: 'none',
        cursor: 'crosshair',
        overflow: 'hidden',
      }}
      onMouseMove={handleMouseMove}
      onMouseDown={handleMouseDown}
      onMouseUp={handleMouseUp}
      onWheel={handleWheel}
      onKeyDown={handleKeyDown}
      onKeyUp={handleKeyUp}
    >
      {/* WebRTC live video */}
      <video
        ref={videoRef}
        autoPlay
        playsInline
        muted
        style={{
          width: '100%',
          height: '100%',
          objectFit: 'contain',
          display: streamMode === 'webrtc' ? 'block' : 'none',
        }}
      />

      {/* WebSocket fallback: rendered as an <img> updated on each frame message */}
      {streamMode === 'websocket' && fallbackSrc && (
        <img
          src={fallbackSrc}
          alt="browser stream"
          style={{ width: '100%', height: '100%', objectFit: 'contain' }}
          draggable={false}
        />
      )}

      {/* Loading overlay */}
      {streamMode === 'none' && (
        <div style={{
          position: 'absolute',
          inset: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          flexDirection: 'column',
          gap: 16,
          color: '#94a3b8',
        }}>
          <Spinner />
          <p style={{ fontSize: 14 }}>Establishing stream…</p>
        </div>
      )}
    </div>
  )
}

function Spinner() {
  return (
    <div style={{
      width: 40,
      height: 40,
      border: '3px solid #2d3148',
      borderTop: '3px solid #6366f1',
      borderRadius: '50%',
      animation: 'spin 0.8s linear infinite',
    }}>
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}
