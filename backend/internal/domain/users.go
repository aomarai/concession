package domain

import (
	"time"

	"github.com/google/uuid"
)

type UserRole string

const (
	RoleUser  UserRole = "user"
	RoleAdmin UserRole = "admin"
)

type User struct {
	BaseUUID
	Username        string     `json:"username" gorm:"uniqueIndex;not null"`
	Email           string     `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash    string     `json:"-" gorm:"not null"`
	IsEmailVerified bool       `json:"is_email_verified" gorm:"default:false;not null"`
	Role            UserRole   `json:"role" gorm:"type:varchar(20);default:'user';not null"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`

	// Profile metadata
	DisplayName string `json:"display_name" gorm:"not null"`
	AvatarURL   string `json:"avatar_url" gorm:"index"`

	// Relationships
	Reviews        []Review            `json:"reviews,omitempty" gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	Watchlists     []Watchlist         `json:"watchlists,omitempty" gorm:"foreignKey:OwnerID;constraint:OnDelete:CASCADE"`
	Collaborations []Collaborator      `json:"collaborations,omitempty" gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	WatchProgress  []UserWatchProgress `json:"watch_progress,omitempty" gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	OAuthAccounts  []OAuthAccount      `json:"oauth_accounts,omitempty"`
}

type WatchStatus string

const (
	StatusPlanToWatch WatchStatus = "plan_to_watch"
	StatusWatching    WatchStatus = "watching"
	StatusCompleted   WatchStatus = "completed"
	StatusDropped     WatchStatus = "dropped"
)

type ItemType string

const (
	ItemTypeMovie ItemType = "movie"
	ItemTypeShow  ItemType = "show"
)

type UserWatchProgress struct {
	BaseUUID
	UserID   uuid.UUID   `json:"user_id" gorm:"type:uuid;not null;uniqueIndex:idx_user_item"`
	ItemType ItemType    `json:"item_type" gorm:"type:varchar(20);not null;uniqueIndex:idx_user_item"` // "movie" or "show"
	ItemID   uint64      `json:"item_id" gorm:"not null;uniqueIndex:idx_user_item"`                    // Movie ID or Show ID
	Status   WatchStatus `json:"status" gorm:"type:varchar(20);not null"`

	// TV Show specific tracking
	LastSeasonNum  uint32    `json:"last_season_num"`  // e.g. Season 2
	LastEpisodeNum uint32    `json:"last_episode_num"` // e.g. Episode 4
	WatchedAt      time.Time `json:"watched_at"`
}
