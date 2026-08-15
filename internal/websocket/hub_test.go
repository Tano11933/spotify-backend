package websocket

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Test di file ini menguji hub TANPA koneksi WebSocket sungguhan.
//
// Itu mungkin karena test berada di package yang sama (package websocket, bukan
// websocket_test), sehingga bisa membuat Client langsung dengan field unexported.
// Field conn dibiarkan nil dan itu aman: loop hub hanya menyentuh client.send
// dan client.userID — conn cuma dipakai oleh readPump/writePump, yang tidak
// dijalankan di sini.
//
// Nilai utama test ini adalah saat dijalankan dengan `go test -race`: race
// detector akan menandai kalau ada dua goroutine yang menyentuh map clients
// tanpa sinkronisasi.

func newTestClient() *Client {
	return &Client{
		userID: uuid.New(),
		send:   make(chan []byte, sendBufferSize),
	}
}

func TestHubBroadcastReachesAllClients(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	const clientCount = 5
	clients := make([]*Client, clientCount)
	for i := range clients {
		clients[i] = newTestClient()
		hub.register <- clients[i]
	}

	hub.Publish(Event{Type: "song:created", Payload: map[string]any{"id": 1}}, nil)

	for i, c := range clients {
		select {
		case raw := <-c.send:
			var event Event
			if err := json.Unmarshal(raw, &event); err != nil {
				t.Fatalf("client %d: payload bukan JSON valid: %v", i, err)
			}
			if event.Type != "song:created" {
				t.Errorf("client %d: type = %q, ingin %q", i, event.Type, "song:created")
			}
			if event.At.IsZero() {
				t.Errorf("client %d: field At tidak terisi otomatis", i)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("client %d tidak menerima broadcast", i)
		}
	}
}

func TestHubBroadcastSkipsExcludedClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	sender := newTestClient()
	other := newTestClient()
	hub.register <- sender
	hub.register <- other

	hub.Publish(Event{Type: "song:playing"}, sender)

	select {
	case <-other.send:
		// benar: client lain menerima
	case <-time.After(2 * time.Second):
		t.Fatal("client lain seharusnya menerima broadcast")
	}

	select {
	case raw := <-sender.send:
		t.Fatalf("pengirim seharusnya dikecualikan, tapi menerima: %s", raw)
	case <-time.After(200 * time.Millisecond):
		// benar: pengirim tidak menerima apa-apa
	}
}

func TestHubDropsSlowClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	// Client dengan buffer nol dan tanpa pembaca: pengiriman apapun akan
	// langsung "penuh". Hub harus membuangnya, bukan ikut memblokir.
	slow := &Client{userID: uuid.New(), send: make(chan []byte)}
	healthy := newTestClient()
	hub.register <- slow
	hub.register <- healthy

	hub.Publish(Event{Type: "song:created"}, nil)

	// Bukti bahwa hub tidak ikut membeku: client yang sehat tetap terlayani.
	select {
	case <-healthy.send:
	case <-time.After(2 * time.Second):
		t.Fatal("hub terblokir oleh client lambat — client sehat tidak terlayani")
	}

	// Channel client lambat harus sudah ditutup oleh hub.
	select {
	case _, open := <-slow.send:
		if open {
			t.Fatal("client lambat tidak dibuang")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel client lambat tidak ditutup")
	}
}

// TestHubConcurrentAccess adalah alasan utama file ini ada.
//
// Ia menabrakkan register, unregister, dan broadcast dari banyak goroutine
// sekaligus. Dijalankan dengan -race, ia membuktikan bahwa map clients milik hub
// benar-benar hanya disentuh satu goroutine.
func TestHubConcurrentAccess(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	const workers = 40

	// WaitGroup adalah penghitung: Add menaikkan, Done menurunkan, Wait
	// memblokir sampai nol. Ini cara menunggu sekumpulan goroutine selesai.
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			client := newTestClient()

			select {
			case hub.register <- client:
			case <-time.After(2 * time.Second):
				t.Error("register timeout")
				return
			}

			// Buang isi antrean supaya client ini tidak dianggap lambat.
			done := make(chan struct{})
			go func() {
				for range client.send {
				}
				close(done)
			}()

			hub.Publish(Event{Type: "song:created", Payload: map[string]any{"from": i}}, nil)

			select {
			case hub.unregister <- client:
			case <-time.After(2 * time.Second):
				t.Error("unregister timeout")
				return
			}

			<-done
		}()
	}

	wg.Wait()
}

func TestHubStopClosesAllClients(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := newTestClient()
	hub.register <- client

	hub.Stop()

	// Setelah Stop, channel send setiap client harus ditutup supaya writePump
	// masing-masing berhenti dan tidak ada goroutine yang bocor.
	select {
	case _, open := <-client.send:
		if open {
			t.Fatal("channel send masih terbuka setelah hub.Stop()")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hub.Stop() tidak menutup channel client")
	}
}
