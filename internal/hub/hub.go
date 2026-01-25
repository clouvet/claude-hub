package hub

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"claude-hub/internal/process"
	"claude-hub/internal/session"
	"claude-hub/internal/watcher"

	"github.com/gorilla/websocket"
)

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	// Sessions maps session ID to Session
	sessions map[string]*session.Session

	// Clients maps session ID to set of clients
	clients map[string]map[*Client]bool

	// Process manager
	processMgr *process.Manager

	// Terminal detector
	detector *process.TerminalDetector

	// Watchers maps session ID to file watcher
	watchers map[string]*watcher.SessionWatcher

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Broadcast message to all clients in a session
	broadcast chan *BroadcastMessage

	mu sync.RWMutex
}

// BroadcastMessage represents a message to broadcast to a session
type BroadcastMessage struct {
	SessionID string
	Data      []byte
	Exclude   *Client // Optional: don't send to this client
}

// NewHub creates a new Hub
func NewHub() *Hub {
	h := &Hub{
		sessions:   make(map[string]*session.Session),
		clients:    make(map[string]map[*Client]bool),
		processMgr: process.NewManager(),
		detector:   process.NewTerminalDetector(),
		watchers:   make(map[string]*watcher.SessionWatcher),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *BroadcastMessage, 256),
	}

	// Start terminal detection loop
	go h.detectTerminalLoop()

	return h
}

// NewClient creates a new client
func (h *Hub) NewClient(conn *websocket.Conn, sessionID, clientID string) *Client {
	return &Client{
		hub:       h,
		conn:      conn,
		send:      make(chan []byte, 256),
		sessionID: sessionID,
		clientID:  clientID,
	}
}

// RegisterClient registers a client with the hub
func (h *Hub) RegisterClient(client *Client) {
	h.register <- client
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastToSession(message)
		}
	}
}

func (h *Hub) registerClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Create client set for session if it doesn't exist
	if h.clients[client.sessionID] == nil {
		h.clients[client.sessionID] = make(map[*Client]bool)
	}

	// Add client to session
	h.clients[client.sessionID][client] = true

	// Create session if it doesn't exist
	sess := h.sessions[client.sessionID]
	if sess == nil {
		sess = session.NewSession(client.sessionID)
		h.sessions[client.sessionID] = sess
		log.Printf("Created new session: %s", client.sessionID)
	}

	sess.IncrementClients()

	log.Printf("Client %s connected to session %s (%d clients total)",
		client.clientID, client.sessionID, len(h.clients[client.sessionID]))

	// Spawn Claude process if this is the first client and no process exists
	if sess.GetClientCount() == 1 && sess.GetState() == session.StateIdle {
		go h.spawnClaudeForSession(client.sessionID, sess)
	}

	// Send system message to new client
	systemMsg := map[string]interface{}{
		"type":      "system",
		"message":   "Connected to Claude Hub (Phase 2)",
		"sessionId": client.sessionID,
	}
	if data, err := json.Marshal(systemMsg); err == nil {
		select {
		case client.send <- data:
		default:
			close(client.send)
			delete(h.clients[client.sessionID], client)
		}
	}
}

func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client.sessionID]; ok {
		if _, ok := h.clients[client.sessionID][client]; ok {
			delete(h.clients[client.sessionID], client)
			close(client.send)

			if sess := h.sessions[client.sessionID]; sess != nil {
				sess.DecrementClients()
			}

			log.Printf("Client %s disconnected from session %s (%d clients remaining)",
				client.clientID, client.sessionID, len(h.clients[client.sessionID]))

			// Clean up empty session (keep process running for now)
			if len(h.clients[client.sessionID]) == 0 {
				delete(h.clients, client.sessionID)
				log.Printf("No more clients for session %s (process still running)", client.sessionID)
			}
		}
	}
}

func (h *Hub) broadcastToSession(message *BroadcastMessage) {
	h.mu.RLock()
	clients := h.clients[message.SessionID]
	h.mu.RUnlock()

	for client := range clients {
		// Skip excluded client (e.g., the sender)
		if message.Exclude != nil && client == message.Exclude {
			continue
		}

		select {
		case client.send <- message.Data:
		default:
			// Client send buffer full, close it
			close(client.send)
			h.mu.Lock()
			delete(h.clients[message.SessionID], client)
			h.mu.Unlock()
		}
	}
}

// handleClientMessage processes messages from clients
func (h *Hub) handleClientMessage(client *Client, msg *ClientMessage) {
	log.Printf("[%s] Received %s message from client %s",
		client.sessionID, msg.Type, client.clientID)

	switch msg.Type {
	case "user":
		h.handleUserMessage(client, msg)

	case "interrupt":
		h.handleInterrupt(client)

	default:
		log.Printf("[%s] Unknown message type: %s", client.sessionID, msg.Type)
	}
}

