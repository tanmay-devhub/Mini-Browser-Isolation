# Mini Browser Isolation System

A self-hosted browser isolation platform. Each session runs headless Chromium inside its own Docker container. The rendered viewport is streamed to the browser client via WebRTC (VP8 video) with an automatic JPEG-over-WebSocket fallback. Mouse, keyboard, and scroll input travels back through the same channel.

```
┌─────────────────────────────────────────────────────────────────┐
│                         User Browser                            │
│   React/Vite UI  ──WebRTC video──▶  <video>  /  WS fallback    │
│                  ◀──input events──  DataChannel / WebSocket     │
└────────────────────────────┬────────────────────────────────────┘
                             │  REST + WebSocket + WebRTC signal
┌────────────────────────────▼────────────────────────────────────┐
│                       Orchestrator (Go/Gin)                     │
│  POST /api/sessions   GET /api/sessions/:id   DELETE            │
│  POST /api/sessions/:id/offer   GET …/:id/ice                   │
│  GET  /ws/sessions/:id  (bidirectional WS proxy)                │
│  GET  /metrics   GET /healthz                                   │
└───────────────┬─────────────────────────────┬───────────────────┘
                │ Docker API                  │ HTTP proxy
    ┌───────────▼───────────┐    ┌────────────▼────────────┐
    │   Runner container    │    │   Runner container      │
    │  headless Chromium    │    │  headless Chromium      │
    │  Pion WebRTC          │    │  WS fallback streamer   │
    │  CDP input injection  │    │  CDP input injection    │
    └───────────────────────┘    └─────────────────────────┘
                             │
              ┌──────────────▼──────────────┐
              │  coturn  (STUN / TURN)       │
              └──────────────────────────────┘
              ┌──────────────────────────────┐
              │  Prometheus  +  Grafana       │
              └──────────────────────────────┘
```

## Requirements

| Tool | Version |
|---|---|
| Docker Desktop (or Docker Engine + Compose) | 24+ |
| Go | 1.24+ |
| Node.js | 20+ |
| GNU Make | any |

> **Linux only for production.** Chromium inside Docker runs on Linux containers. Docker Desktop on Windows/macOS works via the Linux VM.

## Quick start

```bash
git clone https://github.com/<you>/mini-browser-isolation.git
cd mini-browser-isolation

# Build images and start all services
make dev
```

Then open **http://localhost:5173** in your browser.

`make dev` builds:
- `mini-browser-orchestrator:latest` — the Go REST + signaling server
- `mini-browser-runner:latest` — the Chromium + WebRTC agent
- `mini-browser-frontend:latest` — the React/Vite UI

and starts: `orchestrator`, `coturn`, `frontend`, `prometheus`, `grafana`.

### Available `make` targets

| Target | Description |
|---|---|
| `make dev` | Build images + `docker compose up` |
| `make build-go` | Build orchestrator and runner binaries locally |
| `make build-frontend` | `vite build` the frontend |
| `make test-unit` | `go test ./...` for both Go modules |
| `make test-e2e` | Playwright e2e suite (requires running stack) |
| `make k8s-apply` | Apply Kubernetes manifests in `infra/k8s/` |

## Configuration

