package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"spotify-backend/internal/model"
)

// SearchRepository menjalankan pencarian lintas tipe.
//
// Tiga mekanisme digabung dalam satu klausa WHERE:
//
//  1. `to_tsvector('simple', kolom) @@ plainto_tsquery('simple', ?)` —
//     pencocokan kata penuh, dipercepat index GIN tsvector.
//  2. `kolom ILIKE '%kata%'` — pencarian substring, dipercepat index trigram
//     (gin_trgm_ops).
//  3. `word_similarity(?, kolom) >= 0.3` — kemiripan per kata, menangkap typo
//     ("sinja" -> "Senja"). Cabang ini tidak memakai index; lihat catatan di
//     searchCondition untuk jalur peningkatan skalanya.
//
// Urutan hasil memakai ts_rank lalu similarity() supaya judul yang persis
// lebih dulu tampil daripada yang sekadar mirip.
type SearchRepository struct {
	db *gorm.DB
}

func NewSearchRepository(db *gorm.DB) *SearchRepository {
	return &SearchRepository{db: db}
}

func (r *SearchRepository) SearchSongs(ctx context.Context, term string, limit, offset int) ([]model.Song, int64, error) {
	var songs []model.Song
	var total int64

	where, args := searchCondition("title", term)
	order := searchOrder("title", term)

	if err := r.db.WithContext(ctx).
		Model(&model.Song{}).
		Where(where, args...).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Preload("Artist").
		Preload("Album").
		Where(where, args...).
		Clauses(order).
		Limit(limit).
		Offset(offset).
		Find(&songs).Error
	return songs, total, err
}

func (r *SearchRepository) SearchArtists(ctx context.Context, term string, limit, offset int) ([]model.Artist, int64, error) {
	var artists []model.Artist
	var total int64

	where, args := searchCondition("name", term)
	order := searchOrder("name", term)

	if err := r.db.WithContext(ctx).
		Model(&model.Artist{}).
		Where(where, args...).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Where(where, args...).
		Clauses(order).
		Limit(limit).
		Offset(offset).
		Find(&artists).Error
	return artists, total, err
}

func (r *SearchRepository) SearchAlbums(ctx context.Context, term string, limit, offset int) ([]model.Album, int64, error) {
	var albums []model.Album
	var total int64

	where, args := searchCondition("title", term)
	order := searchOrder("title", term)

	if err := r.db.WithContext(ctx).
		Model(&model.Album{}).
		Where(where, args...).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Preload("Artist").
		Where(where, args...).
		Clauses(order).
		Limit(limit).
		Offset(offset).
		Find(&albums).Error
	return albums, total, err
}

func (r *SearchRepository) SearchPlaylists(ctx context.Context, term string, limit, offset int) ([]model.Playlist, int64, error) {
	var playlists []model.Playlist
	var total int64

	where, args := searchCondition("name", term)
	order := searchOrder("name", term)

	if err := r.db.WithContext(ctx).
		Model(&model.Playlist{}).
		Where("is_public = ?", true).
		Where(where, args...).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Preload("User").
		Where("is_public = ?", true).
		Where(where, args...).
		Clauses(order).
		Limit(limit).
		Offset(offset).
		Find(&playlists).Error
	return playlists, total, err
}

// searchCondition membangun klausa WHERE + argumennya.
//
// Tiga jalur digabung dengan OR:
//   - tsquery : pencocokan kata penuh (index GIN tsvector)
//   - ILIKE   : substring, mis. "senja" cocok dengan "Zona Senja"
//   - word_similarity: kemiripan per kata — inilah yang menangkap typo
//     seperti "sinja" -> "Senja" (whole-string similarity hanya 0.21,
//     word_similarity 0.33).
//
// Catatan skala: cabang word_similarity dihitung sebagai fungsi sehingga tidak
// memakai index. Untuk katalog sebesar ini tidak masalah; kalau nanti besar,
// ganti ke operator `%>` (index GIN) sambil menurunkan
// pg_trgm.word_similarity_threshold — threshold default 0.6 terlalu ketat
// untuk kata pendek.
func searchCondition(column, term string) (string, []any) {
	where := fmt.Sprintf(
		"to_tsvector('simple', %[1]s) @@ plainto_tsquery('simple', ?) OR %[1]s ILIKE ? OR word_similarity(?, %[1]s) >= 0.3",
		column,
	)
	return where, []any{term, "%" + term + "%", term}
}

// searchOrder membangun ORDER BY: skor kata penuh dulu, lalu kemiripan
// trigram. Dibungkus clause.OrderBy karena GORM tidak menerima clause.Expr
// mentah di method Order().
func searchOrder(column, term string) clause.OrderBy {
	return clause.OrderBy{
		Expression: clause.Expr{
			SQL: fmt.Sprintf(
				"ts_rank(to_tsvector('simple', %[1]s), plainto_tsquery('simple', ?)) DESC, similarity(%[1]s, ?) DESC",
				column,
			),
			Vars: []any{term, term},
		},
	}
}