// handleUserMessage sends a user message to Claude
func (h *Hub) handleUserMessage(client *Client, msg *ClientMessage) {
	// Broadcast to other clients
	userMsg := map[string]interface{}{
		"type": "user_message",
		"message": map[string]interface{}{
			"role":    "user",
			"content": msg.Content,
		},
	}

	data, _ := json.Marshal(userMsg)
	h.broadcast <- &BroadcastMessage{
		SessionID: client.sessionID,
		Data:      data,
		Exclude:   client,
	}

	// Send to Claude process
	if err := h.processMgr.SendMessage(client.sessionID, msg.Content); err != nil {
		log.Printf("[%s] Error sending message to Claude: %v", client.sessionID, err)
		errMsg, _ := json.Marshal(map[string]interface{}{
			"type":    "error",
			"message": "Failed to send message to Claude: " + err.Error(),
		})
		select {
		case client.send <- errMsg:
		default:
		}
	} else {
		// Send processing indicator
		processingMsg, _ := json.Marshal(map[string]interface{}{
			"type":         "processing",
			"isProcessing": true,
		})
		h.broadcast <- &BroadcastMessage{
			SessionID: client.sessionID,
			Data:      processingMsg,
		}
	}
}

// handleInterrupt interrupts the current Claude generation
func (h *Hub) handleInterrupt(client *Client) {
	if err := h.processMgr.Kill(client.sessionID); err != nil {
		log.Printf("[%s] Error interrupting Claude: %v", client.sessionID, err)
	} else {
		resultMsg, _ := json.Marshal(map[string]interface{}{
			"type": "result",
		})
		h.broadcast <- &BroadcastMessage{
			SessionID: client.sessionID,
			Data:      resultMsg,
		}

		// Respawn process for next message
		if sess := h.GetSession(client.sessionID); sess != nil {
			go h.spawnClaudeForSession(client.sessionID, sess)
		}
	}
}

// GetSession returns a session by ID (for future use)
func (h *Hub) GetSession(sessionID string) *session.Session {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sessions[sessionID]
}

// spawnClaudeForSession spawns a Claude process for a session
func (h *Hub) spawnClaudeForSession(sessionID string, sess *session.Session) {
	log.Printf("[%s] Spawning headless Claude process", sessionID)

	hp, err := h.processMgr.Spawn(sessionID, sess.CWD, sess.ClaudeUUID)
	if err != nil {
		log.Printf("[%s] Failed to spawn Claude: %v", sessionID, err)
		errMsg, _ := json.Marshal(map[string]interface{}{
			"type":    "error",
			"message": "Failed to start Claude process",
		})
		h.broadcast <- &BroadcastMessage{
			SessionID: sessionID,
			Data:      errMsg,
		}
		return
	}

	sess.SetState(session.StateWebOnly)

	// Handle output from Claude
	go h.handleClaudeOutput(sessionID, hp)
}

// handleClaudeOutput reads output from Claude and broadcasts to clients
func (h *Hub) handleClaudeOutput(sessionID string, hp *process.HeadlessProcess) {
	sess := h.GetSession(sessionID)

	// Register this PID with the detector
	if hp.Cmd != nil && hp.Cmd.Process != nil {
		h.detector.RegisterOwnPID(hp.Cmd.Process.Pid)
		defer h.detector.UnregisterOwnPID(hp.Cmd.Process.Pid)
	}

	for {
		select {
		case msg, ok := <-hp.OutputChan:
			if !ok {
				// Channel closed, process terminated
				log.Printf("[%s] Claude process output channel closed", sessionID)
				return
			}

			// Update session with Claude UUID if this is an init message
			if msg.Type == "system" && msg.Subtype == "init" && msg.SessionID != "" {
				if sess != nil {
					sess.SetClaudeUUID(msg.SessionID)
					log.Printf("[%s] Updated Claude UUID: %s", sessionID, msg.SessionID)
				}
			}

			// Marshal and broadcast to all clients
			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("[%s] Error marshaling Claude output: %v", sessionID, err)
				continue
			}

			h.broadcast <- &BroadcastMessage{
				SessionID: sessionID,
				Data:      data,
			}

		case err, ok := <-hp.ErrorChan:
			if !ok {
				return
			}
			log.Printf("[%s] Claude process error: %v", sessionID, err)
			errMsg, _ := json.Marshal(map[string]interface{}{
				"type":    "error",
				"message": err.Error(),
			})
			h.broadcast <- &BroadcastMessage{
				SessionID: sessionID,
				Data:      errMsg,
			}

		case <-hp.Done():
			log.Printf("[%s] Claude process context cancelled", sessionID)
			return
		}
	}
}

// detectTerminalLoop periodically scans for terminal Claude processes
func (h *Hub) detectTerminalLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		h.scanForTerminalSessions()
	}
}

