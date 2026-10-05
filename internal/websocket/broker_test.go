package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// memoryBroker adalah Broker in-memory untuk test: meniru fan-out Redis
// Pub/Sub tanpa memerlukan server. Setiap subscriber menerima semua pesan;
// subscriber yang penuh dibuang, sama seperti Redis yang tidak pernah
// memblokir publisher.
type memoryBroker struct {
	mu          sync.Mutex
	subscribers map[chan BrokerMessage]struct{}
}

func newMemoryBroker() *memoryBroker {
	return &memoryBroker{subscribers: make(map[chan BrokerMessage]struct{})}
}

func (b *memoryBroker) Publish(_ context.Context, message BrokerMessage) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for subscriber := range b.subscribers {
		select {
		case subscriber <- message:
		default:
		}
	}
	return nil
}

func (b *memoryBroker) Subscribe(ctx context.Context) (<-chan BrokerMessage, error) {
	channel := make(chan BrokerMessage, brokerBufferSize)

	b.mu.Lock()
	b.subscribers[channel] = struct{}{}
	b.mu.Unlock()

	go func() {
		<-ctx.Done()

		// Close di bawah lock yang sama dengan Publish supaya tidak ada
		// pengiriman ke channel yang sudah ditutup.
		b.mu.Lock()
		delete(b.subscribers, channel)
		close(channel)
		b.mu.Unlock()
	}()

	return channel, nil
}

// waitForSubscribers menunggu sampai sejumlah langganan aktif.
//
// Tanpa ini test balapan dengan startup hub: Publish bisa terjadi sebelum
// consumeBroker selesai Subscribe, dan pesannya hilang — persis seperti Redis
// Pub/Sub yang fire-and-forget. Di produksi itu perilaku yang diterima; di
// test, ketidakpastian itu membuat hasilnya flaky.
func (b *memoryBroker) waitForSubscribers(t *testing.T, want int) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		count := len(b.subscribers)
		b.mu.Unlock()

		if count >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	b.mu.Lock()
	count := len(b.subscribers)
	b.mu.Unlock()
	t.Fatalf("broker hanya punya %d subscriber, mau %d", count, want)
}

// failingBroker meniru Redis yang sedang mati.
type failingBroker struct{}

func (failingBroker) Publish(context.Context, BrokerMessage) error {
	return errors.New("broker mati")
}

func (failingBroker) Subscribe(context.Context) (<-chan BrokerMessage, error) {
	return nil, errors.New("broker mati")
}

func startBrokerHub(t *testing.T, broker Broker, instanceID string) *Hub {
	t.Helper()

	hub := NewHub()
	hub.SetBroker(broker, instanceID)
	go hub.Run()
	t.Cleanup(hub.Stop)

	return hub
}

// receiveEvent menunggu satu event dari client; ok=false kalau timeout.
func receiveEvent(t *testing.T, client *Client, timeout time.Duration) (Event, bool) {
	t.Helper()

	select {
	case raw := <-client.send:
		var event Event
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("payload bukan JSON valid: %v", err)
		}
		return event, true
	case <-time.After(timeout):
		return Event{}, false
	}
}

// TestBrokerFanoutReachesOtherHubOnce mengunci dua hal sekaligus: client di
// instance lain menerima event, dan instance pengirim TIDAK memproses ulang
// pesannya sendiri (kalau tidak, client lokal menerima event dua kali).
func TestBrokerFanoutReachesOtherHubOnce(t *testing.T) {
	broker := newMemoryBroker()
	hubA := startBrokerHub(t, broker, "instance-a")
	hubB := startBrokerHub(t, broker, "instance-b")
	broker.waitForSubscribers(t, 2)

	local := newTestClient()
	remote := newTestClient()
	hubA.register <- local
	hubB.register <- remote

	hubA.Publish(Event{Type: "song:created", Payload: map[string]any{"id": 1}}, nil)

	if _, ok := receiveEvent(t, remote, 2*time.Second); !ok {
		t.Fatal("client di instance lain tidak menerima event")
	}

	if _, ok := receiveEvent(t, local, 2*time.Second); !ok {
		t.Fatal("client lokal tidak menerima event")
	}

	if event, ok := receiveEvent(t, local, 300*time.Millisecond); ok {
		t.Fatalf("client lokal menerima event dua kali: %+v", event)
	}
}

// TestBrokerTargetedEventReachesOnlyTargetAcrossHubs memastikan notifikasi
// tertarget tetap tertarget setelah menyeberang instance: hanya koneksi milik
// user tujuan di instance lain yang menerimanya.
func TestBrokerTargetedEventReachesOnlyTargetAcrossHubs(t *testing.T) {
	broker := newMemoryBroker()
	hubA := startBrokerHub(t, broker, "instance-a")
	hubB := startBrokerHub(t, broker, "instance-b")
	broker.waitForSubscribers(t, 2)

	target := newTestClient()
	otherOnB := newTestClient()
	onA := newTestClient()
	hubB.register <- target
	hubB.register <- otherOnB
	hubA.register <- onA

	hubA.PublishToUser(target.userID, "notification:new", map[string]any{"id": 7})

	if _, ok := receiveEvent(t, target, 2*time.Second); !ok {
		t.Fatal("user tujuan di instance lain tidak menerima notifikasi")
	}
	if event, ok := receiveEvent(t, otherOnB, 300*time.Millisecond); ok {
		t.Fatalf("user lain ikut menerima notifikasi: %+v", event)
	}
	if event, ok := receiveEvent(t, onA, 300*time.Millisecond); ok {
		t.Fatalf("client di instance asal ikut menerima notifikasi tertarget: %+v", event)
	}
}

// TestBrokerFailureKeepsLocalDeliveryWorking: Redis mati boleh mematikan
// fan-out lintas instance, tapi real-time di instance sendiri harus tetap
// jalan (prinsip fail-open yang sama dengan cache).
func TestBrokerFailureKeepsLocalDeliveryWorking(t *testing.T) {
	hub := startBrokerHub(t, failingBroker{}, "instance-a")

	client := newTestClient()
	hub.register <- client

	hub.Publish(Event{Type: "song:created"}, nil)

	if _, ok := receiveEvent(t, client, 2*time.Second); !ok {
		t.Fatal("client lokal tidak menerima event saat broker gagal")
	}
}
