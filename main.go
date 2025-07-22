package main

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

// Configure the WebSocket upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for simplicity in this example.
		// In production, you would restrict this to your actual client domains.
		return true
	},
}

func main() {
	// Initialize the Hub
	hub := NewHub()
	go hub.Run() // Run the hub in a goroutine

	// Serve static files for a simple HTML client (optional)
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/", fs)

	// WebSocket endpoint
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	log.Println("Chat server started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

// serveWs handles WebSocket requests from the peer.
func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256)}
	client.hub.Register <- client

	// Allow collection of old messages and close connection when client leaves
	go client.WritePump()
	go client.ReadPump()
}