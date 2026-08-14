// Package domain - tests for Watchlist, Collaborator, WatchlistItem
package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupWatchlistTestDB spins up an in-memory SQLite DB for Watchlist,
// Collaborator, and WatchlistItem tests. FK constraint creation is
// disabled — these models reference User (and WatchlistItem references
// Movie/Show), and we don't need real FK enforcement to test the fields
// and hooks declared on these three types.
func setupWatchlistTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(uniqueSQLiteDSN(t)), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}

	if err := db.AutoMigrate(&Watchlist{}, &Collaborator{}, &WatchlistItem{}); err != nil {
		t.Fatalf("failed to migrate Watchlist/Collaborator/WatchlistItem: %v", err)
	}

	return db
}

func ptrUint64(v uint64) *uint64 { return &v }

// ---- Constants --------------------------------------------------------

func TestPrivacyLevelConstants(t *testing.T) {
	cases := []struct {
		name string
		got  PrivacyLevel
		want string
	}{
		{"PrivacyPrivate", PrivacyPrivate, "private"},
		{"PrivacyShared", PrivacyShared, "shared"},
		{"PrivacyPublic", PrivacyPublic, "public"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("expected %s to be %q, got %q", tc.name, tc.want, string(tc.got))
			}
		})
	}
}

func TestCollaboratorRoleConstants(t *testing.T) {
	cases := []struct {
		name string
		got  CollaboratorRole
		want string
	}{
		{"RoleOwner", RoleOwner, "owner"},
		{"RoleEditor", RoleEditor, "editor"},
		{"RoleViewer", RoleViewer, "viewer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("expected %s to be %q, got %q", tc.name, tc.want, string(tc.got))
			}
		})
	}
}

func TestWatchlistTypeConstants(t *testing.T) {
	cases := []struct {
		name string
		got  WatchlistType
		want string
	}{
		{"WatchlistTypeMovie", WatchlistTypeMovie, "movie"},
		{"WatchlistTypeShow", WatchlistTypeShow, "show"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("expected %s to be %q, got %q", tc.name, tc.want, string(tc.got))
			}
		})
	}
}

// ---- Watchlist ----------------------------------------------------------

func TestWatchlistPrivacyDefaultsToPrivate(t *testing.T) {
	db := setupWatchlistTestDB(t)

	w := Watchlist{OwnerID: uuid.New(), Title: "My List", Type: WatchlistTypeMovie}
	if err := db.Create(&w).Error; err != nil {
		t.Fatalf("unexpected error creating watchlist: %v", err)
	}

	var fetched Watchlist
	if err := db.First(&fetched, "id = ?", w.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching watchlist: %v", err)
	}
	if fetched.Privacy != PrivacyPrivate {
		t.Errorf("expected Privacy to default to %q, got %q", PrivacyPrivate, fetched.Privacy)
	}
}

func TestWatchlistBeforeCreateGeneratesIDAndShareToken(t *testing.T) {
	db := setupWatchlistTestDB(t)

	w := Watchlist{OwnerID: uuid.New(), Title: "New List", Type: WatchlistTypeMovie}
	if err := db.Create(&w).Error; err != nil {
		t.Fatalf("unexpected error creating watchlist: %v", err)
	}

	if w.ID == uuid.Nil {
		t.Error("expected ID to be generated — Watchlist's own BeforeCreate must still call BaseUUID's")
	}
	if w.ShareToken == "" {
		t.Error("expected ShareToken to be auto-generated when left empty")
	}
}

func TestWatchlistBeforeCreatePreservesExplicitShareToken(t *testing.T) {
	db := setupWatchlistTestDB(t)

	w := Watchlist{OwnerID: uuid.New(), Title: "New List", Type: WatchlistTypeMovie, ShareToken: "explicit-token"}
	if err := db.Create(&w).Error; err != nil {
		t.Fatalf("unexpected error creating watchlist: %v", err)
	}
	if w.ShareToken != "explicit-token" {
		t.Errorf("expected explicit ShareToken to be preserved, got %q", w.ShareToken)
	}
}

func TestWatchlistShareTokensAreUniqueAcrossCreates(t *testing.T) {
	db := setupWatchlistTestDB(t)

	w1 := Watchlist{OwnerID: uuid.New(), Title: "List A", Type: WatchlistTypeMovie}
	w2 := Watchlist{OwnerID: uuid.New(), Title: "List B", Type: WatchlistTypeShow}
	if err := db.Create(&w1).Error; err != nil {
		t.Fatalf("unexpected error creating first watchlist: %v", err)
	}
	if err := db.Create(&w2).Error; err != nil {
		t.Fatalf("unexpected error creating second watchlist: %v", err)
	}

	if w1.ShareToken == "" || w2.ShareToken == "" {
		t.Fatal("expected both watchlists to have a generated ShareToken")
	}
	if w1.ShareToken == w2.ShareToken {
		t.Errorf("expected different share tokens, got %q for both", w1.ShareToken)
	}
}

