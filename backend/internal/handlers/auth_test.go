package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/auth"
	"github.com/aomarai/concession/internal/config"
	"github.com/aomarai/concession/internal/domain"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---- test setup -----------------------------------------------------------

func setupAuthHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&domain.User{}, &domain.OAuthAccount{}, &domain.Session{}); err != nil {
		t.Fatalf("failed to migrate models: %v", err)
	}
	return db
}

func newTestConfig(cookieSecure bool) *config.Config {
	return &config.Config{
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		GoogleRedirectURL:  "https://app.example.com/auth/google/callback",
		CookieSecure:       cookieSecure,
	}
}

// ginTestContext creates a Gin context from an httptest.Request so handlers
// that expect *gin.Context can be exercised directly.
func ginTestContext(t *testing.T, r *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	return c, w
}

// redirectingTransport rewrites every outgoing request's scheme+host to
// point at a local test server, regardless of what the request was
// originally addressed to.
type redirectingTransport struct {
	target *url.URL
}

func (t *redirectingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func withHijackedHTTPClient(ctx context.Context, testServerURL string) context.Context {
	u, err := url.Parse(testServerURL)
	if err != nil {
		panic(err)
	}
	client := &http.Client{Transport: &redirectingTransport{target: u}}
	return context.WithValue(ctx, oauth2.HTTPClient, client)
}

// ---- generateRandomToken ---------------------------------------------------

func TestGenerateRandomToken(t *testing.T) {
	t.Run("returns a 32-byte URL-safe base64 token", func(t *testing.T) {
		token, err := generateRandomToken()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		decoded, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			t.Fatalf("token is not valid RawURLEncoding base64: %v", err)
		}
		if len(decoded) != 32 {
			t.Errorf("expected 32 decoded bytes, got %d", len(decoded))
		}
	})

	t.Run("generates unique tokens", func(t *testing.T) {
		a, err := generateRandomToken()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		b, err := generateRandomToken()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a == b {
			t.Error("expected two calls to produce different tokens")
		}
	})
}

// ---- HandleGoogleLogin ------------------------------------------------------

func TestHandleGoogleLogin(t *testing.T) {
	db := setupAuthHandlerTestDB(t)

	t.Run("sets oauth_state cookie honoring CookieSecure", func(t *testing.T) {
		h := NewAuthHandler(db, newTestConfig(true))
		r := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
		c, w := ginTestContext(t, r)

		h.HandleGoogleLogin(c)

		resp := w.Result()
		var stateCookie *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "oauth_state" {
				stateCookie = c
			}
		}
		if stateCookie == nil {
			t.Fatal("expected oauth_state cookie to be set")
		}
		if stateCookie.Value == "" {
			t.Error("expected oauth_state cookie to have a non-empty value")
		}
		if !stateCookie.HttpOnly {
			t.Error("expected oauth_state cookie to be HttpOnly")
		}
		if !stateCookie.Secure {
			t.Error("expected oauth_state cookie to be Secure when CookieSecure=true")
		}
		if stateCookie.SameSite != http.SameSiteLaxMode {
			t.Errorf("expected SameSite=Lax, got %v", stateCookie.SameSite)
		}
		if stateCookie.Path != "/" {
			t.Errorf("expected Path=/, got %q", stateCookie.Path)
		}
	})

	t.Run("oauth_state cookie is not Secure when CookieSecure=false", func(t *testing.T) {
		h := NewAuthHandler(db, newTestConfig(false))
		r := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
		c, w := ginTestContext(t, r)

		h.HandleGoogleLogin(c)

		var stateCookie *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == "oauth_state" {
				stateCookie = c
			}
		}
		if stateCookie == nil {
			t.Fatal("expected oauth_state cookie to be set")
		}
		if stateCookie.Secure {
			t.Error("expected oauth_state cookie to NOT be Secure when CookieSecure=false")
		}
	})

	t.Run("redirects to Google's auth URL with expected params", func(t *testing.T) {
		cfg := newTestConfig(true)
		h := NewAuthHandler(db, cfg)
		r := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
		c, w := ginTestContext(t, r)

		h.HandleGoogleLogin(c)

		resp := w.Result()
		if resp.StatusCode != http.StatusTemporaryRedirect {
			t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, resp.StatusCode)
		}

		location := resp.Header.Get("Location")
		u, err := url.Parse(location)
		if err != nil {
			t.Fatalf("Location header is not a valid URL: %v", err)
		}
		if !strings.Contains(u.Host, "google.com") {
			t.Errorf("expected redirect host to be a google.com domain, got %q", u.Host)
		}

		q := u.Query()
		if q.Get("client_id") != cfg.GoogleClientID {
			t.Errorf("expected client_id=%q, got %q", cfg.GoogleClientID, q.Get("client_id"))
		}
		if q.Get("redirect_uri") != cfg.GoogleRedirectURL {
			t.Errorf("expected redirect_uri=%q, got %q", cfg.GoogleRedirectURL, q.Get("redirect_uri"))
		}
		if q.Get("access_type") != "offline" {
			t.Errorf("expected access_type=offline, got %q", q.Get("access_type"))
		}

		var stateCookie *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "oauth_state" {
				stateCookie = c
			}
		}
		if stateCookie == nil {
			t.Fatal("expected oauth_state cookie to be set")
		}
		if q.Get("state") != stateCookie.Value {
			t.Errorf("expected redirect state param (%q) to match oauth_state cookie value (%q)", q.Get("state"), stateCookie.Value)
		}
	})
}

