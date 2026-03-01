package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mini-browser-isolation/orchestrator/metrics"
	"github.com/mini-browser-isolation/orchestrator/runner"
	"github.com/mini-browser-isolation/orchestrator/session"
	"go.uber.org/zap"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // allow all origins for MVP
}

// SessionHandler handles CRUD operations on browser sessions.
type SessionHandler struct {
	manager *session.Manager
	docker  *runner.DockerRunner
	log     *zap.Logger
}

func NewSessionHandler(m *session.Manager, d *runner.DockerRunner, log *zap.Logger) *SessionHandler {
	return &SessionHandler{manager: m, docker: d, log: log}
}

// createRequest is the body for POST /api/sessions.
type createRequest struct {
	URL string `json:"url" binding:"required"`
}

// Create – POST /api/sessions
func (h *SessionHandler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sess, err := h.manager.Create(req.URL)
	if err != nil {
		if err == session.ErrMaxSessions {
			metrics.SessionsTotal.WithLabelValues("rejected").Inc()
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "max concurrent sessions reached"})
			return
		}
		metrics.SessionsTotal.WithLabelValues("error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	metrics.SessionsTotal.WithLabelValues("created").Inc()
	metrics.ActiveSessions.Inc()

	h.log.Info("spawning runner container",
		zap.String("sessionId", sess.ID),
		zap.String("url", req.URL))

	// Spawn the runner container asynchronously so the client gets a sessionId
	// immediately and can begin polling GET /api/sessions/:id for readiness.
	go h.spawnRunner(sess.ID, req.URL)

	c.JSON(http.StatusCreated, gin.H{
		"sessionId": sess.ID,
		"status":    sess.Status,
		"createdAt": sess.CreatedAt,
	})
}

// spawnRunner runs in a goroutine: creates the container and updates session state.
func (h *SessionHandler) spawnRunner(sessionID, targetURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := h.docker.Spawn(ctx, sessionID, targetURL)
	if err != nil {
		h.log.Error("failed to spawn runner", zap.String("sessionId", sessionID), zap.Error(err))
		metrics.ContainerSpawnErrors.Inc()
		_ = h.manager.UpdateStatus(sessionID, session.StatusError, err.Error())
		metrics.ActiveSessions.Dec()
		return
	}

	_ = h.manager.SetContainerInfo(sessionID, result.ContainerID, result.RunnerAddr)

	// Poll runner /healthz until it's up (max 30 s).
	if err := waitForRunner(ctx, result.RunnerAddr, h.log); err != nil {
		h.log.Error("runner never became healthy", zap.String("sessionId", sessionID), zap.Error(err))
		_ = h.manager.UpdateStatus(sessionID, session.StatusError, "runner health check failed")
		_ = h.docker.Stop(context.Background(), result.ContainerID)
		metrics.ActiveSessions.Dec()
		return
	}

	_ = h.manager.UpdateStatus(sessionID, session.StatusReady, "")
	h.log.Info("session ready", zap.String("sessionId", sessionID))
}

// waitForRunner polls http://<addr>/healthz with exponential backoff until ready.
func waitForRunner(ctx context.Context, addr string, log *zap.Logger) error {
	url := "http://" + addr + "/healthz"
	backoff := 500 * time.Millisecond
	client := &http.Client{Timeout: 2 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}

		log.Debug("waiting for runner", zap.String("addr", addr), zap.Duration("backoff", backoff))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		// Exponential backoff capped at 4 s.
		if backoff < 4*time.Second {
			backoff *= 2
		}
	}
}

// Get – GET /api/sessions/:id
func (h *SessionHandler) Get(c *gin.Context) {
	id := c.Param("id")
	sess, err := h.manager.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	uptime := time.Since(sess.CreatedAt).Seconds()
	var cpuPct, memMB float64
	if sess.ContainerID != "" && sess.Status == session.StatusReady {
		cpuPct, memMB = h.docker.Stats(c.Request.Context(), sess.ContainerID)
	}

	c.JSON(http.StatusOK, gin.H{
		"sessionId": sess.ID,
		"status":    sess.Status,
		"createdAt": sess.CreatedAt,
		"url":       sess.URL,
		"error":     sess.ErrorMsg,
		"metrics": gin.H{
			"uptimeSec":  uptime,
			"cpuPercent": cpuPct,
			"memMB":      memMB,
		},
	})
}

