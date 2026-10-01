package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/reviews"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
)

func (a *api) newUser(name string) uuid.UUID {
	a.t.Helper()
	u := domain.User{Username: name, Email: name + "@example.com", DisplayName: strings.ToUpper(name[:1]) + name[1:]}
	if err := a.db.Create(&u).Error; err != nil {
		a.t.Fatal(err)
	}
	return u.ID
}

func (a *api) postReview(user uuid.UUID, path string, rating int) reviews.View {
	a.t.Helper()
	w := a.do(user, http.MethodPost, path, map[string]any{"rating": rating, "title": "Great", "content": "Loved it"})
	a.expect(w, http.StatusCreated, "")
	return decode[reviews.View](a.t, w)
}

func TestReviewLifecycle(t *testing.T) {
	a := newAPI(t)
	ann, bob := a.newUser("ann"), a.newUser("bob")

	r := a.postReview(ann, "/movies/603/reviews", 9)
	if r.Rating != 9 || r.Author.DisplayName != "Ann" || r.Author.ID != ann || r.ItemType != domain.ReviewableMovies {
		t.Errorf("created: %+v", r)
	}
	a.postReview(bob, "/movies/603/reviews", 7)
	a.postReview(ann, "/shows/1396/reviews", 5)

	// Public listing with the rating summary; the author's email never appears.
	w := a.do(bob, http.MethodGet, "/movies/603/reviews", nil)
	a.expect(w, http.StatusOK, "")
	if strings.Contains(w.Body.String(), "@example.com") {
		t.Errorf("author email leaked: %s", w.Body)
	}
	p := decode[reviews.Page](t, w)
	if p.Total != 2 || p.Summary == nil || p.Summary.Count != 2 || p.Summary.Average != 8 || len(p.Reviews) != 2 {
		t.Errorf("listing: %+v %+v", p, p.Summary)
	}
	p = decode[reviews.Page](t, a.do(bob, http.MethodGet, "/movies/603/reviews?page=2&per_page=1", nil))
	if len(p.Reviews) != 1 || p.Page != 2 || p.PerPage != 1 || p.Total != 2 {
		t.Errorf("paging: %+v", p)
	}
	if sp := decode[reviews.Page](t, a.do(bob, http.MethodGet, "/shows/1396/reviews", nil)); sp.Summary.Count != 1 {
		t.Errorf("show listing: %+v", sp)
	}

	// One review per title.
	a.expect(a.do(ann, http.MethodPost, "/movies/603/reviews", map[string]any{"rating": 5}), http.StatusConflict, "conflict")

	// Anyone can read a single review, with its title attached.
	got := decode[reviews.View](t, a.do(bob, http.MethodGet, "/reviews/"+r.ID.String(), nil))
	if got.ID != r.ID || got.Movie == nil || got.Movie.Title != "Movie 603" {
		t.Errorf("get: %+v", got)
	}

	// Only the author can change or delete it.
	a.expect(a.do(bob, http.MethodPatch, "/reviews/"+r.ID.String(), map[string]any{"rating": 1}), http.StatusForbidden, "forbidden")
	a.expect(a.do(bob, http.MethodDelete, "/reviews/"+r.ID.String(), nil), http.StatusForbidden, "forbidden")
	w = a.do(ann, http.MethodPatch, "/reviews/"+r.ID.String(), map[string]any{"rating": 10, "content": "Even better"})
	a.expect(w, http.StatusOK, "")
	if u := decode[reviews.View](t, w); u.Rating != 10 || u.Content != "Even better" || u.Title != "Great" {
		t.Errorf("update: %+v", u)
	}

	mine := decode[reviews.Page](t, a.do(ann, http.MethodGet, "/me/reviews", nil))
	if mine.Total != 2 || mine.Summary != nil || len(mine.Reviews) != 2 || (mine.Reviews[0].Movie == nil && mine.Reviews[0].Show == nil) {
		t.Errorf("mine: %+v", mine)
	}

	a.expect(a.do(ann, http.MethodDelete, "/reviews/"+r.ID.String(), nil), http.StatusNoContent, "")
	a.expect(a.do(ann, http.MethodGet, "/reviews/"+r.ID.String(), nil), http.StatusNotFound, "not_found")
}

