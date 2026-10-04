package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// refreshTimeout membatasi berapa lama refresh di belakang boleh berjalan.
// Tanpa batas, loader yang menggantung akan menahan singleflight untuk key itu
// selamanya dan request stale berikutnya ikut menunggu.
const refreshTimeout = 10 * time.Second

type Store struct {
	rdb *redis.Client

	// group memastikan hanya SATU loader per key yang berjalan pada satu
	// waktu, sekaligus membuat request lain menunggu hasilnya.
	group singleflight.Group
}

func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

func (s *Store) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	raw, err := s.rdb.Get(ctx, key).Bytes()

	if errors.Is(err, redis.Nil) {
		return false, nil // cache miss, bukan kegagalan
	}
	if err != nil {
		return false, fmt.Errorf("cache get %s: %w", key, err)
	}

	if err := json.Unmarshal(raw, dest); err != nil {

		_ = s.rdb.Del(ctx, key).Err()
		return false, fmt.Errorf("cache unmarshal %s: %w", key, err)
	}

	return true, nil
}

func (s *Store) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache marshal %s: %w", key, err)
	}

	if err := s.rdb.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("cache set %s: %w", key, err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	if err := s.rdb.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache delete: %w", err)
	}
	return nil
}

// envelope adalah bentuk nilai yang disimpan GetOrLoadJSON. FetchedAt ikut
// disimpan supaya pembaca bisa tahu apakah nilainya masih segar atau sudah
// masuk jendela stale, tanpa key terpisah untuk timestamp.
type envelope struct {
	Value     json.RawMessage `json:"value"`
	FetchedAt time.Time       `json:"fetched_at"`
}

// GetOrLoadJSON mengembalikan nilai dari cache, atau memuatnya lewat loader
// kalau tidak ada. Dua perlindungan yang membedakannya dari GetJSON biasa:
//
//  1. Singleflight: saat cache miss, hanya SATU request yang menjalankan
//     loader; request lain untuk key yang sama menunggu hasilnya. Tanpa ini,
//     cache yang baru kedaluwarsa membuat puluhan request menabrak database
//     bersamaan (thundering herd).
//
//  2. Stale-while-revalidate: setelah TTL utama habis, nilai lama masih
//     disajikan sampai `ttl + staleTTL` sementara satu goroutine me-refresh di
//     belakang. Pembaca tidak pernah menunggu refresh; yang mereka lihat
//     paling lama adalah data satu generasi lebih tua.
//
// Redis yang bermasalah tidak menggagalkan request: loader dipanggil langsung
// (fail-open), sama seperti perilaku GetJSON di service.
func (s *Store) GetOrLoadJSON(
	ctx context.Context,
	key string,
	ttl time.Duration,
	staleTTL time.Duration,
	loader func(context.Context) (any, error),
	dest any,
) error {
	expiration := ttl + staleTTL

	raw, err := s.rdb.Get(ctx, key).Bytes()
	switch {
	case err == nil:
		value, fetchedAt, decodeErr := decodeEnvelope(raw)
		if decodeErr == nil && time.Since(fetchedAt) <= ttl {
			return json.Unmarshal(value, dest)
		}
		if decodeErr == nil {
			// Nilai lama tapi masih dalam jendela stale: sajikan sekarang,
			// segarkan di belakang.
			s.refreshInBackground(key, ttl, expiration, loader)
			return json.Unmarshal(value, dest)
		}

		// Nilai tidak dikenali (mis. sisa format lama sebelum cache v2):
		// buang supaya tidak menyajikan data yang salah bentuk.
		_ = s.rdb.Del(ctx, key).Err()

	case errors.Is(err, redis.Nil):
		// Miss biasa: lanjut memuat.

	default:
		log.Printf("cache: get %s gagal, memuat langsung dari sumber: %v", key, err)
	}

	// Jalur miss memakai singleflight yang sama dengan refresh di belakang,
	// jadi keduanya tidak pernah memuat key yang sama secara bersamaan.
	value, err, _ := s.group.Do(key, func() (any, error) {
		return s.loadAndStore(ctx, key, ttl, expiration, loader)
	})
	if err != nil {
		return err
	}

	return json.Unmarshal(value.(json.RawMessage), dest)
}

// loadAndStore memeriksa cache sekali lagi (request lain mungkin sudah
// mengisinya selagi kita menunggu giliran singleflight), lalu menjalankan
// loader dan menyimpan hasilnya.
func (s *Store) loadAndStore(
	ctx context.Context,
	key string,
	ttl time.Duration,
	expiration time.Duration,
	loader func(context.Context) (any, error),
) (json.RawMessage, error) {
	if raw, err := s.rdb.Get(ctx, key).Bytes(); err == nil {
		if value, fetchedAt, decodeErr := decodeEnvelope(raw); decodeErr == nil && time.Since(fetchedAt) <= ttl {
			return value, nil
		}
	}

	loaded, err := loader(ctx)
	if err != nil {
		return nil, err
	}

	value, err := json.Marshal(loaded)
	if err != nil {
		return nil, fmt.Errorf("cache marshal %s: %w", key, err)
	}

	raw, err := json.Marshal(envelope{Value: value, FetchedAt: time.Now().UTC()})
	if err != nil {
		return nil, fmt.Errorf("cache envelope %s: %w", key, err)
	}

	// Gagal menulis cache bukan alasan menggagalkan request: nilainya tetap
	// dikembalikan dari hasil loader.
	if err := s.rdb.Set(ctx, key, raw, expiration).Err(); err != nil {
		log.Printf("cache: set %s gagal, nilai tetap dipakai: %v", key, err)
	}

	return value, nil
}

// refreshInBackground menjalankan loadAndStore di goroutine terpisah. Semua
// request stale untuk key yang sama bergabung ke satu singleflight, jadi
// seratus pembaca stale tetap menghasilkan satu refresh.
func (s *Store) refreshInBackground(
	key string,
	ttl time.Duration,
	expiration time.Duration,
	loader func(context.Context) (any, error),
) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
		defer cancel()

		if _, err, _ := s.group.Do(key, func() (any, error) {
			return s.loadAndStore(ctx, key, ttl, expiration, loader)
		}); err != nil {
			// Nilai lama tetap disajikan; refresh akan dicoba lagi pada
			// request stale berikutnya.
			log.Printf("cache: refresh %s gagal: %v", key, err)
		}
	}()
}

// decodeEnvelope mengurai nilai yang disimpan GetOrLoadJSON. Nilai dengan
// bentuk tak dikenal (mis. JSON mentah dari SetJSON) ditolak supaya pembaca
// tidak menerima data setengah jadi.
func decodeEnvelope(raw []byte) (json.RawMessage, time.Time, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, time.Time{}, err
	}
	if env.Value == nil || env.FetchedAt.IsZero() {
		return nil, time.Time{}, errors.New("cache: envelope tidak lengkap")
	}
	return env.Value, env.FetchedAt, nil
}
