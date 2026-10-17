package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/auth"
	"github.com/aomarai/concession/internal/catalog"
	"github.com/aomarai/concession/internal/config"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/events"
	"github.com/aomarai/concession/internal/friends"
	"github.com/aomarai/concession/internal/handlers"
	"github.com/aomarai/concession/internal/logging"
	"github.com/aomarai/concession/internal/notifications"
	"github.com/aomarai/concession/internal/progress"
	"github.com/aomarai/concession/internal/reviews"
	"github.com/aomarai/concession/internal/tmdb"
	"github.com/aomarai/concession/internal/watchlist"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// limitedRouter builds the real router over a private in-memory database.
func limitedRouter(t *testing.T, cfg *config.Config, logger *slog.Logger) (*gin.Engine, *gorm.DB) {
	return limitedRouterNamed(t, cfg, logger, "")
}

// limitedRouterNamed lets one test build several routers (each needs its own
// database name).
func limitedRouterNamed(t *testing.T, cfg *config.Config, logger *slog.Logger, suffix string) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg.SessionCookieName = "session_token"
	db, err := initDB(&config.Config{DBDriver: "sqlite", DBPath: "file:" + t.Name() + suffix + "?mode=memory&cache=private"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeGormDB(db) })
	if logger == nil {
		logger = logging.NewLogger(&config.Config{Environment: "test"})
	}
	catalogSvc := catalog.NewService(db, tmdb.NewClient("t"))
	watchlistSvc := watchlist.NewService(db, catalogSvc)
	r := setupRouter(db, cfg, apiHandlers{
		Auth:      handlers.NewAuthHandler(db, cfg),
		User:      handlers.NewUserHandler(db),
		Catalog:   handlers.NewCatalogHandler(catalogSvc),
		Watchlist: handlers.NewWatchlistHandler(watchlistSvc),
		Collab:    handlers.NewCollaborationHandler(watchlistSvc),
		Progress:  handlers.NewProgressHandler(progress.NewService(db, catalogSvc)),
		Reviews:   handlers.NewReviewHandler(reviews.NewService(db, catalogSvc)),
		Notifs:    handlers.NewNotificationHandler(notifications.NewService(db)),
		Friends:   handlers.NewFriendHandler(friends.NewService(db)),
		Events:    handlers.NewEventsHandler(watchlistSvc, events.NewHub(), time.Minute),
	}, logger)
	return r, db
}

func newUserWithSession(t *testing.T, db *gorm.DB, name string) (domain.User, *http.Cookie) {
	t.Helper()
	u := domain.User{Username: name, Email: name + "@example.com", DisplayName: name}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	token, err := auth.CreateSession(context.Background(), db, u.ID, "ua", "ip")
	if err != nil {
		t.Fatal(err)
	}
	return u, &http.Cookie{Name: "session_token", Value: token}
}

func call(r *gin.Engine, method, path, remote string, cookie *http.Cookie, header ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(`{"user":"nobody","role":"editor"}`))
	req.RemoteAddr = remote
	if cookie != nil {
		req.AddCookie(cookie)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSignInRoutesAreLimitedPerClientIP(t *testing.T) {
	r, _ := limitedRouter(t, &config.Config{RateLimitAuthPerMinute: 2}, nil)
	ip := "198.51.100.1:5000"
	for i := 0; i < 2; i++ {
		if w := call(r, http.MethodGet, "/api/v1/auth/google/login", ip, nil); w.Code != http.StatusTemporaryRedirect {
			t.Fatalf("login %d = %d", i+1, w.Code)
		}
	}
	w := call(r, http.MethodGet, "/api/v1/auth/google/callback", ip, nil) // shares the budget
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), `"rate_limited"`) {
		t.Errorf("callback = %d %s", w.Code, w.Body)
	}
	if secs, err := strconv.Atoi(w.Header().Get("Retry-After")); err != nil || secs < 1 {
		t.Errorf("Retry-After = %q", w.Header().Get("Retry-After"))
	}
	if w := call(r, http.MethodPost, "/api/v1/auth/logout", ip, nil); w.Code != http.StatusTooManyRequests {
		t.Errorf("logout from the same IP = %d, want 429", w.Code)
	}
	if w := call(r, http.MethodGet, "/api/v1/auth/google/login", "198.51.100.2:5000", nil); w.Code != http.StatusTemporaryRedirect {
		t.Errorf("another IP = %d, want 307", w.Code)
	}
}

