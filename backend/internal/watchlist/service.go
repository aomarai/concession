// Package watchlist implements watchlist CRUD, item management, and role-based
// access. Errors use the svcerr sentinels so handlers can map them to HTTP.
package watchlist

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/events"
	"github.com/aomarai/concession/internal/keyedlock"
	"github.com/aomarai/concession/internal/logging"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	maxTitleLen       = 200
	maxDescriptionLen = 2000
	maxNotesLen       = 2000
)

// Catalog resolves TMDB IDs to stored titles; *catalog.Service implements it.
type Catalog interface {
	EnsureMovie(ctx context.Context, tmdbID int64) (*domain.Movie, error)
	EnsureShow(ctx context.Context, tmdbID int64) (*domain.Show, error)
}

// Notifier delivers notifications; *notifications.Service implements it. It
// may be nil, and failures never fail the action that triggered them.
type Notifier interface {
	Notify(ctx context.Context, userID, actorID uuid.UUID, typ domain.NotificationType, subject, link string) error
}

// Publisher receives live-update events; *events.Hub implements it. It may be
// nil.
type Publisher interface {
	Publish(ev events.Event)
}

type Service struct {
	DB       *gorm.DB
	Catalog  Catalog
	Notifier Notifier
	Events   Publisher

	locks keyedlock.Locks
}

// publish announces a change to a watchlist to connected clients.
func (s *Service) publish(typ string, listID, actor uuid.UUID, item *uuid.UUID) {
	if s.Events != nil {
		s.Events.Publish(events.Event{Type: typ, ListID: listID, ActorID: actor, ItemID: item})
	}
}

// CanView reports whether the user may read the watchlist (ErrNotFound if
// not). Live-update streams use it to authorize and to re-check access while
// a stream is open.
func (s *Service) CanView(ctx context.Context, userID, id uuid.UUID) error {
	_, _, err := s.access(ctx, userID, id)
	return err
}

func (s *Service) notify(ctx context.Context, to, actor uuid.UUID, typ domain.NotificationType, subject, link string) {
	if s.Notifier == nil {
		return
	}
	if err := s.Notifier.Notify(ctx, to, actor, typ, subject, link); err != nil {
		logging.FromContext(ctx).Warn("could not send notification", "type", typ, "error", err)
	}
}

// notifyMembers tells everyone with access to the list (its owner and accepted
// collaborators) except the actor.
func (s *Service) notifyMembers(ctx context.Context, w domain.Watchlist, actor uuid.UUID, typ domain.NotificationType) {
	if s.Notifier == nil {
		return
	}
	var ids []uuid.UUID
	err := s.DB.WithContext(ctx).Model(&domain.Collaborator{}).
		Where("watchlist_id = ? AND status = ?", w.ID, domain.CollaboratorAccepted).Pluck("user_id", &ids).Error
	if err != nil {
		logging.FromContext(ctx).Warn("could not look up who to notify", "error", err)
		return
	}
	for _, id := range append(ids, w.OwnerID) {
		if id != actor {
			s.notify(ctx, id, actor, typ, w.Title, "/watchlists/"+w.ID.String())
		}
	}
}

func NewService(db *gorm.DB, catalog Catalog) *Service {
	return &Service{DB: db, Catalog: catalog}
}

