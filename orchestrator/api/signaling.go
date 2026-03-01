package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mini-browser-isolation/orchestrator/config"
	"github.com/mini-browser-isolation/orchestrator/metrics"
	"github.com/mini-browser-isolation/orchestrator/session"
	"go.uber.org/zap"
)

// SignalingHandler proxies WebRTC SDP offer/answer between the frontend and the runner.
type SignalingHandler struct {
	manager *session.Manager
	cfg     *config.Config
	log     *zap.Logger
}

func NewSignalingHandler(m *session.Manager, cfg *config.Config, log *zap.Logger) *SignalingHandler {
	return &SignalingHandler{manager: m, cfg: cfg, log: log}
}

// Offer – POST /api/sessions/:id/offer
// The frontend sends an SDP offer; this handler proxies it to the runner,
// which returns an SDP answer. The answer is returned to the frontend.
func (h *SignalingHandler) Offer(c *gin.Context) {
	id := c.Param("id")
	sess, err := h.manager.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		metrics.SignalingRequestsTotal.WithLabelValues("offer", "404").Inc()
		return
	}

	if sess.Status != session.StatusReady {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("session not ready: %s", sess.Status)})
		metrics.SignalingRequestsTotal.WithLabelValues("offer", "409").Inc()
		return
	}

	// Read the raw SDP offer body (JSON: { "type": "offer", "sdp": "..." }).
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	// Proxy the offer to the runner's /offer endpoint.
	runnerURL := fmt.Sprintf("http://%s/offer", sess.RunnerAddr)
	h.log.Info("proxying SDP offer to runner",
		zap.String("sessionId", id),
		zap.String("runnerURL", runnerURL))

	resp, err := proxyPost(runnerURL, body)
	if err != nil {
		h.log.Error("runner offer proxy failed", zap.String("sessionId", id), zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "runner unreachable"})
		metrics.SignalingRequestsTotal.WithLabelValues("offer", "502").Inc()
		return
	}
	defer resp.Body.Close()

	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read runner answer"})
		return
	}

	metrics.SignalingRequestsTotal.WithLabelValues("offer", "200").Inc()
	c.Data(http.StatusOK, "application/json", answer)
}

// ICEConfig – GET /api/sessions/:id/ice
// Returns the ICE server configuration (STUN + optional TURN) for this session.
func (h *SignalingHandler) ICEConfig(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.manager.Get(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	iceServers := []gin.H{
		{"urls": []string{h.cfg.STUNHost}},
	}

	if h.cfg.TURNEnabled {
		iceServers = append(iceServers, gin.H{
			"urls":       []string{fmt.Sprintf("turn:%s:%s", h.cfg.TURNHost, h.cfg.TURNPort)},
			"username":   h.cfg.TURNUsername,
			"credential": h.cfg.TURNCredential,
		})
	}

	c.JSON(http.StatusOK, gin.H{"iceServers": iceServers})
}

// proxyPost sends a POST request with the given body and returns the response.
func proxyPost(url string, body []byte) (*http.Response, error) {
	return http.Post(url, "application/json", bytes.NewReader(body)) //nolint:gosec
}
