package service

import (
	"context"

	"github.com/google/uuid"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/pkg/pagination"
)

// recommendationGenreLimit membatasi genre yang dianggap "favorit" supaya
// rekomendasi tetap fokus dan query-nya murah.
const recommendationGenreLimit = 3

type RecommendationService struct {
	repo       *repository.RecommendationRepository
	artistRepo *repository.ArtistRepository
}

func NewRecommendationService(
	repo *repository.RecommendationRepository,
	artistRepo *repository.ArtistRepository,
) *RecommendationService {
	return &RecommendationService{repo: repo, artistRepo: artistRepo}
}

// RelatedArtists adalah "Fans also like": artist yang pengikutnya beririsan.
//
// Kalau belum ada irisan sama sekali (database baru, belum ada yang mengikuti
// artist), hasilnya jatuh ke artist lain yang berbagi genre supaya halaman
// ini tidak kosong di demo.
func (s *RecommendationService) RelatedArtists(ctx context.Context, artistID uint, params pagination.Params) (pagination.Page[model.Artist], error) {
	exists, err := s.artistRepo.Exists(ctx, artistID)
	if err != nil {
		return pagination.Page[model.Artist]{}, err
	}
	if !exists {
		return pagination.Page[model.Artist]{}, ErrArtistNotFound
	}

	artists, total, err := s.repo.RelatedArtists(ctx, artistID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[model.Artist]{}, err
	}

	if total == 0 {
		genreIDs, err := s.repo.ArtistGenreIDs(ctx, artistID)
		if err != nil {
			return pagination.Page[model.Artist]{}, err
		}
		if len(genreIDs) > 0 {
			artists, total, err = s.repo.ArtistsByGenreIDs(ctx, genreIDs, artistID, params.Limit, params.Offset)
			if err != nil {
				return pagination.Page[model.Artist]{}, err
			}
		}
	}

	return pagination.NewPage(artists, total, params), nil
}

// MadeForYou menyusun rekomendasi dari genre favorit user, hasil irisan
// riwayat putar dengan genre artist. User yang belum punya riwayat putar
// mendapat lagu terpopuler global sebagai gantinya.
func (s *RecommendationService) MadeForYou(ctx context.Context, userID uuid.UUID, params pagination.Params) (pagination.Page[model.Song], error) {
	genreIDs, err := s.repo.TopGenreIDs(ctx, userID, recommendationGenreLimit)
	if err != nil {
		return pagination.Page[model.Song]{}, err
	}

	if len(genreIDs) > 0 {
		songs, total, err := s.repo.SongsByGenres(ctx, userID, genreIDs, params.Limit, params.Offset)
		if err != nil {
			return pagination.Page[model.Song]{}, err
		}
		if total > 0 {
			return pagination.NewPage(songs, total, params), nil
		}
	}

	songs, total, err := s.repo.SongsByPlayCount(ctx, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[model.Song]{}, err
	}
	return pagination.NewPage(songs, total, params), nil
}