All configuration is via environment variables on the orchestrator container.

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8090` | Orchestrator HTTP port |
| `MAX_SESSIONS` | `10` | Maximum concurrent browser sessions |
| `SESSION_TIMEOUT` | `30m` | Idle session reaper interval |
| `RUNNER_IMAGE` | `mini-browser-runner:latest` | Docker image for runner containers |
| `RUNNER_NETWORK` | `mini-browser-net` | Docker network runner containers join |
| `RUNNER_SHM_SIZE` | `1073741824` | `/dev/shm` size in bytes (1 GB) |
| `RUNNER_CHROME_FLAGS` | see compose | Extra Chromium flags passed to runners |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `STUN_HOST` | `stun:stun.l.google.com:19302` | STUN server |
| `TURN_ENABLED` | `false` | Enable TURN relay |
| `TURN_HOST` | `coturn` | TURN server hostname |
| `TURN_PORT` | `3478` | TURN server port |
| `TURN_USERNAME` | `user` | TURN credential |
| `TURN_CREDENTIAL` | `password` | TURN credential |
| `WEBRTC_TIMEOUT_SEC` | `15` | Seconds before falling back to WebSocket |
| `FALLBACK_FPS` | `5` | WebSocket fallback target frame rate |

## API reference

### Sessions

| Method | Path | Body / Response |
|---|---|---|
| `POST` | `/api/sessions` | `{"url":"https://example.com"}` → `{"sessionId","status","createdAt"}` |
| `GET` | `/api/sessions/:id` | → `{"sessionId","status","url","error","metrics"}` |
| `DELETE` | `/api/sessions/:id` | `204 No Content` |

**Status values:** `pending` → `ready` → `terminated` / `error`

### WebRTC signaling

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/sessions/:id/ice` | ICE server config (STUN + TURN) |
| `POST` | `/api/sessions/:id/offer` | Proxy SDP offer to runner, returns SDP answer |

### WebSocket (fallback + input)

```
GET /ws/sessions/:id  (Upgrade: websocket)
```

Bidirectional proxy. The runner pushes `{"type":"frame","data":"<base64 PNG>"}` messages; the client sends JSON input events.

### Observability

| Endpoint | Description |
|---|---|
| `GET /healthz` | Liveness probe |
| `GET /metrics` | Prometheus scrape endpoint |
| `http://localhost:9090` | Prometheus UI |
| `http://localhost:3000` | Grafana (admin/admin) |

## Input event protocol

Input events are sent as JSON text frames over the WebSocket or WebRTC DataChannel:

```json
{ "type": "mousemove",   "x": 640, "y": 360 }
{ "type": "mousedown",   "x": 640, "y": 360, "button": "left" }
{ "type": "mouseup",     "x": 640, "y": 360, "button": "left" }
{ "type": "scroll",      "x": 640, "y": 360, "deltaX": 0, "deltaY": 100 }
{ "type": "keydown",     "key": "Enter" }
```

## Kubernetes

```bash
# Build and push images to your registry first, then:
make k8s-apply
```

Manifests in `infra/k8s/`:
- `orchestrator-deployment.yaml` — Deployment + Service + HPA (CPU 70%)
- `coturn-deployment.yaml`
- `prometheus-configmap.yaml`

## Project structure

```
mini-browser-isolation/
├── frontend/          # React 18 + TypeScript + Vite
├── orchestrator/      # Go – session management, signaling, metrics
├── runner/            # Go – headless Chromium + WebRTC + WS fallback
├── infra/
│   ├── docker-compose/
│   └── k8s/
├── tests/
│   ├── unit/
│   └── e2e/           # Playwright
├── Makefile
└── go.work
```

## Troubleshooting

**Black screen / no frames**
- Check runner logs: `docker logs <runner-container-id>`
- Chromium needs at least 512 MB RAM and 1 GB `/dev/shm`. Both are set in the compose file.

**WebRTC fails to connect, stuck on fallback**
- WebSocket fallback activates automatically after 15 s. This is expected on networks with strict NAT.
- Enable TURN: set `TURN_ENABLED=true` and provide credentials.

**"max concurrent sessions reached"**
- Increase `MAX_SESSIONS` or delete existing sessions via `DELETE /api/sessions/:id`.

**Runner container exits immediately**
- Docker's default `seccomp` profile blocks syscalls Chromium needs. The compose file sets `seccomp=unconfined` on runner containers automatically.

**`docker.sock` permission denied**
- The orchestrator runs as `root` (`user: "0:0"` in compose). On Linux hosts with rootless Docker, bind-mount the rootless socket instead.

## License

MIT — see [LICENSE](LICENSE).
