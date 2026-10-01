package domain

import (
	"context"
	"strings"

	"gorm.io/gorm"
)

// FindUserByIdentifier finds a user by exact username or by e-mail
// (case-insensitive). It returns gorm.ErrRecordNotFound when there is none.
func FindUserByIdentifier(ctx context.Context, db *gorm.DB, identifier string) (User, error) {
	var u User
	identifier = strings.TrimSpace(identifier)
	err := db.WithContext(ctx).Where("username = ? OR LOWER(email) = LOWER(?)", identifier, identifier).First(&u).Error
	return u, err
}
