package service

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	"spotify-backend/pkg/pagination"
)

// ActivityRecorder mencatat peristiwa publik user untuk feed. Interface ini
// sengaja sempit: PlayerService dan PlaylistService cukup "mengabari feed",
// tidak perlu tahu isi FeedService. Pencatatannya best-effort, kegagalan tidak
// boleh membatalkan pemutaran atau pembuatan playlist.
type ActivityRecorder interface {
	RecordSongPlayed(ctx context.Context, userID uuid.UUID, songID uint)
	RecordPlaylistCreated(ctx context.Context, userID uuid.UUID, playlistID uint)
}

// FeedEntry adalah satu baris feed yang sudah dirakit dengan objeknya.
type FeedEntry struct {
	Type      string          `json:"type"`
	CreatedAt time.Time       `json:"created_at"`
	User      PublicUser      `json:"user"`
	Song      *model.Song     `json:"song,omitempty"`
	Playlist  *model.Playlist `json:"playlist,omitempty"`
}

type FeedService struct {
	activityRepo *repository.ActivityRepository
	userRepo     *repository.UserRepository
	songRepo     *repository.SongRepository
	playlistRepo *repository.PlaylistRepository
}

func NewFeedService(
	activityRepo *repository.ActivityRepository,
	userRepo *repository.UserRepository,
	songRepo *repository.SongRepository,
	playlistRepo *repository.PlaylistRepository,
) *FeedService {
	return &FeedService{
		activityRepo: activityRepo,
		userRepo:     userRepo,
		songRepo:     songRepo,
		playlistRepo: playlistRepo,
	}
}

func (s *FeedService) RecordSongPlayed(ctx context.Context, userID uuid.UUID, songID uint) {
	activity := model.Activity{
		UserID:    userID,
		Type:      model.ActivitySongPlayed,
		SongID:    &songID,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.activityRepo.Create(ctx, &activity); err != nil {
		log.Printf("feed: gagal mencatat song_played user=%s: %v", userID, err)
	}
}

func (s *FeedService) RecordPlaylistCreated(ctx context.Context, userID uuid.UUID, playlistID uint) {
	activity := model.Activity{
		UserID:     userID,
		Type:       model.ActivityPlaylistCreated,
		PlaylistID: &playlistID,
		CreatedAt:  time.Now().UTC(),
	}

	if err := s.activityRepo.Create(ctx, &activity); err != nil {
		log.Printf("feed: gagal mencatat playlist_created user=%s: %v", userID, err)
	}
}

// GetFeed merakit aktivitas user yang diikuti viewer. Relasinya diambil dengan
// tiga query bulk (user, lagu, playlist), bukan query per baris feed.
func (s *FeedService) GetFeed(ctx context.Context, userID uuid.UUID, params pagination.Params) (pagination.Page[FeedEntry], error) {
	activities, total, err := s.activityRepo.FindFeed(ctx, userID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[FeedEntry]{}, err
	}
	if len(activities) == 0 {
		return pagination.NewPage([]FeedEntry{}, total, params), nil
	}

	userIDs := make([]uuid.UUID, 0, len(activities))
	songIDs := make([]uint, 0, len(activities))
	playlistIDs := make([]uint, 0, len(activities))

	seenUsers := map[uuid.UUID]bool{}
	seenSongs := map[uint]bool{}
	seenPlaylists := map[uint]bool{}

	for _, activity := range activities {
		if !seenUsers[activity.UserID] {
			seenUsers[activity.UserID] = true
			userIDs = append(userIDs, activity.UserID)
		}
		if activity.SongID != nil && !seenSongs[*activity.SongID] {
			seenSongs[*activity.SongID] = true
			songIDs = append(songIDs, *activity.SongID)
		}
		if activity.PlaylistID != nil && !seenPlaylists[*activity.PlaylistID] {
			seenPlaylists[*activity.PlaylistID] = true
			playlistIDs = append(playlistIDs, *activity.PlaylistID)
		}
	}

	users, err := s.userRepo.FindByIDs(ctx, userIDs)
	if err != nil {
		return pagination.Page[FeedEntry]{}, err
	}

	songs, err := s.songRepo.FindByIDs(ctx, songIDs)
	if err != nil {
		return pagination.Page[FeedEntry]{}, err
	}

	playlists, err := s.playlistRepo.FindByIDs(ctx, playlistIDs)
	if err != nil {
		return pagination.Page[FeedEntry]{}, err
	}

	usersByID := make(map[uuid.UUID]model.User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}

	songsByID := make(map[uint]model.Song, len(songs))
	for _, song := range songs {
		songsByID[song.ID] = song
	}

	playlistsByID := make(map[uint]model.Playlist, len(playlists))
	for _, playlist := range playlists {
		playlistsByID[playlist.ID] = playlist
	}

	entries := make([]FeedEntry, 0, len(activities))
	for _, activity := range activities {
		actor, ok := usersByID[activity.UserID]
		if !ok {
			continue
		}

		entry := FeedEntry{
			Type:      activity.Type,
			CreatedAt: activity.CreatedAt,
			User:      PublicUser{ID: actor.ID, Name: actor.Name, CreatedAt: actor.CreatedAt},
		}

		switch activity.Type {
		case model.ActivitySongPlayed:
			if activity.SongID == nil {
				continue
			}
			song, ok := songsByID[*activity.SongID]
			if !ok {
				continue
			}
			entry.Song = &song

		case model.ActivityPlaylistCreated:
			if activity.PlaylistID == nil {
				continue
			}
			playlist, ok := playlistsByID[*activity.PlaylistID]
			if !ok {
				continue
			}
			entry.Playlist = &playlist

		default:
			continue
		}

		entries = append(entries, entry)
	}

	return pagination.NewPage(entries, total, params), nil
}
