package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/auth"
	"github.com/aomarai/concession/internal/catalog"
	"github.com/aomarai/concession/internal/config"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/handlers"
	"github.com/aomarai/concession/internal/logging"
	"github.com/aomarai/concession/internal/progress"
	"github.com/aomarai/concession/internal/reviews"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/aomarai/concession/internal/tmdb"
	"github.com/aomarai/concession/internal/watchlist"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// serverEnv points the app at an isolated in-memory SQLite DB on a random port.
func serverEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ENV", "test")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_PATH", "file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared")
	t.Setenv("PORT", "0")
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173")
}

func TestRunServesAndShutsDownGracefully(t *testing.T) {
	serverEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrCh := make(chan net.Addr, 1)
	done := make(chan error, 1)
	go func() { done <- run(ctx, func(a net.Addr) { addrCh <- a }) }()

	var base string
	select {
	case a := <-addrCh:
		base = "http://127.0.0.1:" + portOf(a)
	case err := <-done:
		t.Fatalf("run exited early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("server did not become ready")
	}

	get := func(path string, hdr map[string]string) *http.Response {
		req, _ := http.NewRequest(http.MethodGet, base+path, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	if resp := get("/healthz", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz = %d", resp.StatusCode)
	}
	if resp := get("/api/v1/me", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("/api/v1/me without session = %d, want 401", resp.StatusCode)
	}
	resp := get("/healthz", map[string]string{"Origin": "http://localhost:5173"})
	if resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Error("expected CORS header for the dev origin")
	}
	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("expected request logger middleware to set X-Request-ID")
	}
	_, _ = io.Copy(io.Discard, resp.Body)

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("graceful shutdown returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func portOf(a net.Addr) string {
	_, p, _ := net.SplitHostPort(a.String())
	return p
}

func TestRunConfigError(t *testing.T) {
	serverEnv(t)
	t.Setenv("COOKIE_SAME_SITE", "none") // requires COOKIE_SECURE=true
	err := run(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "load configuration") {
		t.Errorf("expected config error, got %v", err)
	}
}

func TestRunDatabaseError(t *testing.T) {
	serverEnv(t)
	t.Setenv("DB_PATH", "/nonexistent-dir/definitely/not/here.db")
	err := run(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "initialize database") {
		t.Errorf("expected database error, got %v", err)
	}
}

func TestRunListenError(t *testing.T) {
	serverEnv(t)
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	t.Setenv("PORT", portOf(ln.Addr()))

	err = run(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "listen") {
		t.Errorf("expected listen error, got %v", err)
	}
}

func TestRunDefaultsToPort8080WhenUnset(t *testing.T) {
	serverEnv(t)
	t.Setenv("PORT", "")
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		t.Skip("port 8080 unavailable on this machine")
	}
	_ = ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan net.Addr, 1)
	done := make(chan error, 1)
	go func() { done <- run(ctx, func(a net.Addr) { got <- a }) }()
	select {
	case a := <-got:
		if portOf(a) != "8080" {
			t.Errorf("port = %s, want 8080", portOf(a))
		}
	case err := <-done:
		t.Fatalf("run exited: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("not ready")
	}
	cancel()
	<-done
}

func TestInitDBPostgresUnreachable(t *testing.T) {
	cfg := &config.Config{DBDriver: "postgres", DBHost: "127.0.0.1", DBPort: "1", DBUser: "u", DBPassword: "p", DBName: "n"}
	if _, err := initDB(cfg); err == nil {
		t.Error("expected connection error for unreachable postgres")
	}
}

func TestInitDBSQLiteDefaultPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	db, err := initDB(&config.Config{DBDriver: "sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
}

func TestSetupRouterRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{CORSAllowedOrigins: []string{"http://x"}, SessionCookieName: "session_token"}
	db, err := initDB(&config.Config{DBDriver: "sqlite", DBPath: "file:router?mode=memory&cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	logger := logging.NewLogger(&config.Config{Environment: "test"})
	catalogSvc := catalog.NewService(db, tmdb.NewClient("t"))
	r := setupRouter(db, cfg, apiHandlers{
		Auth:      handlers.NewAuthHandler(db, cfg),
		User:      handlers.NewUserHandler(db),
		Catalog:   handlers.NewCatalogHandler(catalogSvc),
		Watchlist: handlers.NewWatchlistHandler(watchlist.NewService(db, catalogSvc)),
		Progress:  handlers.NewProgressHandler(progress.NewService(db, catalogSvc)),
		Reviews:   handlers.NewReviewHandler(reviews.NewService(db, catalogSvc)),
	}, logger)

	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/api/v1/me", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/search?q=x", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/movies/1", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/watchlists", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/me/progress", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/me/reviews", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/movies/1/reviews", http.StatusUnauthorized},
		{http.MethodPost, "/api/v1/auth/logout", http.StatusOK},
		{http.MethodGet, "/api/v1/auth/google/login", http.StatusTemporaryRedirect},
		{http.MethodGet, "/api/v1/nope", http.StatusNotFound},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, w.Code, tc.want)
		}
	}
}

