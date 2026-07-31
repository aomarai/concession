package domain

import (
	"fmt"
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

type MovieWatchlist struct {
	BaseUUID
	OwnerID     uuid.UUID    `json:"owner_id" gorm:"type:uuid;not null;index"`
	Title       string       `json:"title" gorm:"not null"`
	Description string       `json:"description"`
	Privacy     PrivacyLevel `json:"privacy" gorm:"type:varchar(20);default:'private';not null"`
	ShareToken  string       `json:"share_token" gorm:"uniqueIndex"`

	// Relationships
	Owner         User                 `json:"owner" gorm:"foreignKey:OwnerID"`
	Collaborators []Collaborator       `json:"collaborators,omitempty" gorm:"foreignKey:MovieWatchlistID;constraint:OnDelete:CASCADE"`
	Items         []MovieWatchlistItem `json:"items,omitempty" gorm:"foreignKey:WatchlistID;constraint:OnDelete:CASCADE"`
}

type MovieWatchlistItem struct {
	BaseUUID
	WatchlistID uuid.UUID `json:"watchlist_id" gorm:"type:uuid;not null;index"`
	MovieID     uint64    `json:"movie_id" gorm:"not null;index"` // Strict Foreign Key to Movie
	AddedByID   uuid.UUID `json:"added_by_id" gorm:"type:uuid;not null"`
	Position    int       `json:"position" gorm:"default:0"`
	Notes       string    `json:"notes"`

	// Direct foreign key relations
	Movie   Movie `json:"movie" gorm:"foreignKey:MovieID"`
	AddedBy User  `json:"added_by" gorm:"foreignKey:AddedByID"`
}

type ShowWatchlist struct {
	BaseUUID
	OwnerID     uuid.UUID    `json:"owner_id" gorm:"type:uuid;not null;index"`
	Title       string       `json:"title" gorm:"not null"`
	Description string       `json:"description"`
	Privacy     PrivacyLevel `json:"privacy" gorm:"type:varchar(20);default:'private';not null"`
	ShareToken  string       `json:"share_token" gorm:"uniqueIndex"`

	// Relationships
	Owner         User                `json:"owner" gorm:"foreignKey:OwnerID"`
	Collaborators []Collaborator      `json:"collaborators,omitempty" gorm:"foreignKey:ShowWatchlistID;constraint:OnDelete:CASCADE"`
	Items         []ShowWatchlistItem `json:"items,omitempty" gorm:"foreignKey:WatchlistID;constraint:OnDelete:CASCADE"`
}

type ShowWatchlistItem struct {
	BaseUUID
	WatchlistID uuid.UUID `json:"watchlist_id" gorm:"type:uuid;not null;index"`
	ShowID      uint64    `json:"show_id" gorm:"not null;index"` // Strict Foreign Key to Show
	AddedByID   uuid.UUID `json:"added_by_id" gorm:"type:uuid;not null"`
	Position    int       `json:"position" gorm:"default:0"`
	Notes       string    `json:"notes"`

	// Direct foreign key relations
	Show    Show `json:"show" gorm:"foreignKey:ShowID"`
	AddedBy User `json:"added_by" gorm:"foreignKey:AddedByID"`
}

type Collaborator struct {
	BaseUUID
	UserID           uuid.UUID        `json:"user_id" gorm:"type:uuid;not null;index"`
	MovieWatchlistID *uuid.UUID       `json:"movie_watchlist_id,omitempty" gorm:"type:uuid;index"`
	ShowWatchlistID  *uuid.UUID       `json:"show_watchlist_id,omitempty" gorm:"type:uuid;index"`
	Role             CollaboratorRole `json:"role" gorm:"type:varchar(20);default:'editor';not null"`
	JoinedAt         time.Time        `json:"joined_at"`

	// Relationships
	User           User            `json:"user" gorm:"foreignKey:UserID"`
	MovieWatchlist *MovieWatchlist `json:"movie_watchlist,omitempty" gorm:"foreignKey:MovieWatchlistID;constraint:OnDelete:CASCADE"`
	ShowWatchlist  *ShowWatchlist  `json:"show_watchlist,omitempty" gorm:"foreignKey:ShowWatchlistID;constraint:OnDelete:CASCADE"`
}

func (c *Collaborator) BeforeSave(tx *gorm.DB) error {
	movieSet := c.MovieWatchlistID != nil
	showSet := c.ShowWatchlistID != nil

	if movieSet == showSet {
		return fmt.Errorf("exactly one of MovieWatchlistID or ShowWatchlistID must be set for collaborator %s", c.ID)
	}
	return nil
}
