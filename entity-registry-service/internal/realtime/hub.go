package realtime

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Event struct {
	Type   string    `json:"type"`
	Entity string    `json:"entity,omitempty"`
	ID     string    `json:"id,omitempty"`
	At     time.Time `json:"at"`
}

type Hub struct {
	upgrader websocket.Upgrader

	mu      sync.Mutex
	clients map[*client]struct{}
}

type client struct {
	conn *websocket.Conn
	send chan []byte
}

func NewHub() *Hub {
	return &Hub{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(_ *http.Request) bool {
				// Served behind api-gateway which enforces auth.
				return true
			},
		},
		clients: map[*client]struct{}{},
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	c := &client{conn: conn, send: make(chan []byte, 16)}
	h.addClient(c)

	go h.writePump(c)
	h.readPump(c)
}

func (h *Hub) Broadcast(ev Event) {
	ev.At = time.Now().UTC()
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.send <- b:
		default:
			// Slow client; drop it.
			delete(h.clients, c)
			close(c.send)
			_ = c.conn.Close()
		}
	}
}

func (h *Hub) addClient(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
}

func (h *Hub) removeClient(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
		_ = c.conn.Close()
	}
}

func (h *Hub) readPump(c *client) {
	defer h.removeClient(c)
	c.conn.SetReadLimit(1024)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *Hub) writePump(c *client) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
