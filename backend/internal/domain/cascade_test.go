package domain

import (
	"context"
	"errors"
	"testing"

	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func allModels() []any {
	return []any{
		&User{}, &OAuthAccount{}, &Movie{}, &Show{}, &Season{}, &Episode{}, &Genre{},
		&Review{}, &Watchlist{}, &WatchlistItem{}, &Collaborator{}, &UserWatchProgress{}, &Session{},
	}
}

func count(t *testing.T, db *gorm.DB, model any, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Model(model).Where(query, args...).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func seedShow(t *testing.T, db *gorm.DB) (Show, Season, Episode) {
	t.Helper()
	show := Show{Name: "S", TVDBID: 1}
	season := Season{SeasonNumber: 1}
	if err := db.Create(&show).Error; err != nil {
		t.Fatal(err)
	}
	season.ShowID = show.ID
	if err := db.Create(&season).Error; err != nil {
		t.Fatal(err)
	}
	ep := Episode{ShowID: show.ID, SeasonID: season.ID, Title: "E", EpisodeNumber: 1}
	if err := db.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	return show, season, ep
}

func TestDeleteSeasonCascade(t *testing.T) {
	db := testutil.NewDB(t, allModels()...)
	show, season, _ := seedShow(t, db)

	if err := DeleteSeasonCascade(context.Background(), db, season.ID); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, &Episode{}, "season_id = ?", season.ID); n != 0 {
		t.Errorf("episodes remain: %d", n)
	}
	if n := count(t, db, &Season{}, "id = ?", season.ID); n != 0 {
		t.Errorf("season remains: %d", n)
	}
	if n := count(t, db, &Show{}, "id = ?", show.ID); n != 1 {
		t.Errorf("show should be untouched, got %d", n)
	}
}

func TestDeleteShowCascade(t *testing.T) {
	db := testutil.NewDB(t, allModels()...)
	show, _, _ := seedShow(t, db)

	if err := DeleteShowCascade(context.Background(), db, show.ID); err != nil {
		t.Fatal(err)
	}
	for _, m := range []any{&Episode{}, &Season{}} {
		if n := count(t, db, m, "show_id = ?", show.ID); n != 0 {
			t.Errorf("%T rows remain: %d", m, n)
		}
	}
	if n := count(t, db, &Show{}, "id = ?", show.ID); n != 0 {
		t.Errorf("show remains: %d", n)
	}
}

func TestShowAndSeasonCascadeFailures(t *testing.T) {
	for _, table := range []string{"episodes", "seasons", "shows"} {
		t.Run("season/"+table, func(t *testing.T) {
			db := testutil.NewDB(t, allModels()...)
			_, season, _ := seedShow(t, db)
			testutil.FailOn(t, db, "delete", table)
			err := DeleteSeasonCascade(context.Background(), db, season.ID)
			if table == "shows" {
				if err != nil {
					t.Fatalf("season cascade never touches shows: %v", err)
				}
				return
			}
			if !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("expected injected error, got %v", err)
			}
			if n := count(t, db, &Episode{}, "season_id = ?", season.ID); n != 1 {
				t.Errorf("failed cascade must roll back; episodes = %d", n)
			}
		})
		t.Run("show/"+table, func(t *testing.T) {
			db := testutil.NewDB(t, allModels()...)
			show, _, _ := seedShow(t, db)
			testutil.FailOn(t, db, "delete", table)
			if err := DeleteShowCascade(context.Background(), db, show.ID); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("expected injected error, got %v", err)
			}
		})
	}
}

type userGraph struct {
	user, other User
	owned       Watchlist
	foreign     Watchlist
}

func seedUserGraph(t *testing.T, db *gorm.DB, owns bool) userGraph {
	t.Helper()
	g := userGraph{
		user:  User{Username: "u", Email: "u@x.com", DisplayName: "U"},
		other: User{Username: "o", Email: "o@x.com", DisplayName: "O"},
	}
	for _, u := range []*User{&g.user, &g.other} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	g.foreign = Watchlist{OwnerID: g.other.ID, Title: "theirs", Type: WatchlistTypeMovie}
	if err := db.Create(&g.foreign).Error; err != nil {
		t.Fatal(err)
	}
	mid := uint64(1)
	rows := []any{
		&Review{UserID: g.user.ID, Rating: 5, ReviewableID: 1},
		&Collaborator{UserID: g.user.ID, WatchlistID: g.foreign.ID},
		&UserWatchProgress{UserID: g.user.ID, ItemType: ItemTypeMovie, ItemID: 1, Status: StatusWatching},
	}
	if owns {
		g.owned = Watchlist{OwnerID: g.user.ID, Title: "mine", Type: WatchlistTypeMovie}
		if err := db.Create(&g.owned).Error; err != nil {
			t.Fatal(err)
		}
		rows = append(rows,
			&WatchlistItem{WatchlistID: g.owned.ID, ItemType: WatchlistTypeMovie, MovieID: &mid, AddedByID: g.user.ID},
			&Collaborator{UserID: g.other.ID, WatchlistID: g.owned.ID},
		)
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatal(err)
		}
	}
	return g
}

