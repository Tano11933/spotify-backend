package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/pkg/pagination"
)

// PlayerStateResponse adalah bentuk state pemutaran yang dikirim ke client.
// Song boleh null: user yang belum pernah memutar apa pun tetap menerima 200
// dengan state kosong, bukan 404 — "belum ada" di sini keadaan yang sah.
type PlayerStateResponse struct {
	Song            *model.Song `json:"song"`
	PositionSeconds int         `json:"position_seconds"`
	UpdatedAt       *time.Time  `json:"updated_at,omitempty"`
}

// PlayHistoryEntry menggabungkan waktu putar dengan objek lagunya.
type PlayHistoryEntry struct {
	PlayedAt time.Time  `json:"played_at"`
	Song     model.Song `json:"song"`
}

type PlayerService struct {
	repo        *repository.PlayerRepository
	songRepo    *repository.SongRepository
	broadcaster EventBroadcaster
}

func NewPlayerService(
	repo *repository.PlayerRepository,
	songRepo *repository.SongRepository,
	broadcaster EventBroadcaster,
) *PlayerService {
	return &PlayerService{repo: repo, songRepo: songRepo, broadcaster: broadcaster}
}

/* -------------------------------------------------------------------------
 * State pemutaran
 * ---------------------------------------------------------------------- */

func (s *PlayerService) GetState(ctx context.Context, userID uuid.UUID) (*PlayerStateResponse, error) {
	state, err := s.repo.GetState(ctx, userID)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return &PlayerStateResponse{}, nil
	}

	song, err := s.songRepo.FindByID(ctx, state.SongID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// FK cascade seharusnya mencegah ini; kalau toh terjadi, perlakukan
			// sebagai "tidak ada state" alih-alih membalas 500.
			return &PlayerStateResponse{}, nil
		}
		return nil, err
	}

	updatedAt := state.UpdatedAt
	return &PlayerStateResponse{
		Song:            song,
		PositionSeconds: state.PositionSeconds,
		UpdatedAt:       &updatedAt,
	}, nil
}

// UpdateState menyinkronkan posisi pemutaran (resume/progress) TANPA mencatat
// riwayat — memindahkan slider tidak dihitung sebagai satu kali putar.
func (s *PlayerService) UpdateState(ctx context.Context, userID uuid.UUID, songID uint, position int) (*PlayerStateResponse, error) {
	song, err := s.findSong(ctx, songID)
	if err != nil {
		return nil, err
	}

	if err := s.repo.UpsertState(ctx, userID, songID, position); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	return &PlayerStateResponse{Song: song, PositionSeconds: position, UpdatedAt: &now}, nil
}

// Play memulai pemutaran: menulis state, mencatat riwayat, menaikkan
// penghitung putar, lalu menyiarkannya lewat WebSocket.
//
// Menyimpan play_count di sini berarti cache detail album (yang meng-embed
// lagu) bisa tertinggal sampai TTL 5 menit — dapat diterima karena angka ini
// untuk analitik, dan invalidasi cache tiap pemutaran justru jauh lebih mahal.
func (s *PlayerService) Play(ctx context.Context, userID uuid.UUID, songID uint) (*PlayerStateResponse, error) {
	song, err := s.findSong(ctx, songID)
	if err != nil {
		return nil, err
	}

	if err := s.repo.UpsertState(ctx, userID, songID, 0); err != nil {
		return nil, err
	}
	if err := s.repo.AddHistory(ctx, userID, songID); err != nil {
		return nil, err
	}
	if err := s.repo.IncrementPlayCount(ctx, songID); err != nil {
		return nil, err
	}

	if s.broadcaster != nil {
		s.broadcaster.BroadcastUserEvent(userID.String(), EventSongPlaying, map[string]any{"song": song})
	}

	now := time.Now().UTC()
	return &PlayerStateResponse{Song: song, PositionSeconds: 0, UpdatedAt: &now}, nil
}

/* -------------------------------------------------------------------------
 * Antrean
 * ---------------------------------------------------------------------- */

func (s *PlayerService) GetQueue(ctx context.Context, userID uuid.UUID, params pagination.Params) (pagination.Page[model.Song], error) {
	songs, total, err := s.repo.FindQueue(ctx, userID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[model.Song]{}, err
	}
	return pagination.NewPage(songs, total, params), nil
}

func (s *PlayerService) AddToQueue(ctx context.Context, userID uuid.UUID, songID uint) error {
	if _, err := s.findSong(ctx, songID); err != nil {
		return err
	}
	return s.repo.AppendToQueue(ctx, userID, songID)
}

func (s *PlayerService) RemoveFromQueue(ctx context.Context, userID uuid.UUID, songID uint) error {
	return s.repo.RemoveFromQueue(ctx, userID, songID)
}

/* -------------------------------------------------------------------------
 * Riwayat
 * ---------------------------------------------------------------------- */

func (s *PlayerService) GetHistory(ctx context.Context, userID uuid.UUID, params pagination.Params) (pagination.Page[PlayHistoryEntry], error) {
	entries, total, err := s.repo.FindHistory(ctx, userID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[PlayHistoryEntry]{}, err
	}
	if len(entries) == 0 {
		return pagination.NewPage([]PlayHistoryEntry{}, total, params), nil
	}

	// Ambil lagunya sekaligus (satu query + preload), lalu susun ulang sesuai
	// urutan riwayat — riwayat tidak menyimpan detail lagu supaya tidak ada
	// data ganda yang bisa basi.
	ids := make([]uint, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.SongID)
	}

	songs, err := s.songRepo.FindByIDs(ctx, ids)
	if err != nil {
		return pagination.Page[PlayHistoryEntry]{}, err
	}

	byID := make(map[uint]model.Song, len(songs))
	for _, song := range songs {
		byID[song.ID] = song
	}

	result := make([]PlayHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		song, ok := byID[entry.SongID]
		if !ok {
			// Lagu terhapus di antara dua query — lewati entri ini.
			continue
		}
		result = append(result, PlayHistoryEntry{PlayedAt: entry.PlayedAt, Song: song})
	}

	return pagination.NewPage(result, total, params), nil
}

func (s *PlayerService) findSong(ctx context.Context, songID uint) (*model.Song, error) {
	song, err := s.songRepo.FindByID(ctx, songID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrSongNotFound
		}
		return nil, err
	}
	return song, nil
}
