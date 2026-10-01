package watchlist

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
)

func TestAddItem(t *testing.T) {
	e := newEnv(t)
	movies := e.newList(t, domain.WatchlistTypeMovie)
	shows := e.newList(t, domain.WatchlistTypeShow)

	t.Run("appends in order and records who added it", func(t *testing.T) {
		a, err := e.svc.AddItem(ctx, e.owner, movies, 1, "  first ")
		if err != nil {
			t.Fatal(err)
		}
		b, err := e.svc.AddItem(ctx, e.owner, movies, 2, "")
		if err != nil {
			t.Fatal(err)
		}
		if a.Position != 0 || b.Position != 1 || a.Notes != "first" || a.AddedByID != e.owner ||
			a.ItemType != domain.WatchlistTypeMovie || a.Movie == nil || a.Show != nil {
			t.Errorf("unexpected items %+v %+v", a, b)
		}
	})

	t.Run("show lists take shows", func(t *testing.T) {
		it, err := e.svc.AddItem(ctx, e.owner, shows, 5, "")
		if err != nil || it.Show == nil || it.Movie != nil || it.ItemType != domain.WatchlistTypeShow {
			t.Errorf("got %+v, %v", it, err)
		}
	})

	t.Run("duplicates are rejected, per list", func(t *testing.T) {
		if _, err := e.svc.AddItem(ctx, e.owner, movies, 1, ""); !errors.Is(err, svcerr.ErrDuplicate) {
			t.Errorf("movie dup: %v", err)
		}
		if _, err := e.svc.AddItem(ctx, e.owner, shows, 5, ""); !errors.Is(err, svcerr.ErrDuplicate) {
			t.Errorf("show dup: %v", err)
		}
		other := e.newList(t, domain.WatchlistTypeMovie)
		if _, err := e.svc.AddItem(ctx, e.owner, other, 1, ""); err != nil {
			t.Errorf("same title on another list should be fine: %v", err)
		}
	})

	t.Run("a removed title can be added again", func(t *testing.T) {
		l := e.newList(t, domain.WatchlistTypeMovie)
		ids := e.addMovies(t, l, 77)
		if err := e.svc.RemoveItem(ctx, e.owner, l, ids[0]); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.AddItem(ctx, e.owner, l, 77, ""); err != nil {
			t.Errorf("re-adding after removal failed: %v", err)
		}
	})

	t.Run("editors may add, viewers may not", func(t *testing.T) {
		editor := e.member(t, movies, domain.RoleEditor)
		if it, err := e.svc.AddItem(ctx, editor, movies, 3, ""); err != nil || it.AddedByID != editor {
			t.Errorf("editor: %+v, %v", it, err)
		}
		viewer := e.member(t, movies, domain.RoleViewer)
		if _, err := e.svc.AddItem(ctx, viewer, movies, 4, ""); !errors.Is(err, svcerr.ErrForbidden) {
			t.Errorf("viewer: %v", err)
		}
	})

	t.Run("not found and validation", func(t *testing.T) {
		if _, err := e.svc.AddItem(ctx, uuid.New(), movies, 9, ""); !errors.Is(err, svcerr.ErrNotFound) {
			t.Errorf("stranger: %v", err)
		}
		if _, err := e.svc.AddItem(ctx, e.owner, movies, 9, strings.Repeat("n", 2001)); !errors.Is(err, svcerr.ErrInvalid) {
			t.Errorf("long notes: %v", err)
		}
	})

	t.Run("catalog errors pass through", func(t *testing.T) {
		boom := errors.New("catalog down")
		e.cat.err = boom
		defer func() { e.cat.err = nil }()
		if _, err := e.svc.AddItem(ctx, e.owner, movies, 50, ""); !errors.Is(err, boom) {
			t.Errorf("movie: %v", err)
		}
		if _, err := e.svc.AddItem(ctx, e.owner, shows, 50, ""); !errors.Is(err, boom) {
			t.Errorf("show: %v", err)
		}
	})
}