func TestAPILimitIsPerAuthenticatedUser(t *testing.T) {
	r, db := limitedRouter(t, &config.Config{RateLimitAPIPerMinute: 3}, nil)
	_, ada := newUserWithSession(t, db, "ada")
	_, grace := newUserWithSession(t, db, "grace")
	for i := 0; i < 3; i++ {
		if w := call(r, http.MethodGet, "/api/v1/me", "203.0.113.9:1", ada); w.Code != http.StatusOK {
			t.Fatalf("request %d = %d", i+1, w.Code)
		}
	}
	if w := call(r, http.MethodGet, "/api/v1/me", "203.0.113.9:1", ada); w.Code != http.StatusTooManyRequests {
		t.Errorf("fourth request = %d, want 429", w.Code)
	}
	// Same IP, other user: own budget. Anonymous requests are 401s, not counted.
	if w := call(r, http.MethodGet, "/api/v1/me", "203.0.113.9:1", grace); w.Code != http.StatusOK {
		t.Errorf("another user = %d, want 200", w.Code)
	}
	for i := 0; i < 5; i++ {
		if w := call(r, http.MethodGet, "/api/v1/me", "203.0.113.9:1", nil); w.Code != http.StatusUnauthorized {
			t.Errorf("anonymous = %d, want 401", w.Code)
		}
	}
}

func TestAnonymousRequestsCountAgainstThePerIPLimit(t *testing.T) {
	// Every request with a bad session costs a database lookup before the
	// per-user limit can apply, so authenticated routes also have a per-IP
	// budget that is charged before the session is checked.
	r, db := limitedRouter(t, &config.Config{RateLimitIPPerMinute: 3}, nil)
	_, ada := newUserWithSession(t, db, "ada")
	bad := &http.Cookie{Name: "session_token", Value: "not-a-real-session"}
	ip := "203.0.113.50:1"
	if w := call(r, http.MethodGet, "/api/v1/me", ip, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("no cookie = %d, want 401", w.Code)
	}
	if w := call(r, http.MethodGet, "/api/v1/me", ip, bad); w.Code != http.StatusUnauthorized {
		t.Errorf("bad cookie = %d, want 401", w.Code)
	}
	if w := call(r, http.MethodGet, "/api/v1/me", ip, ada); w.Code != http.StatusOK {
		t.Errorf("valid session = %d, want 200", w.Code)
	}
	for _, c := range []*http.Cookie{nil, bad, ada} {
		if w := call(r, http.MethodGet, "/api/v1/me", ip, c); w.Code != http.StatusTooManyRequests {
			t.Errorf("over the per-IP budget = %d, want 429", w.Code)
		}
	}
	if w := call(r, http.MethodGet, "/api/v1/me", "203.0.113.51:1", ada); w.Code != http.StatusOK {
		t.Errorf("another IP = %d, want 200", w.Code)
	}
}

func TestOnlyXForwardedForIsTrusted(t *testing.T) {
	// X-Real-IP is whatever the client sent unless the proxy overwrites it, so
	// it must not pick the client's identity even from a trusted proxy.
	r, _ := limitedRouter(t, &config.Config{RateLimitAuthPerMinute: 1, TrustedProxies: []string{"10.0.0.0/8"}}, nil)
	login := func(realIP string) int {
		return call(r, http.MethodGet, "/api/v1/auth/google/login", "10.1.1.1:80", nil, "X-Real-IP", realIP).Code
	}
	if login("198.51.100.1") != http.StatusTemporaryRedirect {
		t.Fatal("the first request should pass")
	}
	if login("198.51.100.2") != http.StatusTooManyRequests {
		t.Error("a different X-Real-IP must not buy a fresh budget")
	}
}