// ---- validateState ----------------------------------------------------------

func TestValidateState(t *testing.T) {
	t.Run("rejects a request with no oauth_state cookie", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?state=abc", nil)
		c, w := ginTestContext(t, r)

		if validateState(c) {
			t.Error("expected validateState to return false with no cookie")
		}
		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Result().StatusCode)
		}
	})

	t.Run("rejects a request where state param doesn't match the cookie", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?state=wrong-value", nil)
		r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "correct-value"})
		c, _ := ginTestContext(t, r)

		if validateState(c) {
			t.Error("expected validateState to return false on state mismatch")
		}
	})

	t.Run("accepts a matching state and clears the cookie", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?state=matching-value", nil)
		r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "matching-value"})
		c, w := ginTestContext(t, r)

		if !validateState(c) {
			t.Fatal("expected validateState to return true on matching state")
		}

		var cleared *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == "oauth_state" {
				cleared = c
			}
		}
		if cleared == nil {
			t.Fatal("expected validateState to clear the oauth_state cookie")
		}
		if cleared.MaxAge >= 0 {
			t.Errorf("expected MaxAge < 0 to clear the cookie, got %d", cleared.MaxAge)
		}
	})
}

// ---- session cookie helpers -------------------------------------------------

func TestSetSessionCookie(t *testing.T) {
	cfg := newTestConfig(true)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	c, w := ginTestContext(t, r)
	setSessionCookie(c, cfg, "raw-token-value")

	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session_token" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("expected session_token cookie to be set")
	}
	if cookie.Value != "raw-token-value" {
		t.Errorf("expected cookie value %q, got %q", "raw-token-value", cookie.Value)
	}
	if !cookie.HttpOnly {
		t.Error("expected session_token cookie to be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax, got %v", cookie.SameSite)
	}
	wantExpiry := time.Now().Add(30 * 24 * time.Hour)
	if cookie.Expires.Before(wantExpiry.Add(-time.Minute)) || cookie.Expires.After(wantExpiry.Add(time.Minute)) {
		t.Errorf("expected Expires around %v, got %v", wantExpiry, cookie.Expires)
	}
}

func TestSetSessionCookieAlwaysSecure(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	c, w := ginTestContext(t, r)
	cfg := newTestConfig(true)
	setSessionCookie(c, cfg, "token")

	for _, c := range w.Result().Cookies() {
		if c.Name == "session_token" && !c.Secure {
			t.Error("expected session_token's current implementation to always set Secure=true")
		}
	}
}

func TestClearSessionCookie(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	c, w := ginTestContext(t, r)
	cfg := newTestConfig(true)
	clearSessionCookie(c, cfg)

	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session_token" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("expected session_token cookie to be set (to clear it)")
	}
	if cookie.MaxAge >= 0 {
		t.Errorf("expected MaxAge < 0 to clear the cookie, got %d", cookie.MaxAge)
	}
	if cookie.Value != "" {
		t.Errorf("expected cleared cookie value to be empty, got %q", cookie.Value)
	}
}

// ---- HandleLogout -----------------------------------------------------------

func TestHandleLogout(t *testing.T) {
	db := setupAuthHandlerTestDB(t)
	h := NewAuthHandler(db, newTestConfig(true))

	t.Run("revokes an existing session and clears the cookie", func(t *testing.T) {
		user := domain.User{Username: "logout-user", Email: "logout@example.com", PasswordHash: "x", DisplayName: "Logout User"}
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("unexpected error creating user: %v", err)
		}
		rawToken, err := auth.CreateSession(context.Background(), db, user.ID, "test-agent", "127.0.0.1")
		if err != nil {
			t.Fatalf("unexpected error creating session: %v", err)
		}

		r := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		r.AddCookie(&http.Cookie{Name: "session_token", Value: rawToken})
		c, w := ginTestContext(t, r)

		h.HandleLogout(c)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Result().StatusCode)
		}

		if _, err := auth.ValidateSession(context.Background(), db, rawToken); err == nil {
			t.Error("expected session to be revoked after logout")
		}

		var cleared *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == "session_token" {
				cleared = c
			}
		}
		if cleared == nil || cleared.MaxAge >= 0 {
			t.Error("expected session_token cookie to be cleared after logout")
		}
	})

	t.Run("does not error when there's no session cookie", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		c, w := ginTestContext(t, r)

		h.HandleLogout(c)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Result().StatusCode)
		}
	})
}

