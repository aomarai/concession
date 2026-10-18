package domain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// purgeScenario is two people who share a lot: a is purged, b must keep
// everything that is not a's.
type purgeScenario struct {
	a, b            User
	listA, listAOld Watchlist // owned by a (listAOld was soft-deleted earlier)
	listB           Watchlist // owned by b, a collaborates on it
	itemByA         WatchlistItem
	itemByB         WatchlistItem
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func seedPurgeScenario(t *testing.T, db *gorm.DB) purgeScenario {
	t.Helper()
	var s purgeScenario
	s.a = User{Username: "a@example.com", Email: "a@example.com", DisplayName: "A"}
	s.b = User{Username: "b@example.com", Email: "b@example.com", DisplayName: "B"}
	must(t, db.Create(&s.a).Error)
	must(t, db.Create(&s.b).Error)

	var movies [4]Movie
	for i := range movies {
		movies[i] = Movie{TMDBID: int64(1000 + i), Title: fmt.Sprintf("M%d", i)}
		must(t, db.Create(&movies[i]).Error)
	}
	item := func(list Watchlist, by User, m Movie) WatchlistItem {
		it := WatchlistItem{WatchlistID: list.ID, ItemType: WatchlistTypeMovie, MovieID: &m.ID, AddedByID: by.ID}
		must(t, db.Create(&it).Error)
		return it
	}
	newList := func(owner User, title string) Watchlist {
		w := Watchlist{OwnerID: owner.ID, Title: title, Type: WatchlistTypeMovie, Privacy: PrivacyShared}
		must(t, db.Create(&w).Error)
		return w
	}

	s.listA = newList(s.a, "A's list")
	item(s.listA, s.a, movies[0])
	must(t, db.Create(&Collaborator{UserID: s.b.ID, WatchlistID: s.listA.ID, Role: RoleEditor, Status: CollaboratorAccepted, InvitedBy: &s.a.ID}).Error)

	s.listAOld = newList(s.a, "A's deleted list")
	item(s.listAOld, s.a, movies[1])
	must(t, DeleteWatchlistCascade(context.Background(), db, s.listAOld.ID)) // soft-deleted rows must go too

	s.listB = newList(s.b, "B's list")
	s.itemByA = item(s.listB, s.a, movies[2])
	s.itemByB = item(s.listB, s.b, movies[3])
	must(t, db.Create(&Collaborator{UserID: s.a.ID, WatchlistID: s.listB.ID, Role: RoleEditor, Status: CollaboratorAccepted, InvitedBy: &s.b.ID}).Error)

	for _, u := range []User{s.a, s.b} {
		must(t, db.Create(&Review{UserID: u.ID, Rating: 7, ReviewableID: movies[0].ID, ReviewableType: ReviewableMovies}).Error)
		must(t, db.Create(&UserWatchProgress{UserID: u.ID, ItemType: ItemTypeMovie, ItemID: movies[0].ID, Status: StatusWatching}).Error)
		must(t, db.Create(&Session{UserID: u.ID, TokenHash: "h-" + u.DisplayName, ExpiresAt: time.Now().Add(time.Hour)}).Error)
		must(t, db.Create(&OAuthAccount{UserID: u.ID, Provider: "google", ProviderUserID: "g-" + u.DisplayName}).Error)
	}
	// a's review is soft-deleted, like any review removed through the normal flow.
	must(t, db.Where("user_id = ?", s.a.ID).Delete(&Review{}).Error)

	lo, hi := OrderedPair(s.a.ID, s.b.ID)
	must(t, db.Create(&Friendship{UserAID: lo, UserBID: hi, RequestedBy: s.a.ID, Status: FriendshipAccepted}).Error)

	// a notified b (actor a), b notified a (actor b), and an unrelated one for b.
	for _, n := range []Notification{
		{UserID: s.b.ID, ActorID: s.a.ID, Type: NotificationFriendRequest, Message: "A sent a request"},
		{UserID: s.a.ID, ActorID: s.b.ID, Type: NotificationFriendAccepted, Message: "B accepted"},
		{UserID: s.b.ID, ActorID: s.b.ID, Type: NotificationItemAdded, Message: "B did a thing"},
	} {
		n := n
		must(t, db.Create(&n).Error)
	}
	return s
}

func unscopedCount(t *testing.T, db *gorm.DB, model any, query string, args ...any) int64 {
	t.Helper()
	var n int64
	must(t, db.Unscoped().Model(model).Where(query, args...).Count(&n).Error)
	return n
}

func assertPurged(t *testing.T, db *gorm.DB, s purgeScenario) {
	t.Helper()
	a, b := s.a.ID, s.b.ID
	gone := []struct {
		name  string
		model any
		where string
		args  []any
	}{
		{"user", &User{}, "id = ?", []any{a}},
		{"sessions", &Session{}, "user_id = ?", []any{a}},
		{"oauth accounts", &OAuthAccount{}, "user_id = ?", []any{a}},
		{"reviews (even soft-deleted)", &Review{}, "user_id = ?", []any{a}},
		{"progress", &UserWatchProgress{}, "user_id = ?", []any{a}},
		{"owned lists (even soft-deleted)", &Watchlist{}, "owner_id = ?", []any{a}},
		{"items on owned lists (even soft-deleted)", &WatchlistItem{}, "watchlist_id IN ?", []any{[]uuid.UUID{s.listA.ID, s.listAOld.ID}}},
		{"collaborations on owned lists", &Collaborator{}, "watchlist_id IN ?", []any{[]uuid.UUID{s.listA.ID, s.listAOld.ID}}},
		{"memberships", &Collaborator{}, "user_id = ?", []any{a}},
		{"friendships", &Friendship{}, "user_a_id = ? OR user_b_id = ?", []any{a, a}},
		{"notifications to a", &Notification{}, "user_id = ?", []any{a}},
		{"notifications caused by a", &Notification{}, "actor_id = ?", []any{a}},
		{"anything added by a", &WatchlistItem{}, "added_by_id = ?", []any{a}},
	}
	for _, g := range gone {
		if n := unscopedCount(t, db, g.model, g.where, g.args...); n != 0 {
			t.Errorf("%s: %d rows remain", g.name, n)
		}
	}

	// b keeps everything that is not a's.
	if n := unscopedCount(t, db, &User{}, "id = ?", b); n != 1 {
		t.Errorf("b was removed")
	}
	if n := unscopedCount(t, db, &Watchlist{}, "id = ?", s.listB.ID); n != 1 {
		t.Errorf("b's list was removed")
	}
	if n := unscopedCount(t, db, &Review{}, "user_id = ?", b); n != 1 {
		t.Errorf("b's review was removed")
	}
	if n := unscopedCount(t, db, &UserWatchProgress{}, "user_id = ?", b); n != 1 {
		t.Errorf("b's progress was removed")
	}
	if n := unscopedCount(t, db, &Session{}, "user_id = ?", b); n != 1 {
		t.Errorf("b's session was removed")
	}
	if n := unscopedCount(t, db, &OAuthAccount{}, "user_id = ?", b); n != 1 {
		t.Errorf("b's oauth account was removed")
	}
	if n := unscopedCount(t, db, &Notification{}, "user_id = ? AND actor_id = ?", b, b); n != 1 {
		t.Errorf("b's unrelated notification was removed")
	}

	// What a added to b's list stays, now attributed to the list's owner.
	var kept WatchlistItem
	must(t, db.First(&kept, "id = ?", s.itemByA.ID).Error)
	if kept.AddedByID != b {
		t.Errorf("an item a added to b's list should now be attributed to b, got %v", kept.AddedByID)
	}
	var other WatchlistItem
	must(t, db.First(&other, "id = ?", s.itemByB.ID).Error)
	if other.AddedByID != b {
		t.Errorf("b's own item changed owner")
	}
}

func TestPurgeUserRemovesEverythingOfTheUser(t *testing.T) {
	db := testutil.NewDB(t, allModels()...)
	s := seedPurgeScenario(t, db)
	must(t, PurgeUser(context.Background(), db, s.a.ID))
	assertPurged(t, db, s)
}

func TestPurgeUserIsANoOpForUnknownUsers(t *testing.T) {
	db := testutil.NewDB(t, allModels()...)
	s := seedPurgeScenario(t, db)
	must(t, PurgeUser(context.Background(), db, uuid.New()))
	if n := unscopedCount(t, db, &User{}, "1 = 1"); n != 2 {
		t.Errorf("users = %d, nothing should have been deleted", n)
	}
	must(t, PurgeUser(context.Background(), db, s.a.ID))
	must(t, PurgeUser(context.Background(), db, s.a.ID)) // twice is fine
}

func TestPurgeUserFreesTheEmailAndUsername(t *testing.T) {
	db := testutil.NewDB(t, allModels()...)
	s := seedPurgeScenario(t, db)
	must(t, PurgeUser(context.Background(), db, s.a.ID))
	// Unlike a soft delete, the unique indexes no longer hold the address, so
	// the same Google account can sign up again.
	again := User{Username: s.a.Username, Email: s.a.Email, DisplayName: "A again"}
	if err := db.Create(&again).Error; err != nil {
		t.Errorf("re-registering after deletion failed: %v", err)
	}
}

func TestPurgeUserIsAllOrNothing(t *testing.T) {
	steps := []struct{ op, table string }{
		{"query", "users"}, {"delete", "watchlist_items"}, {"delete", "collaborators"},
		{"delete", "watchlists"}, {"update", "watchlist_items"}, {"delete", "reviews"},
		{"delete", "user_watch_progresses"}, {"delete", "friendships"}, {"delete", "notifications"},
		{"delete", "sessions"}, {"delete", "o_auth_accounts"}, {"delete", "users"},
	}
	for _, step := range steps {
		t.Run(step.op+"_"+step.table, func(t *testing.T) {
			db := testutil.NewDB(t, allModels()...)
			s := seedPurgeScenario(t, db)
			testutil.FailOn(t, db, step.op, step.table)
			err := PurgeUser(context.Background(), db, s.a.ID)
			if !errors.Is(err, testutil.ErrInjected) {
				t.Fatalf("error = %v, want the injected failure", err)
			}
			if !strings.Contains(err.Error(), "purge user") {
				t.Errorf("error should say what was being done: %v", err)
			}
			// The transaction rolled back: nothing of a's is gone.
			checks := []struct {
				table string
				model any
				query string
			}{
				{"users", &User{}, "id = ?"}, {"sessions", &Session{}, "user_id = ?"},
				{"o_auth_accounts", &OAuthAccount{}, "user_id = ?"}, {"notifications", &Notification{}, "actor_id = ?"},
				{"collaborators", &Collaborator{}, "user_id = ?"}, {"watchlists", &Watchlist{}, "owner_id = ?"},
			}
			for _, c := range checks {
				if step.op == "query" && c.table == step.table {
					continue // the injected failure would break this very check
				}
				if n := unscopedCount(t, db, c.model, c.query, s.a.ID); n == 0 {
					t.Errorf("%T was partly deleted despite the failure", c.model)
				}
			}
		})
	}
}

// postgresSchema opens a private schema on the Postgres named by
// TEST_POSTGRES_DSN, with the real foreign keys, or skips the test. The schema
// is dropped afterwards, so it is safe to point at a shared database.
func postgresSchema(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	schema := "purge_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	must(t, err)
	must(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{Logger: logger.Discard})
	must(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	must(t, db.AutoMigrate(allModels()...)) // with the real foreign keys
	return db
}

// TestPurgeUserOnPostgres runs the full scenario on a real Postgres, where the
// foreign keys are enforced (SQLite tests create none). It is skipped unless
// TEST_POSTGRES_DSN is set; CI provides a Postgres service for it.
func TestPurgeUserOnPostgres(t *testing.T) {
	db := postgresSchema(t)
	s := seedPurgeScenario(t, db)
	must(t, PurgeUser(context.Background(), db, s.a.ID))
	assertPurged(t, db, s)
}

// TestPurgeUserWaitsForInFlightWrites: a request that was already authenticated
// can still be writing a row that points at the user when deletion starts.
// Without the lock on the user row, the purge would clear that table, then
// fail at the final delete once the write commits (a foreign-key violation).
func TestPurgeUserWaitsForInFlightWrites(t *testing.T) {
	db := postgresSchema(t)
	s := seedPurgeScenario(t, db)

	inFlight := db.Begin()
	must(t, inFlight.Error)
	must(t, inFlight.Create(&Notification{UserID: s.b.ID, ActorID: s.a.ID, Type: NotificationItemAdded, Message: "late"}).Error)

	done := make(chan error, 1)
	go func() { done <- PurgeUser(context.Background(), db, s.a.ID) }()

	select {
	case err := <-done:
		_ = inFlight.Rollback()
		t.Fatalf("the purge finished (%v) while a write to the user was still in flight; it must wait", err)
	case <-time.After(500 * time.Millisecond):
	}
	must(t, inFlight.Commit().Error)
	select {
	case err := <-done:
		must(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the purge never finished after the write committed")
	}
	assertPurged(t, db, s) // including the notification that arrived mid-purge
}

// A user with more lists than Postgres allows bind parameters in one statement
// (65,535) must still be deletable.
func TestPurgeUserWithVeryManyLists(t *testing.T) {
	db := postgresSchema(t)
	s := seedPurgeScenario(t, db)
	must(t, db.Exec(`INSERT INTO watchlists (id, owner_id, title, type, privacy, share_token, created_at, updated_at)
		SELECT gen_random_uuid(), ?, 'bulk', 'movie', 'private', md5(random()::text || g::text), now(), now()
		FROM generate_series(1, 70000) g`, s.a.ID).Error)

	must(t, PurgeUser(context.Background(), db, s.a.ID))
	if n := unscopedCount(t, db, &Watchlist{}, "owner_id = ?", s.a.ID); n != 0 {
		t.Errorf("%d lists remain", n)
	}
	assertPurged(t, db, s)
}
