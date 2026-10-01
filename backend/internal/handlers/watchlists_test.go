package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/aomarai/concession/internal/watchlist"
	"github.com/google/uuid"
)

type listBody struct {
	Watchlists []watchlist.Summary `json:"watchlists"`
}

func (a *api) createList(user uuid.UUID, typ string) watchlist.Summary {
	a.t.Helper()
	w := a.do(user, http.MethodPost, "/watchlists", map[string]any{"title": "Friday", "type": typ})
	a.expect(w, http.StatusCreated, "")
	return decode[watchlist.Summary](a.t, w)
}

func (a *api) addItem(user uuid.UUID, list uuid.UUID, tmdbID int64) watchlist.ItemView {
	a.t.Helper()
	w := a.do(user, http.MethodPost, "/watchlists/"+list.String()+"/items", map[string]any{"tmdb_id": tmdbID})
	a.expect(w, http.StatusCreated, "")
	return decode[watchlist.ItemView](a.t, w)
}

func TestWatchlistLifecycle(t *testing.T) {
	a := newAPI(t)
	owner := uuid.New()

	l := a.createList(owner, "movie")
	if l.Role != domain.RoleOwner || l.Privacy != domain.PrivacyPrivate || l.ShareToken == "" {
		t.Errorf("created list wrong: %+v", l)
	}
	id := l.ID.String()

	got := decode[listBody](t, a.do(owner, http.MethodGet, "/watchlists", nil))
	if len(got.Watchlists) != 1 || got.Watchlists[0].ID != l.ID {
		t.Errorf("list: %+v", got)
	}

	w := a.do(owner, http.MethodPatch, "/watchlists/"+id, map[string]any{"title": "Saturday", "privacy": "shared"})
	a.expect(w, http.StatusOK, "")
	if u := decode[watchlist.Summary](t, w); u.Title != "Saturday" || u.Privacy != domain.PrivacyShared {
		t.Errorf("update: %+v", u)
	}

	i1 := a.addItem(owner, l.ID, 10)
	i2 := a.addItem(owner, l.ID, 20)
	i3 := a.addItem(owner, l.ID, 30)

	a.expect(a.do(owner, http.MethodPut, "/watchlists/"+id+"/items/order",
		map[string]any{"item_ids": []uuid.UUID{i3.ID, i1.ID, i2.ID}}), http.StatusNoContent, "")

	w = a.do(owner, http.MethodPatch, "/watchlists/"+id+"/items/"+i1.ID.String(), map[string]any{"notes": "with Sam"})
	a.expect(w, http.StatusOK, "")
	if it := decode[watchlist.ItemView](t, w); it.Notes != "with Sam" {
		t.Errorf("notes: %+v", it)
	}

	a.expect(a.do(owner, http.MethodDelete, "/watchlists/"+id+"/items/"+i2.ID.String(), nil), http.StatusNoContent, "")

	d := decode[watchlist.Detail](t, a.do(owner, http.MethodGet, "/watchlists/"+id, nil))
	if len(d.Items) != 2 || d.Items[0].ID != i3.ID || d.Items[1].ID != i1.ID || d.Items[1].Notes != "with Sam" ||
		d.Items[0].Movie == nil || d.Items[0].Movie.Title != "Movie 30" {
		t.Errorf("detail: %+v", d)
	}

	a.expect(a.do(owner, http.MethodDelete, "/watchlists/"+id, nil), http.StatusNoContent, "")
	a.expect(a.do(owner, http.MethodGet, "/watchlists/"+id, nil), http.StatusNotFound, "not_found")
}

