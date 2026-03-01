package session_test

import (
	"testing"
	"time"

	"github.com/mini-browser-isolation/orchestrator/session"
	"go.uber.org/zap"
)

func newManager(max int) *session.Manager {
	log, _ := zap.NewDevelopment()
	return session.NewManager(max, 30*time.Minute, log)
}

func TestCreate(t *testing.T) {
	m := newManager(5)
	defer m.Stop()

	s, err := m.Create("https://example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if s.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if s.Status != session.StatusStarting {
		t.Fatalf("expected StatusStarting, got %s", s.Status)
	}
}

func TestMaxSessions(t *testing.T) {
	m := newManager(2)
	defer m.Stop()

	if _, err := m.Create("https://a.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create("https://b.com"); err != nil {
		t.Fatal(err)
	}
	_, err := m.Create("https://c.com")
	if err != session.ErrMaxSessions {
		t.Fatalf("expected ErrMaxSessions, got %v", err)
	}
}

func TestGetNotFound(t *testing.T) {
	m := newManager(5)
	defer m.Stop()

	_, err := m.Get("nonexistent-id")
	if err != session.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateStatus(t *testing.T) {
	m := newManager(5)
	defer m.Stop()

	s, _ := m.Create("https://example.com")
	if err := m.UpdateStatus(s.ID, session.StatusReady, ""); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get(s.ID)
	if got.Status != session.StatusReady {
		t.Fatalf("expected StatusReady, got %s", got.Status)
	}
}

func TestTerminate(t *testing.T) {
	m := newManager(5)
	defer m.Stop()

	s, _ := m.Create("https://example.com")
	if err := m.Terminate(s.ID); err != nil {
		t.Fatal(err)
	}

	// After termination, a new session should be creatable again
	// (terminated ones don't count against max).
	_, err := m.Create("https://example.com")
	if err != nil {
		t.Fatalf("expected to create session after termination, got %v", err)
	}
}

func TestTerminateNotFound(t *testing.T) {
	m := newManager(5)
	defer m.Stop()

	err := m.Terminate("does-not-exist")
	if err != session.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestActiveCount(t *testing.T) {
	m := newManager(10)
	defer m.Stop()

	s1, _ := m.Create("https://a.com")
	s2, _ := m.Create("https://b.com")

	if m.ActiveCount() != 2 {
		t.Fatalf("expected 2 active, got %d", m.ActiveCount())
	}

	m.Terminate(s1.ID)
	_ = s2

	if m.ActiveCount() != 1 {
		t.Fatalf("expected 1 active after termination, got %d", m.ActiveCount())
	}
}

func TestTouch(t *testing.T) {
	m := newManager(5)
	defer m.Stop()

	s, _ := m.Create("https://example.com")
	before := s.LastActive
	time.Sleep(10 * time.Millisecond)
	m.Touch(s.ID)

	got, _ := m.Get(s.ID)
	if !got.LastActive.After(before) {
		t.Fatal("expected LastActive to be updated by Touch")
	}
}
