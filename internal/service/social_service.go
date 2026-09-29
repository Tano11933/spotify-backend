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

// ErrCannotFollowSelf dikembalikan saat user mencoba mengikuti dirinya sendiri.
// Migrasi juga menegakkan ini lewat CHECK constraint; service memvalidasi lebih
// awal supaya pesannya ramah (422), bukan error database (500).
var ErrCannotFollowSelf = errors.New("you cannot follow yourself")

// PublicUser adalah bentuk aman user untuk dilihat orang lain. Model User
// memuat email; bentuk ini sengaja tidak, supaya data privat tidak bocor lewat
// endpoint publik.
type PublicUser struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// PublicProfile menambahkan statistik sosial dan status follow viewer.
type PublicProfile struct {
	PublicUser
	Followers       int64 `json:"followers"`
	Following       int64 `json:"following"`
	PublicPlaylists int64 `json:"public_playlists"`
	IsFollowing     bool  `json:"is_following"`
}

type SocialService struct {
	followRepo   *repository.FollowRepository
	userRepo     *repository.UserRepository
	playlistRepo *repository.PlaylistRepository
}

func NewSocialService(
	followRepo *repository.FollowRepository,
	userRepo *repository.UserRepository,
	playlistRepo *repository.PlaylistRepository,
) *SocialService {
	return &SocialService{followRepo: followRepo, userRepo: userRepo, playlistRepo: playlistRepo}
}

// GetProfile merakit profil publik beserta statistiknya. viewerID boleh nil
// (request anonim) dan sengaja tidak dihitung saat viewer melihat profilnya
// sendiri.
func (s *SocialService) GetProfile(ctx context.Context, userID uuid.UUID, viewerID *uuid.UUID) (*PublicProfile, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	followers, err := s.followRepo.CountFollowers(ctx, userID)
	if err != nil {
		return nil, err
	}

	following, err := s.followRepo.CountFollowing(ctx, userID)
	if err != nil {
		return nil, err
	}

	publicPlaylists, err := s.playlistRepo.CountPublicByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	isFollowing := false
	if viewerID != nil && *viewerID != userID {
		isFollowing, err = s.followRepo.IsFollowing(ctx, *viewerID, userID)
		if err != nil {
			return nil, err
		}
	}

	return &PublicProfile{
		PublicUser:      PublicUser{ID: user.ID, Name: user.Name, CreatedAt: user.CreatedAt},
		Followers:       followers,
		Following:       following,
		PublicPlaylists: publicPlaylists,
		IsFollowing:     isFollowing,
	}, nil
}

func (s *SocialService) Follow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	if followerID == followeeID {
		return ErrCannotFollowSelf
	}

	if _, err := s.userRepo.FindByID(ctx, followeeID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrUserNotFound
		}
		return err
	}

	return s.followRepo.Follow(ctx, followerID, followeeID)
}

func (s *SocialService) Unfollow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	return s.followRepo.Unfollow(ctx, followerID, followeeID)
}

func (s *SocialService) GetFollowers(ctx context.Context, userID uuid.UUID, params pagination.Params) (pagination.Page[PublicUser], error) {
	if _, err := s.ensureUserExists(ctx, userID); err != nil {
		return pagination.Page[PublicUser]{}, err
	}

	users, total, err := s.followRepo.FindFollowers(ctx, userID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[PublicUser]{}, err
	}
	return pagination.NewPage(toPublicUsers(users), total, params), nil
}

func (s *SocialService) GetFollowing(ctx context.Context, userID uuid.UUID, params pagination.Params) (pagination.Page[PublicUser], error) {
	if _, err := s.ensureUserExists(ctx, userID); err != nil {
		return pagination.Page[PublicUser]{}, err
	}

	users, total, err := s.followRepo.FindFollowing(ctx, userID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[PublicUser]{}, err
	}
	return pagination.NewPage(toPublicUsers(users), total, params), nil
}

func (s *SocialService) GetPublicPlaylists(ctx context.Context, userID uuid.UUID, params pagination.Params) (pagination.Page[model.Playlist], error) {
	if _, err := s.ensureUserExists(ctx, userID); err != nil {
		return pagination.Page[model.Playlist]{}, err
	}

	playlists, total, err := s.playlistRepo.FindPagePublicByUser(ctx, userID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[model.Playlist]{}, err
	}
	return pagination.NewPage(playlists, total, params), nil
}

func (s *SocialService) ensureUserExists(ctx context.Context, userID uuid.UUID) (*model.User, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

func toPublicUsers(users []model.User) []PublicUser {
	result := make([]PublicUser, 0, len(users))
	for _, user := range users {
		result = append(result, PublicUser{ID: user.ID, Name: user.Name, CreatedAt: user.CreatedAt})
	}
	return result
}
