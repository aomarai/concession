package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupUserTestDB spins up an in-memory SQLite DB for User-related tests.
// FK constraint creation is disabled — these tests don't need real FK
// enforcement, and cascade-delete behavior is tested separately in
// user_cascade_test.go with FK constraints turned on.
//
// Note: AutoMigrate does NOT transitively create tables for a model's
// has-many associations (e.g. passing only &User{} would not create the
// `reviews`, `watchlists`, or `collaborators` tables, even though User
// declares those as related fields) — each type that needs a table has to
// be listed explicitly here.
func setupUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(uniqueSQLiteDSN(t)), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}

	if err := db.AutoMigrate(&User{}, &UserWatchProgress{}, &Review{}, &Watchlist{}, &Collaborator{}); err != nil {
		t.Fatalf("failed to migrate User and its related models: %v", err)
	}

	return db
}

func newTestUser(suffix string) User {
	return User{
		Username:     "user_" + suffix,
		Email:        suffix + "@example.com",
		PasswordHash: "irrelevant-hash",
		DisplayName:  "Test User " + suffix,
	}
}

func TestUserRoleConstants(t *testing.T) {
	cases := []struct {
		name string
		got  UserRole
		want string
	}{
		{"RoleUser", RoleUser, "user"},
		{"RoleAdmin", RoleAdmin, "admin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("expected %s to be %q, got %q", tc.name, tc.want, string(tc.got))
			}
		})
	}
}

func TestWatchStatusConstants(t *testing.T) {
	cases := []struct {
		name string
		got  WatchStatus
		want string
	}{
		{"StatusPlanToWatch", StatusPlanToWatch, "plan_to_watch"},
		{"StatusWatching", StatusWatching, "watching"},
		{"StatusCompleted", StatusCompleted, "completed"},
		{"StatusDropped", StatusDropped, "dropped"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("expected %s to be %q, got %q", tc.name, tc.want, string(tc.got))
			}
		})
	}
}

func TestItemTypeConstants(t *testing.T) {
	cases := []struct {
		name string
		got  ItemType
		want string
	}{
		{"ItemTypeMovie", ItemTypeMovie, "movie"},
		{"ItemTypeShow", ItemTypeShow, "show"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("expected %s to be %q, got %q", tc.name, tc.want, string(tc.got))
			}
		})
	}
}

func TestUserRoleDefaultsToUser(t *testing.T) {
	db := setupUserTestDB(t)

	u := newTestUser("role-default")
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("unexpected error creating user: %v", err)
	}

	var fetched User
	if err := db.First(&fetched, "id = ?", u.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching user: %v", err)
	}
	if fetched.Role != RoleUser {
		t.Errorf("expected Role to default to %q, got %q", RoleUser, fetched.Role)
	}
}

func TestUserIsEmailVerifiedDefaultsFalse(t *testing.T) {
	db := setupUserTestDB(t)

	u := newTestUser("verify-default")
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("unexpected error creating user: %v", err)
	}

	var fetched User
	if err := db.First(&fetched, "id = ?", u.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching user: %v", err)
	}
	if fetched.IsEmailVerified {
		t.Error("expected IsEmailVerified to default to false")
	}
}

func TestUserUniqueUsername(t *testing.T) {
	db := setupUserTestDB(t)

	first := newTestUser("dupe-username")
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("unexpected error creating first user: %v", err)
	}

	second := newTestUser("dupe-username")
	second.Email = "different-email@example.com" // keep email distinct so only username collides
	if err := db.Create(&second).Error; err == nil {
		t.Fatal("expected error creating user with duplicate username, got nil")
	}
}

func TestUserUniqueEmail(t *testing.T) {
	db := setupUserTestDB(t)

	first := newTestUser("dupe-email")
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("unexpected error creating first user: %v", err)
	}

	second := newTestUser("different-username")
	second.Email = first.Email // force the collision
	if err := db.Create(&second).Error; err == nil {
		t.Fatal("expected error creating user with duplicate email, got nil")
	}
}

func TestUserPasswordHashExcludedFromJSON(t *testing.T) {
	u := newTestUser("json-check")
	u.PasswordHash = "super-secret-password-hash"

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("unexpected error marshaling user: %v", err)
	}

	out := string(data)
	if strings.Contains(out, "super-secret-password-hash") {
		t.Error("PasswordHash value leaked into JSON output despite json:\"-\" tag")
	}
	if strings.Contains(out, "PasswordHash") || strings.Contains(out, "password_hash") {
		t.Error("PasswordHash field name leaked into JSON output")
	}
}

