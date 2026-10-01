package watchlist

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
)

var ctx = context.Background()

func TestCreate(t *testing.T) {
	e := newEnv(t)

	t.Run("defaults to private and trims", func(t *testing.T) {
		w, err := e.svc.Create(ctx, e.owner, CreateInput{Title: "  Friday night  ", Description: " cozy ", Type: domain.WatchlistTypeMovie})
		if err != nil {
			t.Fatal(err)
		}
		if w.Title != "Friday night" || w.Description != "cozy" || w.Privacy != domain.PrivacyPrivate ||
			w.Role != domain.RoleOwner || w.ShareToken == "" || w.ItemCount != 0 || w.OwnerID != e.owner {
			t.Errorf("unexpected summary: %+v", w)
		}
	})

	cases := []struct {
		name string
		in   CreateInput
		msg  string
	}{
		{"empty title", CreateInput{Title: "  ", Type: domain.WatchlistTypeMovie}, "title is required"},
		{"long title", CreateInput{Title: strings.Repeat("x", 201), Type: domain.WatchlistTypeMovie}, "title is too long"},
		{"long description", CreateInput{Title: "t", Description: strings.Repeat("x", 2001), Type: domain.WatchlistTypeMovie}, "description is too long"},
		{"bad privacy", CreateInput{Title: "t", Privacy: "secret", Type: domain.WatchlistTypeMovie}, "privacy must be"},
		{"bad type", CreateInput{Title: "t", Type: "podcast"}, "type must be movie or show"},
		{"missing type", CreateInput{Title: "t"}, "type must be movie or show"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.Create(ctx, e.owner, tc.in)
			if !errors.Is(err, svcerr.ErrInvalid) || !strings.Contains(svcerr.Message(err), tc.msg) {
				t.Errorf("got %v", err)
			}
		})
	}

	t.Run("db failure", func(t *testing.T) {
		e := newEnv(t)
		testutil.FailOn(t, e.db, "create", "watchlists")
		if _, err := e.svc.Create(ctx, e.owner, CreateInput{Title: "t", Type: domain.WatchlistTypeShow}); !errors.Is(err, testutil.ErrInjected) {
			t.Errorf("got %v", err)
		}
	})
}

func TestCreateDefaultLists(t *testing.T) {
	e := newEnv(t)
	if err := CreateDefaultLists(e.db, e.owner); err != nil {
		t.Fatal(err)
	}
	lists, err := e.svc.List(ctx, e.owner)
	if err != nil || len(lists) != len(DefaultLists) {
		t.Fatalf("lists=%v err=%v", lists, err)
	}
	testutil.FailOn(t, e.db, "create", "watchlists")
	if err := CreateDefaultLists(e.db, uuid.New()); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("got %v", err)
	}
}

func TestListShowsOwnedAndSharedWithRoles(t *testing.T) {
	e := newEnv(t)
	mine := e.newList(t, domain.WatchlistTypeMovie)
	e.addMovies(t, mine, 1, 2)

	other := Service{DB: e.db, Catalog: e.cat}
	theirs, _ := other.Create(ctx, uuid.New(), CreateInput{Title: "Theirs", Type: domain.WatchlistTypeShow})
	unrelated, _ := other.Create(ctx, uuid.New(), CreateInput{Title: "Unrelated", Type: domain.WatchlistTypeShow})
	if err := e.db.Create(&domain.Collaborator{UserID: e.owner, WatchlistID: theirs.ID, Role: domain.RoleViewer}).Error; err != nil {
		t.Fatal(err)
	}

	got, err := e.svc.List(ctx, e.owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 lists, got %d", len(got))
	}
	byID := map[uuid.UUID]Summary{}
	for _, s := range got {
		byID[s.ID] = s
	}
	if s := byID[mine]; s.Role != domain.RoleOwner || s.ItemCount != 2 || s.ShareToken == "" {
		t.Errorf("owned list wrong: %+v", s)
	}
	if s := byID[theirs.ID]; s.Role != domain.RoleViewer || s.ShareToken != "" {
		t.Errorf("shared list must show role and hide the share token: %+v", s)
	}
	if _, ok := byID[unrelated.ID]; ok {
		t.Error("unrelated list leaked")
	}
}

