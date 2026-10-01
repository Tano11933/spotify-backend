package service

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/pkg/pagination"
)

// ChartSongEntry adalah lagu chart dengan jumlah putar 7 hari sebagai field
// tambahan. Embedding membuat field lagunya tetap rata di JSON.
type ChartSongEntry struct {
	model.Song
	Plays int64 `json:"plays"`
}

// ChartArtistEntry sama polanya untuk artist.
type ChartArtistEntry struct {
	model.Artist
	Plays int64 `json:"plays"`
}

type ChartService struct {
	repo       *repository.ChartRepository
	songRepo   *repository.SongRepository
	redis      *redis.Client
	refreshTTL time.Duration
}

func NewChartService(
	repo *repository.ChartRepository,
	songRepo *repository.SongRepository,
	redisClient *redis.Client,
	refreshTTL time.Duration,
) *ChartService {
	return &ChartService{repo: repo, songRepo: songRepo, redis: redisClient, refreshTTL: refreshTTL}
}

// ensureFresh menyegarkan materialized view kalau penanda di Redis sudah
// kedaluwarsa. SETNX membuat hanya SATU request yang melakukan refresh;
// request lain langsung membaca data yang ada, jadi latency chart tidak
// menunggu refresh. Worker terjadwal menyusul di Phase E.
//
// Kegagalan refresh tidak fatal: data lama tetap dibaca dan penandanya dihapus
// supaya request berikutnya mencoba lagi, bukan menunggu TTL penuh.
func (s *ChartService) ensureFresh(ctx context.Context) {
	ok, err := s.redis.SetNX(ctx, cacheKeyChartsRefreshed, time.Now().UTC().Unix(), s.refreshTTL).Result()
	if err != nil {
		log.Printf("charts: penanda refresh gagal: %v (refresh tetap dijalankan)", err)
	}

	if err == nil && !ok {
		return
	}

	if err := s.repo.Refresh(ctx); err != nil {
		log.Printf("charts: refresh materialized view gagal: %v", err)
		s.redis.Del(ctx, cacheKeyChartsRefreshed)
	}
}

func (s *ChartService) TopSongs(ctx context.Context, params pagination.Params) (pagination.Page[ChartSongEntry], error) {
	s.ensureFresh(ctx)

	rows, total, err := s.repo.TopSongPlays(ctx, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[ChartSongEntry]{}, err
	}
	if len(rows) == 0 {
		return pagination.NewPage([]ChartSongEntry{}, total, params), nil
	}

	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.SongID)
	}

	songs, err := s.songRepo.FindByIDs(ctx, ids)
	if err != nil {
		return pagination.Page[ChartSongEntry]{}, err
	}

	songsByID := make(map[uint]model.Song, len(songs))
	for _, song := range songs {
		songsByID[song.ID] = song
	}

	entries := make([]ChartSongEntry, 0, len(rows))
	for _, row := range rows {
		song, ok := songsByID[row.SongID]
		if !ok {
			// Lagu sudah dihapus tapi view belum di-refresh; lewati saja
			// daripada menampilkan baris kosong.
			continue
		}
		entries = append(entries, ChartSongEntry{Song: song, Plays: row.Plays})
	}

	return pagination.NewPage(entries, total, params), nil
}

func (s *ChartService) TopArtists(ctx context.Context, params pagination.Params) (pagination.Page[ChartArtistEntry], error) {
	s.ensureFresh(ctx)

	rows, total, err := s.repo.TopArtists(ctx, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[ChartArtistEntry]{}, err
	}

	entries := make([]ChartArtistEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, ChartArtistEntry{
			Artist: model.Artist{
				ID:        row.ID,
				Name:      row.Name,
				Bio:       row.Bio,
				ImageURL:  row.ImageURL,
				CreatedAt: row.CreatedAt,
				UpdatedAt: row.UpdatedAt,
			},
			Plays: row.Plays,
		})
	}

	return pagination.NewPage(entries, total, params), nil
}
