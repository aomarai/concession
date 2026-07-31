package domain

import (
	"time"

	"github.com/google/uuid"
)

type OAuthAccount struct {
	BaseUUID
	UserID         uuid.UUID `json:"user_id" gorm:"type:uuid;not null;index"`
	Provider       string    `json:"provider" gorm:"type:varchar(30);not null;uniqueIndex:idx_provider_user"`
	ProviderUserID string    `json:"provider_user_id" gorm:"not null;uniqueIndex:idx_provider_user"`
	AccessToken    string    `json:"-"`
	RefreshToken   string    `json:"-"`

	// Relationships
	User User `json:"user" gorm:"foreignKey:UserID"`
}

type RefreshToken struct {
	BaseUUID
	UserID    uuid.UUID `json:"user_id" gorm:"type:uuid;not null;index"`
	TokenHash string    `json:"-" gorm:"not null;index"`
	UserAgent string    `json:"user_agent"`
	IPAddress string    `json:"'ip_address'"`
	ExpiresAt time.Time `json:"expires_at" gorm:"not null;index"`
	Revoked   bool      `json:"revoked" gorm:"default:false"`

	// Relationships
	User User `json:"user" gorm:"foreignKey:UserID"`
}
