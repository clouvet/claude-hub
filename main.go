package main

import (
	"log"
	"net/http"

	"claude-hub/internal/hub"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Only accept connections from localhost (sprite-mobile proxy)
		return true
	},
}

func serveWs(h *hub.Hub, w http.ResponseWriter, r *http.Request) {
	// Get session ID from query params
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "session parameter required", http.StatusBadRequest)
		return
	}

	// Upgrade connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}

	// Create and register client with hub
	client := h.NewClient(conn, sessionID, r.RemoteAddr)
	h.RegisterClient(client)

	// Start read and write pumps
	go client.WritePump()
	go client.ReadPump()
}

func main() {
	log.Println("Starting Claude Hub on :9090")

	// Create hub and run it
	h := hub.NewHub()
	go h.Run()

	// Set up HTTP routes
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(h, w, r)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Start server
	server := &http.Server{Addr: ":9090"}
	log.Fatal(server.ListenAndServe())
}
