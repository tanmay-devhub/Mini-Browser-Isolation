import { useRef, useCallback, useState } from 'react'
import { api } from '../api/client'

export type StreamMode = 'webrtc' | 'websocket' | 'none'

const WEBRTC_TIMEOUT_MS = 15_000

export function useWebRTC(sessionId: string | null) {
  const pcRef = useRef<RTCPeerConnection | null>(null)
  const dcRef = useRef<RTCDataChannel | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const [streamMode, setStreamMode] = useState<StreamMode>('none')
  const [fallbackSrc, setFallbackSrc] = useState<string | null>(null)
  const webrtcTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const sendInput = useCallback((event: object) => {
    const msg = JSON.stringify(event)
    if (dcRef.current?.readyState === 'open') {
      dcRef.current.send(msg)
    } else if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(msg)
    }
  }, [])

  const connectWebSocket = useCallback((sid: string) => {
    console.warn('[stream] Falling back to WebSocket streaming')
    setStreamMode('websocket')

    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${protocol}://${window.location.host}/ws/sessions/${sid}`)
    wsRef.current = ws

    // Keepalive pong handler.
    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        if (msg.type === 'frame' && msg.data) {
          setFallbackSrc(`data:image/png;base64,${msg.data}`)
        }
      } catch {
        // ignore non-JSON
      }
    }
    ws.onerror = (e) => console.error('[ws] error', e)
    ws.onclose = () => console.info('[ws] closed')
  }, [])

  const connect = useCallback(async (sid: string) => {
    if (!sid) return

    // Get ICE config from orchestrator.
    const iceConfig = await api.getICEConfig(sid)
    const pc = new RTCPeerConnection({ iceServers: iceConfig.iceServers })
    pcRef.current = pc

    // Add a transceiver for receiving video from the runner.
    pc.addTransceiver('video', { direction: 'recvonly' })

    // Open the "input" data channel for sending mouse/keyboard events.
    const dc = pc.createDataChannel('input', { ordered: false, maxRetransmits: 0 })
    dcRef.current = dc

    // Attach video stream to the <video> element when it arrives.
    pc.ontrack = (e) => {
      if (videoRef.current && e.streams[0]) {
        videoRef.current.srcObject = e.streams[0]
        videoRef.current.play().catch(console.warn)
        setStreamMode('webrtc')
        if (webrtcTimer.current) {
          clearTimeout(webrtcTimer.current)
          webrtcTimer.current = null
        }
      }
    }

    pc.oniceconnectionstatechange = () => {
      console.info('[ice] state:', pc.iceConnectionState)
      if (pc.iceConnectionState === 'failed' || pc.iceConnectionState === 'disconnected') {
        connectWebSocket(sid)
      }
    }

    // Fall back to WebSocket if WebRTC has not connected within WEBRTC_TIMEOUT_MS.
    webrtcTimer.current = setTimeout(() => {
      if (streamMode !== 'webrtc') {
        connectWebSocket(sid)
      }
    }, WEBRTC_TIMEOUT_MS)

    // Create and send SDP offer.
    const offer = await pc.createOffer()
    await pc.setLocalDescription(offer)

    // Retry signaling with exponential backoff (3 attempts).
    let delay = 500
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        const answer = await api.sendOffer(sid, {
          type: offer.type,
          sdp: offer.sdp ?? '',
        })
        await pc.setRemoteDescription({ type: answer.type as RTCSdpType, sdp: answer.sdp })
        return
      } catch (e) {
        console.warn(`[signaling] attempt ${attempt + 1} failed:`, e)
        if (attempt < 2) {
          await new Promise((r) => setTimeout(r, delay))
          delay *= 2
        } else {
          console.error('[signaling] all retries exhausted; falling back to WS')
          connectWebSocket(sid)
        }
      }
    }
  }, [connectWebSocket, streamMode])

  const disconnect = useCallback(() => {
    if (webrtcTimer.current) clearTimeout(webrtcTimer.current)
    dcRef.current?.close()
    pcRef.current?.close()
    wsRef.current?.close()
    dcRef.current = null
    pcRef.current = null
    wsRef.current = null
    setStreamMode('none')
    setFallbackSrc(null)
  }, [])

  return { connect, disconnect, sendInput, videoRef, streamMode, fallbackSrc }
}
