//go:build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"spotify-backend/pkg/cache"
)

// TestCacheSingleflightLoadsOnce menabrakkan banyak request bersamaan ke key
// yang sama saat cache kosong. Loader harus jalan SEKALI, sisanya menunggu
// hasil yang sama — inilah anti thundering herd-nya.
func TestCacheSingleflightLoadsOnce(t *testing.T) {
	store := cache.NewStore(testApp.Redis)
	ctx := context.Background()
	key := "test:singleflight:" + t.Name()

	var calls atomic.Int64
	loader := func(context.Context) (any, error) {
		calls.Add(1)
		time.Sleep(150 * time.Millisecond)
		return []string{"a", "b"}, nil
	}

	const workers = 20
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			var dest []string
			if err := store.GetOrLoadJSON(ctx, key, time.Minute, time.Minute, loader, &dest); err != nil {
				errs <- err
				return
			}
			if len(dest) != 2 || dest[0] != "a" {
				errs <- fmt.Errorf("nilai cache salah: %v", dest)
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("GetOrLoadJSON: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader dipanggil %d kali, mau 1 (singleflight bocor)", got)
	}
}

// TestCacheStaleWhileRevalidate mengunci perilaku stale: setelah TTL utama
// habis, pembaca tetap dilayani nilai lama TANPA menunggu loader, sementara
// refresh berjalan di belakang dan nilai barunya menggantikan yang lama.
func TestCacheStaleWhileRevalidate(t *testing.T) {
	store := cache.NewStore(testApp.Redis)
	ctx := context.Background()
	key := "test:swr:" + t.Name()

	var calls atomic.Int64
	loader := func(context.Context) (any, error) {
		return []string{fmt.Sprintf("v%d", calls.Add(1))}, nil
	}

	var first []string
	if err := store.GetOrLoadJSON(ctx, key, 100*time.Millisecond, 10*time.Second, loader, &first); err != nil {
		t.Fatalf("muat pertama: %v", err)
	}
	if len(first) != 1 || first[0] != "v1" {
		t.Fatalf("nilai pertama = %v, mau [v1]", first)
	}

	// Lewati TTL utama supaya nilai masuk jendela stale.
	time.Sleep(150 * time.Millisecond)

	var second []string
	if err := store.GetOrLoadJSON(ctx, key, 100*time.Millisecond, 10*time.Second, loader, &second); err != nil {
		t.Fatalf("muat kedua: %v", err)
	}
	if len(second) != 1 || second[0] != "v1" {
		t.Fatalf("pembaca stale dapat %v, mau tetap v1", second)
	}

	// Refresh berjalan di belakang; tunggu nilainya benar-benar tergantikan.
	// Pemeriksaannya lewat isi cache, bukan jumlah panggilan loader, supaya
	// tidak balapan dengan penulisan hasil refresh.
	deadline := time.Now().Add(5 * time.Second)
	var third []string

	for time.Now().Before(deadline) {
		if err := store.GetOrLoadJSON(ctx, key, 100*time.Millisecond, 10*time.Second, loader, &third); err != nil {
			t.Fatalf("muat ketiga: %v", err)
		}
		if len(third) == 1 && third[0] != "v1" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if len(third) != 1 || third[0] == "v1" {
		t.Fatalf("nilai tidak pernah di-refresh, masih %v", third)
	}
	if calls.Load() < 2 {
		t.Fatalf("loader dipanggil %d kali, mau minimal 2 (refresh tidak jalan)", calls.Load())
	}
}
