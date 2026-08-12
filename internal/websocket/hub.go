package websocket

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/google/uuid"
)

const (
	writeWait = 10 * time.Second
	pongWait = 60 * time.Second
	pingInterval = (pongWait * 9) / 10
	maxMessageSize = 4096
	sendBufferSize = 16
)

const EventConnectionAck = "connection:ack"

type ConnHandlers struct {
	// OnConnect dipanggil setelah client terdaftar di hub. Boleh nil.
	OnConnect func(client *Client)

	// OnMessage dipanggil untuk setiap pesan valid dari client. Boleh nil.
	OnMessage func(client *Client, event Event)
}

// Event adalah bentuk baku setiap pesan yang lewat di WebSocket, dua arah.
type Event struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`

	UserID string    `json:"user_id,omitempty"`
	At     time.Time `json:"at"`
}

// Client mewakili satu koneksi WebSocket.
type Client struct {
	conn   *websocket.Conn
	userID uuid.UUID

	send chan []byte
}

func (c *Client) UserID() uuid.UUID { return c.userID }

// broadcastRequest adalah pesan internal ke loop hub.
type broadcastRequest struct {
	data []byte

	exclude *Client
}

type Hub struct {
	clients map[*Client]struct{}

	register   chan *Client
	unregister chan *Client
	broadcast  chan broadcastRequest
	done       chan struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*Client]struct{}),

		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan broadcastRequest, 64),
		done:       make(chan struct{}),
	}
}

func (h *Hub) Run() {
	for {

		select {
		case client := <-h.register:
			h.clients[client] = struct{}{}
			log.Printf("websocket: client connected (user=%s, total=%d)", client.userID, len(h.clients))

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)

				close(client.send)

				log.Printf("websocket: client disconnected (user=%s, total=%d)", client.userID, len(h.clients))
			}

		case req := <-h.broadcast:
			h.dispatch(req)

		case <-h.done:
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			log.Println("websocket: hub stopped")
			return
		}
	}
}

func (h *Hub) dispatch(req broadcastRequest) {
	for client := range h.clients {
		if client == req.exclude {
			continue
		}

		select {
		case client.send <- req.data:
		default:
			log.Printf("websocket: dropping slow client (user=%s)", client.userID)
			delete(h.clients, client)
			close(client.send)
		}
	}
}

func (h *Hub) Stop() {
	close(h.done)
}

func (h *Hub) BroadcastEvent(eventType string, payload any) {
	h.Publish(Event{Type: eventType, Payload: payload}, nil)
}

func (h *Hub) Publish(event Event, exclude *Client) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("websocket: failed to encode event %s: %v", event.Type, err)
		return
	}

	select {
	case h.broadcast <- broadcastRequest{data: data, exclude: exclude}:
	case <-h.done:
	}
}

func (h *Hub) ServeConn(conn *websocket.Conn, userID uuid.UUID, handlers ConnHandlers) {
	client := &Client{
		conn:   conn,
		userID: userID,
		send:   make(chan []byte, sendBufferSize),
	}

	select {
	case h.register <- client:
	case <-h.done:
		_ = conn.Close()
		return
	}

	defer func() {
		select {
		case h.unregister <- client:
		case <-h.done:
		}
		_ = conn.Close()
	}()

	// writePump jalan di goroutine terpisah: satu-satunya penulis ke conn.
	go client.writePump()

	if handlers.OnConnect != nil {
		handlers.OnConnect(client)
	}

	client.readPump(h, handlers.OnMessage)
}

// readPump membaca pesan masuk sampai koneksi tertutup atau error.
func (c *Client) readPump(h *Hub, onMessage func(client *Client, event Event)) {
	c.conn.SetReadLimit(maxMessageSize)

	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		messageType, data, err := c.conn.ReadMessage()
		if err != nil {

			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseNormalClosure,
				websocket.CloseGoingAway,
				websocket.CloseNoStatusReceived,
			) {
				log.Printf("websocket: unexpected close (user=%s): %v", c.userID, err)
			}
			return
		}

		if messageType != websocket.TextMessage {
			continue
		}

		var event Event
		if err := json.Unmarshal(data, &event); err != nil {
			c.sendError("invalid json payload")
			continue
		}

		event.UserID = c.userID.String()

		if onMessage != nil {
			onMessage(c, event)
		}
	}
}

// writePump adalah satu-satunya goroutine yang menulis ke koneksi ini.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingInterval)

	defer ticker.Stop()

	for {
		select {
		case data, ok := <-c.send:

			if !ok {
				_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
				_ = c.conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}

			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
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

// Send mengirim satu event hanya ke client ini (bukan broadcast).
func (c *Client) Send(event Event) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	// Non-blocking, dengan alasan yang sama seperti di dispatch(): jangan pernah
	// menggantung karena satu client lambat.
	select {
	case c.send <- data:
	default:
	}
}

func (c *Client) sendError(message string) {
	c.Send(Event{Type: "error", Payload: map[string]string{"message": message}})
}
