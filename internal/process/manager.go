package process

import (
	"fmt"
	"log"
	"sync"
)

// Manager manages Claude processes
type Manager struct {
	processes map[string]*HeadlessProcess
	mu        sync.RWMutex
}

// NewManager creates a new process manager
func NewManager() *Manager {
	return &Manager{
		processes: make(map[string]*HeadlessProcess),
	}
}

// Spawn spawns a new headless Claude process for a session
func (m *Manager) Spawn(sessionID, cwd, claudeSessionID string) (*HeadlessProcess, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if process already exists
	if existing, ok := m.processes[sessionID]; ok {
		return existing, nil
	}

	// Create new process
	hp, err := NewHeadlessProcess(sessionID, cwd, claudeSessionID)
	if err != nil {
		return nil, err
	}

	m.processes[sessionID] = hp

	// Monitor process termination
	go func() {
		if err := hp.Wait(); err != nil {
			log.Printf("[%s] Claude process exited with error: %v", sessionID, err)
		} else {
			log.Printf("[%s] Claude process exited normally", sessionID)
		}

		// Clean up
		m.mu.Lock()
		delete(m.processes, sessionID)
		m.mu.Unlock()
	}()

	return hp, nil
}

// Get returns a process by session ID
func (m *Manager) Get(sessionID string) (*HeadlessProcess, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if hp, ok := m.processes[sessionID]; ok {
		return hp, nil
	}

	return nil, fmt.Errorf("no process found for session %s", sessionID)
}

// Kill terminates a process
func (m *Manager) Kill(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if hp, ok := m.processes[sessionID]; ok {
		if err := hp.Kill(); err != nil {
			return err
		}
		delete(m.processes, sessionID)
		return nil
	}

	return fmt.Errorf("no process found for session %s", sessionID)
}

// SendMessage sends a message to a session's Claude process
func (m *Manager) SendMessage(sessionID string, content interface{}) error {
	hp, err := m.Get(sessionID)
	if err != nil {
		return err
	}

	return hp.SendMessage(content)
}