// Summary is a watchlist as shown in listings.
type Summary struct {
	ID          uuid.UUID               `json:"id"`
	OwnerID     uuid.UUID               `json:"owner_id"`
	Title       string                  `json:"title"`
	Description string                  `json:"description"`
	Privacy     domain.PrivacyLevel     `json:"privacy"`
	Type        domain.WatchlistType    `json:"type"`
	Role        domain.CollaboratorRole `json:"role"`
	ItemCount   int64                   `json:"item_count"`
	// ShareToken is only included for the owner.
	ShareToken string    `json:"share_token,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ItemView is one title on a watchlist.
type ItemView struct {
	ID        uuid.UUID            `json:"id"`
	ItemType  domain.WatchlistType `json:"item_type"`
	Position  int                  `json:"position"`
	Notes     string               `json:"notes"`
	AddedByID uuid.UUID            `json:"added_by_id"`
	Movie     *domain.Movie        `json:"movie,omitempty"`
	Show      *domain.Show         `json:"show,omitempty"`
	CreatedAt time.Time            `json:"created_at"`
}

// Detail is a watchlist with its items.
type Detail struct {
	Summary
	Items []ItemView `json:"items"`
}

func toSummary(w domain.Watchlist, role domain.CollaboratorRole, count int64) Summary {
	s := Summary{
		ID: w.ID, OwnerID: w.OwnerID, Title: w.Title, Description: w.Description,
		Privacy: w.Privacy, Type: w.Type, Role: role, ItemCount: count,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
	}
	if role == domain.RoleOwner {
		s.ShareToken = w.ShareToken
	}
	return s
}

// access loads a watchlist and the caller's role on it: owner, an accepted
// collaborator's role, or viewer for anyone on a public list. Everyone else
// (including users with only a pending invite) gets svcerr.ErrNotFound.
func (s *Service) access(ctx context.Context, userID, id uuid.UUID) (domain.Watchlist, domain.CollaboratorRole, error) {
	db := s.DB.WithContext(ctx)
	var w domain.Watchlist
	if err := db.First(&w, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return w, "", svcerr.ErrNotFound
		}
		return w, "", err
	}
	if w.OwnerID == userID {
		return w, domain.RoleOwner, nil
	}
	var c domain.Collaborator
	err := db.Where("watchlist_id = ? AND user_id = ? AND status = ?", id, userID, domain.CollaboratorAccepted).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if w.Privacy == domain.PrivacyPublic {
			return w, domain.RoleViewer, nil // public lists are readable by any signed-in user
		}
		return w, "", svcerr.ErrNotFound
	}
	if err != nil {
		return w, "", err
	}
	return w, c.Role, nil
}

func canEdit(role domain.CollaboratorRole) bool {
	return role == domain.RoleOwner || role == domain.RoleEditor
}

type CreateInput struct {
	Title       string
	Description string
	Privacy     domain.PrivacyLevel // default private
	Type        domain.WatchlistType
}

func validTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", svcerr.Invalid("title is required")
	}
	if utf8.RuneCountInString(title) > maxTitleLen {
		return "", svcerr.Invalid("title is too long")
	}
	return title, nil
}

func validDescription(d string) (string, error) {
	d = strings.TrimSpace(d)
	if utf8.RuneCountInString(d) > maxDescriptionLen {
		return "", svcerr.Invalid("description is too long")
	}
	return d, nil
}

func validPrivacy(p domain.PrivacyLevel) error {
	switch p {
	case domain.PrivacyPrivate, domain.PrivacyShared, domain.PrivacyPublic:
		return nil
	}
	return svcerr.Invalid("privacy must be private, shared or public")
}

// Create makes a new watchlist owned by ownerID.
func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, in CreateInput) (*Summary, error) {
	title, err := validTitle(in.Title)
	if err != nil {
		return nil, err
	}
	desc, err := validDescription(in.Description)
	if err != nil {
		return nil, err
	}
	if in.Privacy == "" {
		in.Privacy = domain.PrivacyPrivate
	}
	if err := validPrivacy(in.Privacy); err != nil {
		return nil, err
	}
	if in.Type != domain.WatchlistTypeMovie && in.Type != domain.WatchlistTypeShow {
		return nil, svcerr.Invalid("type must be movie or show")
	}

	w := domain.Watchlist{OwnerID: ownerID, Title: title, Description: desc, Privacy: in.Privacy, Type: in.Type}
	if err := s.DB.WithContext(ctx).Create(&w).Error; err != nil {
		return nil, err
	}
	sum := toSummary(w, domain.RoleOwner, 0)
	return &sum, nil
}

// DefaultLists are created for every new user.
var DefaultLists = []CreateInput{
	{Title: "Movies to watch", Type: domain.WatchlistTypeMovie},
	{Title: "Shows to watch", Type: domain.WatchlistTypeShow},
}

// CreateDefaultLists adds the starter lists for a new user inside tx.
func CreateDefaultLists(tx *gorm.DB, userID uuid.UUID) error {
	for _, d := range DefaultLists {
		w := domain.Watchlist{OwnerID: userID, Title: d.Title, Privacy: domain.PrivacyPrivate, Type: d.Type}
		if err := tx.Create(&w).Error; err != nil {
			return err
		}
	}
	return nil
}

// List returns the watchlists the user owns or collaborates on, newest first.
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]Summary, error) {
	db := s.DB.WithContext(ctx)

	var collabs []domain.Collaborator
	if err := db.Where("user_id = ? AND status = ?", userID, domain.CollaboratorAccepted).Find(&collabs).Error; err != nil {
		return nil, err
	}
	roles := make(map[uuid.UUID]domain.CollaboratorRole, len(collabs))
	ids := make([]uuid.UUID, 0, len(collabs))
	for _, c := range collabs {
		roles[c.WatchlistID] = c.Role
		ids = append(ids, c.WatchlistID)
	}

	var lists []domain.Watchlist
	q := db.Where("owner_id = ?", userID)
	if len(ids) > 0 {
		q = q.Or("id IN ?", ids)
	}
	if err := q.Order("created_at DESC, id").Find(&lists).Error; err != nil {
		return nil, err
	}

	counts, err := s.itemCounts(ctx, lists)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(lists))
	for _, w := range lists {
		role := domain.RoleOwner
		if w.OwnerID != userID {
			role = roles[w.ID]
		}
		out = append(out, toSummary(w, role, counts[w.ID]))
	}
	return out, nil
}

func (s *Service) itemCounts(ctx context.Context, lists []domain.Watchlist) (map[uuid.UUID]int64, error) {
	counts := make(map[uuid.UUID]int64, len(lists))
	if len(lists) == 0 {
		return counts, nil
	}
	ids := make([]uuid.UUID, len(lists))
	for i, w := range lists {
		ids[i] = w.ID
	}
	var rows []struct {
		WatchlistID uuid.UUID
		N           int64
	}
	err := s.DB.WithContext(ctx).Model(&domain.WatchlistItem{}).
		Select("watchlist_id, COUNT(*) AS n").Where("watchlist_id IN ?", ids).
		Group("watchlist_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		counts[r.WatchlistID] = r.N
	}
	return counts, nil
}

// Get returns a watchlist with its items in order.
func (s *Service) Get(ctx context.Context, userID, id uuid.UUID) (*Detail, error) {
	w, role, err := s.access(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, w, role)
}

func (s *Service) detail(ctx context.Context, w domain.Watchlist, role domain.CollaboratorRole) (*Detail, error) {
	var items []domain.WatchlistItem
	err := s.DB.WithContext(ctx).Preload("Movie.Genres").Preload("Show.Genres").
		Where("watchlist_id = ?", w.ID).Order("position, created_at, id").Find(&items).Error
	if err != nil {
		return nil, err
	}
	d := &Detail{Summary: toSummary(w, role, int64(len(items))), Items: make([]ItemView, len(items))}
	for i, it := range items {
		d.Items[i] = toItemView(it)
	}
	return d, nil
}

func toItemView(it domain.WatchlistItem) ItemView {
	return ItemView{
		ID: it.ID, ItemType: it.ItemType, Position: it.Position, Notes: it.Notes,
		AddedByID: it.AddedByID, Movie: it.Movie, Show: it.Show, CreatedAt: it.CreatedAt,
	}
}

type UpdateInput struct {
	Title       *string
	Description *string
	Privacy     *domain.PrivacyLevel
}

// Update changes a watchlist's metadata. Owner only.
func (s *Service) Update(ctx context.Context, userID, id uuid.UUID, in UpdateInput) (*Summary, error) {
	w, role, err := s.access(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if role != domain.RoleOwner {
		return nil, svcerr.ErrForbidden
	}
	if in.Title != nil {
		if w.Title, err = validTitle(*in.Title); err != nil {
			return nil, err
		}
	}
	if in.Description != nil {
		if w.Description, err = validDescription(*in.Description); err != nil {
			return nil, err
		}
	}
	if in.Privacy != nil {
		if err := validPrivacy(*in.Privacy); err != nil {
			return nil, err
		}
		w.Privacy = *in.Privacy
	}
	if err := s.DB.WithContext(ctx).Omit("Owner", "Collaborators", "Items").Save(&w).Error; err != nil {
		return nil, err
	}
	var count int64
	if err := s.DB.WithContext(ctx).Model(&domain.WatchlistItem{}).Where("watchlist_id = ?", id).Count(&count).Error; err != nil {
		return nil, err
	}
	s.publish(events.ListUpdated, id, userID, nil)
	sum := toSummary(w, role, count)
	return &sum, nil
}

// Delete removes a watchlist with its items and collaborators. Owner only.
func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	_, role, err := s.access(ctx, userID, id)
	if err != nil {
		return err
	}
	if role != domain.RoleOwner {
		return svcerr.ErrForbidden
	}
	if err := domain.DeleteWatchlistCascade(ctx, s.DB, id); err != nil {
		return err
	}
	s.publish(events.ListDeleted, id, userID, nil)
	return nil
}

// AddItem adds the title with the given TMDB ID to the end of a watchlist.
// Owners and editors only. The title kind must match the watchlist type.
func (s *Service) AddItem(ctx context.Context, userID, id uuid.UUID, tmdbID int64, notes string) (*ItemView, error) {
	w, role, err := s.access(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if !canEdit(role) {
		return nil, svcerr.ErrForbidden
	}
	if notes, err = validNotes(notes); err != nil {
		return nil, err
	}

	item := domain.WatchlistItem{WatchlistID: id, ItemType: w.Type, AddedByID: userID, Notes: notes}
	switch w.Type {
	case domain.WatchlistTypeMovie:
		m, err := s.Catalog.EnsureMovie(ctx, tmdbID)
		if err != nil {
			return nil, err
		}
		item.MovieID, item.Movie = &m.ID, m
	default:
		sh, err := s.Catalog.EnsureShow(ctx, tmdbID)
		if err != nil {
			return nil, err
		}
		item.ShowID, item.Show = &sh.ID, sh
	}

	defer s.locks.Lock(id.String())()
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		col, val := "movie_id", any(item.MovieID)
		if item.ShowID != nil {
			col, val = "show_id", item.ShowID
		}
		var existing int64
		if err := tx.Model(&domain.WatchlistItem{}).
			Where("watchlist_id = ? AND "+col+" = ?", id, val).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return svcerr.ErrDuplicate
		}
		var last struct{ Max *int }
		if err := tx.Model(&domain.WatchlistItem{}).Select("MAX(position) AS max").
			Where("watchlist_id = ?", id).Scan(&last).Error; err != nil {
			return err
		}
		if last.Max != nil {
			item.Position = *last.Max + 1
		}
		return tx.Omit("Movie", "Show", "AddedBy").Create(&item).Error
	})
	if err != nil {
		return nil, err
	}
	s.notifyMembers(ctx, w, userID, domain.NotificationItemAdded)
	s.publish(events.ItemAdded, id, userID, &item.ID)
	v := toItemView(item)
	return &v, nil
}

func validNotes(n string) (string, error) {
	n = strings.TrimSpace(n)
	if utf8.RuneCountInString(n) > maxNotesLen {
		return "", svcerr.Invalid("notes are too long")
	}
	return n, nil
}

func (s *Service) editableItem(ctx context.Context, userID, id, itemID uuid.UUID) (domain.WatchlistItem, error) {
	var item domain.WatchlistItem
	_, role, err := s.access(ctx, userID, id)
	if err != nil {
		return item, err
	}
	if !canEdit(role) {
		return item, svcerr.ErrForbidden
	}
	err = s.DB.WithContext(ctx).Where("id = ? AND watchlist_id = ?", itemID, id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return item, svcerr.ErrNotFound
	}
	return item, err
}

// UpdateItemNotes changes an item's notes. Owners and editors only.
func (s *Service) UpdateItemNotes(ctx context.Context, userID, id, itemID uuid.UUID, notes string) (*ItemView, error) {
	item, err := s.editableItem(ctx, userID, id, itemID)
	if err != nil {
		return nil, err
	}
	if item.Notes, err = validNotes(notes); err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).Model(&item).Update("notes", item.Notes).Error; err != nil {
		return nil, err
	}
	s.publish(events.ItemUpdated, id, userID, &item.ID)
	v := toItemView(item)
	return &v, nil
}

// RemoveItem deletes an item and closes the gap in positions. Owners and
// editors only.
func (s *Service) RemoveItem(ctx context.Context, userID, id, itemID uuid.UUID) error {
	if _, err := s.editableItem(ctx, userID, id, itemID); err != nil {
		return err
	}
	defer s.locks.Lock(id.String())()
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND watchlist_id = ?", itemID, id).Delete(&domain.WatchlistItem{}).Error; err != nil {
			return err
		}
		return renumber(tx, id)
	})
	if err != nil {
		return err
	}
	s.publish(events.ItemRemoved, id, userID, &itemID)
	return nil
}

// renumber rewrites positions of a list's items to 0..n-1 keeping their order.
func renumber(tx *gorm.DB, id uuid.UUID) error {
	var items []domain.WatchlistItem
	if err := tx.Where("watchlist_id = ?", id).Order("position, created_at, id").Find(&items).Error; err != nil {
		return err
	}
	for i, it := range items {
		if it.Position == i {
			continue
		}
		if err := tx.Model(&domain.WatchlistItem{}).Where("id = ?", it.ID).Update("position", i).Error; err != nil {
			return err
		}
	}
	return nil
}

// Reorder sets the order of all items. itemIDs must list every item on the
// watchlist exactly once. Owners and editors only.
func (s *Service) Reorder(ctx context.Context, userID, id uuid.UUID, itemIDs []uuid.UUID) error {
	_, role, err := s.access(ctx, userID, id)
	if err != nil {
		return err
	}
	if !canEdit(role) {
		return svcerr.ErrForbidden
	}
	defer s.locks.Lock(id.String())()
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current []domain.WatchlistItem
		if err := tx.Where("watchlist_id = ?", id).Find(&current).Error; err != nil {
			return err
		}
		if len(current) != len(itemIDs) {
			return svcerr.Invalid("item_ids must list every item exactly once")
		}
		have := make(map[uuid.UUID]bool, len(current))
		for _, it := range current {
			have[it.ID] = true
		}
		for _, itemID := range itemIDs {
			if !have[itemID] {
				return svcerr.Invalid("item_ids must list every item exactly once")
			}
			delete(have, itemID) // a repeated ID is no longer in the set
		}
		for pos, itemID := range itemIDs {
			if err := tx.Model(&domain.WatchlistItem{}).Where("id = ?", itemID).Update("position", pos).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.publish(events.ItemsReordered, id, userID, nil)
	return nil
}
