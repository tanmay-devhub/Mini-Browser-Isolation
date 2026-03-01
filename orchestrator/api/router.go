package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mini-browser-isolation/orchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewRouter builds and returns the Gin engine with all routes registered.
func NewRouter(sh *SessionHandler, sig *SignalingHandler) *gin.Engine {
	r := gin.New()

	// Structured JSON logging + recovery middleware.
	r.Use(gin.Recovery())
	r.Use(requestLogger())
	r.Use(prometheusMiddleware())

	// Liveness probe for k8s / docker-compose health checks.
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Prometheus metrics scrape endpoint.
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	api := r.Group("/api")
	{
		sessions := api.Group("/sessions")
		{
			sessions.POST("", sh.Create)
			sessions.GET("/:id", sh.Get)
			sessions.DELETE("/:id", sh.Delete)

			// WebRTC signaling sub-routes.
			sessions.POST("/:id/offer", sig.Offer)
			sessions.GET("/:id/ice", sig.ICEConfig)
		}
	}

	// WebSocket signaling + fallback frame streaming.
	r.GET("/ws/sessions/:id", sh.WebSocket)

	return r
}

// prometheusMiddleware records request duration and count.
func prometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		dur := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		metrics.HTTPRequestDuration.WithLabelValues(c.Request.Method, c.FullPath(), status).Observe(dur)
	}
}

// requestLogger suppresses gin's default text logger (zap handles structured logging).
func requestLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(_ gin.LogFormatterParams) string { return "" })
}
