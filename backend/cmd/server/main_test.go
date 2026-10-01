package main

import (
	"context"
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
	"github.com/aomarai/concession/internal/tmdb"
	"github.com/gin-gonic/gin"
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
	r := setupRouter(db, cfg, handlers.NewAuthHandler(db, cfg), handlers.NewUserHandler(db), handlers.NewCatalogHandler(catalog.NewService(db, tmdb.NewClient("t"))), logger)

	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/api/v1/me", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/search?q=x", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/movies/1", http.StatusUnauthorized},
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addrCh := make(chan net.Addr, 1)
	done := make(chan error, 1)
	go func() { done <- run(ctx, func(a net.Addr) { addrCh <- a }) }()
	addr := <-addrCh

	// An unfinished request keeps the connection active so Shutdown has to
	// wait for it and hits the (tiny) timeout.
	conn, err := net.Dial("tcp", "127.0.0.1:"+portOf(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte("GET /healthz HTTP/1.1\r\nHost: x\r\n")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

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

			cancel()
			if err := <-done; err != nil {
				t.Errorf("shutdown: %v", err)
			}
		})
	}
}
