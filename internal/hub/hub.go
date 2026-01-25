package hub

import (
	"encoding/json"
	"log"
	"sync"

	"claude-hub/internal/session"

	"github.com/gorilla/websocket"
)

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	// Sessions maps session ID to Session
	sessions map[string]*session.Session

	// Clients maps session ID to set of clients
	clients map[string]map[*Client]bool

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
	return &Hub{
		sessions:   make(map[string]*session.Session),
		clients:    make(map[string]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *BroadcastMessage, 256),
	}
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
	if h.sessions[client.sessionID] == nil {
		h.sessions[client.sessionID] = session.NewSession(client.sessionID)
		log.Printf("Created new session: %s", client.sessionID)
	}

	log.Printf("Client %s connected to session %s (%d clients total)",
		client.clientID, client.sessionID, len(h.clients[client.sessionID]))

	// Send system message to new client
	systemMsg := map[string]interface{}{
		"type":      "system",
		"message":   "Connected to Claude Hub",
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

			log.Printf("Client %s disconnected from session %s (%d clients remaining)",
				client.clientID, client.sessionID, len(h.clients[client.sessionID]))

			// Clean up empty session
			if len(h.clients[client.sessionID]) == 0 {
				delete(h.clients, client.sessionID)
				log.Printf("No more clients for session %s", client.sessionID)
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
		// For Phase 1, just echo the message back to all clients
		h.echoUserMessage(client, msg)

	case "interrupt":
		// For Phase 1, just log it
		log.Printf("[%s] Interrupt received (not yet implemented)", client.sessionID)

	default:
		log.Printf("[%s] Unknown message type: %s", client.sessionID, msg.Type)
	}
}

// echoUserMessage broadcasts a user message to all clients in the session
func (h *Hub) echoUserMessage(client *Client, msg *ClientMessage) {
	// Create a user_message broadcast
	userMsg := map[string]interface{}{
		"type": "user_message",
		"message": map[string]interface{}{
			"role":      "user",
			"content":   msg.Content,
			"timestamp": nil, // Will be populated in Phase 2
		},
	}

	data, err := json.Marshal(userMsg)
	if err != nil {
		log.Printf("Error marshaling user message: %v", err)
		return
	}

	// Broadcast to all clients except the sender
	h.broadcast <- &BroadcastMessage{
		SessionID: client.sessionID,
		Data:      data,
		Exclude:   client,
	}

	// Send confirmation to sender
	confirmMsg := map[string]interface{}{
		"type":    "system",
		"message": "Message received (Phase 1 - echo mode)",
	}
	if confirmData, err := json.Marshal(confirmMsg); err == nil {
		select {
		case client.send <- confirmData:
		default:
		}
	}
}

// GetSession returns a session by ID (for future use)
func (h *Hub) GetSession(sessionID string) *session.Session {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sessions[sessionID]
}
