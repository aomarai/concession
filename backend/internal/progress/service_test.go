package progress

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ctx = context.Background()

func models() []any {
	return []any{&domain.Movie{}, &domain.Show{}, &domain.Genre{}, &domain.Review{}, &domain.UserWatchProgress{}}
}

type fakeCatalog struct {
	db  *gorm.DB
	err error
}

func (f *fakeCatalog) EnsureMovie(_ context.Context, id int64) (*domain.Movie, error) {
	if f.err != nil {
		return nil, f.err
	}
	m := domain.Movie{TMDBID: id, Title: fmt.Sprintf("Movie %d", id)}
	return &m, f.db.Where("tmdb_id = ?", id).FirstOrCreate(&m).Error
}

func (f *fakeCatalog) EnsureShow(_ context.Context, id int64) (*domain.Show, error) {
	if f.err != nil {
		return nil, f.err
	}
	s := domain.Show{TMDBID: &id, Name: fmt.Sprintf("Show %d", id)}
	return &s, f.db.Where("tmdb_id = ?", id).FirstOrCreate(&s).Error
}

func newSvc(t *testing.T) (*Service, *fakeCatalog) {
	t.Helper()
	db := testutil.NewDB(t, models()...)
	cat := &fakeCatalog{db: db}
	return NewService(db, cat), cat
}

func TestParseKind(t *testing.T) {
	if k, ok := ParseKind("movies"); !ok || k != domain.ItemTypeMovie {
		t.Errorf("movies: %v %v", k, ok)
	}
	if k, ok := ParseKind("shows"); !ok || k != domain.ItemTypeShow {
		t.Errorf("shows: %v %v", k, ok)
	}
	if _, ok := ParseKind("books"); ok {
		t.Error("books should be rejected")
	}
}

func TestSetMovieCreatesThenUpdates(t *testing.T) {
	s, _ := newSvc(t)
	user := uuid.New()

	v, err := s.Set(ctx, user, domain.ItemTypeMovie, 603, SetInput{Status: domain.StatusPlanToWatch})
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != domain.StatusPlanToWatch || v.Movie == nil || v.Movie.Title != "Movie 603" || v.Show != nil || v.WatchedAt.IsZero() {
		t.Errorf("unexpected view %+v", v)
	}

	v2, err := s.Set(ctx, user, domain.ItemTypeMovie, 603, SetInput{Status: domain.StatusCompleted})
	if err != nil {
		t.Fatal(err)
	}
	if v2.Status != domain.StatusCompleted || v2.ItemID != v.ItemID || v2.WatchedAt.Before(v.WatchedAt) {
		t.Errorf("update wrong: %+v", v2)
	}
	var n int64
	s.DB.Model(&domain.UserWatchProgress{}).Count(&n)
	if n != 1 {
		t.Errorf("update must not create a second row, got %d", n)
	}
}

func TestSetShowTracksSeasonAndEpisode(t *testing.T) {
	s, _ := newSvc(t)
	user := uuid.New()
	v, err := s.Set(ctx, user, domain.ItemTypeShow, 1396, SetInput{Status: domain.StatusWatching, Season: 2, Episode: 4})
	if err != nil {
		t.Fatal(err)
	}
	if v.Show == nil || v.Movie != nil || v.LastSeasonNum != 2 || v.LastEpisodeNum != 4 {
		t.Errorf("unexpected view %+v", v)
	}
	// Season alone is fine (started the season, no episode yet).
	if _, err := s.Set(ctx, user, domain.ItemTypeShow, 1396, SetInput{Status: domain.StatusWatching, Season: 3}); err != nil {
		t.Errorf("season only: %v", err)
	}
}

