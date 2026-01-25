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
	ID              string
	ClaudeUUID      string // The .jsonl filename
	CWD             string
	State           SessionState
	LastActivity    time.Time
	ConnectedClients int

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

// SetClaudeUUID sets the Claude session UUID
func (s *Session) SetClaudeUUID(uuid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ClaudeUUID = uuid
}

// IncrementClients increments the connected client count
func (s *Session) IncrementClients() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ConnectedClients++
}

// DecrementClients decrements the connected client count
func (s *Session) DecrementClients() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ConnectedClients > 0 {
		s.ConnectedClients--
	}
}

// GetClientCount returns the number of connected clients
func (s *Session) GetClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ConnectedClients
}
