package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/pkg/storage"
)

var (
	// ErrAudioUnavailable: lagu tidak punya berkas unggahan maupun URL eksternal.
	ErrAudioUnavailable = errors.New("audio is not available for this song")

	ErrEmptyAudioFile       = errors.New("audio file is empty")
	ErrAudioTooLarge        = errors.New("audio file is too large")
	ErrUnsupportedAudioType = errors.New("unsupported audio format")
	ErrInvalidAudioContent  = errors.New("invalid audio content: use MP3 or PCM WAV (8/16-bit mono/stereo)")
)

// Keep MIME types for legacy uploads even though new uploads accept MP3/WAV only.
var audioContentTypes = map[string]string{
	".mp3": "audio/mpeg",
	".wav": "audio/wav",
	".ogg": "audio/ogg",
	".m4a": "audio/mp4",
	".aac": "audio/aac",
}

// AudioSource adalah hasil resolusi berkas audio sebuah lagu.
// File bernilai nil kalau lagu hanya punya FileURL eksternal (data seeder) —
// handler akan mengalihkan (redirect) ke URL itu.
type AudioSource struct {
	Song *model.Song
	File io.ReadSeekCloser
	Size int64
}

type MediaService struct {
	repo    *repository.SongRepository
	storage storage.Storage
	maxSize int64
}

func NewMediaService(repo *repository.SongRepository, store storage.Storage, maxSize int64) *MediaService {
	return &MediaService{repo: repo, storage: store, maxSize: maxSize}
}

// ContentTypeForAudio menebak content type dari ekstensi key yang tersimpan.
func ContentTypeForAudio(key string) string {
	if contentType, ok := audioContentTypes[strings.ToLower(filepath.Ext(key))]; ok {
		return contentType
	}
	return "application/octet-stream"
}

// UploadAudio menyimpan berkas audio untuk sebuah lagu.
//
// Urutan operasinya disengaja: simpan berkas baru dulu, baru perbarui
// audio_key di database, dan hapus berkas lama paling akhir. Kalau ada langkah
// yang gagal, yang tertinggal hanya berkas yatim di storage — bukan baris
// database yang menunjuk ke berkas yang tidak ada.
func (s *MediaService) UploadAudio(
	ctx context.Context,
	songID uint,
	filename string,
	size int64,
	r io.Reader,
) (*model.Song, error) {
	song, err := s.repo.FindByID(ctx, songID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrSongNotFound
		}
		return nil, err
	}

	if size <= 0 {
		return nil, ErrEmptyAudioFile
	}
	if s.maxSize > 0 && size > s.maxSize {
		return nil, ErrAudioTooLarge
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".mp3" && ext != ".wav" {
		return nil, ErrUnsupportedAudioType
	}
	limit := s.maxSize
	if limit <= 0 {
		limit = 20 * 1024 * 1024
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read audio: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, ErrAudioTooLarge
	}
	if len(data) == 0 {
		return nil, ErrEmptyAudioFile
	}
	if !validAudio(data, ext) {
		return nil, ErrInvalidAudioContent
	}

	key := fmt.Sprintf("songs/%d-%d%s", songID, time.Now().UnixNano(), ext)
	if _, err := s.storage.Save(ctx, key, bytes.NewReader(data)); err != nil {
		_ = s.storage.Delete(ctx, key)
		return nil, fmt.Errorf("save audio: %w", err)
	}

	previousKey, err := s.repo.ReplaceAudioKey(ctx, songID, key)
	if err != nil {
		// Database gagal: jangan tinggalkan berkas yang tidak dirujuk siapa pun.
		_ = s.storage.Delete(ctx, key)
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrSongNotFound
		}
		return nil, fmt.Errorf("update song audio key: %w", err)
	}

	if previousKey != "" && previousKey != key {
		_ = s.storage.Delete(ctx, previousKey)
	}

	song.AudioKey = key
	return song, nil
}

// OpenAudio menyiapkan sumber audio sebuah lagu untuk streaming.
func (s *MediaService) OpenAudio(ctx context.Context, songID uint) (*AudioSource, error) {
	song, err := s.repo.FindByID(ctx, songID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrSongNotFound
		}
		return nil, err
	}

	if song.AudioKey != "" {
		file, size, err := s.storage.Open(ctx, song.AudioKey)
		if err != nil {
			return nil, fmt.Errorf("open audio: %w", err)
		}
		return &AudioSource{Song: song, File: file, Size: size}, nil
	}

	// Tanpa berkas unggahan: andalkan URL eksternal (data seeder).
	if song.FileURL == "" {
		return nil, ErrAudioUnavailable
	}

	return &AudioSource{Song: song}, nil
}
