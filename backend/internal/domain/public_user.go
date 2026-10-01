package domain

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PublicUser is the part of a user's profile that other users may see. It
// deliberately has no e-mail address or username.
type PublicUser struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
}

// LoadPublicUsers fetches the public profiles of the given users, keyed by ID.
// Users that no longer exist are simply absent from the result.
func LoadPublicUsers(ctx context.Context, db *gorm.DB, ids []uuid.UUID) (map[uuid.UUID]PublicUser, error) {
	out := make(map[uuid.UUID]PublicUser, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var users []User
	if err := db.WithContext(ctx).Select("id", "display_name", "avatar_url").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, u := range users {
		out[u.ID] = PublicUser{ID: u.ID, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL}
	}
	return out, nil
}

// PublicUserOrID returns the profile for id, or a bare profile carrying only
// the ID when the account no longer exists.
func PublicUserOrID(profiles map[uuid.UUID]PublicUser, id uuid.UUID) PublicUser {
	if p, ok := profiles[id]; ok {
		return p
	}
	return PublicUser{ID: id}
}