func TestAddItemDBFailures(t *testing.T) {
	for _, tc := range []struct {
		name, op, table string
		after           int
	}{
		{"duplicate check", "query", "watchlist_items", 0},
		{"position lookup", "row", "watchlist_items", 0},
		{"insert", "create", "watchlist_items", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			l := e.newList(t, domain.WatchlistTypeMovie)
			testutil.FailAfter(t, e.db, tc.op, tc.table, tc.after)
			if _, err := e.svc.AddItem(ctx, e.owner, l, 1, ""); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestConcurrentAddsOfTheSameTitleCreateOneItem(t *testing.T) {
	db := testutil.NewFileDB(t, models()...)
	cat := &fakeCatalog{db: db}
	svc := NewService(db, cat)
	owner := uuid.New()
	w, err := svc.Create(ctx, owner, CreateInput{Title: "L", Type: domain.WatchlistTypeMovie})
	if err != nil {
		t.Fatal(err)
	}
	// Prime the title so concurrent catalog lookups don't race on FirstOrCreate.
	if _, err := cat.EnsureMovie(ctx, 42); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.AddItem(ctx, owner, w.ID, 42, "")
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
	if svc.locks.Len() != 0 {
		t.Errorf("locks leaked: %d", svc.locks.Len())
	}
}

func TestUpdateItemNotes(t *testing.T) {
	e := newEnv(t)
	l := e.newList(t, domain.WatchlistTypeMovie)
	items := e.addMovies(t, l, 1)

	it, err := e.svc.UpdateItemNotes(ctx, e.owner, l, items[0], "  rewatch with Sam ")
	if err != nil || it.Notes != "rewatch with Sam" {
		t.Fatalf("got %+v, %v", it, err)
	}
	d, _ := e.svc.Get(ctx, e.owner, l)
	if d.Items[0].Notes != "rewatch with Sam" {
		t.Error("notes not persisted")
	}

	if _, err := e.svc.UpdateItemNotes(ctx, e.owner, l, items[0], strings.Repeat("n", 2001)); !errors.Is(err, svcerr.ErrInvalid) {
		t.Errorf("long notes: %v", err)
	}
	viewer := e.member(t, l, domain.RoleViewer)
	if _, err := e.svc.UpdateItemNotes(ctx, viewer, l, items[0], "x"); !errors.Is(err, svcerr.ErrForbidden) {
		t.Errorf("viewer: %v", err)
	}
	if _, err := e.svc.UpdateItemNotes(ctx, uuid.New(), l, items[0], "x"); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("stranger: %v", err)
	}
	if _, err := e.svc.UpdateItemNotes(ctx, e.owner, l, uuid.New(), "x"); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("unknown item: %v", err)
	}
	// An item id from a different list must not be reachable through this one.
	other := e.newList(t, domain.WatchlistTypeMovie)
	otherItems := e.addMovies(t, other, 2)
	if _, err := e.svc.UpdateItemNotes(ctx, e.owner, l, otherItems[0], "x"); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("cross-list item: %v", err)
	}

	testutil.FailOn(t, e.db, "update", "watchlist_items")
	if _, err := e.svc.UpdateItemNotes(ctx, e.owner, l, items[0], "y"); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("db failure: %v", err)
	}
}

func TestEditableItemLookupFailure(t *testing.T) {
	e := newEnv(t)
	l := e.newList(t, domain.WatchlistTypeMovie)
	items := e.addMovies(t, l, 1)
	testutil.FailOn(t, e.db, "query", "watchlist_items")
	if _, err := e.svc.UpdateItemNotes(ctx, e.owner, l, items[0], "x"); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("got %v", err)
	}
}

