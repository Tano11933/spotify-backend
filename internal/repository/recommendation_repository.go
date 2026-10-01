package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"spotify-backend/internal/model"
)

// RecommendationRepository menyediakan query heuristik untuk halaman
// discovery: "Fans also like" dan "Made for you". Semuanya SQL agregat, tanpa
// model machine learning, jadi bisa dipakai di skala demo tanpa worker.
type RecommendationRepository struct {
	db *gorm.DB
}

func NewRecommendationRepository(db *gorm.DB) *RecommendationRepository {
	return &RecommendationRepository{db: db}
}

// RelatedArtists mencari artist yang pengikutnya beririsan: user yang
// mengikuti artistID juga mengikuti artist lain. Diurutkan dari irisan
// terbanyak, lalu nama.
func (r *RecommendationRepository) RelatedArtists(ctx context.Context, artistID uint, limit, offset int) ([]model.Artist, int64, error) {
	var artists []model.Artist
	var total int64

	base := func() *gorm.DB {
		return r.db.WithContext(ctx).
			Model(&model.Artist{}).
			Joins("JOIN followed_artists AS other ON other.artist_id = artists.id").
			Joins("JOIN followed_artists AS origin ON origin.user_id = other.user_id AND origin.artist_id = ?", artistID).
			Where("artists.id <> ?", artistID)
	}

	if err := base().Distinct("artists.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base().
		Select("artists.*, COUNT(*) AS shared_followers").
		Group("artists.id").
		Order("shared_followers DESC, artists.name ASC").
		Limit(limit).
		Offset(offset).
		Find(&artists).Error
	return artists, total, err
}

// ArtistsByGenreIDs adalah fallback "Fans also like" saat belum ada irisan
// pengikut: artist lain yang berbagi genre dengan artist asal.
func (r *RecommendationRepository) ArtistsByGenreIDs(ctx context.Context, genreIDs []uint, excludeArtistID uint, limit, offset int) ([]model.Artist, int64, error) {
	var artists []model.Artist
	var total int64

	base := func() *gorm.DB {
		return r.db.WithContext(ctx).
			Model(&model.Artist{}).
			Joins("JOIN artist_genres ON artist_genres.artist_id = artists.id").
			Where("artist_genres.genre_id IN ?", genreIDs).
			Where("artists.id <> ?", excludeArtistID)
	}

	if err := base().Distinct("artists.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base().
		Select("artists.*").
		Group("artists.id").
		Order("artists.name ASC").
		Limit(limit).
		Offset(offset).
		Find(&artists).Error
	return artists, total, err
}

// ArtistGenreIDs mengembalikan id genre yang dimiliki satu artist. Dipakai
// service untuk memilih fallback dan untuk menebak selera user.
func (r *RecommendationRepository) ArtistGenreIDs(ctx context.Context, artistID uint) ([]uint, error) {
	var ids []uint
	err := r.db.WithContext(ctx).
		Table("artist_genres").
		Where("artist_id = ?", artistID).
		Pluck("genre_id", &ids).Error
	return ids, err
}

// TopGenreIDs menebak genre favorit user dari lagu yang paling sering
// diputarnya: riwayat putar di-join ke artist dan genre artist-nya, lalu
// dihitung per genre.
func (r *RecommendationRepository) TopGenreIDs(ctx context.Context, userID uuid.UUID, limit int) ([]uint, error) {
	var ids []uint
	err := r.db.WithContext(ctx).
		Table("play_histories AS ph").
		Select("ag.genre_id").
		Joins("JOIN songs AS s ON s.id = ph.song_id").
		Joins("JOIN artist_genres AS ag ON ag.artist_id = s.artist_id").
		Where("ph.user_id = ?", userID).
		Group("ag.genre_id").
		Order("COUNT(*) DESC").
		Limit(limit).
		Pluck("ag.genre_id", &ids).Error
	return ids, err
}

// SongsByGenres mengembalikan lagu dari genre pilihan user. Lagu yang BELUM
// diputar dalam 7 hari terakhir didahulukan, baru kemudian yang sudah, supaya
// rekomendasi terasa segar alih-alih mengulang putaran terakhir.
func (r *RecommendationRepository) SongsByGenres(ctx context.Context, userID uuid.UUID, genreIDs []uint, limit, offset int) ([]model.Song, int64, error) {
	var songs []model.Song
	var total int64

	const genreJoin = "JOIN artist_genres AS ag ON ag.artist_id = songs.artist_id"
	const recentJoin = "LEFT JOIN play_histories AS recent ON recent.song_id = songs.id AND recent.user_id = ? AND recent.played_at >= now() - interval '7 days'"

	if err := r.db.WithContext(ctx).
		Model(&model.Song{}).
		Joins(genreJoin).
		Where("ag.genre_id IN ?", genreIDs).
		Distinct("songs.id").
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Model(&model.Song{}).
		Joins(genreJoin).
		Joins(recentJoin, userID).
		Where("ag.genre_id IN ?", genreIDs).
		Select("songs.*").
		Group("songs.id").
		Order("(COUNT(recent.id) = 0) DESC, songs.play_count DESC, songs.id ASC").
		Preload("Artist").
		Preload("Album").
		Limit(limit).
		Offset(offset).
		Find(&songs).Error
	return songs, total, err
}

// SongsByPlayCount adalah fallback "Made for you" untuk user yang belum punya
// riwayat putar, atau saat genre favoritnya tidak menyisakan lagu sama sekali:
// lagu terpopuler secara global.
func (r *RecommendationRepository) SongsByPlayCount(ctx context.Context, limit, offset int) ([]model.Song, int64, error) {
	var songs []model.Song
	var total int64

	if err := r.db.WithContext(ctx).Model(&model.Song{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Order("play_count DESC, id ASC").
		Preload("Artist").
		Preload("Album").
		Limit(limit).
		Offset(offset).
		Find(&songs).Error
	return songs, total, err
}
