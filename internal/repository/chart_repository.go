package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ChartRepository membaca dan menyegarkan materialized view
// chart_song_plays_7d yang dibuat migrasi 0008. Isinya jumlah putar per lagu
// selama 7 hari terakhir.
type ChartRepository struct {
	db *gorm.DB
}

func NewChartRepository(db *gorm.DB) *ChartRepository {
	return &ChartRepository{db: db}
}

// SongPlays adalah satu baris chart lagu: id lagu dan jumlah putarnya.
type SongPlays struct {
	SongID uint  `gorm:"column:song_id"`
	Plays  int64 `gorm:"column:plays"`
}

// ArtistPlays adalah satu baris chart artist. Datanya lengkap karena query-nya
// join ke tabel artists; tidak perlu hydrate terpisah seperti lagu.
type ArtistPlays struct {
	ID        uint      `gorm:"column:id"`
	Name      string    `gorm:"column:name"`
	Bio       string    `gorm:"column:bio"`
	ImageURL  string    `gorm:"column:image_url"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
	Plays     int64     `gorm:"column:plays"`
}

// Refresh menghitung ulang view. CONCURRENTLY dipakai supaya pembaca tidak
// terkunci selama refresh; syaratnya unique index, dan itu dibuat migrasi 0008.
func (r *ChartRepository) Refresh(ctx context.Context) error {
	return r.db.WithContext(ctx).
		Exec("REFRESH MATERIALIZED VIEW CONCURRENTLY chart_song_plays_7d").Error
}

func (r *ChartRepository) TopSongPlays(ctx context.Context, limit, offset int) ([]SongPlays, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Table("chart_song_plays_7d").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []SongPlays
	err := r.db.WithContext(ctx).
		Table("chart_song_plays_7d").
		Select("song_id, plays").
		Order("plays DESC, song_id ASC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error
	return rows, total, err
}

func (r *ChartRepository) TopArtists(ctx context.Context, limit, offset int) ([]ArtistPlays, int64, error) {
	// Satu lagu bisa diputar berkali-kali, jadi join-nya digabung dulu dengan
	// SUM dan COUNT DISTINCT; tanpa itu satu artist bisa terhitung berulang.
	base := func() *gorm.DB {
		return r.db.WithContext(ctx).
			Table("artists AS a").
			Joins("JOIN songs AS s ON s.artist_id = a.id").
			Joins("JOIN chart_song_plays_7d AS c ON c.song_id = s.id")
	}

	var total int64
	if err := base().Distinct("a.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ArtistPlays
	err := base().
		Select("a.id, a.name, a.bio, a.image_url, a.created_at, a.updated_at, SUM(c.plays) AS plays").
		Group("a.id").
		Order("plays DESC, a.name ASC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error
	return rows, total, err
}
