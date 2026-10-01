package watchlist

import (
	"context"
	"errors"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/userref"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Person is the public part of a user's profile (never their email).
type Person = domain.PublicUser

// Member is a user's membership of a watchlist.
type Member struct {
	Person
	Role     domain.CollaboratorRole   `json:"role"`
	Status   domain.CollaboratorStatus `json:"status"`
	JoinedAt *time.Time                `json:"joined_at,omitempty"`
}

// Members lists who has access to a watchlist. Pending invitations are only
// shown to the owner.
type Members struct {
	Owner   Member   `json:"owner"`
	Members []Member `json:"members"`
	Pending []Member `json:"pending,omitempty"`
}

// Invite is a pending invitation as seen by the invited user.
type Invite struct {
	ID            uuid.UUID               `json:"id"`
	WatchlistID   uuid.UUID               `json:"watchlist_id"`
	WatchlistName string                  `json:"watchlist_title"`
	Role          domain.CollaboratorRole `json:"role"`
	InvitedBy     *Person                 `json:"invited_by,omitempty"`
	CreatedAt     time.Time               `json:"created_at"`
}

func validInviteRole(r domain.CollaboratorRole) bool {
	return r == domain.RoleEditor || r == domain.RoleViewer
}

// ListMembers returns the list's owner and collaborators. Any role may view
// it, except that only the owner sees pending invitations.
func (s *Service) ListMembers(ctx context.Context, userID, id uuid.UUID) (*Members, error) {
	w, role, err := s.access(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	var rows []domain.Collaborator
	if err := s.DB.WithContext(ctx).Where("watchlist_id = ?", id).Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := []uuid.UUID{w.OwnerID}
	for _, c := range rows {
		ids = append(ids, c.UserID)
	}
	people, err := domain.LoadPublicUsers(ctx, s.DB, ids)
	if err != nil {
		return nil, err
	}

	out := &Members{
		Owner:   Member{Person: domain.PublicUserOrID(people, w.OwnerID), Role: domain.RoleOwner, Status: domain.CollaboratorAccepted},
		Members: []Member{},
	}
	for _, c := range rows {
		m := Member{Person: domain.PublicUserOrID(people, c.UserID), Role: c.Role, Status: c.Status}
		if !c.JoinedAt.IsZero() {
			joined := c.JoinedAt
			m.JoinedAt = &joined
		}
		switch {
		case c.Status == domain.CollaboratorAccepted:
			out.Members = append(out.Members, m)
		case role == domain.RoleOwner:
			out.Pending = append(out.Pending, m)
		}
	}
	return out, nil
}

// InviteUser invites a user (by username, e-mail or ID) to collaborate as an
// editor or viewer. Owner only. The invitation must be accepted before it
// grants access.
func (s *Service) InviteUser(ctx context.Context, ownerID, id uuid.UUID, ref userref.Ref, role domain.CollaboratorRole) (*Member, error) {
	w, myRole, err := s.access(ctx, ownerID, id)
	if err != nil {
		return nil, err
	}
	if myRole != domain.RoleOwner {
		return nil, svcerr.ErrForbidden
	}
	if !validInviteRole(role) {
		return nil, svcerr.Invalid("role must be editor or viewer")
	}
	target, err := userref.Resolve(ctx, s.DB, ref)
	if err != nil {
		return nil, err
	}
	if target.ID == w.OwnerID {
		return nil, svcerr.Invalid("you already own this list")
	}

	defer s.locks.Lock("members:" + id.String())()
	db := s.DB.WithContext(ctx)
	var existing domain.Collaborator
	err = db.Where("watchlist_id = ? AND user_id = ?", id, target.ID).First(&existing).Error
	switch {
	case err == nil && existing.Status == domain.CollaboratorAccepted:
		return nil, svcerr.Duplicate("That user already collaborates on this list")
	case err == nil:
		return nil, svcerr.Duplicate("That user has already been invited")
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}

	c := domain.Collaborator{UserID: target.ID, WatchlistID: id, Role: role, Status: domain.CollaboratorPending, InvitedBy: &ownerID}
	if err := db.Omit("User", "Watchlist").Create(&c).Error; err != nil {
		return nil, err
	}
	s.notify(ctx, target.ID, ownerID, domain.NotificationWatchlistInvite, w.Title, "/invites")
	return &Member{
		Person: Person{ID: target.ID, DisplayName: target.DisplayName, AvatarURL: target.AvatarURL},
		Role:   role, Status: domain.CollaboratorPending,
	}, nil
}

// SetRole changes a collaborator's role (also while their invitation is still
// pending). Owner only.
func (s *Service) SetRole(ctx context.Context, ownerID, id, targetID uuid.UUID, role domain.CollaboratorRole) error {
	w, myRole, err := s.access(ctx, ownerID, id)
	if err != nil {
		return err
	}
	if myRole != domain.RoleOwner {
		return svcerr.ErrForbidden
	}
	if !validInviteRole(role) {
		return svcerr.Invalid("role must be editor or viewer")
	}
	if targetID == w.OwnerID {
		return svcerr.Invalid("the owner's role cannot be changed")
	}
	res := s.DB.WithContext(ctx).Model(&domain.Collaborator{}).
		Where("watchlist_id = ? AND user_id = ?", id, targetID).Update("role", role)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return svcerr.ErrNotFound
	}
	return nil
}

// RemoveMember removes a collaborator or cancels their pending invitation.
// The owner can remove anyone; a collaborator can only remove themselves
// (leave the list). The owner cannot leave: they delete the list instead.
// Removal is permanent so the user can be invited again later.
func (s *Service) RemoveMember(ctx context.Context, actorID, id, targetID uuid.UUID) error {
	w, role, err := s.access(ctx, actorID, id)
	if err != nil {
		return err
	}
	if targetID == w.OwnerID {
		return svcerr.Invalid("the owner cannot be removed; delete the list instead")
	}
	if role != domain.RoleOwner && actorID != targetID {
		return svcerr.ErrForbidden
	}
	res := s.DB.WithContext(ctx).Unscoped().
		Where("watchlist_id = ? AND user_id = ?", id, targetID).Delete(&domain.Collaborator{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return svcerr.ErrNotFound
	}
	return nil
}

// ListInvites returns the pending invitations addressed to the user.
func (s *Service) ListInvites(ctx context.Context, userID uuid.UUID) ([]Invite, error) {
	db := s.DB.WithContext(ctx)
	var rows []domain.Collaborator
	if err := db.Where("user_id = ? AND status = ?", userID, domain.CollaboratorPending).
		Order("created_at DESC, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Invite, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}

	listIDs := make([]uuid.UUID, len(rows))
	var inviters []uuid.UUID
	for i, c := range rows {
		listIDs[i] = c.WatchlistID
		if c.InvitedBy != nil {
			inviters = append(inviters, *c.InvitedBy)
		}
	}
	var lists []domain.Watchlist
	if err := db.Select("id", "title").Where("id IN ?", listIDs).Find(&lists).Error; err != nil {
		return nil, err
	}
	titles := make(map[uuid.UUID]string, len(lists))
	for _, l := range lists {
		titles[l.ID] = l.Title
	}
	people, err := domain.LoadPublicUsers(ctx, s.DB, inviters)
	if err != nil {
		return nil, err
	}
	for _, c := range rows {
		inv := Invite{ID: c.ID, WatchlistID: c.WatchlistID, WatchlistName: titles[c.WatchlistID], Role: c.Role, CreatedAt: c.CreatedAt}
		if c.InvitedBy != nil {
			p := domain.PublicUserOrID(people, *c.InvitedBy)
			inv.InvitedBy = &p
		}
		out = append(out, inv)
	}
	return out, nil
}

// pendingInvite finds an invitation addressed to the user.
func (s *Service) pendingInvite(ctx context.Context, userID, inviteID uuid.UUID) (domain.Collaborator, error) {
	var c domain.Collaborator
	err := s.DB.WithContext(ctx).Where("id = ? AND user_id = ? AND status = ?", inviteID, userID, domain.CollaboratorPending).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c, svcerr.ErrNotFound
	}
	return c, err
}

// AcceptInvite turns a pending invitation into membership.
func (s *Service) AcceptInvite(ctx context.Context, userID, inviteID uuid.UUID) error {
	c, err := s.pendingInvite(ctx, userID, inviteID)
	if err != nil {
		return err
	}
	err = s.DB.WithContext(ctx).Model(&domain.Collaborator{}).Where("id = ?", c.ID).
		Updates(map[string]any{"status": domain.CollaboratorAccepted, "joined_at": time.Now()}).Error
	if err != nil {
		return err
	}
	if c.InvitedBy != nil {
		var w domain.Watchlist
		// Best effort: a missing title only makes the message less specific.
		_ = s.DB.WithContext(ctx).Select("id", "title").First(&w, "id = ?", c.WatchlistID).Error
		s.notify(ctx, *c.InvitedBy, userID, domain.NotificationInviteAccepted, w.Title, "/watchlists/"+c.WatchlistID.String())
	}
	return nil
}

// DeclineInvite deletes a pending invitation.
func (s *Service) DeclineInvite(ctx context.Context, userID, inviteID uuid.UUID) error {
	c, err := s.pendingInvite(ctx, userID, inviteID)
	if err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Unscoped().Where("id = ?", c.ID).Delete(&domain.Collaborator{}).Error
}