func TestWatchlistDuplicateExplicitShareTokenRejected(t *testing.T) {
	db := setupWatchlistTestDB(t)

	w1 := Watchlist{OwnerID: uuid.New(), Title: "List A", Type: WatchlistTypeMovie, ShareToken: "same-token"}
	if err := db.Create(&w1).Error; err != nil {
		t.Fatalf("unexpected error creating first watchlist: %v", err)
	}

	w2 := Watchlist{OwnerID: uuid.New(), Title: "List B", Type: WatchlistTypeShow, ShareToken: "same-token"}
	if err := db.Create(&w2).Error; err == nil {
		t.Fatal("expected error creating watchlist with a duplicate explicit ShareToken, got nil")
	}
}

// ---- Collaborator ---------------------------------------------------------

func TestCollaboratorRoleDefaultsToEditor(t *testing.T) {
	db := setupWatchlistTestDB(t)

	c := Collaborator{UserID: uuid.New(), WatchlistID: uuid.New(), JoinedAt: time.Now().UTC()}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("unexpected error creating collaborator: %v", err)
	}

	var fetched Collaborator
	if err := db.First(&fetched, "id = ?", c.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching collaborator: %v", err)
	}
	if fetched.Role != RoleEditor {
		t.Errorf("expected Role to default to %q, got %q", RoleEditor, fetched.Role)
	}
}

func TestCollaboratorBeforeCreateGeneratesID(t *testing.T) {
	db := setupWatchlistTestDB(t)

	c := Collaborator{UserID: uuid.New(), WatchlistID: uuid.New(), Role: RoleViewer, JoinedAt: time.Now().UTC()}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("unexpected error creating collaborator: %v", err)
	}
	if c.ID == uuid.Nil {
		t.Error("expected ID to be generated by BaseUUID's BeforeCreate hook")
	}
}

// ---- WatchlistItem ----------------------------------------------------------

func TestWatchlistItemHasNoDefaultItemType(t *testing.T) {
	// Documents actual current behavior: unlike Watchlist.Privacy or
	// Collaborator.Role, ItemType has no `default:` tag, so an item
	// created without one set stores an empty string, not
	// WatchlistTypeMovie. If a default was intended, the model needs
	// `gorm:"type:varchar(20);not null;default:'movie';index"` added.
	db := setupWatchlistTestDB(t)

	item := WatchlistItem{WatchlistID: uuid.New(), MovieID: ptrUint64(1), AddedByID: uuid.New()}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("unexpected error creating watchlist item: %v", err)
	}

	var fetched WatchlistItem
	if err := db.First(&fetched, "id = ?", item.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching watchlist item: %v", err)
	}
	if fetched.ItemType != "" {
		t.Errorf("expected ItemType to be empty with no default set, got %q", fetched.ItemType)
	}
}

func TestWatchlistItemBeforeCreateGeneratesID(t *testing.T) {
	db := setupWatchlistTestDB(t)

	item := WatchlistItem{
		WatchlistID: uuid.New(),
		ItemType:    WatchlistTypeMovie,
		MovieID:     ptrUint64(1),
		AddedByID:   uuid.New(),
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("unexpected error creating watchlist item: %v", err)
	}
	if item.ID == uuid.Nil {
		t.Error("expected ID to be generated by BaseUUID's BeforeCreate hook")
	}
}

func TestWatchlistItemPositionDefaultsToZero(t *testing.T) {
	db := setupWatchlistTestDB(t)

	item := WatchlistItem{
		WatchlistID: uuid.New(),
		ItemType:    WatchlistTypeMovie,
		MovieID:     ptrUint64(1),
		AddedByID:   uuid.New(),
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("unexpected error creating watchlist item: %v", err)
	}

	var fetched WatchlistItem
	if err := db.First(&fetched, "id = ?", item.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching watchlist item: %v", err)
	}
	if fetched.Position != 0 {
		t.Errorf("expected Position to default to 0, got %d", fetched.Position)
	}
}

func TestWatchlistItemMovieAndShowArePointers(t *testing.T) {
	db := setupWatchlistTestDB(t)

	movieItem := WatchlistItem{
		WatchlistID: uuid.New(),
		ItemType:    WatchlistTypeMovie,
		MovieID:     ptrUint64(42),
		AddedByID:   uuid.New(),
	}
	if err := db.Create(&movieItem).Error; err != nil {
		t.Fatalf("unexpected error creating movie item: %v", err)
	}

	var fetched WatchlistItem
	if err := db.First(&fetched, "id = ?", movieItem.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching watchlist item: %v", err)
	}
	if fetched.MovieID == nil || *fetched.MovieID != 42 {
		t.Errorf("expected MovieID to be 42, got %v", fetched.MovieID)
	}
	if fetched.ShowID != nil {
		t.Errorf("expected ShowID to be nil for a movie item, got %v", *fetched.ShowID)
	}
}