func TestRemoveItem(t *testing.T) {
	e := newEnv(t)
	l := e.newList(t, domain.WatchlistTypeMovie)
	items := e.addMovies(t, l, 1, 2, 3)

	if err := e.svc.RemoveItem(ctx, e.owner, l, items[1]); err != nil {
		t.Fatal(err)
	}
	d, _ := e.svc.Get(ctx, e.owner, l)
	if len(d.Items) != 2 || d.Items[0].ID != items[0] || d.Items[1].ID != items[2] || d.Items[1].Position != 1 {
		t.Errorf("positions not closed up: %+v", d.Items)
	}

	viewer := e.member(t, l, domain.RoleViewer)
	if err := e.svc.RemoveItem(ctx, viewer, l, items[0]); !errors.Is(err, svcerr.ErrForbidden) {
		t.Errorf("viewer: %v", err)
	}
	if err := e.svc.RemoveItem(ctx, e.owner, l, items[1]); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("already removed: %v", err)
	}
}

func TestRemoveItemDBFailures(t *testing.T) {
	for _, tc := range []struct {
		name, op, table string
		after           int
	}{
		{"delete", "delete", "watchlist_items", 0},
		{"renumber lookup", "query", "watchlist_items", 1},
		{"renumber update", "update", "watchlist_items", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			l := e.newList(t, domain.WatchlistTypeMovie)
			items := e.addMovies(t, l, 1, 2, 3)
			testutil.FailAfter(t, e.db, tc.op, tc.table, tc.after)
			if err := e.svc.RemoveItem(ctx, e.owner, l, items[0]); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestReorder(t *testing.T) {
	e := newEnv(t)
	l := e.newList(t, domain.WatchlistTypeMovie)
	ids := e.addMovies(t, l, 1, 2, 3)

	want := []uuid.UUID{ids[2], ids[0], ids[1]}
	if err := e.svc.Reorder(ctx, e.owner, l, want); err != nil {
		t.Fatal(err)
	}
	got := e.order(t, l)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	// A new item lands after the reordered ones.
	it, _ := e.svc.AddItem(ctx, e.owner, l, 4, "")
	if it.Position != 3 {
		t.Errorf("new item position = %d", it.Position)
	}

	editor := e.member(t, l, domain.RoleEditor)
	if err := e.svc.Reorder(ctx, editor, l, append(want, it.ID)); err != nil {
		t.Errorf("editor: %v", err)
	}
	viewer := e.member(t, l, domain.RoleViewer)
	if err := e.svc.Reorder(ctx, viewer, l, want); !errors.Is(err, svcerr.ErrForbidden) {
		t.Errorf("viewer: %v", err)
	}
	if err := e.svc.Reorder(ctx, uuid.New(), l, want); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("stranger: %v", err)
	}
}

func TestReorderValidation(t *testing.T) {
	e := newEnv(t)
	l := e.newList(t, domain.WatchlistTypeMovie)
	ids := e.addMovies(t, l, 1, 2)

	for name, in := range map[string][]uuid.UUID{
		"missing item":    {ids[0]},
		"extra item":      {ids[0], ids[1], uuid.New()},
		"unknown item":    {ids[0], uuid.New()},
		"repeated item":   {ids[0], ids[0]},
		"empty":           nil,
		"foreign list id": {ids[0], func() uuid.UUID { o := e.newList(t, domain.WatchlistTypeMovie); return e.addMovies(t, o, 9)[0] }()},
	} {
		if err := e.svc.Reorder(ctx, e.owner, l, in); !errors.Is(err, svcerr.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// The failed attempts must not have changed anything.
	got := e.order(t, l)
	if got[0] != ids[0] || got[1] != ids[1] {
		t.Errorf("order changed: %v", got)
	}
}

func TestReorderDBFailures(t *testing.T) {
	for _, tc := range []struct {
		name, op, table string
	}{
		{"load items", "query", "watchlist_items"},
		{"write positions", "update", "watchlist_items"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			l := e.newList(t, domain.WatchlistTypeMovie)
			ids := e.addMovies(t, l, 1, 2)
			testutil.FailOn(t, e.db, tc.op, tc.table)
			if err := e.svc.Reorder(ctx, e.owner, l, []uuid.UUID{ids[1], ids[0]}); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}