func TestUserLastLoginAtOmitEmpty(t *testing.T) {
	t.Run("omitted when nil", func(t *testing.T) {
		u := newTestUser("last-login-nil")
		data, err := json.Marshal(u)
		if err != nil {
			t.Fatalf("unexpected error marshaling user: %v", err)
		}
		if strings.Contains(string(data), "last_login_at") {
			t.Error("expected last_login_at to be omitted when LastLoginAt is nil")
		}
	})

	t.Run("present when set", func(t *testing.T) {
		u := newTestUser("last-login-set")
		now := time.Now()
		u.LastLoginAt = &now
		data, err := json.Marshal(u)
		if err != nil {
			t.Fatalf("unexpected error marshaling user: %v", err)
		}
		if !strings.Contains(string(data), "last_login_at") {
			t.Error("expected last_login_at to be present when LastLoginAt is set")
		}
	})
}

func TestUserWatchProgressUniqueUserItem(t *testing.T) {
	db := setupUserTestDB(t)
	userID := uuid.New()

	first := UserWatchProgress{
		UserID:   userID,
		ItemType: ItemTypeShow,
		ItemID:   1,
		Status:   StatusWatching,
	}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("unexpected error creating first watch progress: %v", err)
	}

	t.Run("rejects duplicate (user_id, item_type, item_id)", func(t *testing.T) {
		dup := UserWatchProgress{UserID: userID, ItemType: ItemTypeShow, ItemID: 1, Status: StatusCompleted}
		if err := db.Create(&dup).Error; err == nil {
			t.Fatal("expected error creating duplicate watch progress entry, got nil")
		}
	})

	t.Run("allows same item with a different item_type", func(t *testing.T) {
		// Same numeric ItemID but a movie instead of a show — a real
		// scenario since Movie/Show IDs aren't unique across each other.
		other := UserWatchProgress{UserID: userID, ItemType: ItemTypeMovie, ItemID: 1, Status: StatusPlanToWatch}
		if err := db.Create(&other).Error; err != nil {
			t.Fatalf("unexpected error creating watch progress with different item_type: %v", err)
		}
	})

	t.Run("allows same item_type with a different item_id", func(t *testing.T) {
		other := UserWatchProgress{UserID: userID, ItemType: ItemTypeShow, ItemID: 2, Status: StatusDropped}
		if err := db.Create(&other).Error; err != nil {
			t.Fatalf("unexpected error creating watch progress with different item_id: %v", err)
		}
	})

	t.Run("allows the same item for a different user", func(t *testing.T) {
		other := UserWatchProgress{UserID: uuid.New(), ItemType: ItemTypeShow, ItemID: 1, Status: StatusWatching}
		if err := db.Create(&other).Error; err != nil {
			t.Fatalf("unexpected error creating watch progress for different user: %v", err)
		}
	})
}

func TestUserWatchProgressFieldsRoundTrip(t *testing.T) {
	db := setupUserTestDB(t)

	progress := UserWatchProgress{
		UserID:         uuid.New(),
		ItemType:       ItemTypeShow,
		ItemID:         42,
		Status:         StatusWatching,
		LastSeasonNum:  2,
		LastEpisodeNum: 4,
		WatchedAt:      time.Date(2026, 1, 15, 20, 0, 0, 0, time.UTC),
	}
	if err := db.Create(&progress).Error; err != nil {
		t.Fatalf("unexpected error creating watch progress: %v", err)
	}

	var fetched UserWatchProgress
	if err := db.First(&fetched, "id = ?", progress.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching watch progress: %v", err)
	}
	if fetched.LastSeasonNum != 2 || fetched.LastEpisodeNum != 4 {
		t.Errorf("expected season 2 episode 4, got season %d episode %d", fetched.LastSeasonNum, fetched.LastEpisodeNum)
	}
	if !fetched.WatchedAt.Equal(progress.WatchedAt) {
		t.Errorf("expected WatchedAt %v, got %v", progress.WatchedAt, fetched.WatchedAt)
	}
}

// Note: there's deliberately no test here calling plain db.Delete(&user)
// and expecting Reviews/Watchlists/etc. to cascade. Since User soft-deletes
// (BaseUUID's DeletedAt), a plain Delete is an UPDATE, not a DELETE, so the
// `constraint:OnDelete:CASCADE` tags on User's relationships never actually
// fire — same situation as Season. The correct way to delete a user is
// DeleteUserCascade (user_cascade.go), which is fully tested in
// user_cascade_test.go.
