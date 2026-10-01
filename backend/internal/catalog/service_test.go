package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/tmdb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const movieJSON = `{"id":603,"imdb_id":"tt0133093","title":"The Matrix","original_title":"The Matrix",
"overview":"A hacker.","tagline":"Free your mind","poster_path":"/p.jpg","release_date":"1999-03-30",
"runtime":136,"vote_average":8.2,"vote_count":100,
"genres":[{"id":28,"name":"Action"},{"id":878,"name":"Science Fiction"}],
"credits":{"cast":[{"name":"Keanu Reeves"},{"name":"Carrie-Anne Moss"}]}}`

const showJSON = `{"id":1396,"name":"Breaking Bad","overview":"Chemistry.",
"genres":[{"id":18,"name":"Drama"}],
"seasons":[{"season_number":1,"name":"Season 1","air_date":"2008-01-20"},{"season_number":2,"name":"Season 2"}],
"external_ids":{"imdb_id":"tt0903747","tvdb_id":81189},
"content_ratings":{"results":[{"iso_3166_1":"US","rating":"TV-MA"}]},
"credits":{"cast":[{"name":"Bryan Cranston"}]}}`

const seasonJSON = `{"season_number":1,"name":"Season 1","episodes":[
{"episode_number":1,"season_number":1,"name":"Pilot","air_date":"2008-01-20","runtime":58,
 "guest_stars":[{"name":"Guest A"}],"crew":[{"name":"Vince Gilligan","job":"Writer"},{"name":"Dir D","job":"Director"}]},
{"episode_number":2,"season_number":1,"name":"Cat's in the Bag"}]}`

func newTestService(t *testing.T, hits *atomic.Int32) *Service {
	t.Helper()
	return newUpstreamService(t, hits, &sync.Map{})
}

// newUpstreamService is newTestService with per-path status overrides: store
// an int (status code) or string (response body) under a URL path to override
// the fake TMDB's answer for it.
func newUpstreamService(t *testing.T, hits *atomic.Int32, status *sync.Map) *Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if v, ok := status.Load(r.URL.Path); ok {
			switch v := v.(type) {
			case int:
				w.WriteHeader(v)
			case string: // body override
				_, _ = w.Write([]byte(v))
			}
			return
		}
		switch {
		case r.URL.Path == "/movie/603":
			_, _ = w.Write([]byte(movieJSON))
		case r.URL.Path == "/tv/1396":
			_, _ = w.Write([]byte(showJSON))
		case r.URL.Path == "/tv/1396/season/1":
			_, _ = w.Write([]byte(seasonJSON))
		case strings.HasPrefix(r.URL.Path, "/genre/"):
			_, _ = w.Write([]byte(`{"genres":[{"id":28,"name":"Action"},{"id":18,"name":"Drama"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	name := strings.ReplaceAll(t.Name(), "/", "_")
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close() // drops the named in-memory DB so -count=N reruns start clean
		}
	})
	if err := db.AutoMigrate(&domain.Movie{}, &domain.Show{}, &domain.Season{}, &domain.Episode{}, &domain.Genre{}, &domain.Review{}); err != nil {
		t.Fatal(err)
	}
	return NewService(db, tmdb.NewClient("tok", tmdb.WithBaseURL(srv.URL), tmdb.WithCacheTTL(0)))
}

func TestEnsureMovieStoresAndCaches(t *testing.T) {
	var hits atomic.Int32
	s := newTestService(t, &hits)
	ctx := context.Background()

	m, err := s.EnsureMovie(ctx, 603)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == 0 || m.Title != "The Matrix" || m.TMDBID != 603 || len(m.Genres) != 2 {
		t.Fatalf("unexpected movie: %+v", m)
	}
	if m.ReleaseDate.Year() != 1999 || len(m.Actors) != 2 {
		t.Errorf("date/actors not mapped: %v %v", m.ReleaseDate, m.Actors)
	}

	again, err := s.EnsureMovie(ctx, 603)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != m.ID || hits.Load() != 1 {
		t.Errorf("expected fresh DB hit (id %d vs %d, upstream hits %d)", again.ID, m.ID, hits.Load())
	}
	if len(again.Genres) != 2 {
		t.Errorf("genres not preloaded: %d", len(again.Genres))
	}
}

func TestEnsureMovieRefreshesStaleKeepingID(t *testing.T) {
	var hits atomic.Int32
	s := newTestService(t, &hits)
	ctx := context.Background()

	m, _ := s.EnsureMovie(ctx, 603)
	old := time.Now().Add(-48 * time.Hour)
	s.DB.Model(&domain.Movie{}).Where("id = ?", m.ID).UpdateColumn("updated_at", old)

	refreshed, err := s.EnsureMovie(ctx, 603)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ID != m.ID || hits.Load() != 2 {
		t.Errorf("expected refresh with same ID; id %d vs %d, hits %d", refreshed.ID, m.ID, hits.Load())
	}
	var count int64
	s.DB.Model(&domain.Movie{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 movie row, got %d", count)
	}
}

func TestEnsureMovieNotFound(t *testing.T) {
	var hits atomic.Int32
	s := newTestService(t, &hits)
	if _, err := s.EnsureMovie(context.Background(), 1); err != tmdb.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestEnsureShowCapturesTVDBAndSeasons(t *testing.T) {
	var hits atomic.Int32
	s := newTestService(t, &hits)

	show, err := s.EnsureShow(context.Background(), 1396)
	if err != nil {
		t.Fatal(err)
	}
	if show.TVDBID == nil || *show.TVDBID != 81189 {
		t.Errorf("tvdb id = %v", show.TVDBID)
	}
	if show.TMDBID == nil || *show.TMDBID != 1396 || show.ContentRating != "TV-MA" {
		t.Errorf("unexpected show: %+v", show)
	}
	if len(show.Seasons) != 2 || show.Seasons[0].SeasonNumber != 1 || len(show.Genres) != 1 {
		t.Errorf("seasons/genres wrong: %+v", show)
	}
}

func TestEnsureSeasonStoresEpisodesIdempotently(t *testing.T) {
	var hits atomic.Int32
	s := newTestService(t, &hits)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		season, err := s.EnsureSeason(ctx, 1396, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(season.Episodes) != 2 || season.Episodes[0].Title != "Pilot" {
			t.Fatalf("episodes wrong: %+v", season.Episodes)
		}
		ep := season.Episodes[0]
		if len(ep.Writers) != 1 || ep.Writers[0] != "Vince Gilligan" || len(ep.Directors) != 1 || len(ep.GuestStars) != 1 {
			t.Errorf("crew mapping wrong: %+v", ep)
		}
	}
	var eps, seasons int64
	s.DB.Model(&domain.Episode{}).Count(&eps)
	s.DB.Model(&domain.Season{}).Count(&seasons)
	if eps != 2 || seasons != 2 {
		t.Errorf("expected 2 episodes and 2 seasons, got %d and %d", eps, seasons)
	}
}

func TestSyncGenres(t *testing.T) {
	var hits atomic.Int32
	s := newTestService(t, &hits)
	if err := s.SyncGenres(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.SyncGenres(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n int64
	s.DB.Model(&domain.Genre{}).Count(&n)
	if n != 2 {
		t.Errorf("expected 2 genres, got %d", n)
	}
}

func TestSearchEmptyQueryShortCircuits(t *testing.T) {
	var hits atomic.Int32
	s := newTestService(t, &hits)
	res, err := s.Search(context.Background(), "  ", 1)
	if err != nil || len(res.Results) != 0 || hits.Load() != 0 {
		t.Errorf("res=%v err=%v hits=%d", res, err, hits.Load())
	}
}