func TestDeleteWatchlistCascade(t *testing.T) {
	db := testutil.NewDB(t, allModels()...)
	g := seedUserGraph(t, db, true)

	if err := DeleteWatchlistCascade(context.Background(), db, g.owned.ID); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, &WatchlistItem{}, "watchlist_id = ?", g.owned.ID); n != 0 {
		t.Errorf("items remain: %d", n)
	}
	if n := count(t, db, &Collaborator{}, "watchlist_id = ?", g.owned.ID); n != 0 {
		t.Errorf("collaborators remain: %d", n)
	}
	if n := count(t, db, &Watchlist{}, "id = ?", g.owned.ID); n != 0 {
		t.Errorf("watchlist remains: %d", n)
	}
	if n := count(t, db, &Watchlist{}, "id = ?", g.foreign.ID); n != 1 {
		t.Errorf("other watchlist must be untouched, got %d", n)
	}
}

func TestDeleteWatchlistCascadeFailures(t *testing.T) {
	for _, table := range []string{"watchlist_items", "collaborators", "watchlists"} {
		t.Run(table, func(t *testing.T) {
			db := testutil.NewDB(t, allModels()...)
			g := seedUserGraph(t, db, true)
			testutil.FailOn(t, db, "delete", table)
			if err := DeleteWatchlistCascade(context.Background(), db, g.owned.ID); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("expected injected error, got %v", err)
			}
			if n := count(t, db, &WatchlistItem{}, "watchlist_id = ?", g.owned.ID); n != 1 {
				t.Errorf("failed cascade must roll back; items = %d", n)
			}
		})
	}
}

func TestDeleteUserCascade(t *testing.T) {
	db := testutil.NewDB(t, allModels()...)
	g := seedUserGraph(t, db, true)

	if err := DeleteUserCascade(context.Background(), db, g.user.ID); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name  string
		model any
		q     string
		arg   any
	}{
		{"reviews", &Review{}, "user_id = ?", g.user.ID},
		{"owned watchlists", &Watchlist{}, "owner_id = ?", g.user.ID},
		{"owned items", &WatchlistItem{}, "watchlist_id = ?", g.owned.ID},
		{"collaborations", &Collaborator{}, "user_id = ?", g.user.ID},
		{"collaborators on owned lists", &Collaborator{}, "watchlist_id = ?", g.owned.ID},
		{"progress", &UserWatchProgress{}, "user_id = ?", g.user.ID},
		{"user", &User{}, "id = ?", g.user.ID},
	}
	for _, c := range checks {
		if n := count(t, db, c.model, c.q, c.arg); n != 0 {
			t.Errorf("%s remain: %d", c.name, n)
		}
	}
	if n := count(t, db, &User{}, "id = ?", g.other.ID); n != 1 {
		t.Errorf("other user must survive, got %d", n)
	}
	if n := count(t, db, &Watchlist{}, "id = ?", g.foreign.ID); n != 1 {
		t.Errorf("other user's watchlist must survive, got %d", n)
	}
}

func TestDeleteUserCascadeFailures(t *testing.T) {
	cases := []struct {
		op, table string
		owns      bool
	}{
		{"delete", "reviews", true},
		{"query", "watchlists", true},
		{"delete", "watchlist_items", true},
		{"delete", "collaborators", false},
		{"delete", "user_watch_progresses", false},
		{"delete", "users", false},
	}
	for _, tc := range cases {
		t.Run(tc.op+"/"+tc.table, func(t *testing.T) {
			db := testutil.NewDB(t, allModels()...)
			g := seedUserGraph(t, db, tc.owns)
			testutil.FailOn(t, db, tc.op, tc.table)
			if err := DeleteUserCascade(context.Background(), db, g.user.ID); !errors.Is(err, testutil.ErrInjected) {
				t.Fatalf("expected injected error, got %v", err)
			}
			if n := count(t, db, &User{}, "id = ?", g.user.ID); n != 1 {
				t.Errorf("failed cascade must roll back; user rows = %d", n)
			}
		})
	}
}

func TestWatchlistShareTokenEntropyFailure(t *testing.T) {
	orig := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("no entropy") }
	t.Cleanup(func() { randRead = orig })

	if _, err := generateShareToken(); err == nil {
		t.Error("expected generateShareToken to fail")
	}
	db := testutil.NewDB(t, &Watchlist{})
	w := Watchlist{OwnerID: uuid.New(), Title: "t", Type: WatchlistTypeMovie}
	if err := db.Create(&w).Error; err == nil {
		t.Error("expected create to fail when share token cannot be generated")
	}
}
