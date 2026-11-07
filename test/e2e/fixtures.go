package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"spotify-backend/internal/model"
	"spotify-backend/pkg/storage"
)

func seed(ctx context.Context, db *gorm.DB, dir string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte("BrowserTest123!"), bcrypt.MinCost)
	if err != nil {
		return err
	}
	db = db.WithContext(ctx)
	admin := model.User{
		Name: "Admin E2E", Email: "admin@e2e.test",
		PasswordHash: string(hash), Role: model.RoleAdmin,
	}
	if err := db.Create(&admin).Error; err != nil {
		return err
	}
	artist := model.Artist{Name: "Artis E2E"}
	if err := db.Create(&artist).Error; err != nil {
		return err
	}
	album := model.Album{
		Title: "Album E2E", ArtistID: artist.ID,
		ReleaseDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := db.Create(&album).Error; err != nil {
		return err
	}
	files, err := storage.NewLocal(dir)
	if err != nil {
		return err
	}
	for i, seconds := range []int{45, 30} {
		key := fmt.Sprintf("fixture-%d.wav", i+1)
		if _, err := files.Save(ctx, key, bytes.NewReader(wav(seconds, 220*(i+1)))); err != nil {
			return err
		}
		song := model.Song{
			Title: fmt.Sprintf("Nada E2E %d", i+1), Duration: seconds,
			ArtistID: artist.ID, AlbumID: &album.ID, AudioKey: key,
		}
		if err := db.Create(&song).Error; err != nil {
			return err
		}
	}
	return nil
}

// Generate original mono PCM tones without using copyrighted recordings.
func wav(seconds, frequency int) []byte {
	const sampleRate = 8000
	samples := sampleRate * seconds
	data := make([]byte, 44+samples*2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1) // PCM
	binary.LittleEndian.PutUint16(data[22:], 1) // Mono
	binary.LittleEndian.PutUint32(data[24:], sampleRate)
	binary.LittleEndian.PutUint32(data[28:], sampleRate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(samples*2))
	for i := range samples {
		sample := int16(1200 * math.Sin(2*math.Pi*float64(i*frequency)/sampleRate))
		binary.LittleEndian.PutUint16(data[44+i*2:], uint16(sample))
	}
	return data
}
