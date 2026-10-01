package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/aomarai/concession/internal/tmdb"
	"gorm.io/gorm"
)

func ageOut(t *testing.T, db *gorm.DB, model any) {
	t.Helper()
	if err := db.Model(model).Where("1 = 1").UpdateColumn("updated_at", time.Now().Add(-48*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
}

func TestActorsAreCapped(t *testing.T) {
	var c tmdb.Credits
	for i := 0; i < 25; i++ {
		c.Cast = append(c.Cast, struct {
			Name string `json:"name"`
		}{fmt.Sprint("actor", i)})
	}
	if got := actors(c); len(got) != maxActors || got[0] != "actor0" {
		t.Errorf("got %d actors: %v", len(got), got)
	}
}

func TestParseDateInvalid(t *testing.T) {
	if !parseDate("").IsZero() || !parseDate("garbage").IsZero() {
		t.Error("invalid dates should parse to the zero time")
	}
}

func TestUpsertGenresEmptyIsNoop(t *testing.T) {
	var hits atomic.Int32
	s := newTestService(t, &hits)
	if err := upsertGenres(s.DB, nil); err != nil {
		t.Fatal(err)
	}
}

func TestStaleTitlesFallBackWhenTMDBFails(t *testing.T) {
	var hits atomic.Int32
	status := &sync.Map{}
	s := newUpstreamService(t, &hits, status)
	ctx := context.Background()

	movie, _ := s.EnsureMovie(ctx, 603)
	show, _ := s.EnsureShow(ctx, 1396)
	ageOut(t, s.DB, &domain.Movie{})
	ageOut(t, s.DB, &domain.Show{})

	t.Run("upstream 500 serves stale copy", func(t *testing.T) {
		status.Store("/movie/603", http.StatusInternalServerError)
		status.Store("/tv/1396", http.StatusInternalServerError)
		m, err := s.EnsureMovie(ctx, 603)
		if err != nil || m.ID != movie.ID {
			t.Errorf("movie: %v, %v", m, err)
		}
		sh, err := s.EnsureShow(ctx, 1396)
		if err != nil || sh.ID != show.ID {
			t.Errorf("show: %v, %v", sh, err)
		}
	})

	t.Run("upstream 404 is surfaced", func(t *testing.T) {
		status.Store("/movie/603", http.StatusNotFound)
		status.Store("/tv/1396", http.StatusNotFound)
		if _, err := s.EnsureMovie(ctx, 603); !errors.Is(err, tmdb.ErrNotFound) {
			t.Errorf("movie: %v", err)
		}
		if _, err := s.EnsureShow(ctx, 1396); !errors.Is(err, tmdb.ErrNotFound) {
			t.Errorf("show: %v", err)
		}
	})
}

func TestUncachedUpstreamFailuresAreReturned(t *testing.T) {
	var hits atomic.Int32
	status := &sync.Map{}
	s := newUpstreamService(t, &hits, status)
	ctx := context.Background()

	status.Store("/movie/603", http.StatusInternalServerError)
	if _, err := s.EnsureMovie(ctx, 603); err == nil || errors.Is(err, tmdb.ErrNotFound) {
		t.Errorf("movie: %v", err)
	}
	status.Store("/tv/1396", http.StatusInternalServerError)
	if _, err := s.EnsureShow(ctx, 1396); err == nil {
		t.Error("show: expected error")
	}
	status.Delete("/tv/1396")
	status.Store("/tv/1396/season/1", http.StatusInternalServerError)
	if _, err := s.EnsureSeason(ctx, 1396, 1); err == nil {
		t.Error("season: expected error")
	}
	status.Store("/tv/1396", http.StatusNotFound)
	if _, err := s.EnsureSeason(ctx, 1396, 1); err != nil {
		// show already cached locally, so a 404 upstream is irrelevant here
		t.Logf("season after show 404: %v", err)
	}
	status.Store("/genre/movie/list", http.StatusInternalServerError)
	if err := s.SyncGenres(ctx); err == nil {
		t.Error("genres: expected error")
	}
	if _, err := s.Search(ctx, "x", 1); err == nil {
		t.Error("search against 404 fake should error")
	}
}

type dbFailure struct {
	name, op, table string
	after           int
}

func runDBFailures(t *testing.T, cases []dbFailure, call func(s *Service) error) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			s := newTestService(t, &hits)
			testutil.FailAfter(t, s.DB, tc.op, tc.table, tc.after)
			if err := call(s); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("expected injected error, got %v", err)
			}
		})
	}
}

func TestEnsureMovieDBFailures(t *testing.T) {
	runDBFailures(t, []dbFailure{
		{"lookup", "query", "movies", 0},
		{"genre upsert", "create", "genres", 0},
		{"movie save", "create", "movies", 0},
		{"genre association", "create", "movie_genres", 0},
	}, func(s *Service) error { _, err := s.EnsureMovie(context.Background(), 603); return err })
}

func TestEnsureShowDBFailures(t *testing.T) {
	runDBFailures(t, []dbFailure{
		{"lookup", "query", "shows", 0},
		{"genre upsert", "create", "genres", 0},
		{"show save", "create", "shows", 0},
		{"genre association", "create", "show_genres", 0},
		{"season lookup", "query", "seasons", 0},
		{"season save", "create", "seasons", 0},
		{"reload", "query", "shows", 1},
	}, func(s *Service) error { _, err := s.EnsureShow(context.Background(), 1396); return err })
}

func TestEnsureSeasonDBFailures(t *testing.T) {
	runDBFailures(t, []dbFailure{
		{"show ensure", "query", "shows", 0},
		{"show reload", "query", "seasons", 2},
		{"season upsert", "query", "seasons", 3},
		{"episode lookup", "query", "episodes", 0},
		{"episode save", "create", "episodes", 0},
		{"reload", "query", "episodes", 2},
	}, func(s *Service) error { _, err := s.EnsureSeason(context.Background(), 1396, 1); return err })
}

func TestSyncGenresDBFailure(t *testing.T) {
	runDBFailures(t, []dbFailure{{"genre upsert", "create", "genres", 0}},
		func(s *Service) error { return s.SyncGenres(context.Background()) })
}

func TestEnsureSeasonUnknownSeason(t *testing.T) {
	var hits atomic.Int32
	s := newTestService(t, &hits)
	if _, err := s.EnsureSeason(context.Background(), 1396, 99); !errors.Is(err, tmdb.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