// Delete – DELETE /api/sessions/:id
func (h *SessionHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	sess, err := h.manager.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	if sess.Status == session.StatusTerminated {
		c.Status(http.StatusNoContent)
		return
	}

	start := sess.CreatedAt
	if err := h.manager.Terminate(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	metrics.ActiveSessions.Dec()
	metrics.SessionDurationSeconds.Observe(time.Since(start).Seconds())

	if sess.ContainerID != "" {
		go func() {
			if err := h.docker.Stop(context.Background(), sess.ContainerID); err != nil {
				h.log.Error("failed to stop container",
					zap.String("containerID", sess.ContainerID[:12]),
					zap.Error(err))
			}
		}()
	}

	c.Status(http.StatusNoContent)
}

// WebSocket – GET /ws/sessions/:id
// Full bidirectional proxy:
//   client → orchestrator → runner  (input events)
//   runner → orchestrator → client  (frame data / status)
func (h *SessionHandler) WebSocket(c *gin.Context) {
	id := c.Param("id")
	sess, err := h.manager.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Upgrade browser connection.
	clientConn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.log.Error("ws upgrade failed", zap.String("sessionId", id), zap.Error(err))
		return
	}
	defer clientConn.Close()

	h.log.Info("ws client connected", zap.String("sessionId", id))

	// Connect to the runner WS.
	runnerConn := connectRunnerWS(sess.RunnerAddr, id, h.log)
	if runnerConn == nil {
		h.log.Warn("runner WS unavailable, closing client", zap.String("sessionId", id))
		return
	}
	defer runnerConn.Close()

	// done is closed when either side disconnects.
	done := make(chan struct{})
	closeOnce := sync.Once{}
	closeDone := func() { closeOnce.Do(func() { close(done) }) }

	// runner → client: forward every frame message the runner sends.
	go func() {
		defer closeDone()
		for {
			msgType, msg, err := runnerConn.ReadMessage()
			if err != nil {
				h.log.Info("runner WS closed", zap.String("sessionId", id), zap.Error(err))
				return
			}
			if err := clientConn.WriteMessage(msgType, msg); err != nil {
				h.log.Info("client WS write error", zap.String("sessionId", id), zap.Error(err))
				return
			}
		}
	}()

	// client → runner: forward input events and handle pings.
	go func() {
		defer closeDone()
		// Reset read deadline on every pong so the connection stays alive.
		clientConn.SetReadDeadline(time.Now().Add(60 * time.Second))
		clientConn.SetPongHandler(func(string) error {
			clientConn.SetReadDeadline(time.Now().Add(60 * time.Second))
			h.manager.Touch(id)
			return nil
		})
		for {
			msgType, msg, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
			h.manager.Touch(id)
			if msgType == websocket.TextMessage {
				_ = runnerConn.WriteMessage(websocket.TextMessage, msg)
			}
		}
	}()

	// Keepalive pings to the browser every 15 s.
	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-done:
			return
		case <-pingTicker.C:
			if err := clientConn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// connectRunnerWS dials the runner WebSocket with retries (runner may still be starting).
func connectRunnerWS(runnerAddr, sessionID string, log *zap.Logger) *websocket.Conn {
	if runnerAddr == "" {
		return nil
	}
	wsURL := "ws://" + runnerAddr + "/ws"
	backoff := 300 * time.Millisecond
	for attempt := 0; attempt < 8; attempt++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err == nil {
			log.Info("connected to runner WS", zap.String("sessionId", sessionID), zap.String("url", wsURL))
			return conn
		}
		log.Debug("runner WS dial attempt failed, retrying",
			zap.String("sessionId", sessionID),
			zap.Int("attempt", attempt+1),
			zap.Duration("backoff", backoff),
			zap.Error(err))
		time.Sleep(backoff)
		if backoff < 3*time.Second {
			backoff *= 2
		}
	}
	log.Warn("could not connect to runner WS after retries", zap.String("sessionId", sessionID))
	return nil
}