func TestSetValidation(t *testing.T) {
	s, _ := newSvc(t)
	user := uuid.New()
	cases := map[string]struct {
		kind domain.ItemType
		in   SetInput
	}{
		"bad status":             {domain.ItemTypeMovie, SetInput{Status: "binged"}},
		"empty status":           {domain.ItemTypeShow, SetInput{}},
		"movie with season":      {domain.ItemTypeMovie, SetInput{Status: domain.StatusWatching, Season: 1}},
		"movie with episode":     {domain.ItemTypeMovie, SetInput{Status: domain.StatusWatching, Episode: 1}},
		"episode without season": {domain.ItemTypeShow, SetInput{Status: domain.StatusWatching, Episode: 3}},
	}
	for name, tc := range cases {
		if _, err := s.Set(ctx, user, tc.kind, 1, tc.in); !errors.Is(err, svcerr.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	var n int64
	s.DB.Model(&domain.Movie{}).Count(&n)
	if n != 0 {
		t.Error("validation must run before any title is fetched")
	}
}

func TestSetFailures(t *testing.T) {
	user := uuid.New()
	ok := SetInput{Status: domain.StatusWatching}

	t.Run("catalog errors pass through", func(t *testing.T) {
		s, cat := newSvc(t)
		boom := errors.New("tmdb down")
		cat.err = boom
		if _, err := s.Set(ctx, user, domain.ItemTypeMovie, 1, ok); !errors.Is(err, boom) {
			t.Errorf("movie: %v", err)
		}
		if _, err := s.Set(ctx, user, domain.ItemTypeShow, 1, ok); !errors.Is(err, boom) {
			t.Errorf("show: %v", err)
		}
	})
	for _, tc := range []struct{ name, op string }{{"lookup", "query"}, {"save", "create"}} {
		t.Run("db "+tc.name, func(t *testing.T) {
			s, _ := newSvc(t)
			testutil.FailOn(t, s.DB, tc.op, "user_watch_progresses")
			if _, err := s.Set(ctx, user, domain.ItemTypeMovie, 1, ok); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestConcurrentSetsKeepOneRow(t *testing.T) {
	db := testutil.NewFileDB(t, models()...)
	cat := &fakeCatalog{db: db}
	s := NewService(db, cat)
	user := uuid.New()
	if _, err := cat.EnsureMovie(ctx, 7); err != nil {
		t.Fatal(err)
	}

	statuses := []domain.WatchStatus{domain.StatusPlanToWatch, domain.StatusWatching, domain.StatusCompleted, domain.StatusDropped}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Set(ctx, user, domain.ItemTypeMovie, 7, SetInput{Status: statuses[i%len(statuses)]})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent set failed: %v", err)
		}
	}
	var n int64
	db.Model(&domain.UserWatchProgress{}).Count(&n)
	if n != 1 {
		t.Errorf("expected one progress row, got %d", n)
	}
	if s.locks.Len() != 0 {
		t.Errorf("locks leaked: %d", s.locks.Len())
	}
}

func TestGetAndDelete(t *testing.T) {
	s, _ := newSvc(t)
	user, other := uuid.New(), uuid.New()
	if _, err := s.Set(ctx, user, domain.ItemTypeMovie, 5, SetInput{Status: domain.StatusCompleted}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Set(ctx, user, domain.ItemTypeShow, 6, SetInput{Status: domain.StatusWatching, Season: 1}); err != nil {
		t.Fatal(err)
	}

	if v, err := s.Get(ctx, user, domain.ItemTypeMovie, 5); err != nil || v.Status != domain.StatusCompleted || v.Movie == nil {
		t.Errorf("movie get: %+v, %v", v, err)
	}
	if v, err := s.Get(ctx, user, domain.ItemTypeShow, 6); err != nil || v.Show == nil || v.LastSeasonNum != 1 {
		t.Errorf("show get: %+v, %v", v, err)
	}
	// Not tracked by this user, unknown title, and tracked-by-someone-else are all not found.
	if _, err := s.Get(ctx, other, domain.ItemTypeMovie, 5); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("other user: %v", err)
	}
	if _, err := s.Get(ctx, user, domain.ItemTypeMovie, 999); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("unknown title: %v", err)
	}
	if _, err := s.Get(ctx, user, domain.ItemTypeShow, 999); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("unknown show: %v", err)
	}

	if err := s.Delete(ctx, other, domain.ItemTypeMovie, 5); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("other user's delete: %v", err)
	}
	if err := s.Delete(ctx, user, domain.ItemTypeMovie, 5); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, user, domain.ItemTypeMovie, 5); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
	if err := s.Delete(ctx, user, domain.ItemTypeShow, 999); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("delete unknown title: %v", err)
	}
}

