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

// EventNotificationNew adalah event WebSocket untuk notifikasi baru. Dikirim
// TERTARGET ke user penerima, bukan broadcast.
const EventNotificationNew = "notification:new"

// NotificationPublisher mengirim event real-time ke satu user. Dipenuhi hub
// WebSocket lewat PublishToUser.
type NotificationPublisher interface {
	PublishToUser(userID uuid.UUID, eventType string, payload any)
}

// FollowNotifier dipakai SocialService untuk mengabari user saat ada pengikut
// baru tanpa bergantung pada seluruh isi NotificationService.
type FollowNotifier interface {
	NotifyFollow(ctx context.Context, followeeID, followerID uuid.UUID)
}

// NotificationEntry adalah notifikasi yang sudah dirakit dengan actor-nya.
type NotificationEntry struct {
	ID        uint64      `json:"id"`
	Type      string      `json:"type"`
	CreatedAt time.Time   `json:"created_at"`
	Actor     *PublicUser `json:"actor,omitempty"`
	ReadAt    *time.Time  `json:"read_at,omitempty"`
}

// NotificationPage menambahkan jumlah belum dibaca ke envelope standar, supaya
// frontend bisa menampilkan badge tanpa request terpisah.
type NotificationPage struct {
	pagination.Page[NotificationEntry]
	Unread int64 `json:"unread"`
}

type NotificationService struct {
	repo      *repository.NotificationRepository
	userRepo  *repository.UserRepository
	publisher NotificationPublisher
}

func NewNotificationService(
	repo *repository.NotificationRepository,
	userRepo *repository.UserRepository,
	publisher NotificationPublisher,
) *NotificationService {
	return &NotificationService{repo: repo, userRepo: userRepo, publisher: publisher}
}

// NotifyFollow membuat notifikasi "pengikut baru" dan mengirimnya real-time.
//
// Kegagalan apa pun hanya di-log: follow-nya sendiri sudah tersimpan, dan
// notifikasi yang tidak terkirim bukan alasan membatalkan aksi user.
func (s *NotificationService) NotifyFollow(ctx context.Context, followeeID, followerID uuid.UUID) {
	notification := model.Notification{
		UserID:    followeeID,
		Type:      model.NotificationUserFollowed,
		ActorID:   &followerID,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, &notification); err != nil {
		log.Printf("notifications: gagal menyimpan notifikasi follow untuk user=%s: %v", followeeID, err)
		return
	}

	if s.publisher == nil {
		return
	}

	actor := s.publicUser(ctx, followerID)
	s.publisher.PublishToUser(followeeID, EventNotificationNew, map[string]any{
		"notification": NotificationEntry{
			ID:        notification.ID,
			Type:      notification.Type,
			CreatedAt: notification.CreatedAt,
			Actor:     actor,
		},
	})
}

func (s *NotificationService) GetMine(ctx context.Context, userID uuid.UUID, params pagination.Params) (NotificationPage, error) {
	notifications, total, err := s.repo.FindByUser(ctx, userID, params.Limit, params.Offset)
	if err != nil {
		return NotificationPage{}, err
	}

	unread, err := s.repo.CountUnread(ctx, userID)
	if err != nil {
		return NotificationPage{}, err
	}

	// Kumpulkan actor sekali (query bulk), bukan per notifikasi.
	actorIDs := make([]uuid.UUID, 0, len(notifications))
	seen := map[uuid.UUID]bool{}
	for _, notification := range notifications {
		if notification.ActorID != nil && !seen[*notification.ActorID] {
			seen[*notification.ActorID] = true
			actorIDs = append(actorIDs, *notification.ActorID)
		}
	}

	actors, err := s.userRepo.FindByIDs(ctx, actorIDs)
	if err != nil {
		return NotificationPage{}, err
	}

	actorsByID := make(map[uuid.UUID]model.User, len(actors))
	for _, actor := range actors {
		actorsByID[actor.ID] = actor
	}

	entries := make([]NotificationEntry, 0, len(notifications))
	for _, notification := range notifications {
		entry := NotificationEntry{
			ID:        notification.ID,
			Type:      notification.Type,
			CreatedAt: notification.CreatedAt,
			ReadAt:    notification.ReadAt,
		}

		if notification.ActorID != nil {
			if actor, ok := actorsByID[*notification.ActorID]; ok {
				entry.Actor = &PublicUser{ID: actor.ID, Name: actor.Name, CreatedAt: actor.CreatedAt}
			}
		}

		entries = append(entries, entry)
	}

	return NotificationPage{
		Page:   pagination.NewPage(entries, total, params),
		Unread: unread,
	}, nil
}

func (s *NotificationService) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return s.repo.MarkAllRead(ctx, userID)
}

func (s *NotificationService) publicUser(ctx context.Context, userID uuid.UUID) *PublicUser {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil
	}
	return &PublicUser{ID: user.ID, Name: user.Name, CreatedAt: user.CreatedAt}
}