func TestWatchlistAccessControl(t *testing.T) {
	a := newAPI(t)
	owner, stranger := uuid.New(), uuid.New()
	l := a.createList(owner, "movie")
	id := l.ID.String()
	it := a.addItem(owner, l.ID, 1)

	viewer, editor := uuid.New(), uuid.New()
	a.db.Create(&domain.Collaborator{UserID: viewer, WatchlistID: l.ID, Role: domain.RoleViewer})
	a.db.Create(&domain.Collaborator{UserID: editor, WatchlistID: l.ID, Role: domain.RoleEditor})

	// Strangers learn nothing: every route answers 404.
	itemPath := "/watchlists/" + id + "/items/" + it.ID.String()
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/watchlists/" + id, nil},
		{http.MethodPatch, "/watchlists/" + id, map[string]any{"title": "x"}},
		{http.MethodDelete, "/watchlists/" + id, nil},
		{http.MethodPost, "/watchlists/" + id + "/items", map[string]any{"tmdb_id": 2}},
		{http.MethodPatch, itemPath, map[string]any{"notes": "x"}},
		{http.MethodDelete, itemPath, nil},
		{http.MethodPut, "/watchlists/" + id + "/items/order", map[string]any{"item_ids": []uuid.UUID{it.ID}}},
	} {
		a.expect(a.do(stranger, tc.method, tc.path, tc.body), http.StatusNotFound, "not_found")
	}

	// Viewers read but cannot change anything.
	a.expect(a.do(viewer, http.MethodGet, "/watchlists/"+id, nil), http.StatusOK, "")
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/watchlists/" + id + "/items", map[string]any{"tmdb_id": 2}},
		{http.MethodPatch, itemPath, map[string]any{"notes": "x"}},
		{http.MethodDelete, itemPath, nil},
		{http.MethodPut, "/watchlists/" + id + "/items/order", map[string]any{"item_ids": []uuid.UUID{it.ID}}},
		{http.MethodPatch, "/watchlists/" + id, map[string]any{"title": "x"}},
		{http.MethodDelete, "/watchlists/" + id, nil},
	} {
		a.expect(a.do(viewer, tc.method, tc.path, tc.body), http.StatusForbidden, "forbidden")
	}

	// Editors manage items but not the list itself.
	a.addItem(editor, l.ID, 3)
	a.expect(a.do(editor, http.MethodPatch, "/watchlists/"+id, map[string]any{"title": "x"}), http.StatusForbidden, "forbidden")
	a.expect(a.do(editor, http.MethodDelete, "/watchlists/"+id, nil), http.StatusForbidden, "forbidden")

	// Collaborators see the list, with their role, in their own listing.
	got := decode[listBody](t, a.do(viewer, http.MethodGet, "/watchlists", nil))
	if len(got.Watchlists) != 1 || got.Watchlists[0].Role != domain.RoleViewer || got.Watchlists[0].ShareToken != "" {
		t.Errorf("viewer listing: %+v", got)
	}
}

func TestWatchlistRequestErrors(t *testing.T) {
	a := newAPI(t)
	u := uuid.New()
	l := a.createList(u, "show")
	id := l.ID.String()
	item := a.addItem(u, l.ID, 5)

	cases := []struct {
		name         string
		method, path string
		body         any
		status       int
		code         string
	}{
		{"create: missing title", http.MethodPost, "/watchlists", map[string]any{"type": "movie"}, 400, "bad_request"},
		{"create: bad type", http.MethodPost, "/watchlists", map[string]any{"title": "t", "type": "x"}, 400, "bad_request"},
		{"get: malformed id", http.MethodGet, "/watchlists/nope", nil, 400, "bad_request"},
		{"get: unknown id", http.MethodGet, "/watchlists/" + uuid.NewString(), nil, 404, "not_found"},
		{"update: malformed id", http.MethodPatch, "/watchlists/nope", map[string]any{}, 400, "bad_request"},
		{"update: bad json", http.MethodPatch, "/watchlists/" + id, "{", 400, "bad_request"},
		{"update: bad privacy", http.MethodPatch, "/watchlists/" + id, map[string]any{"privacy": "x"}, 400, "bad_request"},
		{"delete: malformed id", http.MethodDelete, "/watchlists/nope", nil, 400, "bad_request"},
		{"add: malformed list id", http.MethodPost, "/watchlists/nope/items", map[string]any{"tmdb_id": 1}, 400, "bad_request"},
		{"add: bad json", http.MethodPost, "/watchlists/" + id + "/items", "{", 400, "bad_request"},
		{"add: missing tmdb_id", http.MethodPost, "/watchlists/" + id + "/items", map[string]any{}, 400, "bad_request"},
		{"add: negative tmdb_id", http.MethodPost, "/watchlists/" + id + "/items", map[string]any{"tmdb_id": -1}, 400, "bad_request"},
		{"add: duplicate", http.MethodPost, "/watchlists/" + id + "/items", map[string]any{"tmdb_id": 5}, 409, "conflict"},
		{"add: TMDB has no such title", http.MethodPost, "/watchlists/" + id + "/items", map[string]any{"tmdb_id": 404}, 404, "not_found"},
		{"add: TMDB unreachable", http.MethodPost, "/watchlists/" + id + "/items", map[string]any{"tmdb_id": 502}, 502, "upstream_error"},
		{"item update: malformed list id", http.MethodPatch, "/watchlists/nope/items/" + item.ID.String(), map[string]any{}, 400, "bad_request"},
		{"item update: malformed item id", http.MethodPatch, "/watchlists/" + id + "/items/nope", map[string]any{}, 400, "bad_request"},
		{"item update: bad json", http.MethodPatch, "/watchlists/" + id + "/items/" + item.ID.String(), "{", 400, "bad_request"},
		{"item update: unknown item", http.MethodPatch, "/watchlists/" + id + "/items/" + uuid.NewString(), map[string]any{}, 404, "not_found"},
		{"item delete: malformed list id", http.MethodDelete, "/watchlists/nope/items/" + item.ID.String(), nil, 400, "bad_request"},
		{"item delete: malformed item id", http.MethodDelete, "/watchlists/" + id + "/items/nope", nil, 400, "bad_request"},
		{"reorder: malformed list id", http.MethodPut, "/watchlists/nope/items/order", map[string]any{}, 400, "bad_request"},
		{"reorder: bad json", http.MethodPut, "/watchlists/" + id + "/items/order", "{", 400, "bad_request"},
		{"reorder: not every item", http.MethodPut, "/watchlists/" + id + "/items/order", map[string]any{"item_ids": []uuid.UUID{}}, 400, "bad_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a.expect(a.do(u, tc.method, tc.path, tc.body), tc.status, tc.code)
		})
	}
}

