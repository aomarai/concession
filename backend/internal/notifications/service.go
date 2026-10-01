// Package notifications stores and serves per-user notifications (invitations,
// items added to shared lists, friend requests). Other services create them
// through Notify; recipients list them and mark them read.
package notifications

import (
	"context"
	"fmt"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/paging"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	DB *gorm.DB
}

func NewService(db *gorm.DB) *Service { return &Service{DB: db} }

// View is a notification as returned by the API.
type View struct {
	ID        uuid.UUID               `json:"id"`
	Type      domain.NotificationType `json:"type"`
	Message   string                  `json:"message"`
	IsRead    bool                    `json:"is_read"`
	LinkURL   string                  `json:"link_url,omitempty"`
	Actor     domain.PublicUser       `json:"actor"`
	CreatedAt time.Time               `json:"created_at"`
}

// Page is one page of notifications.
type Page struct {
	Notifications []View `json:"notifications"`
	UnreadCount   int64  `json:"unread_count"`
	Page          int    `json:"page"`
	PerPage       int    `json:"per_page"`
	Total         int64  `json:"total"`
}

// templates build the message; %[1]s is the actor's name, %[2]s the subject
// (usually a watchlist title).
var templates = map[domain.NotificationType]string{
	domain.NotificationWatchlistInvite: `%[1]s invited you to collaborate on "%[2]s"`,
	domain.NotificationInviteAccepted:  `%[1]s accepted your invitation to "%[2]s"`,
	domain.NotificationItemAdded:       `%[1]s added a title to "%[2]s"`,
	domain.NotificationFriendRequest:   `%[1]s sent you a friend request`,
	domain.NotificationFriendAccepted:  `%[1]s accepted your friend request`,
}

// Notify tells userID that actorID did something. subject is what it was done
// to (e.g. a list title) and link is where the UI should send the reader.
// Notifying yourself is a no-op.
func (s *Service) Notify(ctx context.Context, userID, actorID uuid.UUID, typ domain.NotificationType, subject, link string) error {
	if userID == actorID {
		return nil
	}
	tmpl, ok := templates[typ]
	if !ok {
		return fmt.Errorf("notifications: unknown type %q", typ)
	}
	profiles, err := domain.LoadPublicUsers(ctx, s.DB, []uuid.UUID{actorID})
	if err != nil {
		return err
	}
	name := domain.PublicUserOrID(profiles, actorID).DisplayName
	if name == "" {
		name = "Someone"
	}
	n := domain.Notification{
		UserID: userID, ActorID: actorID, Type: typ, LinkURL: link,
		Message: fmt.Sprintf(tmpl, name, subject),
	}
	return s.DB.WithContext(ctx).Omit("Actor").Create(&n).Error
}

// List returns a page of the user's notifications, newest first, with the
// total number of unread ones.
func (s *Service) List(ctx context.Context, userID uuid.UUID, unreadOnly bool, page, perPage int) (*Page, error) {
	page, perPage, err := paging.Normalize(page, perPage)
	if err != nil {
		return nil, err
	}
	db := s.DB.WithContext(ctx)
	out := &Page{Page: page, PerPage: perPage}

	q := db.Model(&domain.Notification{}).Where("user_id = ?", userID)
	if unreadOnly {
		q = q.Where("is_read = ?", false)
	}
	q = q.Session(&gorm.Session{}) // safe to reuse for both the count and the page
	if err := q.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	if out.UnreadCount, err = s.UnreadCount(ctx, userID); err != nil {
		return nil, err
	}

	var rows []domain.Notification
	if err := q.Order("created_at DESC, id").Limit(perPage).Offset(paging.Offset(page, perPage)).Find(&rows).Error; err != nil {
		return nil, err
	}
	actorIDs := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		actorIDs[i] = r.ActorID
	}
	profiles, err := domain.LoadPublicUsers(ctx, s.DB, actorIDs)
	if err != nil {
		return nil, err
	}
	out.Notifications = make([]View, len(rows))
	for i, r := range rows {
		out.Notifications[i] = View{
			ID: r.ID, Type: r.Type, Message: r.Message, IsRead: r.IsRead, LinkURL: r.LinkURL,
			Actor: domain.PublicUserOrID(profiles, r.ActorID), CreatedAt: r.CreatedAt,
		}
	}
	return out, nil
}

// UnreadCount returns how many notifications the user has not read.
func (s *Service) UnreadCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&domain.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).Count(&n).Error
	return n, err
}

// MarkRead marks one of the user's notifications as read. Someone else's
// notification, or an unknown ID, is ErrNotFound.
func (s *Service) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	res := s.DB.WithContext(ctx).Model(&domain.Notification{}).
		Where("id = ? AND user_id = ?", id, userID).Update("is_read", true)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return svcerr.ErrNotFound
	}
	return nil
}

// MarkAllRead marks every unread notification of the user as read and returns
// how many changed.
func (s *Service) MarkAllRead(ctx context.Context, userID uuid.UUID) (int64, error) {
	res := s.DB.WithContext(ctx).Model(&domain.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).Update("is_read", true)
	return res.RowsAffected, res.Error
}
