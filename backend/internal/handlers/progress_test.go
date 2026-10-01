package handlers

import (
	"net/http"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/progress"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
)

type progressList struct {
	Progress []progress.View `json:"progress"`
}

func TestProgressLifecycle(t *testing.T) {
	a := newAPI(t)
	u, other := uuid.New(), uuid.New()

	w := a.do(u, http.MethodPut, "/me/progress/movies/603", map[string]any{"status": "completed"})
	a.expect(w, http.StatusOK, "")
	if v := decode[progress.View](t, w); v.Status != domain.StatusCompleted || v.Movie == nil || v.Movie.Title != "Movie 603" {
		t.Errorf("set movie: %+v", v)
	}
	w = a.do(u, http.MethodPut, "/me/progress/shows/1396", map[string]any{"status": "watching", "last_season_num": 2, "last_episode_num": 5})
	a.expect(w, http.StatusOK, "")
	if v := decode[progress.View](t, w); v.LastSeasonNum != 2 || v.LastEpisodeNum != 5 || v.Show == nil {
		t.Errorf("set show: %+v", v)
	}
	a.do(other, http.MethodPut, "/me/progress/movies/603", map[string]any{"status": "dropped"})

	if v := decode[progress.View](t, a.do(u, http.MethodGet, "/me/progress/shows/1396", nil)); v.Status != domain.StatusWatching {
		t.Errorf("get: %+v", v)
	}

	all := decode[progressList](t, a.do(u, http.MethodGet, "/me/progress", nil))
	if len(all.Progress) != 2 {
		t.Errorf("all: %+v", all)
	}
	shows := decode[progressList](t, a.do(u, http.MethodGet, "/me/progress?kind=shows", nil))
	if len(shows.Progress) != 1 || shows.Progress[0].Show == nil {
		t.Errorf("shows: %+v", shows)
	}
	done := decode[progressList](t, a.do(u, http.MethodGet, "/me/progress?status=completed", nil))
	if len(done.Progress) != 1 {
		t.Errorf("completed: %+v", done)
	}

	a.expect(a.do(u, http.MethodDelete, "/me/progress/movies/603", nil), http.StatusNoContent, "")
	a.expect(a.do(u, http.MethodGet, "/me/progress/movies/603", nil), http.StatusNotFound, "not_found")
	a.expect(a.do(u, http.MethodDelete, "/me/progress/movies/603", nil), http.StatusNotFound, "not_found")
}

func TestProgressRequestErrors(t *testing.T) {
	a := newAPI(t)
	u := uuid.New()
	for _, tc := range []struct {
		name, method, path string
		body               any
		status             int
		code               string
	}{
		{"list: bad kind", http.MethodGet, "/me/progress?kind=books", nil, 400, "bad_request"},
		{"list: bad status", http.MethodGet, "/me/progress?status=binged", nil, 400, "bad_request"},
		{"get: bad kind", http.MethodGet, "/me/progress/books/1", nil, 400, "bad_request"},
		{"get: bad id", http.MethodGet, "/me/progress/movies/abc", nil, 400, "bad_request"},
		{"get: not tracked", http.MethodGet, "/me/progress/movies/1", nil, 404, "not_found"},
		{"set: bad json", http.MethodPut, "/me/progress/movies/1", "{", 400, "bad_request"},
		{"set: bad status", http.MethodPut, "/me/progress/movies/1", map[string]any{"status": "binged"}, 400, "bad_request"},
		{"set: movie with season", http.MethodPut, "/me/progress/movies/1", map[string]any{"status": "watching", "last_season_num": 1}, 400, "bad_request"},
		{"set: negative season", http.MethodPut, "/me/progress/shows/1", map[string]any{"status": "watching", "last_season_num": -1}, 400, "bad_request"},
		{"set: bad kind", http.MethodPut, "/me/progress/books/1", map[string]any{"status": "watching"}, 400, "bad_request"},
		{"set: TMDB has no such title", http.MethodPut, "/me/progress/movies/404", map[string]any{"status": "watching"}, 404, "not_found"},
		{"set: TMDB unreachable", http.MethodPut, "/me/progress/shows/502", map[string]any{"status": "watching"}, 502, "upstream_error"},
		{"delete: bad kind", http.MethodDelete, "/me/progress/books/1", nil, 400, "bad_request"},
		{"delete: not tracked", http.MethodDelete, "/me/progress/shows/1", nil, 404, "not_found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.expect(a.do(u, tc.method, tc.path, tc.body), tc.status, tc.code)
		})
	}
}

func TestProgressServerError(t *testing.T) {
	a := newAPI(t)
	u := uuid.New()
	testutil.FailOn(t, a.db, "query", "user_watch_progresses")
	a.expect(a.do(u, http.MethodGet, "/me/progress", nil), http.StatusInternalServerError, "internal_error")
}
