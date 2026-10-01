// Package friends implements friend requests and the friends list. A friend
// request is a pending domain.Friendship; accepting it makes the two users
// friends. Friends can be invited to watchlists by ID.
package friends

import (
	"context"
	"errors"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/keyedlock"
	"github.com/aomarai/concession/internal/logging"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/userref"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Notifier delivers notifications; *notifications.Service implements it. It
// may be nil, and failures never fail the action that triggered them.
type Notifier interface {
	Notify(ctx context.Context, userID, actorID uuid.UUID, typ domain.NotificationType, subject, link string) error
}

type Service struct {
	DB       *gorm.DB
	Notifier Notifier

	locks keyedlock.Locks
}

func NewService(db *gorm.DB) *Service { return &Service{DB: db} }

// Entry is a friend or a friend request, from the point of view of one user.
// User is the other person.
type Entry struct {
	ID         uuid.UUID               `json:"id"`
	User       domain.PublicUser       `json:"user"`
	Status     domain.FriendshipStatus `json:"status"`
	CreatedAt  time.Time               `json:"created_at"`
	AcceptedAt *time.Time              `json:"accepted_at,omitempty"`
}

// Requests are the user's pending friend requests.
type Requests struct {
	Incoming []Entry `json:"incoming"`
	Outgoing []Entry `json:"outgoing"`
}

func (s *Service) notify(ctx context.Context, to, actor uuid.UUID, typ domain.NotificationType) {
	if s.Notifier == nil {
		return
	}
	if err := s.Notifier.Notify(ctx, to, actor, typ, "", "/friends"); err != nil {
		logging.FromContext(ctx).Warn("could not send notification", "type", typ, "error", err)
	}
}

func (s *Service) entries(ctx context.Context, me uuid.UUID, rows []domain.Friendship) ([]Entry, error) {
	ids := make([]uuid.UUID, len(rows))
	for i, f := range rows {
		ids[i] = f.Other(me)
	}
	profiles, err := domain.LoadPublicUsers(ctx, s.DB, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, len(rows))
	for i, f := range rows {
		out[i] = Entry{
			ID: f.ID, User: domain.PublicUserOrID(profiles, ids[i]), Status: f.Status,
			CreatedAt: f.CreatedAt, AcceptedAt: f.AcceptedAt,
		}
	}
	return out, nil
}

// Send sends a friend request to the referenced user. If that user has already
// sent you a request, it is accepted instead, and the returned entry is
// accepted.
func (s *Service) Send(ctx context.Context, me uuid.UUID, ref userref.Ref) (*Entry, error) {
	target, err := userref.Resolve(ctx, s.DB, ref)
	if err != nil {
		return nil, err
	}
	if target.ID == me {
		return nil, svcerr.Invalid("you cannot send a friend request to yourself")
	}
	lo, hi := domain.OrderedPair(me, target.ID)

	defer s.locks.Lock("friends:" + lo.String() + hi.String())()
	db := s.DB.WithContext(ctx)
	var f domain.Friendship
	err = db.Where("user_a_id = ? AND user_b_id = ?", lo, hi).First(&f).Error
	switch {
	case err == nil && f.Status == domain.FriendshipAccepted:
		return nil, svcerr.Duplicate("You are already friends")
	case err == nil && f.RequestedBy == me:
		return nil, svcerr.Duplicate("You have already sent this person a friend request")
	case err == nil:
		// They asked first: saying yes is the only sensible reading.
		if err := s.accept(ctx, &f); err != nil {
			return nil, err
		}
		s.notify(ctx, target.ID, me, domain.NotificationFriendAccepted)
	case errors.Is(err, gorm.ErrRecordNotFound):
		f = domain.Friendship{UserAID: lo, UserBID: hi, RequestedBy: me, Status: domain.FriendshipPending}
		if err := db.Create(&f).Error; err != nil {
			return nil, err
		}
		s.notify(ctx, target.ID, me, domain.NotificationFriendRequest)
	default:
		return nil, err
	}
	entries, err := s.entries(ctx, me, []domain.Friendship{f})
	if err != nil {
		return nil, err
	}
	return &entries[0], nil
}

func (s *Service) accept(ctx context.Context, f *domain.Friendship) error {
	now := time.Now()
	if err := s.DB.WithContext(ctx).Model(f).Updates(map[string]any{"status": domain.FriendshipAccepted, "accepted_at": now}).Error; err != nil {
		return err
	}
	f.Status, f.AcceptedAt = domain.FriendshipAccepted, &now
	return nil
}

// pendingFor finds a pending request addressed to me.
func (s *Service) pendingFor(ctx context.Context, me, requestID uuid.UUID) (domain.Friendship, error) {
	var f domain.Friendship
	err := s.DB.WithContext(ctx).Where("id = ? AND status = ? AND (user_a_id = ? OR user_b_id = ?) AND requested_by <> ?",
		requestID, domain.FriendshipPending, me, me, me).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return f, svcerr.ErrNotFound
	}
	return f, err
}

// Accept accepts a friend request addressed to me.
func (s *Service) Accept(ctx context.Context, me, requestID uuid.UUID) error {
	f, err := s.pendingFor(ctx, me, requestID)
	if err != nil {
		return err
	}
	if err := s.accept(ctx, &f); err != nil {
		return err
	}
	s.notify(ctx, f.RequestedBy, me, domain.NotificationFriendAccepted)
	return nil
}

// Decline deletes a friend request addressed to me.
func (s *Service) Decline(ctx context.Context, me, requestID uuid.UUID) error {
	f, err := s.pendingFor(ctx, me, requestID)
	if err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Unscoped().Delete(&domain.Friendship{}, "id = ?", f.ID).Error
}

// Remove ends the relationship with another user: it unfriends them, or
// cancels a request I sent them, or declines one they sent me. There is
// nothing to remove if the two have no friendship row (ErrNotFound).
func (s *Service) Remove(ctx context.Context, me, other uuid.UUID) error {
	lo, hi := domain.OrderedPair(me, other)
	res := s.DB.WithContext(ctx).Unscoped().Where("user_a_id = ? AND user_b_id = ?", lo, hi).Delete(&domain.Friendship{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return svcerr.ErrNotFound
	}
	return nil
}

// List returns my friends, most recently befriended first.
func (s *Service) List(ctx context.Context, me uuid.UUID) ([]Entry, error) {
	var rows []domain.Friendship
	err := s.DB.WithContext(ctx).Where("(user_a_id = ? OR user_b_id = ?) AND status = ?", me, me, domain.FriendshipAccepted).
		Order("accepted_at DESC, id").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return s.entries(ctx, me, rows)
}

// ListRequests returns the pending requests sent to me and sent by me.
func (s *Service) ListRequests(ctx context.Context, me uuid.UUID) (*Requests, error) {
	var rows []domain.Friendship
	err := s.DB.WithContext(ctx).Where("(user_a_id = ? OR user_b_id = ?) AND status = ?", me, me, domain.FriendshipPending).
		Order("created_at DESC, id").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	entries, err := s.entries(ctx, me, rows)
	if err != nil {
		return nil, err
	}
	out := &Requests{Incoming: []Entry{}, Outgoing: []Entry{}}
	for i, f := range rows {
		if f.RequestedBy == me {
			out.Outgoing = append(out.Outgoing, entries[i])
		} else {
			out.Incoming = append(out.Incoming, entries[i])
		}
	}
	return out, nil
}
