// Package account lets people change their own profile, set or change a local
// password, and delete their account. Sign-in is Google OAuth or a local
// password; the profile is a display name and a username either way.
package account

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const maxDisplayNameLength = 50

// usernamePattern is deliberately narrow. No "@" means a chosen username can
// never look like someone's e-mail address, which invitations and friend
// requests also accept; lower case only means uniqueness is exact.
var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{3,32}$`)

type Service struct {
	DB *gorm.DB
}

func NewService(db *gorm.DB) *Service { return &Service{DB: db} }

// ProfileUpdate holds the fields to change; nil leaves a field alone.
type ProfileUpdate struct {
	DisplayName *string
	Username    *string
}

// UpdateProfile validates and applies a profile change and returns the user
// as stored. A request with any invalid field changes nothing.
func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, in ProfileUpdate) (*domain.User, error) {
	changes := map[string]any{}
	if in.DisplayName != nil {
		name, err := CleanDisplayName(*in.DisplayName)
		if err != nil {
			return nil, err
		}
		changes["display_name"] = name
	}
	if in.Username != nil {
		name, err := CleanUsername(*in.Username)
		if err != nil {
			return nil, err
		}
		changes["username"] = name
	}
	if len(changes) == 0 {
		return nil, svcerr.Invalid("provide display_name or username")
	}

	res := s.DB.WithContext(ctx).Model(&domain.User{}).Where("id = ?", userID).Updates(changes)
	switch {
	case errors.Is(res.Error, gorm.ErrDuplicatedKey):
		// The unique index is the real guard, so this holds across processes.
		return nil, svcerr.Duplicate("That username is already taken")
	case res.Error != nil:
		return nil, res.Error
	case res.RowsAffected == 0:
		return nil, svcerr.NotFound("User not found")
	}
	var u domain.User
	if err := s.DB.WithContext(ctx).First(&u, "id = ?", userID).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func CleanDisplayName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	switch {
	case name == "":
		return "", svcerr.Invalid("display_name is required")
	case utf8.RuneCountInString(name) > maxDisplayNameLength:
		return "", svcerr.Invalid("display_name must be at most 50 characters")
	case strings.IndexFunc(name, unicode.IsControl) >= 0:
		return "", svcerr.Invalid("display_name must not contain control characters")
	}
	return name, nil
}

// CleanUsername lower-cases and validates a chosen username. Registration and
// profile updates share it so the two can never disagree.
func CleanUsername(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if !usernamePattern.MatchString(name) {
		return "", svcerr.Invalid("username must be 3 to 32 characters: letters, digits, dots, dashes and underscores")
	}
	return name, nil
}

// Delete permanently removes the account and everything in it. The caller
// must type their username or e-mail address (any case) to confirm, since
// there is no way back.
func (s *Service) Delete(ctx context.Context, userID uuid.UUID, confirm string) error {
	var u domain.User
	err := s.DB.WithContext(ctx).First(&u, "id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return svcerr.NotFound("User not found")
	}
	if err != nil {
		return err
	}
	confirm = strings.TrimSpace(confirm)
	if confirm == "" || !(strings.EqualFold(confirm, u.Username) || strings.EqualFold(confirm, u.Email)) {
		return svcerr.Invalid("confirm must be your username or e-mail address")
	}
	return domain.PurgeUser(ctx, s.DB, userID)
}
