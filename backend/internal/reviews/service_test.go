package reviews

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/paging"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ctx = context.Background()

func models() []any {
	return []any{&domain.User{}, &domain.Movie{}, &domain.Show{}, &domain.Genre{}, &domain.Review{}}
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
	s := domain.Show{TMDBID: &id, TVDBID: &id, Name: fmt.Sprintf("Show %d", id)}
	return &s, f.db.Where("tmdb_id = ?", id).FirstOrCreate(&s).Error
}

type env struct {
	svc *Service
	db  *gorm.DB
	cat *fakeCatalog
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testutil.NewDB(t, models()...)
	cat := &fakeCatalog{db: db}
	return &env{svc: NewService(db, cat), db: db, cat: cat}
}

func (e *env) user(t *testing.T, name string) uuid.UUID {
	t.Helper()
	u := domain.User{Username: name, Email: name + "@example.com", DisplayName: strings.ToUpper(name[:1]) + name[1:], AvatarURL: "http://pic/" + name}
	if err := e.db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func (e *env) review(t *testing.T, user uuid.UUID, kind domain.ReviewableItem, tmdb int64, rating int) *View {
	t.Helper()
	v, err := e.svc.Create(ctx, user, kind, tmdb, Input{Rating: rating, Title: "T", Content: "C"})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCreate(t *testing.T) {
	e := newEnv(t)
	ann := e.user(t, "ann")

	v, err := e.svc.Create(ctx, ann, domain.ReviewableMovies, 603, Input{Rating: 9, Title: "  Great ", Content: " Loved it. "})
	if err != nil {
		t.Fatal(err)
	}
	if v.Rating != 9 || v.Title != "Great" || v.Content != "Loved it." || v.ItemType != domain.ReviewableMovies ||
		v.Author.ID != ann || v.Author.DisplayName != "Ann" || v.Author.AvatarURL != "http://pic/ann" || v.ID == uuid.Nil {
		t.Errorf("unexpected view %+v", v)
	}

	t.Run("rating only", func(t *testing.T) {
		if _, err := e.svc.Create(ctx, ann, domain.ReviewableShows, 1396, Input{Rating: 10}); err != nil {
			t.Errorf("a rating without text should be fine: %v", err)
		}
	})
	t.Run("one review per title and user", func(t *testing.T) {
		if _, err := e.svc.Create(ctx, ann, domain.ReviewableMovies, 603, Input{Rating: 5}); !errors.Is(err, svcerr.ErrDuplicate) {
			t.Errorf("got %v", err)
		}
		bob := e.user(t, "bob")
		if _, err := e.svc.Create(ctx, bob, domain.ReviewableMovies, 603, Input{Rating: 5}); err != nil {
			t.Errorf("another user may review it: %v", err)
		}
		// A show with the same TMDB ID as a movie is a different title.
		if _, err := e.svc.Create(ctx, ann, domain.ReviewableShows, 603, Input{Rating: 5}); err != nil {
			t.Errorf("a show is not the movie with the same TMDB ID: %v", err)
		}
	})
}

func TestCreateValidation(t *testing.T) {
	e := newEnv(t)
	u := e.user(t, "ann")
	for name, in := range map[string]Input{
		"rating 0":     {Rating: 0},
		"rating 11":    {Rating: 11},
		"rating -1":    {Rating: -1},
		"long title":   {Rating: 5, Title: strings.Repeat("x", 201)},
		"long content": {Rating: 5, Content: strings.Repeat("x", 10001)},
	} {
		if _, err := e.svc.Create(ctx, u, domain.ReviewableMovies, 1, in); !errors.Is(err, svcerr.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	var n int64
	e.db.Model(&domain.Movie{}).Count(&n)
	if n != 0 {
		t.Error("validation must run before the title is fetched")
	}
	for _, r := range []int{MinRating, MaxRating} {
		if _, err := e.svc.Create(ctx, u, domain.ReviewableMovies, int64(r), Input{Rating: r}); err != nil {
			t.Errorf("boundary rating %d: %v", r, err)
		}
	}
}

func TestCreateFailures(t *testing.T) {
	t.Run("catalog errors pass through", func(t *testing.T) {
		e := newEnv(t)
		boom := errors.New("tmdb down")
		e.cat.err = boom
		for _, kind := range []domain.ReviewableItem{domain.ReviewableMovies, domain.ReviewableShows} {
			if _, err := e.svc.Create(ctx, uuid.New(), kind, 1, Input{Rating: 5}); !errors.Is(err, boom) {
				t.Errorf("%s: %v", kind, err)
			}
		}
	})
	for _, tc := range []struct{ name, op, table string }{
		{"duplicate check", "query", "reviews"},
		{"insert", "create", "reviews"},
		{"author lookup", "query", "users"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			testutil.FailOn(t, e.db, tc.op, tc.table)
			if _, err := e.svc.Create(ctx, e.user(t, "ann"), domain.ReviewableMovies, 1, Input{Rating: 5}); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestConcurrentCreatesOfTheSameReview(t *testing.T) {
	db := testutil.NewFileDB(t, models()...)
	cat := &fakeCatalog{db: db}
	s := NewService(db, cat)
	u := domain.User{Username: "u", Email: "u@x.com", DisplayName: "U"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := cat.EnsureMovie(ctx, 5); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Create(ctx, u.ID, domain.ReviewableMovies, 5, Input{Rating: 7})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var ok, dup int
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, svcerr.ErrDuplicate):
			dup++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || dup != 11 {
		t.Errorf("expected 1 success and 11 duplicates, got %d and %d", ok, dup)
	}
	if s.locks.Len() != 0 {
		t.Errorf("locks leaked: %d", s.locks.Len())
	}
}

func TestGet(t *testing.T) {
	e := newEnv(t)
	ann := e.user(t, "ann")
	created := e.review(t, ann, domain.ReviewableMovies, 603, 8)
	show := e.review(t, ann, domain.ReviewableShows, 1396, 6)

	v, err := e.svc.Get(ctx, created.ID)
	if err != nil || v.Movie == nil || v.Movie.Title != "Movie 603" || v.Show != nil || v.Author.DisplayName != "Ann" {
		t.Errorf("movie review: %+v, %v", v, err)
	}
	v, err = e.svc.Get(ctx, show.ID)
	if err != nil || v.Show == nil || v.Movie != nil {
		t.Errorf("show review: %+v, %v", v, err)
	}
	if _, err := e.svc.Get(ctx, uuid.New()); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
	for _, table := range []string{"reviews", "users", "movies"} {
		e2 := newEnv(t)
		r := e2.review(t, e2.user(t, "ann"), domain.ReviewableMovies, 1, 5)
		testutil.FailOn(t, e2.db, "query", table)
		if _, err := e2.svc.Get(ctx, r.ID); !errors.Is(err, testutil.ErrInjected) {
			t.Errorf("%s: %v", table, err)
		}
	}
}

func TestUpdate(t *testing.T) {
	e := newEnv(t)
	ann, bob := e.user(t, "ann"), e.user(t, "bob")
	r := e.review(t, ann, domain.ReviewableMovies, 1, 5)
	rating, title, content := 9, "  New title ", "  New text "

	v, err := e.svc.Update(ctx, ann, r.ID, UpdateInput{Rating: &rating, Title: &title, Content: &content})
	if err != nil || v.Rating != 9 || v.Title != "New title" || v.Content != "New text" || v.Author.ID != ann {
		t.Fatalf("got %+v, %v", v, err)
	}
	// Omitted fields are left alone.
	only := 3
	v, _ = e.svc.Update(ctx, ann, r.ID, UpdateInput{Rating: &only})
	if v.Rating != 3 || v.Title != "New title" || v.Content != "New text" {
		t.Errorf("partial update changed other fields: %+v", v)
	}
	if v, err := e.svc.Update(ctx, ann, r.ID, UpdateInput{}); err != nil || v.Rating != 3 {
		t.Errorf("empty update: %+v, %v", v, err)
	}

	if _, err := e.svc.Update(ctx, bob, r.ID, UpdateInput{Rating: &rating}); !errors.Is(err, svcerr.ErrForbidden) {
		t.Errorf("other user: %v", err)
	}
	if _, err := e.svc.Update(ctx, ann, uuid.New(), UpdateInput{}); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
	bad, long := 11, strings.Repeat("x", 10001)
	if _, err := e.svc.Update(ctx, ann, r.ID, UpdateInput{Rating: &bad}); !errors.Is(err, svcerr.ErrInvalid) {
		t.Errorf("bad rating: %v", err)
	}
	if _, err := e.svc.Update(ctx, ann, r.ID, UpdateInput{Content: &long}); !errors.Is(err, svcerr.ErrInvalid) {
		t.Errorf("long content: %v", err)
	}
	// A rejected update must not have changed anything.
	if v, _ := e.svc.Get(ctx, r.ID); v.Rating != 3 {
		t.Errorf("rejected update leaked: %+v", v)
	}
}

func TestUpdateAndDeleteFailures(t *testing.T) {
	for _, tc := range []struct {
		name, op, table string
		call            func(e *env, u, id uuid.UUID) error
	}{
		{"update lookup", "query", "reviews", func(e *env, u, id uuid.UUID) error {
			r := 4
			_, err := e.svc.Update(ctx, u, id, UpdateInput{Rating: &r})
			return err
		}},
		{"update save", "update", "reviews", func(e *env, u, id uuid.UUID) error {
			r := 4
			_, err := e.svc.Update(ctx, u, id, UpdateInput{Rating: &r})
			return err
		}},
		{"update author lookup", "query", "users", func(e *env, u, id uuid.UUID) error {
			r := 4
			_, err := e.svc.Update(ctx, u, id, UpdateInput{Rating: &r})
			return err
		}},
		{"delete lookup", "query", "reviews", func(e *env, u, id uuid.UUID) error { return e.svc.Delete(ctx, u, id) }},
		{"delete", "delete", "reviews", func(e *env, u, id uuid.UUID) error { return e.svc.Delete(ctx, u, id) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			u := e.user(t, "ann")
			r := e.review(t, u, domain.ReviewableMovies, 1, 5)
			testutil.FailOn(t, e.db, tc.op, tc.table)
			if err := tc.call(e, u, r.ID); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	e := newEnv(t)
	ann, bob := e.user(t, "ann"), e.user(t, "bob")
	r := e.review(t, ann, domain.ReviewableMovies, 1, 5)

	if err := e.svc.Delete(ctx, bob, r.ID); !errors.Is(err, svcerr.ErrForbidden) {
		t.Errorf("other user: %v", err)
	}
	if err := e.svc.Delete(ctx, ann, uuid.New()); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
	if err := e.svc.Delete(ctx, ann, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Get(ctx, r.ID); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("still visible: %v", err)
	}
	// Hard delete: the unique (user, title) index must allow reviewing again.
	if _, err := e.svc.Create(ctx, ann, domain.ReviewableMovies, 1, Input{Rating: 7}); err != nil {
		t.Errorf("re-reviewing after delete: %v", err)
	}
}

func TestListForTitle(t *testing.T) {
	e := newEnv(t)
	var users []uuid.UUID
	for _, n := range []string{"ann", "bob", "cy"} {
		users = append(users, e.user(t, n))
	}
	for i, u := range users {
		e.review(t, u, domain.ReviewableMovies, 603, []int{10, 7, 6}[i]) // average 7.666 -> 7.7
		time.Sleep(2 * time.Millisecond)
	}
	e.review(t, users[0], domain.ReviewableShows, 603, 1) // same TMDB ID, different title

	p, err := e.svc.ListForTitle(ctx, domain.ReviewableMovies, 603, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 3 || p.Summary.Count != 3 || p.Summary.Average != 7.7 || p.Page != 1 || p.PerPage != DefaultPerPage || len(p.Reviews) != 3 {
		t.Fatalf("unexpected page %+v %+v", p, p.Summary)
	}
	if p.Reviews[0].Author.DisplayName != "Cy" || p.Reviews[2].Author.DisplayName != "Ann" || p.Reviews[0].Movie != nil {
		t.Errorf("expected newest first with authors and no embedded title: %+v", p.Reviews)
	}

	p, _ = e.svc.ListForTitle(ctx, domain.ReviewableMovies, 603, 2, 2)
	if len(p.Reviews) != 1 || p.Reviews[0].Author.DisplayName != "Ann" || p.Total != 3 {
		t.Errorf("page 2: %+v", p)
	}
	p, _ = e.svc.ListForTitle(ctx, domain.ReviewableMovies, 603, 9, 2)
	if len(p.Reviews) != 0 || p.Reviews == nil || p.Total != 3 {
		t.Errorf("past the end should be an empty, non-nil page: %+v", p)
	}
	p, _ = e.svc.ListForTitle(ctx, domain.ReviewableShows, 603, 1, 10)
	if p.Summary.Count != 1 || p.Summary.Average != 1 {
		t.Errorf("show summary: %+v", p.Summary)
	}

	// Never-stored titles have no reviews and cost no TMDB call.
	p, err = e.svc.ListForTitle(ctx, domain.ReviewableMovies, 999999, 1, 10)
	if err != nil || p.Total != 0 || p.Summary.Count != 0 || p.Summary.Average != 0 || len(p.Reviews) != 0 || p.Reviews == nil {
		t.Errorf("unknown title: %+v, %v", p, err)
	}
}

func TestListForTitleNoReviewsYet(t *testing.T) {
	e := newEnv(t)
	if _, err := e.cat.EnsureMovie(ctx, 7); err != nil {
		t.Fatal(err)
	}
	p, err := e.svc.ListForTitle(ctx, domain.ReviewableMovies, 7, 1, 10)
	if err != nil || p.Summary.Count != 0 || p.Summary.Average != 0 || p.Total != 0 {
		t.Errorf("stored title without reviews: %+v, %v", p, err)
	}
}

func TestPagination(t *testing.T) {
	e := newEnv(t)
	for _, tc := range []struct{ page, perPage int }{{-1, 5}, {1, -5}} {
		if _, err := e.svc.ListForTitle(ctx, domain.ReviewableMovies, 1, tc.page, tc.perPage); !errors.Is(err, svcerr.ErrInvalid) {
			t.Errorf("%+v: %v", tc, err)
		}
		if _, err := e.svc.ListMine(ctx, uuid.New(), tc.page, tc.perPage); !errors.Is(err, svcerr.ErrInvalid) {
			t.Errorf("mine %+v: %v", tc, err)
		}
	}
	if _, err := e.svc.ListForTitle(ctx, domain.ReviewableMovies, 1, MaxPage+1, 10); !errors.Is(err, svcerr.ErrInvalid) {
		t.Errorf("a page that would overflow the offset must be rejected: %v", err)
	}
	if _, err := e.svc.ListMine(ctx, uuid.New(), int(^uint(0)>>1), 100); !errors.Is(err, svcerr.ErrInvalid) {
		t.Errorf("max int page: %v", err)
	}
	if _, perPage, err := paging.Normalize(1, 5000); err != nil || perPage != MaxPerPage {
		t.Errorf("per_page should be clamped to %d, got %d, %v", MaxPerPage, perPage, err)
	}
}

func TestListForTitleFailures(t *testing.T) {
	for _, tc := range []struct{ name, op, table string }{
		{"title lookup", "query", "movies"},
		{"summary", "row", "reviews"},
		{"page query", "query", "reviews"},
		{"author lookup", "query", "users"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.review(t, e.user(t, "ann"), domain.ReviewableMovies, 1, 5)
			testutil.FailOn(t, e.db, tc.op, tc.table)
			if _, err := e.svc.ListForTitle(ctx, domain.ReviewableMovies, 1, 1, 10); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestListMine(t *testing.T) {
	e := newEnv(t)
	ann, bob := e.user(t, "ann"), e.user(t, "bob")
	e.review(t, ann, domain.ReviewableMovies, 1, 5)
	time.Sleep(2 * time.Millisecond)
	e.review(t, ann, domain.ReviewableShows, 2, 6)
	e.review(t, bob, domain.ReviewableMovies, 1, 7)

	p, err := e.svc.ListMine(ctx, ann, 0, 0)
	if err != nil || p.Total != 2 || len(p.Reviews) != 2 || p.Summary != nil {
		t.Fatalf("got %+v, %v", p, err)
	}
	if p.Reviews[0].Show == nil || p.Reviews[0].Show.Name != "Show 2" || p.Reviews[1].Movie == nil || p.Reviews[1].Movie.Title != "Movie 1" {
		t.Errorf("expected newest first with titles attached: %+v", p.Reviews)
	}
	empty, err := e.svc.ListMine(ctx, uuid.New(), 1, 10)
	if err != nil || empty.Total != 0 || empty.Reviews == nil || len(empty.Reviews) != 0 {
		t.Errorf("no reviews: %+v, %v", empty, err)
	}

	for _, tc := range []struct{ op, table string }{{"query", "reviews"}, {"query", "movies"}} {
		e2 := newEnv(t)
		u := e2.user(t, "ann")
		e2.review(t, u, domain.ReviewableMovies, 1, 5)
		testutil.FailOn(t, e2.db, tc.op, tc.table)
		if _, err := e2.svc.ListMine(ctx, u, 1, 10); !errors.Is(err, testutil.ErrInjected) {
			t.Errorf("%s: %v", tc.table, err)
		}
	}
	e3 := newEnv(t)
	testutil.FailAfter(t, e3.db, "query", "reviews", 1) // the count succeeds, the page query fails
	if _, err := e3.svc.ListMine(ctx, uuid.New(), 1, 10); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("page query: %v", err)
	}
}

func TestReviewsOfDeletedAuthorsStillList(t *testing.T) {
	e := newEnv(t)
	ann := e.user(t, "ann")
	e.review(t, ann, domain.ReviewableMovies, 1, 5)
	if err := e.db.Unscoped().Delete(&domain.User{}, ann).Error; err != nil {
		t.Fatal(err)
	}
	p, err := e.svc.ListForTitle(ctx, domain.ReviewableMovies, 1, 1, 10)
	if err != nil || len(p.Reviews) != 1 || p.Reviews[0].Author.ID != ann || p.Reviews[0].Author.DisplayName != "" {
		t.Errorf("got %+v, %v", p, err)
	}
}
