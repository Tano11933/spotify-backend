package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"spotify-backend/internal/model"
)

// ActivityRepository menyimpan dan membaca aktivitas publik user untuk feed.
type ActivityRepository struct {
	db *gorm.DB
}

func NewActivityRepository(db *gorm.DB) *ActivityRepository {
	return &ActivityRepository{db: db}
}

func (r *ActivityRepository) Create(ctx context.Context, activity *model.Activity) error {
	return r.db.WithContext(ctx).Create(activity).Error
}

// FindFeed mengembalikan aktivitas user-user yang DIIKUTI userID, terbaru
// dulu. JOIN ke user_follows dilakukan saat baca (read-time join): tidak ada
// salinan per pengikut yang harus dijaga tetap sinkron.
func (r *ActivityRepository) FindFeed(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Activity, int64, error) {
	var activities []model.Activity
	var total int64

	if err := r.feedQuery(ctx, userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.feedQuery(ctx, userID).
		Order("activities.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&activities).Error
	return activities, total, err
}

func (r *ActivityRepository) feedQuery(ctx context.Context, userID uuid.UUID) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.Activity{}).
		Joins("JOIN user_follows ON user_follows.followee_id = activities.user_id AND user_follows.follower_id = ?", userID)
}