func TestFriendRequestAndInviteLimitsAreWired(t *testing.T) {
	r, db := limitedRouter(t, &config.Config{RateLimitFriendRequestsPerHour: 1, RateLimitInvitesPerHour: 1}, nil)
	_, ada := newUserWithSession(t, db, "ada")
	invite := "/api/v1/watchlists/00000000-0000-0000-0000-000000000000/collaborators"
	for _, path := range []string{"/api/v1/friends", invite} {
		first := call(r, http.MethodPost, path, "203.0.113.9:1", ada)
		if first.Code == http.StatusTooManyRequests {
			t.Fatalf("POST %s: the first request must not be limited", path)
		}
		if second := call(r, http.MethodPost, path, "203.0.113.9:1", ada); second.Code != http.StatusTooManyRequests {
			t.Errorf("POST %s second = %d, want 429", path, second.Code)
		}
	}
}

func TestEventStreamCapIsWired(t *testing.T) {
	r, db := limitedRouter(t, &config.Config{MaxEventStreamsPerUser: 1}, nil)
	ada, cookie := newUserWithSession(t, db, "ada")
	list := domain.Watchlist{OwnerID: ada.ID, Title: "t", Type: domain.WatchlistTypeMovie, Privacy: domain.PrivacyPrivate}
	if err := db.Create(&list).Error; err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	open := func(ctx context.Context) *http.Response {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/watchlists/"+list.ID.String()+"/events", nil)
		req.AddCookie(cookie)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := open(ctx)
	defer func() { _ = first.Body.Close() }()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first stream = %d", first.StatusCode)
	}
	second := open(context.Background())
	defer func() { _ = second.Body.Close() }()
	if second.StatusCode != http.StatusTooManyRequests {
		t.Errorf("second stream = %d, want 429", second.StatusCode)
	}
}

func TestClientIPComesFromTrustedProxiesOnly(t *testing.T) {
	// Behind a trusted proxy, clients are told apart by X-Forwarded-For.
	r, _ := limitedRouter(t, &config.Config{RateLimitAuthPerMinute: 1, TrustedProxies: []string{"10.0.0.0/8"}}, nil)
	login := func(xff string) int {
		return call(r, http.MethodGet, "/api/v1/auth/google/login", "10.1.1.1:80", nil, "X-Forwarded-For", xff).Code
	}
	if login("203.0.113.7") != http.StatusTemporaryRedirect || login("203.0.113.8") != http.StatusTemporaryRedirect {
		t.Error("two clients behind one trusted proxy have separate budgets")
	}
	if login("203.0.113.7") != http.StatusTooManyRequests {
		t.Error("the same forwarded client must be limited")
	}

	// With no trusted proxies a spoofed header is ignored: the TCP peer counts.
	r, _ = limitedRouterNamed(t, &config.Config{RateLimitAuthPerMinute: 1}, nil, "-untrusted")
	spoof := func(xff string) int {
		return call(r, http.MethodGet, "/api/v1/auth/google/login", "192.0.2.1:80", nil, "X-Forwarded-For", xff).Code
	}
	if spoof("203.0.113.7") != http.StatusTemporaryRedirect || spoof("203.0.113.8") != http.StatusTooManyRequests {
		t.Error("a forged X-Forwarded-For must not buy a fresh budget")
	}
}

func TestInvalidTrustedProxiesAreLoggedNotFatal(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	r, _ := limitedRouter(t, &config.Config{TrustedProxies: []string{"definitely-not-a-cidr"}}, logger)
	if w := call(r, http.MethodGet, "/healthz", "192.0.2.1:80", nil); w.Code != http.StatusOK {
		t.Errorf("healthz = %d", w.Code)
	}
	if !strings.Contains(buf.String(), "trusted proxies") {
		t.Errorf("expected a log line about trusted proxies, got %q", buf.String())
	}
}
