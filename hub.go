package main

import (
	"context"
	"log"

	"github.com/go-redis/redis/v8"
)

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	// This channel will now *only* be used for messages coming from local clients
	// that need to be published to Redis.
	Broadcast chan []byte

	// Register requests from the clients.
	Register chan *Client

	// Unregister requests from clients.
	Unregister chan *Client

	// Redis client for Pub/Sub
	redisClient *redis.Client

	// Context for Redis operations
	redisCtx context.Context
}

func NewHub() *Hub {
	ctx := context.Background()

	// Initialize Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // Redis server address
		Password: "",               // No password set
		DB:       0,                // Default DB
	})

	// Ping Redis to ensure connection
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	log.Println("Connected to Redis successfully.")

	return &Hub{
		Broadcast:   make(chan []byte), // This channel now means "publish to Redis"
		Register:    make(chan *Client),
		Unregister:  make(chan *Client),
		clients:     make(map[*Client]bool),
		redisClient: rdb,
		redisCtx:    ctx,
	}
}

func (h *Hub) Run() {
	// Goroutine for listening to Redis Pub/Sub messages
	go h.subscribeToRedis()

	for {
		select {
		case client := <-h.Register:
			h.clients[client] = true
			log.Printf("Client registered: %s (Total: %d)", client.conn.RemoteAddr().String(), len(h.clients))
			// Optionally, publish a presence message to Redis here

		case client := <-h.Unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Printf("Client unregistered: %s (Total: %d)", client.conn.RemoteAddr().String(), len(h.clients))
				// Optionally, publish a presence message to Redis here
			}
		case message := <-h.Broadcast:
			// A message was received from a client connected to THIS server instance.
			// The ONLY action here should be to publish it to Redis.
			// It will be received back via the subscribeToRedis routine for local broadcast.
			err := h.redisClient.Publish(h.redisCtx, "chat_channel", message).Err()
			if err != nil {
				log.Printf("Error publishing message to Redis: %v", err)
			}
			log.Printf("Published message to Redis: %s", message)
		}
	}
}

// subscribeToRedis listens for messages on the Redis Pub/Sub channel
// and broadcasts them to local clients.
func (h *Hub) subscribeToRedis() {
	pubsub := h.redisClient.Subscribe(h.redisCtx, "chat_channel")
	defer pubsub.Close()

	ch := pubsub.Channel()

	for msg := range ch {
		log.Printf("Received message from Redis: %s", msg.Payload)
		// A message was received from Redis (published by any server in the cluster).
		// Now, broadcast it to all clients connected to THIS server instance.
		h.broadcastToLocalClients([]byte(msg.Payload))
	}
}

// broadcastToLocalClients sends a message to all clients currently connected to THIS server instance.
func (h *Hub) broadcastToLocalClients(message []byte) {
	for client := range h.clients {
		select {
		case client.send <- message:
		default:
			// If client's send buffer is full, disconnect the client
			close(client.send)
			delete(h.clients, client)
			log.Printf("Disconnected client due to full send buffer: %s", client.conn.RemoteAddr().String())
		}
	}
}