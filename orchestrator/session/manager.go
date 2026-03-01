package session

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ErrNotFound is returned when a session ID does not exist.
var ErrNotFound = errors.New("session not found")

// ErrMaxSessions is returned when the session limit is reached.
var ErrMaxSessions = errors.New("max concurrent sessions reached")

// Manager handles the in-memory session store, timeout enforcement, and lifecycle.
type Manager struct {
	mu         sync.RWMutex
	sessions   map[string]*Session
	maxSessions int
	timeout    time.Duration
	log        *zap.Logger
	stopCh     chan struct{}
}

// NewManager creates a Manager and starts the reaper goroutine.
func NewManager(maxSessions int, timeout time.Duration, log *zap.Logger) *Manager {
	m := &Manager{
		sessions:    make(map[string]*Session),
		maxSessions: maxSessions,
		timeout:     timeout,
		log:         log,
		stopCh:      make(chan struct{}),
	}
	go m.reaper()
	return m
}

// Create allocates a new session slot and returns it. The caller must update
// ContainerID and flip Status once the container is running.
func (m *Manager) Create(url string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Count only non-terminated sessions against the cap.
	active := 0
	for _, s := range m.sessions {
		if s.Status != StatusTerminated {
			active++
		}
	}
	if active >= m.maxSessions {
		return nil, ErrMaxSessions
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	sess := &Session{
		ID:         id,
		Status:     StatusStarting,
		URL:        url,
		CreatedAt:  now,
		LastActive: now,
	}
	m.sessions[id] = sess
	m.log.Info("session created", zap.String("sessionId", id), zap.String("url", url))
	return sess, nil
}

// Get returns the session by ID or ErrNotFound.
func (m *Manager) Get(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

// UpdateStatus sets the session status and resets LastActive.
func (m *Manager) UpdateStatus(id string, status Status, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	s.Status = status
	s.LastActive = time.Now().UTC()
	if errMsg != "" {
		s.ErrorMsg = errMsg
	}
	return nil
}

// SetContainerInfo records the container ID and internal runner address.
func (m *Manager) SetContainerInfo(id, containerID, runnerAddr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	s.ContainerID = containerID
	s.RunnerAddr = runnerAddr
	return nil
}

// Touch updates LastActive to prevent premature timeout during active use.
func (m *Manager) Touch(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.LastActive = time.Now().UTC()
	}
}

// Terminate marks the session as terminated (cleanup is done by the caller).
func (m *Manager) Terminate(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	s.Status = StatusTerminated
	m.log.Info("session terminated", zap.String("sessionId", id))
	return nil
}

// List returns a snapshot of all sessions.
func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		cp := *s
		out = append(out, &cp)
	}
	return out
}

// ActiveCount returns the number of non-terminated sessions.
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, s := range m.sessions {
		if s.Status != StatusTerminated {
			count++
		}
	}
	return count
}

// Stop shuts down the reaper goroutine.
func (m *Manager) Stop() {
	close(m.stopCh)
}

// reaper ticks every 30 seconds and terminates sessions that have been idle
// longer than the configured timeout.
func (m *Manager) reaper() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.reap()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) reap() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for id, s := range m.sessions {
		if s.Status == StatusTerminated {
			continue
		}
		if now.Sub(s.LastActive) > m.timeout {
			m.log.Info("session timed out", zap.String("sessionId", id),
				zap.Duration("idle", now.Sub(s.LastActive)))
			s.Status = StatusTerminated
			// The docker cleanup goroutine checks for terminated sessions.
		}
	}
}
