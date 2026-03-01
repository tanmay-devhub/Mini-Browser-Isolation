// Typed API client for the orchestrator REST endpoints.

export interface SessionCreateResponse {
  sessionId: string
  status: string
  createdAt: string
}

export interface SessionStatusResponse {
  sessionId: string
  status: 'starting' | 'ready' | 'error' | 'terminated'
  createdAt: string
  url: string
  error?: string
  metrics: {
    uptimeSec: number
    cpuPercent: number
    memMB: number
  }
}

export interface ICEConfigResponse {
  iceServers: RTCIceServer[]
}

export interface SDPPayload {
  type: string
  sdp: string
}

const BASE = '/api'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(`HTTP ${res.status}: ${text}`)
  }
  if (res.status === 204) return undefined as unknown as T
  return res.json() as Promise<T>
}

export const api = {
  createSession: (url: string) =>
    request<SessionCreateResponse>('/sessions', {
      method: 'POST',
      body: JSON.stringify({ url }),
    }),

  getSession: (id: string) =>
    request<SessionStatusResponse>(`/sessions/${id}`),

  deleteSession: (id: string) =>
    request<void>(`/sessions/${id}`, { method: 'DELETE' }),

  getICEConfig: (id: string) =>
    request<ICEConfigResponse>(`/sessions/${id}/ice`),

  sendOffer: (id: string, offer: SDPPayload) =>
    request<SDPPayload>(`/sessions/${id}/offer`, {
      method: 'POST',
      body: JSON.stringify(offer),
    }),
}
