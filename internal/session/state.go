package session

import (
	"log"
)

// StateTransition represents a state transition
type StateTransition struct {
	From SessionState
	To   SessionState
}

// String returns a human-readable state name
func (s SessionState) String() string {
	switch s {
	case StateIdle:
		return "IDLE"
	case StateWebOnly:
		return "WEB_ONLY"
	case StateTerminalOnly:
		return "TERMINAL_ONLY"
	case StateTransitioning:
		return "TRANSITIONING"
	default:
		return "UNKNOWN"
	}
}

// TransitionTo attempts to transition to a new state
func (s *Session) TransitionTo(newState SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldState := s.State

	// Validate transition
	if !s.isValidTransition(oldState, newState) {
		log.Printf("[%s] Invalid state transition: %s -> %s",
			s.ID, oldState.String(), newState.String())
		return nil // Don't error, just log and ignore
	}

	s.State = newState
	log.Printf("[%s] State transition: %s -> %s",
		s.ID, oldState.String(), newState.String())

	return nil
}

// isValidTransition checks if a state transition is valid
func (s *Session) isValidTransition(from, to SessionState) bool {
	// Define valid transitions
	validTransitions := map[StateTransition]bool{
		{StateIdle, StateWebOnly}:               true,
		{StateWebOnly, StateTransitioning}:      true,
		{StateWebOnly, StateTerminalOnly}:       true, // Direct transition allowed
		{StateTransitioning, StateTerminalOnly}: true,
		{StateTerminalOnly, StateWebOnly}:       true,
		{StateWebOnly, StateIdle}:               true,
		{StateTerminalOnly, StateIdle}:          true,
	}

	return validTransitions[StateTransition{from, to}]
}

// ShouldSpawnHeadless returns true if we should spawn a headless process
func (s *Session) ShouldSpawnHeadless() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State == StateIdle && s.ConnectedClients > 0
}

// ShouldKillHeadless returns true if we should kill the headless process
func (s *Session) ShouldKillHeadless() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State == StateWebOnly
}

// ShouldStartWatching returns true if we should start file watching
func (s *Session) ShouldStartWatching() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State == StateTerminalOnly
}
