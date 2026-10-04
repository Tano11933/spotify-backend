package websocket

import (
	"context"
	"encoding/json"
	"log"

	"github.com/redis/go-redis/v9"
)

// DefaultBrokerChannel adalah channel Redis Pub/Sub untuk event hub.
const DefaultBrokerChannel = "ws:events"

// brokerBufferSize membatasi antrean pesan yang menunggu diproses hub.
// Pesan berlebih dibuang alih-alih memblokir pembaca channel Redis.
const brokerBufferSize = 64

// RedisBroker memakai Redis Pub/Sub sebagai jembatan antar instance.
//
// Redis dipilih karena sudah menjadi infrastruktur proyek ini (cache, rate
// limit, refresh token), jadi tidak ada komponen baru yang harus dioperasikan.
// Sifat Pub/Sub yang fire-and-forget cocok untuk event real-time: event yang
// lewat saat tidak ada subscriber memang boleh hilang, karena WebSocket juga
// tidak menjamin pengiriman.
type RedisBroker struct {
	rdb     *redis.Client
	channel string
}

func NewRedisBroker(rdb *redis.Client, channel string) *RedisBroker {
	if channel == "" {
		channel = DefaultBrokerChannel
	}
	return &RedisBroker{rdb: rdb, channel: channel}
}

func (b *RedisBroker) Publish(ctx context.Context, message BrokerMessage) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return b.rdb.Publish(ctx, b.channel, raw).Err()
}

func (b *RedisBroker) Subscribe(ctx context.Context) (<-chan BrokerMessage, error) {
	pubsub := b.rdb.Subscribe(ctx, b.channel)

	// Receive menunggu konfirmasi SUBSCRIBE dari Redis. Tanpa ini, event yang
	// dipublish tepat setelah Subscribe() bisa hilang karena langganannya
	// belum benar-benar aktif.
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, err
	}

	source := pubsub.Channel()
	out := make(chan BrokerMessage, brokerBufferSize)

	go func() {
		defer close(out)
		defer func() { _ = pubsub.Close() }()

		for message := range source {
			var parsed BrokerMessage
			if err := json.Unmarshal([]byte(message.Payload), &parsed); err != nil {
				// Satu payload rusak tidak boleh menghentikan langganan.
				log.Printf("websocket: broker message tidak bisa diurai: %v", err)
				continue
			}

			select {
			case out <- parsed:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}
