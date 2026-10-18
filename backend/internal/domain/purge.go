package domain

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
// enforces them (see TestPurgeUserOnPostgres). The user row is locked first, so
// a write that was already in flight (it references the user) finishes before
// the purge looks at that table instead of slipping in just before the final
// delete and failing it. Purging an unknown user is a no-op.
func PurgeUser(ctx context.Context, db *gorm.DB, userID uuid.UUID) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A fresh handle per step: a reused gorm.DB keeps the previous step's
		// conditions and would mix them into the next statement.
		hard := func() *gorm.DB { return tx.Unscoped() }

		var locked User
		err := hard().Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", userID).First(&locked).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("purge user: lock user: %w", err)
		}

		// A subquery rather than a list of IDs: a user with very many lists
		// would otherwise exceed the database's bind-parameter limit.
		const ownedLists = "(SELECT id FROM watchlists WHERE owner_id = ?)"
		steps := []struct {
			name string
			run  func() error
		}{
			{"items on owned lists", func() error {
				return hard().Where("watchlist_id IN "+ownedLists, userID).Delete(&WatchlistItem{}).Error
			}},
			{"members of owned lists", func() error {
				return hard().Where("watchlist_id IN "+ownedLists, userID).Delete(&Collaborator{}).Error
			}},
			{"owned lists", func() error { return hard().Where("owner_id = ?", userID).Delete(&Watchlist{}).Error }},
			{"items added to other lists", func() error {
				return hard().Model(&WatchlistItem{}).Where("added_by_id = ?", userID).
					Update("added_by_id", gorm.Expr("(SELECT owner_id FROM watchlists WHERE watchlists.id = watchlist_items.watchlist_id)")).Error
			}},
			{"memberships", func() error { return hard().Where("user_id = ?", userID).Delete(&Collaborator{}).Error }},
			{"reviews", func() error { return hard().Where("user_id = ?", userID).Delete(&Review{}).Error }},
			{"watch progress", func() error { return hard().Where("user_id = ?", userID).Delete(&UserWatchProgress{}).Error }},
			{"friendships", func() error {
				return hard().Where("user_a_id = ? OR user_b_id = ?", userID, userID).Delete(&Friendship{}).Error
			}},
			{"notifications", func() error {
				return hard().Where("user_id = ? OR actor_id = ?", userID, userID).Delete(&Notification{}).Error
			}},
			{"sessions", func() error { return hard().Where("user_id = ?", userID).Delete(&Session{}).Error }},
			{"oauth accounts", func() error { return hard().Where("user_id = ?", userID).Delete(&OAuthAccount{}).Error }},
			{"user", func() error { return hard().Where("id = ?", userID).Delete(&User{}).Error }},
		}
		for _, s := range steps {
			if err := s.run(); err != nil {
				return fmt.Errorf("purge user: %s: %w", s.name, err)
			}
		}
		return nil
	})
}
