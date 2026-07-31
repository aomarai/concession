package domain

import (
	"time"

	"github.com/google/uuid"
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