func TestWatchlistServerErrors(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		body               any
		op, table          string
	}{
		{"create", http.MethodPost, "/watchlists", map[string]any{"title": "t", "type": "movie"}, "create", "watchlists"},
		{"list", http.MethodGet, "/watchlists", nil, "query", "collaborators"},
		{"get", http.MethodGet, "/watchlists/{id}", nil, "query", "watchlist_items"},
		{"update", http.MethodPatch, "/watchlists/{id}", map[string]any{"title": "x"}, "update", "watchlists"},
		{"delete", http.MethodDelete, "/watchlists/{id}", nil, "delete", "watchlists"},
		{"item update", http.MethodPatch, "/watchlists/{id}/items/{item}", map[string]any{"notes": "x"}, "update", "watchlist_items"},
		{"item delete", http.MethodDelete, "/watchlists/{id}/items/{item}", nil, "delete", "watchlist_items"},
		{"reorder", http.MethodPut, "/watchlists/{id}/items/order", map[string]any{"item_ids": []uuid.UUID{}}, "query", "watchlist_items"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newAPI(t)
			u := uuid.New()
			l := a.createList(u, "movie")
			it := a.addItem(u, l.ID, 1)
			path := replace(tc.path, "{id}", l.ID.String(), "{item}", it.ID.String())
			testutil.FailOn(t, a.db, tc.op, tc.table)
			a.expect(a.do(u, tc.method, path, tc.body), http.StatusInternalServerError, "internal_error")
		})
	}
}

func replace(s string, pairs ...string) string {
	return strings.NewReplacer(pairs...).Replace(s)
}

func TestUnauthenticatedRequestsGet500FromHandlers(t *testing.T) {
	a := newAPI(t)
	id, item := uuid.NewString(), uuid.NewString()
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/watchlists"}, {http.MethodGet, "/watchlists"},
		{http.MethodGet, "/watchlists/" + id}, {http.MethodPatch, "/watchlists/" + id},
		{http.MethodDelete, "/watchlists/" + id}, {http.MethodPost, "/watchlists/" + id + "/items"},
		{http.MethodPatch, "/watchlists/" + id + "/items/" + item}, {http.MethodDelete, "/watchlists/" + id + "/items/" + item},
		{http.MethodPut, "/watchlists/" + id + "/items/order"},
		{http.MethodGet, "/me/progress"}, {http.MethodGet, "/me/progress/movies/1"},
		{http.MethodPut, "/me/progress/movies/1"}, {http.MethodDelete, "/me/progress/movies/1"},
	} {
		a.expect(a.do(uuid.Nil, tc.method, tc.path, nil), http.StatusInternalServerError, "internal_error")
	}
}
