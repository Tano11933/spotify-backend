//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	ws "spotify-backend/internal/websocket"
)

// TestRedisBrokerDeliversAcrossInstances mengunci adapter Redis Pub/Sub dengan
// Redis sungguhan: pesan yang dipublish satu broker sampai ke subscriber
// broker lain, lengkap dengan origin dan payload-nya. Logika hub-nya sendiri
// (fan-out, skip origin, tertarget) diuji terpisah di package websocket.
func TestRedisBrokerDeliversAcrossInstances(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Channel unik per test supaya tidak bercampur dengan trafik aplikasi
	// yang berjalan di suite ini.
	channel := "ws:events:test:" + t.Name()

	instanceA := ws.NewRedisBroker(testApp.Redis, channel)
	instanceB := ws.NewRedisBroker(testApp.Redis, channel)

	messages, err := instanceB.Subscribe(ctx)
	if err != nil {
		t.Fatalf("subscribe broker: %v", err)
	}

	payload := []byte(`{"type":"song:created","payload":{"id":1}}`)
	if err := instanceA.Publish(ctx, ws.BrokerMessage{Origin: "instance-a", Data: payload}); err != nil {
		t.Fatalf("publish broker: %v", err)
	}

	select {
	case message := <-messages:
		if message.Origin != "instance-a" {
			t.Fatalf("origin = %q, mau instance-a", message.Origin)
		}
		if string(message.Data) != string(payload) {
			t.Fatalf("data = %s, mau %s", message.Data, payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pesan tidak sampai lewat Redis")
	}
}
