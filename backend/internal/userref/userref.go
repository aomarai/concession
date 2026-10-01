// Package userref resolves "which user do you mean?" in API requests: either a
// username/e-mail the caller typed, or a user ID taken from a friends list.
package userref

import (
	"context"
	"errors"
	"strings"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Ref identifies a user by exactly one of Identifier (username or e-mail) or ID.
type Ref struct {
	Identifier string
	ID         uuid.UUID
}

// Resolve finds the referenced user. A missing or ambiguous reference is a
// validation error; an unknown user is a not-found error with a helpful
// message.
func Resolve(ctx context.Context, db *gorm.DB, ref Ref) (domain.User, error) {
	var u domain.User
	var err error
	ref.Identifier = strings.TrimSpace(ref.Identifier)
	switch {
	case ref.ID != uuid.Nil && ref.Identifier != "":
		return u, svcerr.Invalid("provide either user or user_id, not both")
	case ref.ID != uuid.Nil:
		err = db.WithContext(ctx).First(&u, "id = ?", ref.ID).Error
	case ref.Identifier != "":
		u, err = domain.FindUserByIdentifier(ctx, db, ref.Identifier)
	default:
		return u, svcerr.Invalid("user is required")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return u, svcerr.NotFound("No user found with that username, email or ID")
	}
	return u, err
}
