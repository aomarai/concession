package tmdb

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientSendsBearerAndFiltersPeople(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/search/multi" || r.URL.Query().Get("query") != "dune" {
			t.Errorf("unexpected request %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"page":1,"results":[
			{"id":1,"media_type":"movie","title":"Dune"},
			{"id":2,"media_type":"person","name":"Someone"},
			{"id":3,"media_type":"tv","name":"Dune: Prophecy"}]}`))
	}))
	defer srv.Close()

	c := NewClient("tok", WithBaseURL(srv.URL))
	res, err := c.SearchMulti(context.Background(), "dune", 0)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if len(res.Results) != 2 {
		t.Fatalf("expected people filtered out, got %d results", len(res.Results))
	}
}

func TestClientNotFoundAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/movie/404" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := NewClient("tok", WithBaseURL(srv.URL))

	if _, err := c.GetMovie(context.Background(), 404); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if _, err := c.GetMovie(context.Background(), 1); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("expected generic error, got %v", err)
	}
}

func TestClientCachesSuccessfulResponses(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"id":1,"title":"X"}`))
	}))
	defer srv.Close()

	c := NewClient("tok", WithBaseURL(srv.URL))
	for i := 0; i < 3; i++ {
		if _, err := c.GetMovie(context.Background(), 1); err != nil {
			t.Fatal(err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("expected 1 upstream hit, got %d", hits.Load())
	}

	hits.Store(0)
	nc := NewClient("tok", WithBaseURL(srv.URL), WithCacheTTL(0))
	_, _ = nc.GetMovie(context.Background(), 1)
	_, _ = nc.GetMovie(context.Background(), 1)
	if hits.Load() != 2 {
		t.Errorf("expected 2 hits with cache disabled, got %d", hits.Load())
	}
}

func TestShowContentRating(t *testing.T) {
	var s Show
	if s.ContentRating() != "" {
		t.Error("expected empty rating")
	}
	s.ContentRatings.Results = []struct {
		Country string `json:"iso_3166_1"`
		Rating  string `json:"rating"`
	}{{"DE", "16"}, {"US", "TV-MA"}}
	if s.ContentRating() != "TV-MA" {
		t.Errorf("got %q", s.ContentRating())
	}
}

func TestShowSeasonAndGenreEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tv/1396":
			if r.URL.Query().Get("append_to_response") != "external_ids,content_ratings,credits" {
				t.Errorf("unexpected append_to_response %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"id":1396,"name":"BB","external_ids":{"tvdb_id":81189}}`))
		case "/tv/1396/season/2":
			_, _ = w.Write([]byte(`{"season_number":2,"episodes":[{"episode_number":1,"name":"Seven Thirty-Seven"}]}`))
		case "/genre/movie/list":
			_, _ = w.Write([]byte(`{"genres":[{"id":1,"name":"A"},{"id":2,"name":"B"}]}`))
		case "/genre/tv/list":
			_, _ = w.Write([]byte(`{"genres":[{"id":2,"name":"B"},{"id":3,"name":"C"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := NewClient("tok", WithBaseURL(srv.URL+"/")) // trailing slash is trimmed
	ctx := context.Background()

	show, err := c.GetShow(ctx, 1396)
	if err != nil || show.ExternalIDs.TVDBID == nil || *show.ExternalIDs.TVDBID != 81189 {
		t.Errorf("show: %+v, %v", show, err)
	}
	season, err := c.GetSeason(ctx, 1396, 2)
	if err != nil || len(season.Episodes) != 1 || season.Episodes[0].Name != "Seven Thirty-Seven" {
		t.Errorf("season: %+v, %v", season, err)
	}
	genres, err := c.Genres(ctx)
	if err != nil || len(genres) != 3 {
		t.Errorf("genres should be de-duplicated across movie and tv lists: %+v, %v", genres, err)
	}

	if _, err := c.GetShow(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("show 404: %v", err)
	}
	if _, err := c.GetSeason(ctx, 1396, 9); !errors.Is(err, ErrNotFound) {
		t.Errorf("season 404: %v", err)
	}
}

func TestGenresPropagatesErrors(t *testing.T) {
	for _, failing := range []string{"/genre/movie/list", "/genre/tv/list"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == failing {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"genres":[]}`))
		}))
		if _, err := NewClient("t", WithBaseURL(srv.URL)).Genres(context.Background()); err == nil {
			t.Errorf("%s: expected error", failing)
		}
		srv.Close()
	}
}

func TestSearchErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := NewClient("t", WithBaseURL(srv.URL)).SearchMulti(context.Background(), "x", 1); err == nil {
		t.Error("expected error")
	}
}

func TestRequestFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid URL", func(t *testing.T) {
		c := NewClient("t", WithBaseURL("http://bad host/"))
		if _, err := c.GetMovie(ctx, 1); err == nil {
			t.Error("expected error building request")
		}
	})

	t.Run("custom http client is used and its errors wrapped", func(t *testing.T) {
		boom := errors.New("network down")
		hc := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, boom })}
		c := NewClient("t", WithHTTPClient(hc))
		if _, err := c.GetMovie(ctx, 1); !errors.Is(err, boom) {
			t.Errorf("expected wrapped transport error, got %v", err)
		}
	})

	t.Run("body read error", func(t *testing.T) {
		hc := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(errReader{})}, nil
		})}
		if _, err := NewClient("t", WithHTTPClient(hc)).GetMovie(ctx, 1); err == nil {
			t.Error("expected read error")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{")) }))
		defer srv.Close()
		if _, err := NewClient("t", WithBaseURL(srv.URL)).GetMovie(ctx, 1); err == nil {
			t.Error("expected decode error")
		}
	})

	t.Run("empty base URL option keeps default", func(t *testing.T) {
		if c := NewClient("t", WithBaseURL("")); c.baseURL != DefaultBaseURL {
			t.Errorf("baseURL = %q", c.baseURL)
		}
	})
}

func TestCacheExpires(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()
	c := NewClient("t", WithBaseURL(srv.URL), WithCacheTTL(10*time.Millisecond))
	_, _ = c.GetMovie(context.Background(), 1)
	time.Sleep(30 * time.Millisecond)
	_, _ = c.GetMovie(context.Background(), 1)
	if hits.Load() != 2 {
		t.Errorf("expected expired entry to be refetched, hits = %d", hits.Load())
	}
}

func TestContentRatingFallsBackToFirst(t *testing.T) {
	var s Show
	s.ContentRatings.Results = []struct {
		Country string `json:"iso_3166_1"`
		Rating  string `json:"rating"`
	}{{"DE", "16"}}
	if s.ContentRating() != "16" {
		t.Errorf("got %q", s.ContentRating())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
