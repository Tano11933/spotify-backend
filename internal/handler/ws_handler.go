package handler

import (
	"context"
	"log"
	"time"

	fiberws "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"spotify-backend/internal/middleware"
	"spotify-backend/internal/service"
	ws "spotify-backend/internal/websocket"
)

type WSHandler struct {
	hub         *ws.Hub
	songService *service.SongService
}

func NewWSHandler(hub *ws.Hub, songService *service.SongService) *WSHandler {
	return &WSHandler{hub: hub, songService: songService}
}

func (h *WSHandler) UpgradeGuard(c *fiber.Ctx) error {
	if fiberws.IsWebSocketUpgrade(c) {
		return c.Next()
	}
	return c.Status(fiber.StatusUpgradeRequired).
		JSON(fiber.Map{"error": "websocket upgrade required"})
}

func (h *WSHandler) Handle() fiber.Handler {
	return fiberws.New(func(conn *fiberws.Conn) {
		userID, ok := conn.Locals(middleware.ContextUserID).(uuid.UUID)
		if !ok {
			log.Println("websocket: connection without valid user id, closing")
			return
		}

		h.hub.ServeConn(conn, userID, ws.ConnHandlers{
			OnConnect: h.onConnect,
			OnMessage: h.onMessage,
		})
	})
}

func (h *WSHandler) onConnect(client *ws.Client) {
	client.Send(ws.Event{
		Type:   ws.EventConnectionAck,
		UserID: client.UserID().String(),
	})
}

func (h *WSHandler) onMessage(client *ws.Client, event ws.Event) {
	switch event.Type {
	case service.EventSongPlaying:
		h.handleSongPlaying(client, event)

	case "ping":
		client.Send(ws.Event{Type: "pong"})

	default:
		client.Send(ws.Event{
			Type:    "error",
			Payload: map[string]string{"message": "unknown event type: " + event.Type},
		})
	}
}

func (h *WSHandler) handleSongPlaying(client *ws.Client, event ws.Event) {
	songID, ok := songIDFromPayload(event.Payload)
	if !ok {
		client.Send(ws.Event{
			Type:    "error",
			Payload: map[string]string{"message": "payload must contain a numeric song_id"},
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	song, err := h.songService.GetSongByID(ctx, songID)
	if err != nil {
		client.Send(ws.Event{
			Type:    "error",
			Payload: map[string]string{"message": err.Error()},
		})
		return
	}

	h.hub.Publish(ws.Event{
		Type:    service.EventSongPlaying,
		UserID:  client.UserID().String(),
		Payload: map[string]any{"song": song},
	}, client)
}

func songIDFromPayload(payload any) (uint, bool) {
	values, ok := payload.(map[string]any)
	if !ok {
		return 0, false
	}

	raw, ok := values["song_id"].(float64)
	if !ok || raw <= 0 {
		return 0, false
	}
	return uint(raw), true
}