// ---- HandleGoogleCallback ---------------------------------------------------

func newMockGoogleServer(t *testing.T, userInfo googleUserInfo) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Handle token endpoint
		if r.URL.Path == "/token" {
			err := json.NewEncoder(w).Encode(map[string]string{
				"access_token":  "test-access-token",
				"refresh_token": "test-refresh-token",
				"token_type":    "Bearer",
				"expires_in":    "3600",
			})
			if err != nil {
				return
			}
			return
		}
		// Handle userinfo endpoint
		err := json.NewEncoder(w).Encode(userInfo)
		if err != nil {
			return
		}
	}))
}

func TestHandleGoogleCallback(t *testing.T) {
	db := setupAuthHandlerTestDB(t)
	h := NewAuthHandler(db, newTestConfig(true))

	t.Run("issues a session on a valid callback", func(t *testing.T) {
		userInfo := googleUserInfo{ID: "test-id", Email: "test@example.com", VerifiedEmail: true, Name: "Test User"}
		server := newMockGoogleServer(t, userInfo)
		defer server.Close()

		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
		r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
		r = r.WithContext(withHijackedHTTPClient(r.Context(), server.URL))
		c, w := ginTestContext(t, r)

		h.HandleGoogleCallback(c)

		if w.Result().StatusCode != http.StatusTemporaryRedirect {
			t.Errorf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Result().StatusCode)
		}

		var sessionCookie *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == "session_token" {
				sessionCookie = c
			}
		}
		if sessionCookie == nil {
			t.Fatal("expected session_token cookie to be set")
		}
		if sessionCookie.Value == "" {
			t.Error("expected session_token cookie to have a non-empty value")
		}

		var user domain.User
		if err := db.Where("email = ?", userInfo.Email).First(&user).Error; err != nil {
			t.Fatalf("expected user to exist in DB: %v", err)
		}
		if user.Email != userInfo.Email {
			t.Errorf("expected user email %q, got %q", userInfo.Email, user.Email)
		}
	})

	t.Run("rejects a mismatched state before touching Google or the DB", func(t *testing.T) {
		db := setupAuthHandlerTestDB(t)
		h := NewAuthHandler(db, newTestConfig(true))

		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=wrong", nil)
		r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "right"})
		c, w := ginTestContext(t, r)

		h.HandleGoogleCallback(c)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Result().StatusCode)
		}
	})

	t.Run("rejects a callback with no code", func(t *testing.T) {
		db := setupAuthHandlerTestDB(t)
		h := NewAuthHandler(db, newTestConfig(true))

		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?state=test-state", nil)
		r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
		c, w := ginTestContext(t, r)

		h.HandleGoogleCallback(c)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Result().StatusCode)
		}
	})

	t.Run("returns 500 when the token exchange fails", func(t *testing.T) {
		db := setupAuthHandlerTestDB(t)
		h := NewAuthHandler(db, newTestConfig(true))

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
		r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
		r = r.WithContext(withHijackedHTTPClient(r.Context(), server.URL))
		c, w := ginTestContext(t, r)

		h.HandleGoogleCallback(c)

		if w.Result().StatusCode != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Result().StatusCode)
		}
	})

	t.Run("reuses the existing user on a repeat login", func(t *testing.T) {
		db := setupAuthHandlerTestDB(t)
		h := NewAuthHandler(db, newTestConfig(true))

		userInfo := googleUserInfo{ID: "callback-id-2", Email: "repeat@example.com", VerifiedEmail: true, Name: "Repeat User"}
		server := newMockGoogleServer(t, userInfo)
		defer server.Close()

		doCallback := func() *http.Cookie {
			r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
			r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
			r = r.WithContext(withHijackedHTTPClient(r.Context(), server.URL))
			c, w := ginTestContext(t, r)
			h.HandleGoogleCallback(c)
			for _, c := range w.Result().Cookies() {
				if c.Name == "session_token" {
					return c
				}
			}
			return nil
		}

		first := doCallback()
		second := doCallback()
		if first == nil || second == nil {
			t.Fatal("expected both callbacks to issue a session cookie")
		}

		var userCount int64
		db.Model(&domain.User{}).Where("email = ?", userInfo.Email).Count(&userCount)
		if userCount != 1 {
			t.Errorf("expected exactly 1 user after two logins, got %d", userCount)
		}

		var sessionCount int64
		db.Model(&domain.Session{}).Count(&sessionCount)
		if sessionCount != 2 {
			t.Errorf("expected 2 separate sessions (one per login), got %d", sessionCount)
		}
	})
}
