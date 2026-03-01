package session

import "time"

// Status represents the lifecycle state of a browser session.
type Status string

const (
	StatusStarting    Status = "starting"
	StatusReady       Status = "ready"
	StatusError       Status = "error"
	StatusTerminated  Status = "terminated"
)

// Session represents a single isolated browser session.
type Session struct {
	ID          string    `json:"sessionId"`
	Status      Status    `json:"status"`
	URL         string    `json:"url"`
	ContainerID string    `json:"-"` // Docker container ID or K8s pod name
	RunnerAddr  string    `json:"-"` // internal address of runner (host:port)
	CreatedAt   time.Time `json:"createdAt"`
	LastActive  time.Time `json:"lastActive"`
	ErrorMsg    string    `json:"error,omitempty"`
}

// MetricsSnapshot is returned by GET /api/sessions/:id.
type MetricsSnapshot struct {
	SessionID   string    `json:"sessionId"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UptimeSec   float64   `json:"uptimeSec"`
	CPUPercent  float64   `json:"cpuPercent"`
	MemMB       float64   `json:"memMB"`
}
