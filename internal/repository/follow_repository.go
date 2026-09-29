package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"spotify-backend/internal/model"
)

// FollowRepository menangani relasi follow antar user.
type FollowRepository struct {
	db *gorm.DB
}

func NewFollowRepository(db *gorm.DB) *FollowRepository {
	return &FollowRepository{db: db}
}

// Follow menyimpan relasi follow. ON CONFLICT DO NOTHING membuat follow dua
// kali tidak error, sama seperti pola idempoten di library.
func (r *FollowRepository) Follow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	entry := model.UserFollow{
		FollowerID: followerID,
		FolloweeID: followeeID,
		CreatedAt:  time.Now().UTC(),
	}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&entry).Error
}

func (r *FollowRepository) Unfollow(ctx context.Context, followerID, followeeID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
		Delete(&model.UserFollow{}).Error
}

func (r *FollowRepository) IsFollowing(ctx context.Context, followerID, followeeID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.UserFollow{}).
		Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
		Count(&count).Error
	return count > 0, err
}

func (r *FollowRepository) CountFollowers(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.UserFollow{}).
		Where("followee_id = ?", userID).
		Count(&count).Error
	return count, err
}

func (r *FollowRepository) CountFollowing(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.UserFollow{}).
		Where("follower_id = ?", userID).
		Count(&count).Error
	return count, err
}

// FindFollowers mengembalikan user yang MENGIKUTI userID, terbaru dulu.
func (r *FollowRepository) FindFollowers(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	countQuery := r.db.WithContext(ctx).
		Model(&model.User{}).
		Joins("JOIN user_follows ON user_follows.follower_id = users.id AND user_follows.followee_id = ?", userID)

	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Model(&model.User{}).
		Joins("JOIN user_follows ON user_follows.follower_id = users.id AND user_follows.followee_id = ?", userID).
		Order("user_follows.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&users).Error
	return users, total, err
}

// FindFollowing mengembalikan user yang DIIKUTI userID, terbaru dulu.
func (r *FollowRepository) FindFollowing(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	if err := r.db.WithContext(ctx).
		Model(&model.User{}).
		Joins("JOIN user_follows ON user_follows.followee_id = users.id AND user_follows.follower_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Model(&model.User{}).
		Joins("JOIN user_follows ON user_follows.followee_id = users.id AND user_follows.follower_id = ?", userID).
		Order("user_follows.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&users).Error
	return users, total, err
}
