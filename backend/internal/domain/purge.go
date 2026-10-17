package domain

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PurgeUser permanently deletes a user and everything that is theirs, in one
// transaction. It backs account deletion, so unlike DeleteUserCascade it hard
// deletes (soft-deleted rows included) and removes the e-mail, sessions and
// OAuth links as well, which frees the address to sign up again.
//
//   - Lists they own go, with their items and members.
//   - Their memberships, reviews, watch progress, friendships and sessions go.
//   - Notifications to them go, and so do notifications they caused, which
//     would otherwise keep their name in other people's inboxes.
//   - Items they added to other people's lists stay, re-attributed to the
//     list's owner, so a list does not lose entries when a member leaves.
//
// Every table with a foreign key to users has to be cleared first; Postgres
// enforces them (see TestPurgeUserOnPostgres). Purging an unknown user is a
// no-op.
func PurgeUser(ctx context.Context, db *gorm.DB, userID uuid.UUID) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owned []uuid.UUID
		if err := tx.Unscoped().Model(&Watchlist{}).Where("owner_id = ?", userID).Pluck("id", &owned).Error; err != nil {
			return fmt.Errorf("purge user: find owned lists: %w", err)
		}
		// A fresh handle per step: a reused gorm.DB keeps the previous step's
		// conditions and would mix them into the next statement.
		hard := func() *gorm.DB { return tx.Unscoped() }
		type step struct {
			name string
			run  func() error
		}
		var steps []step
		if len(owned) > 0 { // `IN ()` is not valid everywhere, and there would be nothing to delete
			steps = append(steps,
				step{"items on owned lists", func() error {
					return hard().Where("watchlist_id IN ?", owned).Delete(&WatchlistItem{}).Error
				}},
				step{"members of owned lists", func() error {
					return hard().Where("watchlist_id IN ?", owned).Delete(&Collaborator{}).Error
				}},
				step{"owned lists", func() error {
					return hard().Where("id IN ?", owned).Delete(&Watchlist{}).Error
				}},
			)
		}
		steps = append(steps,
			step{"items added to other lists", func() error {
				return hard().Model(&WatchlistItem{}).Where("added_by_id = ?", userID).
					Update("added_by_id", gorm.Expr("(SELECT owner_id FROM watchlists WHERE watchlists.id = watchlist_items.watchlist_id)")).Error
			}},
			step{"memberships", func() error { return hard().Where("user_id = ?", userID).Delete(&Collaborator{}).Error }},
			step{"reviews", func() error { return hard().Where("user_id = ?", userID).Delete(&Review{}).Error }},
			step{"watch progress", func() error { return hard().Where("user_id = ?", userID).Delete(&UserWatchProgress{}).Error }},
			step{"friendships", func() error {
				return hard().Where("user_a_id = ? OR user_b_id = ?", userID, userID).Delete(&Friendship{}).Error
			}},
			step{"notifications", func() error {
				return hard().Where("user_id = ? OR actor_id = ?", userID, userID).Delete(&Notification{}).Error
			}},
			step{"sessions", func() error { return hard().Where("user_id = ?", userID).Delete(&Session{}).Error }},
			step{"oauth accounts", func() error { return hard().Where("user_id = ?", userID).Delete(&OAuthAccount{}).Error }},
			step{"user", func() error { return hard().Where("id = ?", userID).Delete(&User{}).Error }},
		)
		for _, s := range steps {
			if err := s.run(); err != nil {
				return fmt.Errorf("purge user: %s: %w", s.name, err)
			}
		}
		return nil
	})
}
