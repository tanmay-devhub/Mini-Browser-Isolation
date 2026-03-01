package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SessionsTotal counts session creation attempts by result (created|rejected).
	SessionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "browser_sessions_total",
		Help: "Total number of browser session creation attempts.",
	}, []string{"result"})

	// ActiveSessions tracks currently active (non-terminated) sessions.
	ActiveSessions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "browser_sessions_active",
		Help: "Number of currently active browser sessions.",
	})

	// SessionDurationSeconds records session lifetime on termination.
	SessionDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "browser_session_duration_seconds",
		Help:    "Duration of browser sessions from creation to termination.",
		Buckets: prometheus.DefBuckets,
	})

	// ContainerSpawnErrors counts errors spawning runner containers.
	ContainerSpawnErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "browser_container_spawn_errors_total",
		Help: "Total number of container spawn errors.",
	})

	// SignalingRequestsTotal counts HTTP signaling requests by method and status.
	SignalingRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "browser_signaling_requests_total",
		Help: "Total number of WebRTC signaling HTTP requests.",
	}, []string{"endpoint", "status"})

	// WebRTCFallbacks counts sessions that fell back to WebSocket streaming.
	WebRTCFallbacks = promauto.NewCounter(prometheus.CounterOpts{
		Name: "browser_webrtc_fallbacks_total",
		Help: "Total number of sessions that fell back to WebSocket streaming.",
	})

	// HTTPRequestDuration tracks REST API latency.
	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "browser_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5},
	}, []string{"method", "path", "status"})
)
