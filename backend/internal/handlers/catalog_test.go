package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aomarai/concession/internal/catalog"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/tmdb"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newCatalogRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/multi":
			_, _ = w.Write([]byte(`{"page":1,"results":[{"id":603,"media_type":"movie","title":"The Matrix"}]}`))
		case "/movie/603":
			_, _ = w.Write([]byte(`{"id":603,"title":"The Matrix","genres":[],"credits":{"cast":[]}}`))
		case "/movie/500":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)

	db, err := gorm.Open(sqlite.Open("file:catalog_handler?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.Movie{}, &domain.Show{}, &domain.Season{}, &domain.Episode{}, &domain.Genre{}, &domain.Review{}); err != nil {
		t.Fatal(err)
	}
	svc := catalog.NewService(db, tmdb.NewClient("tok", tmdb.WithBaseURL(upstream.URL)))
	r := gin.New()
	NewCatalogHandler(svc).RegisterRoutes(r.Group("/api/v1"))
	return r
}

func doGet(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestCatalogHandlers(t *testing.T) {
	r := newCatalogRouter(t)

	cases := []struct {
		name, path string
		status     int
		code       string
	}{
		{"search ok", "/api/v1/search?q=matrix", 200, ""},
		{"search bad page", "/api/v1/search?q=x&page=0", 400, "bad_request"},
		{"movie ok", "/api/v1/movies/603", 200, ""},
		{"movie not found", "/api/v1/movies/999", 404, "not_found"},
		{"movie upstream failure", "/api/v1/movies/500", 502, "upstream_error"},
		{"movie bad id", "/api/v1/movies/abc", 400, "bad_request"},
		{"season bad number", "/api/v1/shows/1/seasons/x", 400, "bad_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doGet(r, tc.path)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d (%s)", w.Code, tc.status, w.Body)
			}
			if tc.code != "" {
				var e ErrorResponse
				if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || e.Error.Code != tc.code {
					t.Errorf("error body = %s", w.Body)
				}
			}
		})
	}
}

func TestMovieEndpointReturnsStoredMovie(t *testing.T) {
	r := newCatalogRouter(t)
	w := doGet(r, "/api/v1/movies/603")
	var m domain.Movie
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.ID == 0 || m.TMDBID != 603 {
		t.Errorf("unexpected movie %+v", m)
	}
}
