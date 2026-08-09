package service

import (
	"fmt"
	"log"
)

const (
	cacheKeyArtistList = "artists:all"
	cacheKeyAlbumList  = "albums:all"
)

func cacheKeyArtist(id uint) string { return fmt.Sprintf("artist:%d", id) }
func cacheKeyAlbum(id uint) string  { return fmt.Sprintf("album:%d", id) }

func warnCache(op string, err error) {
	if err != nil {
		log.Printf("cache %s failed, falling back to database: %v", op, err)
	}
}
