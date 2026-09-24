package service

import (
	"context"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/pkg/pagination"
)

// SearchResults mengelompokkan hasil per tipe — bentuk yang sama dipakai
// Spotify: satu query, hasil dipisah per kategori. Grup yang tidak diminta
// klien dibiarkan nil dan hilang dari JSON (omitempty).
type SearchResults struct {
	Tracks    *pagination.Page[model.Song]     `json:"tracks,omitempty"`
	Artists   *pagination.Page[model.Artist]   `json:"artists,omitempty"`
	Albums    *pagination.Page[model.Album]    `json:"albums,omitempty"`
	Playlists *pagination.Page[model.Playlist] `json:"playlists,omitempty"`
}

type SearchService struct {
	repo *repository.SearchRepository
}

func NewSearchService(repo *repository.SearchRepository) *SearchService {
	return &SearchService{repo: repo}
}

// Search menjalankan pencarian untuk tipe yang diminta.
func (s *SearchService) Search(
	ctx context.Context,
	term string,
	types map[string]bool,
	params pagination.Params,
) (SearchResults, error) {
	var results SearchResults

	if types["track"] {
		songs, total, err := s.repo.SearchSongs(ctx, term, params.Limit, params.Offset)
		if err != nil {
			return SearchResults{}, err
		}
		page := pagination.NewPage(songs, total, params)
		results.Tracks = &page
	}

	if types["artist"] {
		artists, total, err := s.repo.SearchArtists(ctx, term, params.Limit, params.Offset)
		if err != nil {
			return SearchResults{}, err
		}
		page := pagination.NewPage(artists, total, params)
		results.Artists = &page
	}

	if types["album"] {
		albums, total, err := s.repo.SearchAlbums(ctx, term, params.Limit, params.Offset)
		if err != nil {
			return SearchResults{}, err
		}
		page := pagination.NewPage(albums, total, params)
		results.Albums = &page
	}

	if types["playlist"] {
		playlists, total, err := s.repo.SearchPlaylists(ctx, term, params.Limit, params.Offset)
		if err != nil {
			return SearchResults{}, err
		}
		page := pagination.NewPage(playlists, total, params)
		results.Playlists = &page
	}

	return results, nil
}
