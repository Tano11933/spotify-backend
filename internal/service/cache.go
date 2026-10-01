package service

import (
	"fmt"
	"log"
)

const (
	cacheKeyArtistList = "artists:all"
	cacheKeyAlbumList  = "albums:all"
	cacheKeyGenreList  = "genres:all"

	// cacheKeyChartsRefreshed menandai kapan materialized view chart terakhir
	// di-refresh. ChartService memakai SETNX pada key ini sebagai penjadwal
	// "lazy": satu request yang menemukan penandanya kedaluwarsa melakukan
	// refresh, sisanya langsung membaca data yang ada.
	cacheKeyChartsRefreshed = "charts:refreshed_at"
)

func cacheKeyArtist(id uint) string { return fmt.Sprintf("artist:%d", id) }
func cacheKeyAlbum(id uint) string  { return fmt.Sprintf("album:%d", id) }
func cacheKeyGenre(id uint) string  { return fmt.Sprintf("genre:%d", id) }

func warnCache(op string, err error) {
	if err != nil {
		log.Printf("cache %s failed, falling back to database: %v", op, err)
	}
}