// scanForTerminalSessions scans for terminal processes and handles state transitions
func (h *Hub) scanForTerminalSessions() {
	processes, err := h.detector.ScanForTerminalSessions()
	if err != nil {
		log.Printf("Error scanning for terminal sessions: %v", err)
		return
	}

	// Check each detected process
	for _, proc := range processes {
		h.handleTerminalDetected(proc.SessionID, proc.PID)
	}

	// Check for terminal processes that have exited
	h.checkTerminalExits()
}

// handleTerminalDetected handles when a terminal Claude process is detected
func (h *Hub) handleTerminalDetected(claudeUUID string, pid int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Find session by Claude UUID
	var sess *session.Session
	var sessionID string
	for id, s := range h.sessions {
		if s.ClaudeUUID == claudeUUID {
			sess = s
			sessionID = id
			break
		}
	}

	if sess == nil {
		// Unknown session, ignore
		return
	}

	// Check current state
	currentState := sess.GetState()

	if currentState == session.StateWebOnly {
		// Terminal process detected while in web-only mode
		// Transition: WEB_ONLY -> TRANSITIONING -> TERMINAL_ONLY
		log.Printf("[%s] Terminal process detected (PID: %d), killing headless", sessionID, pid)

		// Transition to TRANSITIONING
		sess.TransitionTo(session.StateTransitioning)

		// Kill headless process
		if err := h.processMgr.Kill(sessionID); err != nil {
			log.Printf("[%s] Error killing headless process: %v", sessionID, err)
		}

		// Transition to TERMINAL_ONLY
		sess.TransitionTo(session.StateTerminalOnly)

		// Start file watching
		h.startFileWatching(sessionID, sess)

		// Notify clients
		systemMsg, _ := json.Marshal(map[string]interface{}{
			"type":    "system",
			"message": "Switched to terminal mode - file watching active",
		})
		h.broadcast <- &BroadcastMessage{
			SessionID: sessionID,
			Data:      systemMsg,
		}
	}
}

// checkTerminalExits checks if terminal processes have exited
func (h *Hub) checkTerminalExits() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for sessionID, sess := range h.sessions {
		if sess.GetState() != session.StateTerminalOnly {
			continue
		}

		// Check if there's still a terminal process for this session
		proc, _ := h.detector.FindSessionProcess(sess.ClaudeUUID)
		if proc == nil {
			// Terminal process exited
			log.Printf("[%s] Terminal process exited, returning to web mode", sessionID)

			// Stop file watching
			h.stopFileWatching(sessionID)

			// Transition back to WEB_ONLY
			sess.TransitionTo(session.StateWebOnly)

			// Respawn headless if there are connected clients
			if sess.GetClientCount() > 0 {
				go h.spawnClaudeForSession(sessionID, sess)

				// Notify clients
				systemMsg, _ := json.Marshal(map[string]interface{}{
					"type":    "system",
					"message": "Terminal session ended - returning to web mode",
				})
				h.broadcast <- &BroadcastMessage{
					SessionID: sessionID,
					Data:      systemMsg,
				}
			}
		}
	}
}

// startFileWatching starts watching a session file
func (h *Hub) startFileWatching(sessionID string, sess *session.Session) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Don't start if already watching
	if h.watchers[sessionID] != nil {
		return
	}

	// Need Claude UUID to watch
	if sess.ClaudeUUID == "" {
		log.Printf("[%s] Cannot start watching: no Claude UUID", sessionID)
		return
	}

	w, err := watcher.NewSessionWatcher(sessionID, sess.ClaudeUUID)
	if err != nil {
		log.Printf("[%s] Failed to create watcher: %v", sessionID, err)
		return
	}

	h.watchers[sessionID] = w
	w.Start()

	// Handle watcher events
	go h.handleWatcherEvents(sessionID, w)
}

// stopFileWatching stops watching a session file
func (h *Hub) stopFileWatching(sessionID string) {
	h.mu.Lock()
	w := h.watchers[sessionID]
	delete(h.watchers, sessionID)
	h.mu.Unlock()

	if w != nil {
		w.Stop()
		log.Printf("[%s] Stopped file watching", sessionID)
	}
}

// handleWatcherEvents handles events from a file watcher
func (h *Hub) handleWatcherEvents(sessionID string, w *watcher.SessionWatcher) {
	for event := range w.Events() {
		// Convert parsed message to broadcast format
		broadcastMsg := map[string]interface{}{
			"type": "user_message",
			"message": map[string]interface{}{
				"role":      event.Role,
				"content":   event.Content,
				"timestamp": event.Timestamp,
			},
		}

		if event.Role == "assistant" {
			// Send as content delta for assistant messages
			broadcastMsg = map[string]interface{}{
				"type": "content_block_delta",
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": event.Content,
				},
			}
		}

		data, _ := json.Marshal(broadcastMsg)
		h.broadcast <- &BroadcastMessage{
			SessionID: sessionID,
			Data:      data,
		}
	}
}