func TestListEmptyAndFailures(t *testing.T) {
	e := newEnv(t)
	if got, err := e.svc.List(ctx, e.owner); err != nil || len(got) != 0 {
		t.Errorf("got %v, %v", got, err)
	}

	for _, tc := range []struct{ op, table string }{
		{"query", "collaborators"}, {"query", "watchlists"}, {"row", "watchlist_items"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			e := newEnv(t)
			e.newList(t, domain.WatchlistTypeMovie)
			testutil.FailOn(t, e.db, tc.op, tc.table)
			if _, err := e.svc.List(ctx, e.owner); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestGet(t *testing.T) {
	e := newEnv(t)
	id := e.newList(t, domain.WatchlistTypeMovie)
	items := e.addMovies(t, id, 10, 20, 30)

	t.Run("owner sees ordered items with titles", func(t *testing.T) {
		d, err := e.svc.Get(ctx, e.owner, id)
		if err != nil {
			t.Fatal(err)
		}
		if d.Role != domain.RoleOwner || d.ItemCount != 3 || len(d.Items) != 3 {
			t.Fatalf("unexpected detail %+v", d.Summary)
		}
		for i, it := range d.Items {
			if it.ID != items[i] || it.Position != i || it.Movie == nil || it.Show != nil {
				t.Errorf("item %d wrong: %+v", i, it)
			}
		}
		if d.Items[0].Movie.Title != "Movie 10" {
			t.Errorf("title not loaded: %+v", d.Items[0].Movie)
		}
	})

	t.Run("viewer can read but has no share token", func(t *testing.T) {
		v := e.member(t, id, domain.RoleViewer)
		d, err := e.svc.Get(ctx, v, id)
		if err != nil || d.Role != domain.RoleViewer || d.ShareToken != "" {
			t.Errorf("got %+v, %v", d, err)
		}
	})

	t.Run("strangers and unknown ids are not found", func(t *testing.T) {
		if _, err := e.svc.Get(ctx, uuid.New(), id); !errors.Is(err, svcerr.ErrNotFound) {
			t.Errorf("stranger: %v", err)
		}
		if _, err := e.svc.Get(ctx, e.owner, uuid.New()); !errors.Is(err, svcerr.ErrNotFound) {
			t.Errorf("unknown: %v", err)
		}
	})

	for _, tc := range []struct {
		name, table string
		user        func() uuid.UUID
	}{
		{"watchlist lookup", "watchlists", func() uuid.UUID { return e.owner }},
		{"collaborator lookup", "collaborators", uuid.New},
		{"items query", "watchlist_items", func() uuid.UUID { return e.owner }},
	} {
		t.Run("failure/"+tc.name, func(t *testing.T) {
			e2 := newEnv(t)
			lid := e2.newList(t, domain.WatchlistTypeMovie)
			testutil.FailOn(t, e2.db, "query", tc.table)
			user := e2.owner
			if tc.table == "collaborators" {
				user = uuid.New()
			}
			if _, err := e2.svc.Get(ctx, user, lid); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	e := newEnv(t)
	id := e.newList(t, domain.WatchlistTypeMovie)
	e.addMovies(t, id, 1)
	title, desc, priv := "New", "Desc", domain.PrivacyShared

	t.Run("owner updates fields", func(t *testing.T) {
		s, err := e.svc.Update(ctx, e.owner, id, UpdateInput{Title: &title, Description: &desc, Privacy: &priv})
		if err != nil {
			t.Fatal(err)
		}
		if s.Title != "New" || s.Description != "Desc" || s.Privacy != domain.PrivacyShared || s.ItemCount != 1 {
			t.Errorf("unexpected %+v", s)
		}
		d, _ := e.svc.Get(ctx, e.owner, id)
		if d.Title != "New" {
			t.Error("update not persisted")
		}
	})

	t.Run("empty update changes nothing", func(t *testing.T) {
		if _, err := e.svc.Update(ctx, e.owner, id, UpdateInput{}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("editors and viewers are forbidden", func(t *testing.T) {
		for _, role := range []domain.CollaboratorRole{domain.RoleEditor, domain.RoleViewer} {
			u := e.member(t, id, role)
			if _, err := e.svc.Update(ctx, u, id, UpdateInput{Title: &title}); !errors.Is(err, svcerr.ErrForbidden) {
				t.Errorf("%s: %v", role, err)
			}
		}
	})

	t.Run("not found", func(t *testing.T) {
		if _, err := e.svc.Update(ctx, uuid.New(), id, UpdateInput{}); !errors.Is(err, svcerr.ErrNotFound) {
			t.Errorf("got %v", err)
		}
	})

	t.Run("validation", func(t *testing.T) {
		empty, long, bad := " ", strings.Repeat("x", 2001), domain.PrivacyLevel("nope")
		for name, in := range map[string]UpdateInput{
			"title": {Title: &empty}, "description": {Description: &long}, "privacy": {Privacy: &bad},
		} {
			if _, err := e.svc.Update(ctx, e.owner, id, in); !errors.Is(err, svcerr.ErrInvalid) {
				t.Errorf("%s: %v", name, err)
			}
		}
	})

	for _, op := range []string{"update", "query"} {
		t.Run("db failure "+op, func(t *testing.T) {
			e := newEnv(t)
			lid := e.newList(t, domain.WatchlistTypeMovie)
			table := "watchlists"
			after := 0
			if op == "query" {
				table, after = "watchlist_items", 0 // the item count
			}
			testutil.FailAfter(t, e.db, op, table, after)
			if _, err := e.svc.Update(ctx, e.owner, lid, UpdateInput{Title: &title}); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	e := newEnv(t)
	id := e.newList(t, domain.WatchlistTypeMovie)
	e.addMovies(t, id, 1, 2)
	editor := e.member(t, id, domain.RoleEditor)

	if err := e.svc.Delete(ctx, editor, id); !errors.Is(err, svcerr.ErrForbidden) {
		t.Errorf("editor: %v", err)
	}
	if err := e.svc.Delete(ctx, uuid.New(), id); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("stranger: %v", err)
	}
	if err := e.svc.Delete(ctx, e.owner, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Get(ctx, e.owner, id); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("deleted list still visible: %v", err)
	}
	var items, collabs int64
	e.db.Model(&domain.WatchlistItem{}).Where("watchlist_id = ?", id).Count(&items)
	e.db.Model(&domain.Collaborator{}).Where("watchlist_id = ?", id).Count(&collabs)
	if items != 0 || collabs != 0 {
		t.Errorf("cascade left items=%d collaborators=%d", items, collabs)
	}
}