func TestExecuteStopsOnSIGTERM(t *testing.T) {
	serverEnv(t)
	code := execute(func(net.Addr) {
		// execute registered its signal handler before calling ready, so this
		// is delivered to it rather than killing the test process.
		if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
			t.Error(err)
		}
	})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestExecuteReturnsNonZeroOnFailure(t *testing.T) {
	serverEnv(t)
	t.Setenv("DB_PATH", "/nonexistent-dir/x.db")
	if code := execute(nil); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRunReportsShutdownTimeout(t *testing.T) {
	serverEnv(t)
	old := shutdownTimeout
	shutdownTimeout = 50 * time.Millisecond
	t.Cleanup(func() { shutdownTimeout = old })

	// Shutdown waits for connections that are not idle. A connection that has
	// been accepted but has not finished sending its request headers stays in
	// StateNew, which counts, so wait for the server to accept ours.
	accepted := make(chan struct{}, 1)
	connStateHook = func(_ net.Conn, st http.ConnState) {
		if st == http.StateNew {
			select {
			case accepted <- struct{}{}:
			default:
			}
		}
	}
	t.Cleanup(func() { connStateHook = nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addrCh := make(chan net.Addr, 1)
	done := make(chan error, 1)
	go func() { done <- run(ctx, func(a net.Addr) { addrCh <- a }) }()
	addr := <-addrCh

	// An unfinished request keeps the connection busy so Shutdown has to wait
	// for it and hits the (tiny) timeout.
	conn, err := net.Dial("tcp", "127.0.0.1:"+portOf(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("GET /healthz HTTP/1.1\r\nHost: x\r\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("server never accepted the connection")
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected deadline exceeded, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return")
	}
}

func TestInitDBMigrationFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conflict.db")
	raw, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// A view named like a model table makes AutoMigrate's CREATE TABLE fail.
	if err := raw.Exec("CREATE VIEW users AS SELECT 1 AS id").Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := raw.DB()
	_ = sqlDB.Close()

	if _, err := initDB(&config.Config{DBDriver: "sqlite", DBPath: path}); err == nil {
		t.Error("expected migration error")
	}
}

// fakeTMDB serves just enough of the TMDB API for end-to-end tests and counts
// genre requests so tests can wait for the startup sync.
func fakeTMDB(t *testing.T, genreStatus int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var genreHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/genre/"):
			genreHits.Add(1)
			if genreStatus != http.StatusOK {
				w.WriteHeader(genreStatus)
				return
			}
			_, _ = io.WriteString(w, `{"genres":[{"id":28,"name":"Action"}]}`)
		case r.URL.Path == "/movie/603":
			_, _ = io.WriteString(w, `{"id":603,"title":"The Matrix","release_date":"1999-03-30","genres":[{"id":28,"name":"Action"}]}`)
		case r.URL.Path == "/search/multi":
			_, _ = io.WriteString(w, `{"page":1,"results":[{"id":603,"media_type":"movie","title":"The Matrix"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &genreHits
}

func TestRunEndToEndWithTMDB(t *testing.T) {
	for _, tc := range []struct {
		name        string
		genreStatus int
	}{{"genre sync ok", http.StatusOK}, {"genre sync fails but server still runs", http.StatusInternalServerError}} {
		t.Run(tc.name, func(t *testing.T) {
			serverEnv(t)
			tmdbSrv, genreHits := fakeTMDB(t, tc.genreStatus)
			t.Setenv("TMDB_READ_ACCESS_TOKEN", "test-token")
			t.Setenv("TMDB_BASE_URL", tmdbSrv.URL)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			addrCh := make(chan net.Addr, 1)
			done := make(chan error, 1)
			go func() { done <- run(ctx, func(a net.Addr) { addrCh <- a }) }()
			base := "http://127.0.0.1:" + portOf(<-addrCh)

			// Wait for the startup genre sync to reach the fake TMDB.
			deadline := time.Now().Add(5 * time.Second)
			for genreHits.Load() == 0 && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			if genreHits.Load() == 0 {
				t.Fatal("genre sync never called TMDB")
			}

			// Log in by creating a session directly in the app's (shared in-memory) DB.
			db, err := initDB(&config.Config{DBDriver: "sqlite", DBPath: os.Getenv("DB_PATH")})
			if err != nil {
				t.Fatal(err)
			}
			defer closeDB(db)
			user := domain.User{Username: "e2e", Email: "e2e@example.com", DisplayName: "E2E"}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			token, err := auth.CreateSession(ctx, db, user.ID, "ua", "ip")
			if err != nil {
				t.Fatal(err)
			}

			req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/search?q=matrix", nil)
			req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "The Matrix") {
				t.Errorf("search = %d %s", resp.StatusCode, body)
			}

			call := func(method, path, body string) (int, string) {
				req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = resp.Body.Close() }()
				b, _ := io.ReadAll(resp.Body)
				return resp.StatusCode, string(b)
			}

			// Watchlist flow: create a list, add a movie (fetched from TMDB), read it back.
			code, body2 := call(http.MethodPost, "/api/v1/watchlists", `{"title":"Friday","type":"movie"}`)
			if code != http.StatusCreated {
				t.Fatalf("create list = %d %s", code, body2)
			}
			listID := jsonField(t, body2, "id")
			if code, b := call(http.MethodPost, "/api/v1/watchlists/"+listID+"/items", `{"tmdb_id":603}`); code != http.StatusCreated {
				t.Fatalf("add item = %d %s", code, b)
			}
			if code, b := call(http.MethodGet, "/api/v1/watchlists/"+listID, ""); code != http.StatusOK || !strings.Contains(b, "The Matrix") {
				t.Errorf("get list = %d %s", code, b)
			}
			// New users get starter lists; this user was created directly so only ours exists.
			if code, b := call(http.MethodGet, "/api/v1/watchlists", ""); code != http.StatusOK || !strings.Contains(b, `"item_count":1`) {
				t.Errorf("list lists = %d %s", code, b)
			}

			// Progress flow.
			if code, b := call(http.MethodPut, "/api/v1/me/progress/movies/603", `{"status":"completed"}`); code != http.StatusOK {
				t.Errorf("set progress = %d %s", code, b)
			}
			if code, b := call(http.MethodGet, "/api/v1/me/progress?status=completed", ""); code != http.StatusOK || !strings.Contains(b, "The Matrix") {
				t.Errorf("list progress = %d %s", code, b)
			}

			// Review flow: rate the movie, then read the title's reviews and summary.
			code, b := call(http.MethodPost, "/api/v1/movies/603/reviews", `{"rating":9,"title":"Great","content":"Loved it"}`)
			if code != http.StatusCreated {
				t.Fatalf("create review = %d %s", code, b)
			}
			if code, b := call(http.MethodPost, "/api/v1/movies/603/reviews", `{"rating":5}`); code != http.StatusConflict {
				t.Errorf("second review = %d %s, want 409", code, b)
			}
			if code, b := call(http.MethodGet, "/api/v1/movies/603/reviews", ""); code != http.StatusOK ||
				!strings.Contains(b, `"average":9`) || !strings.Contains(b, "Loved it") {
				t.Errorf("list reviews = %d %s", code, b)
			}
			if code, b := call(http.MethodGet, "/api/v1/me/reviews", ""); code != http.StatusOK || !strings.Contains(b, "The Matrix") {
				t.Errorf("my reviews = %d %s", code, b)
			}

			cancel()
			if err := <-done; err != nil {
				t.Errorf("shutdown: %v", err)
			}
		})
	}
}

func jsonField(t *testing.T, body, field string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	s, _ := m[field].(string)
	return s
}

func TestRouterAnswersErrorsInTheStandardShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{SessionCookieName: "session_token"}
	db, err := initDB(&config.Config{DBDriver: "sqlite", DBPath: "file:" + t.Name() + "?mode=memory&cache=private"})
	if err != nil {
		t.Fatal(err)
	}
	defer closeGormDB(db)
	logger := logging.NewLogger(&config.Config{Environment: "test"})
	catalogSvc := catalog.NewService(db, tmdb.NewClient("t"))
	r := setupRouter(db, cfg, apiHandlers{
		Auth:      handlers.NewAuthHandler(db, cfg),
		User:      handlers.NewUserHandler(db),
		Catalog:   handlers.NewCatalogHandler(catalogSvc),
		Watchlist: handlers.NewWatchlistHandler(watchlist.NewService(db, catalogSvc)),
		Progress:  handlers.NewProgressHandler(progress.NewService(db, catalogSvc)),
		Reviews:   handlers.NewReviewHandler(reviews.NewService(db, catalogSvc)),
	}, logger)
	r.GET("/boom", func(*gin.Context) { panic("kaboom") })

	for _, tc := range []struct {
		method, path string
		status       int
		code         string
	}{
		{http.MethodGet, "/nope", http.StatusNotFound, "not_found"},
		{http.MethodDelete, "/healthz", http.StatusMethodNotAllowed, "method_not_allowed"},
		{http.MethodGet, "/boom", http.StatusInternalServerError, "internal_error"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		var body struct {
			Error struct{ Code, Message string }
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != tc.status || body.Error.Code != tc.code || body.Error.Message == "" {
			t.Errorf("%s %s = %d %q, want %d with code %s", tc.method, tc.path, w.Code, w.Body, tc.status, tc.code)
		}
	}
}

func TestInitDBNormalizesLegacyReviewTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	cfg := &config.Config{DBDriver: "sqlite", DBPath: path}

	db, err := initDB(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Rows written by an older version with the plural polymorphic values,
	// including one that was soft-deleted.
	for _, row := range []domain.Review{
		{UserID: uuid.New(), Rating: 5, ReviewableID: 1, ReviewableType: "movies"},
		{UserID: uuid.New(), Rating: 6, ReviewableID: 2, ReviewableType: "shows"},
		{UserID: uuid.New(), Rating: 7, ReviewableID: 3, ReviewableType: domain.ReviewableMovies},
	} {
		row := row
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Where("reviewable_id = ?", 2).Delete(&domain.Review{}).Error; err != nil {
		t.Fatal(err)
	}
	closeGormDB(db)

	for i := 0; i < 2; i++ { // second pass proves it is idempotent
		db, err = initDB(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var legacy int64
		db.Unscoped().Model(&domain.Review{}).Where("reviewable_type IN ?", []string{"movies", "shows"}).Count(&legacy)
		var movies, shows int64
		db.Unscoped().Model(&domain.Review{}).Where("reviewable_type = ?", domain.ReviewableMovies).Count(&movies)
		db.Unscoped().Model(&domain.Review{}).Where("reviewable_type = ?", domain.ReviewableShows).Count(&shows)
		closeGormDB(db)
		if legacy != 0 || movies != 2 || shows != 1 {
			t.Fatalf("pass %d: legacy=%d movies=%d shows=%d", i, legacy, movies, shows)
		}
	}
}

func TestMigrateReportsReviewNormalizationFailure(t *testing.T) {
	db := testutil.NewDB(t)
	testutil.FailOn(t, db, "update", "reviews")
	err := migrate(db)
	if err == nil || !strings.Contains(err.Error(), "normalize review types") {
		t.Errorf("expected a normalization error, got %v", err)
	}
}

// closeGormDB closes the pool so a named in-memory database does not outlive
// the test (matters when tests are repeated with -count=N).
func closeGormDB(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
