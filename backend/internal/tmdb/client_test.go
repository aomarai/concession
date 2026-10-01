package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
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
