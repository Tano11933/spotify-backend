package websocket

import (
	"context"
	"encoding/json"
)

// Broker meneruskan event hub antar instance aplikasi.
//
// Tanpa broker, hub hanya tahu koneksi yang terhubung ke prosesnya sendiri.
// Dengan beberapa instance di belakang load balancer, client di instance A
// tidak akan menerima event yang dipublish instance B. Broker menutup jarak
// itu: setiap event yang dipublish lokal ikut dikirim ke channel bersama, dan
// setiap instance berlangganan channel yang sama.
//
// Implementasi nyata ada di RedisBroker; test memakai broker in-memory.
type Broker interface {
	Publish(ctx context.Context, message BrokerMessage) error

	// Subscribe mengembalikan channel pesan yang ditutup saat ctx selesai.
	Subscribe(ctx context.Context) (<-chan BrokerMessage, error)
}

// BrokerMessage adalah satu event yang menyeberang antar instance.
type BrokerMessage struct {
	// Origin adalah id instance pengirim. Instance itu tidak memproses ulang
	// pesannya sendiri supaya client lokal tidak menerima event dua kali
	// (sekali dari dispatch lokal, sekali dari langganan broker).
	Origin string `json:"origin"`

	// UserID kosong berarti broadcast ke semua client; terisi berarti event
	// hanya boleh sampai ke koneksi milik user tersebut.
	UserID string `json:"user_id,omitempty"`

	// Data adalah event yang SUDAH di-encode, jadi penerima tidak perlu
	// marshal ulang dan formatnya dijamin sama dengan dispatch lokal.
	Data json.RawMessage `json:"data"`
}
