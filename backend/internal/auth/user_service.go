// internal/auth/user_service.go
package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aomarai/concession/internal/account"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/watchlist"
	"github.com/google/uuid"
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

// LocalUserRegistration is what a person supplies when creating a password
// account.
type LocalUserRegistration struct {
	Email       string
	Username    string
	DisplayName string
	Password    string
}

// ErrInvalidCredentials is returned for every local sign-in failure: an unknown
// identifier, an account with no local password, and a wrong password are
// deliberately indistinguishable to the caller.
var ErrInvalidCredentials = errors.New("invalid credentials")

// RegisterLocalUser creates an account with a local password plus its starter
// watchlists, in one transaction. The e-mail is stored lower-cased so the unique
// index is exact.
func (s *UserAuthService) RegisterLocalUser(ctx context.Context, in LocalUserRegistration) (*domain.User, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
		return nil, svcerr.Invalid("a valid e-mail address is required")
	}

	username, err := account.CleanUsername(in.Username)
	if err != nil {
		return nil, err
	}
	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		displayName = username
	}
	if displayName, err = account.CleanDisplayName(displayName); err != nil {
		return nil, err
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}

	// A clear error for the common case; the unique indexes are what actually
	// enforce it, so a duplicate key from a concurrent request is mapped too.
	for _, check := range []struct {
		column string
		value  string
		msg    string
	}{
		{"email", email, "That e-mail already has an account"},
		{"username", username, "That username is already taken"},
	} {
		var taken int64
		if err := s.DB.WithContext(ctx).Model(&domain.User{}).Where(check.column+" = ?", check.value).Count(&taken).Error; err != nil {
			return nil, err
		}
		if taken > 0 {
			return nil, svcerr.Duplicate(check.msg)
		}
	}

	user := domain.User{
		Username:     username,
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: hash,
		HasPassword:  true,
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return watchlist.CreateDefaultLists(tx, user.ID)
	})
	switch {
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return nil, svcerr.Duplicate("That e-mail or username is already taken")
	case err != nil:
		return nil, err
	}

	return &user, nil
}

// AuthenticateLocal checks a password against the account identified by an
// e-mail address or a username and records the sign-in.
func (s *UserAuthService) AuthenticateLocal(ctx context.Context, identifier, password string) (*domain.User, error) {
	id := strings.ToLower(strings.TrimSpace(identifier))
	var user domain.User
	err := s.DB.WithContext(ctx).Where("email = ? OR username = ?", id, id).First(&user).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		burnPasswordCheck(password)
		return nil, ErrInvalidCredentials
	case err != nil:
		return nil, err
	}

	if user.PasswordHash == "" {
		// A Google-only account: spend the same bcrypt work, then refuse.
		burnPasswordCheck(password)
		return nil, ErrInvalidCredentials
	}
	if !CheckPassword(user.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}

	now := time.Now()
	if err := s.DB.WithContext(ctx).Model(&user).Update("last_login_at", now).Error; err != nil {
		return nil, err
	}
	user.LastLoginAt = &now

	return &user, nil
}

// SetPassword sets a local password, or changes an existing one when the
// current password is supplied and correct. An account with no password (a
// Google-only account) needs no current password: the caller is already signed
// in as that user.
func (s *UserAuthService) SetPassword(ctx context.Context, userID uuid.UUID, current, next string) error {
	var user domain.User
	if err := s.DB.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		return err
	}
	if user.PasswordHash != "" && !CheckPassword(user.PasswordHash, current) {
		return svcerr.Invalid("current password is incorrect")
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	if err := s.DB.WithContext(ctx).Model(&user).Update("password_hash", hash).Error; err != nil {
		return err
	}
	user.PasswordHash = hash
	return nil
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
	// Create the user, its OAuth link and the starter watchlists atomically so a
	// failure part-way does not leave a half-created account behind.
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		oauthAccount = domain.OAuthAccount{
			UserID:         user.ID,
			Provider:       "google",
			ProviderUserID: info.ID,
		}
		if err := tx.Create(&oauthAccount).Error; err != nil {
			return err
		}
		return watchlist.CreateDefaultLists(tx, user.ID)
	})
	if err != nil {
		return nil, err
	}

	return &user, nil
}
