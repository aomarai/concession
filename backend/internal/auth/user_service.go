// internal/auth/user_service.go
package auth

import (
	"context"
	"errors"

	"github.com/aomarai/concession/internal/domain"
	"gorm.io/gorm"
)

type GoogleUserInfo struct {
	ID            string
	Email         string
	VerifiedEmail bool
	Name          string
	Picture       string
}

type UserAuthService struct {
	DB *gorm.DB
}

func NewUserAuthService(db *gorm.DB) *UserAuthService {
	return &UserAuthService{DB: db}
}

func (s *UserAuthService) FindOrCreateGoogleUser(ctx context.Context, info GoogleUserInfo) (*domain.User, error) {
	var oauthAccount domain.OAuthAccount
	err := s.DB.WithContext(ctx).
		Where("provider = ? AND provider_user_id = ?", "google", info.ID).
		First(&oauthAccount).Error
	if err == nil {
		var user domain.User
		if err := s.DB.WithContext(ctx).First(&user, "id = ?", oauthAccount.UserID).Error; err != nil {
			return nil, err
		}
		return &user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	user := domain.User{
		Username:        info.Email,
		Email:           info.Email,
		IsEmailVerified: info.VerifiedEmail,
		DisplayName:     info.Name,
		AvatarURL:       info.Picture,
	}
	// Create the user and its OAuth link atomically so a failure on the second
	// insert does not leave an orphaned user behind.
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		oauthAccount = domain.OAuthAccount{
			UserID:         user.ID,
			Provider:       "google",
			ProviderUserID: info.ID,
		}
		return tx.Create(&oauthAccount).Error
	})
	if err != nil {
		return nil, err
	}

	return &user, nil
}
