// Package ws provides an authenticated WebSocket endpoint backed by a single
// broadcast hub. The hub owns all connection state and runs in one goroutine,
// so client maps need no locks — every mutation flows through Run's channels.
package ws

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second    // time allowed to write a message
	pongWait       = 60 * time.Second    // time allowed to read the next pong
	pingPeriod     = (pongWait * 9) / 10 // ping interval; must be < pongWait
	maxMessageSize = 4096                // max inbound message bytes
	sendBuffer     = 16                  // per-client outbound queue depth
)

// Hub fans messages out to connected clients. Construct with NewHub and drive
// with Run.
type Hub struct {
	register   chan *Client
	unregister chan *Client
	broadcast  chan outbound
	clients    map[*Client]struct{}
	byUser     map[string]map[*Client]struct{}
}

type outbound struct {
	payload []byte
	userID  string // empty = all clients; otherwise only that user's clients
}

func NewHub() *Hub {
	return &Hub{
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan outbound, 256),
		clients:    make(map[*Client]struct{}),
		byUser:     make(map[string]map[*Client]struct{}),
	}
}

// Run processes hub events until ctx is cancelled, then closes every client.
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			for c := range h.clients {
				close(c.send)
			}
			h.clients = map[*Client]struct{}{}
			h.byUser = map[string]map[*Client]struct{}{}
			return

		case c := <-h.register:
			h.clients[c] = struct{}{}
			set := h.byUser[c.userID]
			if set == nil {
				set = make(map[*Client]struct{})
				h.byUser[c.userID] = set
			}
			set[c] = struct{}{}

		case c := <-h.unregister:
			h.remove(c)

		case msg := <-h.broadcast:
			targets := h.clients
			if msg.userID != "" {
				targets = h.byUser[msg.userID]
			}
			for c := range targets {
				select {
				case c.send <- msg.payload:
				default:
					// Slow client: drop it rather than block the hub.
					h.remove(c)
				}
			}
		}
	}
}

// remove is idempotent and only ever called from the Run goroutine.
func (h *Hub) remove(c *Client) {
	if _, ok := h.clients[c]; !ok {
		return
	}
	delete(h.clients, c)
	if set, ok := h.byUser[c.userID]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.byUser, c.userID)
		}
	}
	close(c.send)
}

// Broadcast sends payload to every connected client.
func (h *Hub) Broadcast(payload []byte) {
	h.broadcast <- outbound{payload: payload}
}

// SendToUser sends payload to all of a single user's connections (if any).
func (h *Hub) SendToUser(userID string, payload []byte) {
	h.broadcast <- outbound{payload: payload, userID: userID}
}

// Client is a single WebSocket connection. send is written only by the hub.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	userID string
	role   string
}

// readPump pumps inbound messages from the connection into the hub. The demo
// behaviour re-broadcasts each text message as a chat message; replace this
// with your own routing.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		c.hub.Broadcast(encode(Message{Type: "message", From: c.userID, Body: string(data)}))
	}
}

// writePump pumps hub messages to the connection and sends periodic pings.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok { // hub closed the channel
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
