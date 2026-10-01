package domain

import (
	"context"
	"errors"
	"testing"

	"github.com/aomarai/concession/internal/testutil"
	"gorm.io/gorm"
)

func TestStoredTitleID(t *testing.T) {
	db := testutil.NewDB(t, &Movie{}, &Show{}, &Genre{}, &Review{})
	m := Movie{TMDBID: 603, Title: "The Matrix"}
	tmdb, tvdb := int64(1396), int64(81189)
	s := Show{Name: "Breaking Bad", TMDBID: &tmdb, TVDBID: &tvdb}
	if err := db.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if id, err := StoredTitleID(ctx, db, ItemTypeMovie, 603); err != nil || id != m.ID {
		t.Errorf("movie: %d, %v", id, err)
	}
	if id, err := StoredTitleID(ctx, db, ItemTypeShow, 1396); err != nil || id != s.ID {
		t.Errorf("show: %d, %v", id, err)
	}
	// A show's TMDB ID is not a movie's, and unknown IDs are not found.
	if _, err := StoredTitleID(ctx, db, ItemTypeMovie, 1396); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("cross-kind lookup: %v", err)
	}
	if _, err := StoredTitleID(ctx, db, ItemTypeShow, 999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("unknown show: %v", err)
	}
}

func TestLoadTitles(t *testing.T) {
	db := testutil.NewDB(t, &Movie{}, &Show{}, &Genre{}, &Review{})
	m1, m2 := Movie{TMDBID: 1, Title: "A"}, Movie{TMDBID: 2, Title: "B"}
	tmdb, tvdb := int64(9), int64(90)
	s := Show{Name: "S", TMDBID: &tmdb, TVDBID: &tvdb}
	for _, v := range []any{&m1, &m2, &s} {
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()

	movies, shows, err := LoadTitles(ctx, db, []uint64{m1.ID, m2.ID}, []uint64{s.ID})
	if err != nil || len(movies) != 2 || len(shows) != 1 || movies[m2.ID].Title != "B" || shows[s.ID].Name != "S" {
		t.Errorf("got %v %v %v", movies, shows, err)
	}
	movies, shows, err = LoadTitles(ctx, db, nil, nil)
	if err != nil || len(movies) != 0 || len(shows) != 0 {
		t.Errorf("empty input: %v %v %v", movies, shows, err)
	}

	for _, table := range []string{"movies", "shows"} {
		failing := testutil.NewDB(t, &Movie{}, &Show{}, &Genre{}, &Review{})
		testutil.FailOn(t, failing, "query", table)
		if _, _, err := LoadTitles(ctx, failing, []uint64{1}, []uint64{1}); !errors.Is(err, testutil.ErrInjected) {
			t.Errorf("%s: %v", table, err)
		}
	}
}