func TestReviewRequestErrors(t *testing.T) {
	a := newAPI(t)
	u := a.newUser("ann")
	r := a.postReview(u, "/movies/1/reviews", 5)
	id := r.ID.String()

	for _, tc := range []struct {
		name, method, path string
		body               any
		status             int
		code               string
	}{
		{"create: rating missing", http.MethodPost, "/movies/2/reviews", map[string]any{}, 400, "bad_request"},
		{"create: rating too high", http.MethodPost, "/movies/2/reviews", map[string]any{"rating": 11}, 400, "bad_request"},
		{"create: fractional rating", http.MethodPost, "/movies/2/reviews", map[string]any{"rating": 7.5}, 400, "bad_request"},
		{"create: bad json", http.MethodPost, "/shows/2/reviews", "{", 400, "bad_request"},
		{"create: bad id", http.MethodPost, "/movies/abc/reviews", map[string]any{"rating": 5}, 400, "bad_request"},
		{"create: show bad id", http.MethodPost, "/shows/0/reviews", map[string]any{"rating": 5}, 400, "bad_request"},
		{"create: TMDB has no such title", http.MethodPost, "/movies/404/reviews", map[string]any{"rating": 5}, 404, "not_found"},
		{"create: TMDB unreachable", http.MethodPost, "/shows/502/reviews", map[string]any{"rating": 5}, 502, "upstream_error"},
		{"list: bad id", http.MethodGet, "/movies/abc/reviews", nil, 400, "bad_request"},
		{"list: bad page", http.MethodGet, "/movies/1/reviews?page=abc", nil, 400, "bad_request"},
		{"list: bad per_page", http.MethodGet, "/shows/1/reviews?per_page=x", nil, 400, "bad_request"},
		{"list: negative page", http.MethodGet, "/movies/1/reviews?page=-1", nil, 400, "bad_request"},
		{"mine: bad page", http.MethodGet, "/me/reviews?page=abc", nil, 400, "bad_request"},
		{"mine: absurd page", http.MethodGet, "/me/reviews?page=9223372036854775807", nil, 400, "bad_request"},
		{"list: both params bad", http.MethodGet, "/movies/1/reviews?page=a&per_page=b", nil, 400, "bad_request"},
		{"get: malformed id", http.MethodGet, "/reviews/nope", nil, 400, "bad_request"},
		{"get: unknown id", http.MethodGet, "/reviews/" + uuid.NewString(), nil, 404, "not_found"},
		{"update: malformed id", http.MethodPatch, "/reviews/nope", map[string]any{}, 400, "bad_request"},
		{"update: bad json", http.MethodPatch, "/reviews/" + id, "{", 400, "bad_request"},
		{"update: bad rating", http.MethodPatch, "/reviews/" + id, map[string]any{"rating": 0}, 400, "bad_request"},
		{"update: unknown id", http.MethodPatch, "/reviews/" + uuid.NewString(), map[string]any{}, 404, "not_found"},
		{"delete: malformed id", http.MethodDelete, "/reviews/nope", nil, 400, "bad_request"},
		{"delete: unknown id", http.MethodDelete, "/reviews/" + uuid.NewString(), nil, 404, "not_found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.expect(a.do(u, tc.method, tc.path, tc.body), tc.status, tc.code)
		})
	}
}

func TestReviewServerErrors(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		body               any
	}{
		{"list for title", http.MethodGet, "/movies/1/reviews", nil},
		{"mine", http.MethodGet, "/me/reviews", nil},
		{"get", http.MethodGet, "/reviews/{id}", nil},
		{"update", http.MethodPatch, "/reviews/{id}", map[string]any{"rating": 3}},
		{"delete", http.MethodDelete, "/reviews/{id}", nil},
		{"create", http.MethodPost, "/movies/2/reviews", map[string]any{"rating": 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newAPI(t)
			u := a.newUser("ann")
			r := a.postReview(u, "/movies/1/reviews", 5)
			testutil.FailOn(t, a.db, "query", "reviews")
			a.expect(a.do(u, tc.method, strings.ReplaceAll(tc.path, "{id}", r.ID.String()), tc.body), http.StatusInternalServerError, "internal_error")
		})
	}
}

func TestReviewRoutesNeedAnAuthenticatedUser(t *testing.T) {
	a := newAPI(t)
	id := uuid.NewString()
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/movies/1/reviews"}, {http.MethodPost, "/shows/1/reviews"},
		{http.MethodGet, "/me/reviews"}, {http.MethodPatch, "/reviews/" + id}, {http.MethodDelete, "/reviews/" + id},
	} {
		a.expect(a.do(uuid.Nil, tc.method, tc.path, nil), http.StatusInternalServerError, "internal_error")
	}
}
