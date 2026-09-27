// Package storage menyimpan berkas (audio) di balik interface kecil.
//
// Implementasi pertama menulis ke disk lokal; kalau nanti pindah ke
// S3/MinIO, cukup menambah implementasi baru — service dan handler tidak
// tahu-menahu soal lokasi fisik berkasnya.
package storage

import (
	"context"
	"io"
)

// Storage adalah penyimpanan berkas berbasis key, mis. "songs/12-1699.mp3".
type Storage interface {
	// Save menulis isi r ke key, menimpa kalau sudah ada, dan mengembalikan
	// jumlah byte yang tertulis.
	Save(ctx context.Context, key string, r io.Reader) (int64, error)

	// Open membuka berkas untuk dibaca. io.ReadSeekCloser dibutuhkan agar
	// handler bisa melayani HTTP Range (seek ke offset yang diminta).
	Open(ctx context.Context, key string) (io.ReadSeekCloser, int64, error)

	// Delete menghapus berkas. Menghapus key yang tidak ada bukan error.
	Delete(ctx context.Context, key string) error
}
