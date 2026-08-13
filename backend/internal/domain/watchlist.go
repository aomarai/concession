package domain

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PrivacyLevel string

const (
	PrivacyPrivate PrivacyLevel = "private"
	PrivacyShared  PrivacyLevel = "shared"
	PrivacyPublic  PrivacyLevel = "public"
)

type CollaboratorRole string

const (
	RoleOwner  CollaboratorRole = "owner"
	RoleEditor CollaboratorRole = "editor" // Add/remove items
	RoleViewer CollaboratorRole = "viewer" // read only
)

type WatchlistType string

const (
	WatchlistTypeMovie WatchlistType = "movie"
	WatchlistTypeShow  WatchlistType = "show"
)

type Watchlist struct {
	BaseUUID
	OwnerID     uuid.UUID     `json:"owner_id" gorm:"type:uuid;not null;index"`
	Title       string        `json:"title" gorm:"not null"`
	Description string        `json:"description"`
	Privacy     PrivacyLevel  `json:"privacy" gorm:"type:varchar(20);default:'private';not null"`
	ShareToken  string        `json:"share_token" gorm:"uniqueIndex"`
	Type        WatchlistType `json:"type" gorm:"type:varchar(20);not null;index"`

	// Relationships
	Owner         User            `json:"owner" gorm:"foreignKey:OwnerID"`
	Collaborators []Collaborator  `json:"collaborators,omitempty" gorm:"foreignKey:WatchlistID;constraint:OnDelete:CASCADE"`
	Items         []WatchlistItem `json:"items,omitempty" gorm:"foreignKey:WatchlistID;constraint:OnDelete:CASCADE"`
}

type WatchlistItem struct {
	BaseUUID
	WatchlistID uuid.UUID     `json:"watchlist_id" gorm:"type:uuid;not null;index"`
	ItemType    WatchlistType `json:"item_type" gorm:"type:varchar(20);not null;index"`

	MovieID *uint64 `json:"movie_id,omitempty" gorm:"index"`
	ShowID  *uint64 `json:"show_id,omitempty" gorm:"index"`

	AddedByID uuid.UUID `json:"added_by_id" gorm:"type:uuid;not null"`
	Position  int       `json:"position" gorm:"default:0"`
	Notes     string    `json:"notes"`

	// Relationships
	Movie   *Movie `json:"movie,omitempty" gorm:"foreignKey:MovieID"`
	Show    *Show  `json:"show,omitempty" gorm:"foreignKey:ShowID"`
	AddedBy User   `json:"added_by" gorm:"foreignKey:AddedByID"`
}

type Collaborator struct {
	BaseUUID
	UserID      uuid.UUID        `json:"user_id" gorm:"type:uuid;not null;index"`
	WatchlistID uuid.UUID        `json:"watchlist_id" gorm:"type:uuid;not null;index"`
	Role        CollaboratorRole `json:"role" gorm:"type:varchar(20);default:'editor';not null"`
	JoinedAt    time.Time        `json:"joined_at"`

	// Relationships
	User      User      `json:"user" gorm:"foreignKey:UserID"`
	Watchlist Watchlist `json:"watchlist" gorm:"foreignKey:WatchlistID;constraint:OnDelete:CASCADE"`
}

// DeleteWatchlistCascade soft-deletes a watchlist along with its Items and
// Collaborators, in a single transaction. Same reasoning as
// DeleteSeasonCascade/DeleteShowCascade/DeleteUserCascade: Watchlist soft
// deletes (via BaseUUID), so a plain db.Delete(&Watchlist{}) is an UPDATE,
// not a DELETE — the `constraint:OnDelete:CASCADE` tags on
// Watchlist.Items/Collaborators never get a real DELETE to fire on.
func DeleteWatchlistCascade(ctx context.Context, db *gorm.DB, watchlistID uuid.UUID) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return deleteWatchlistCascadeTx(tx, watchlistID)
	})
}

// deleteWatchlistCascadeTx does the actual work within an existing
// transaction, so DeleteUserCascade can reuse it for each watchlist a
// deleted user owns without opening a nested transaction.
func deleteWatchlistCascadeTx(tx *gorm.DB, watchlistID uuid.UUID) error {
	if err := tx.Where("watchlist_id = ?", watchlistID).Delete(&WatchlistItem{}).Error; err != nil {
		return err
	}
	if err := tx.Where("watchlist_id = ?", watchlistID).Delete(&Collaborator{}).Error; err != nil {
		return err
	}
	return tx.Delete(&Watchlist{}, watchlistID).Error
}

// BeforeCreate ensures every Watchlist gets a unique ShareToken.
// ShareToken has a `uniqueIndex` tag but nothing was generating a value for
// it — two Watchlists created without one set explicitly would both insert
// an empty string and collide on that index (an empty string satisfies
// NOT NULL, so unlike an actual NULL it isn't exempt from the unique
// constraint).
//
// IMPORTANT: BaseUUID already defines a BeforeCreate hook (for ID
// generation). Go only promotes a method from an embedded type when the
// outer type doesn't define one of the same name itself — defining
// BeforeCreate directly on Watchlist means GORM calls THIS method instead
// of BaseUUID's, full stop, not both. So this explicitly calls
// w.BaseUUID.BeforeCreate(tx) first to keep ID generation working.
func (w *Watchlist) BeforeCreate(tx *gorm.DB) error {
	if err := w.BaseUUID.BeforeCreate(tx); err != nil {
		return err
	}
	if w.ShareToken == "" {
		token, err := generateShareToken()
		if err != nil {
			return err
		}
		w.ShareToken = token
	}
	return nil
}

func generateShareToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
