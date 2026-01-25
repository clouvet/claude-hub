package session

import (
	"sync"
	"time"
)

// SessionState represents the state of a session
type SessionState int

const (
	StateIdle SessionState = iota
	StateWebOnly
	StateTerminalOnly
	StateTransitioning
)

// Session represents a Claude Code session
type Session struct {
	ID           string
	ClaudeUUID   string // The .jsonl filename (will be set in Phase 2)
	CWD          string
	State        SessionState
	LastActivity time.Time

	mu sync.RWMutex
}

// NewSession creates a new session
func NewSession(id string) *Session {
	return &Session{
		ID:           id,
		State:        StateIdle,
		LastActivity: time.Now(),
		CWD:          "/home/sprite",
	}
}

// UpdateActivity updates the last activity time
func (s *Session) UpdateActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActivity = time.Now()
}

// GetState returns the current state
func (s *Session) GetState() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// SetState sets the session state
func (s *Session) SetState(state SessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
}
