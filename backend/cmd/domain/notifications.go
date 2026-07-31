package domain

import "github.com/google/uuid"

type NotificationType string

const (
	NotificationWatchlistInvite NotificationType = "watchlist_invite"
	NotificationItemAdded       NotificationType = "item_added"
	NotificationFriendRequest   NotificationType = "friend_request"
)

type Notification struct {
	BaseUUID
	UserID  uuid.UUID        `json:"user_id" gorm:"type:uuid;not null;index"`
	ActorID uuid.UUID        `json:"actor_id" gorm:"type:uuid;not null"`
	Type    NotificationType `json:"type" gorm:"type:varchar(30);not null"`
	Message string           `json:"message" gorm:"not null"`
	IsRead  bool             `json:"is_read" gorm:"default:false;index"`
	LinkURL string           `json:"link_url"`

	// Relationships
	Actor User `json:"actor" gorm:"foreignKey:ActorID"`
}
