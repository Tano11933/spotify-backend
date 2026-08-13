package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
)

var (
	ErrPlaylistNotFound      = errors.New("playlist not found")
	ErrPlaylistForbidden     = errors.New("you do not have access to this playlist")
	ErrSongAlreadyInPlaylist = errors.New("song is already in this playlist")
	ErrSongNotInPlaylist     = errors.New("song is not in this playlist")
)

type PlaylistService struct {
	repo     *repository.PlaylistRepository
	songRepo *repository.SongRepository
}

func NewPlaylistService(
	repo *repository.PlaylistRepository,
	songRepo *repository.SongRepository,
) *PlaylistService {
	return &PlaylistService{repo: repo, songRepo: songRepo}
}

func (s *PlaylistService) Create(
	ctx context.Context,
	userID uuid.UUID,
	req model.CreatePlaylistRequest,
) (*model.Playlist, error) {
	playlist := &model.Playlist{
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		IsPublic:    req.IsPublic,

		UserID: userID,
	}

	if err := s.repo.Create(ctx, playlist); err != nil {
		return nil, fmt.Errorf("create playlist: %w", err)
	}
	return playlist, nil
}

func (s *PlaylistService) GetOwned(ctx context.Context, userID uuid.UUID) ([]model.Playlist, error) {
	return s.repo.FindByUserID(ctx, userID)
}

func (s *PlaylistService) GetPublic(ctx context.Context) ([]model.Playlist, error) {
	return s.repo.FindPublic(ctx)
}

func (s *PlaylistService) GetByID(
	ctx context.Context,
	playlistID uint,
	viewerID uuid.UUID,
) (*model.Playlist, error) {
	playlist, err := s.repo.FindByID(ctx, playlistID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPlaylistNotFound
		}
		return nil, fmt.Errorf("find playlist: %w", err)
	}

	if playlist.UserID != viewerID && !playlist.IsPublic {
		return nil, ErrPlaylistNotFound
	}
	return playlist, nil
}

func (s *PlaylistService) Update(
	ctx context.Context,
	playlistID uint,
	userID uuid.UUID,
	req model.UpdatePlaylistRequest,
) (*model.Playlist, error) {
	playlist, err := s.mustOwn(ctx, playlistID, userID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		playlist.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		playlist.Description = strings.TrimSpace(*req.Description)
	}
	if req.IsPublic != nil {
		playlist.IsPublic = *req.IsPublic
	}

	if err := s.repo.Update(ctx, playlist); err != nil {
		return nil, fmt.Errorf("update playlist: %w", err)
	}
	return playlist, nil
}

func (s *PlaylistService) Delete(ctx context.Context, playlistID uint, userID uuid.UUID) error {
	if _, err := s.mustOwn(ctx, playlistID, userID); err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, playlistID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrPlaylistNotFound
		}
		return fmt.Errorf("delete playlist: %w", err)
	}
	return nil
}

func (s *PlaylistService) AddSong(
	ctx context.Context,
	playlistID uint,
	userID uuid.UUID,
	songID uint,
) error {
	if _, err := s.mustOwn(ctx, playlistID, userID); err != nil {
		return err
	}

	if _, err := s.songRepo.FindByID(ctx, songID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSongNotFound
		}
		return fmt.Errorf("find song: %w", err)
	}

	exists, err := s.repo.HasSong(ctx, playlistID, songID)
	if err != nil {
		return fmt.Errorf("check song in playlist: %w", err)
	}
	if exists {
		return ErrSongAlreadyInPlaylist
	}

	if err := s.repo.AddSong(ctx, playlistID, songID); err != nil {
		return fmt.Errorf("add song to playlist: %w", err)
	}
	return nil
}

func (s *PlaylistService) RemoveSong(
	ctx context.Context,
	playlistID uint,
	userID uuid.UUID,
	songID uint,
) error {
	if _, err := s.mustOwn(ctx, playlistID, userID); err != nil {
		return err
	}

	if err := s.repo.RemoveSong(ctx, playlistID, songID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSongNotInPlaylist
		}
		return fmt.Errorf("remove song from playlist: %w", err)
	}
	return nil
}

func (s *PlaylistService) mustOwn(
	ctx context.Context,
	playlistID uint,
	userID uuid.UUID,
) (*model.Playlist, error) {
	playlist, err := s.repo.FindByID(ctx, playlistID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPlaylistNotFound
		}
		return nil, fmt.Errorf("find playlist: %w", err)
	}

	if playlist.UserID != userID {
		return nil, ErrPlaylistForbidden
	}
	return playlist, nil
}
