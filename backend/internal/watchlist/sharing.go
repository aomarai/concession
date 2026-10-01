package watchlist

import (
	"context"
	"errors"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetShared returns the watchlist behind a share link. Links only work while
// the list is "shared" or "public"; a private list (or an unknown or rotated
// token) is ErrNotFound. The caller's own role is used when they are a member,
// otherwise they get read-only viewer access. Signing in is still required.
func (s *Service) GetShared(ctx context.Context, userID uuid.UUID, token string) (*Detail, error) {
	if token == "" {
		return nil, svcerr.ErrNotFound
	}
	var w domain.Watchlist
	err := s.DB.WithContext(ctx).First(&w, "share_token = ?", token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, svcerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if w.Privacy == domain.PrivacyPrivate {
		return nil, svcerr.ErrNotFound
	}
	_, role, err := s.access(ctx, userID, w.ID)
	if errors.Is(err, svcerr.ErrNotFound) {
		role, err = domain.RoleViewer, nil
	}
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, w, role)
}

// RotateShareToken replaces the list's share token, which disables every
// existing link to it. Owner only. It returns the new token.
func (s *Service) RotateShareToken(ctx context.Context, userID, id uuid.UUID) (string, error) {
	w, role, err := s.access(ctx, userID, id)
	if err != nil {
		return "", err
	}
	if role != domain.RoleOwner {
		return "", svcerr.ErrForbidden
	}
	token, err := domain.NewShareToken()
	if err != nil {
		return "", err
	}
	if err := s.DB.WithContext(ctx).Model(&domain.Watchlist{}).Where("id = ?", w.ID).Update("share_token", token).Error; err != nil {
		return "", err
	}
	return token, nil
}