func TestGetAndDeleteFailures(t *testing.T) {
	user := uuid.New()
	for _, tc := range []struct {
		name, op, table string
		after           int
		call            func(s *Service) error
	}{
		{"get title lookup", "query", "movies", 0, func(s *Service) error { _, err := s.Get(ctx, user, domain.ItemTypeMovie, 1); return err }},
		{"get progress lookup", "query", "user_watch_progresses", 0, func(s *Service) error { _, err := s.Get(ctx, user, domain.ItemTypeMovie, 1); return err }},
		{"get attach", "query", "movies", 1, func(s *Service) error { _, err := s.Get(ctx, user, domain.ItemTypeMovie, 1); return err }},
		{"delete title lookup", "query", "movies", 0, func(s *Service) error { return s.Delete(ctx, user, domain.ItemTypeMovie, 1) }},
		{"delete", "delete", "user_watch_progresses", 0, func(s *Service) error { return s.Delete(ctx, user, domain.ItemTypeMovie, 1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newSvc(t)
			if _, err := s.Set(ctx, user, domain.ItemTypeMovie, 1, SetInput{Status: domain.StatusWatching}); err != nil {
				t.Fatal(err)
			}
			testutil.FailAfter(t, s.DB, tc.op, tc.table, tc.after)
			if err := tc.call(s); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestList(t *testing.T) {
	s, _ := newSvc(t)
	user, other := uuid.New(), uuid.New()
	for _, step := range []struct {
		kind   domain.ItemType
		id     int64
		status domain.WatchStatus
	}{
		{domain.ItemTypeMovie, 1, domain.StatusCompleted},
		{domain.ItemTypeShow, 2, domain.StatusWatching},
		{domain.ItemTypeMovie, 3, domain.StatusPlanToWatch},
	} {
		if _, err := s.Set(ctx, user, step.kind, step.id, SetInput{Status: step.status}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // distinct watched_at for ordering
	}
	if _, err := s.Set(ctx, other, domain.ItemTypeMovie, 1, SetInput{Status: domain.StatusDropped}); err != nil {
		t.Fatal(err)
	}

	all, err := s.List(ctx, user, Filter{})
	if err != nil || len(all) != 3 {
		t.Fatalf("all: %v, %v", all, err)
	}
	if all[0].Movie == nil || all[0].Movie.TMDBID != 3 || all[1].Show == nil || all[2].Movie.TMDBID != 1 {
		t.Errorf("expected most recent first with titles attached: %+v", all)
	}

	movies, _ := s.List(ctx, user, Filter{Kind: domain.ItemTypeMovie})
	if len(movies) != 2 {
		t.Errorf("movies: %d", len(movies))
	}
	watching, _ := s.List(ctx, user, Filter{Status: domain.StatusWatching})
	if len(watching) != 1 || watching[0].Show == nil {
		t.Errorf("watching: %+v", watching)
	}
	none, err := s.List(ctx, user, Filter{Kind: domain.ItemTypeShow, Status: domain.StatusDropped})
	if err != nil || none == nil || len(none) != 0 {
		t.Errorf("empty result should be an empty non-nil slice: %v, %v", none, err)
	}

	if _, err := s.List(ctx, user, Filter{Status: "binged"}); !errors.Is(err, svcerr.ErrInvalid) {
		t.Errorf("bad status filter: %v", err)
	}
}

func TestListFailures(t *testing.T) {
	user := uuid.New()
	for _, table := range []string{"user_watch_progresses", "movies", "shows"} {
		t.Run(table, func(t *testing.T) {
			s, _ := newSvc(t)
			s.Set(ctx, user, domain.ItemTypeMovie, 1, SetInput{Status: domain.StatusWatching})
			s.Set(ctx, user, domain.ItemTypeShow, 2, SetInput{Status: domain.StatusWatching})
			testutil.FailOn(t, s.DB, "query", table)
			if _, err := s.List(ctx, user, Filter{}); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}
