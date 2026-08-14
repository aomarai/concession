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
	"golang.org/x/oauth2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ---- test setup -----------------------------------------------------------

func setupAuthHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	// Unique-per-test DSN — see the identical comment in
	// internal/domain/testutil_test.go's uniqueSQLiteDSN. Using the bare
	// "file::memory:?cache=shared" DSN here originally meant every test in
	// this package (and every subtest) shared the same in-memory database
	// for the whole test binary run, which is why session counts included
	// leftovers from unrelated tests.
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

// redirectingTransport rewrites every outgoing request's scheme+host to
// point at a local test server, regardless of what the request was
// originally addressed to. This lets us exercise code that hits hardcoded
// Google URLs (config.GetGoogleOAuthConfig's google.Endpoint,
// fetchGoogleUserInfo's userinfo URL) against an httptest.Server instead.
type redirectingTransport struct {
	target *url.URL
}

func (t *redirectingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// withHijackedHTTPClient returns a context that makes the x/oauth2 package
// route all outgoing HTTP requests to the given test server. This is the
// package's own documented seam for this (oauth2.HTTPClient context key),
// not a hack around it.
func withHijackedHTTPClient(ctx context.Context, testServerURL string) context.Context {
	u, err := url.Parse(testServerURL)
	if err != nil {
		panic(err) // test setup error, not a real failure path
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

// Note: this is nearly identical to auth.GenerateSessionToken (32 random
// bytes, base64 RawURLEncoding). Worth considering consolidating into one
// shared helper at some point so the "how we generate a random token"
// logic only lives in one place — not urgent, just flagging the
// duplication since I noticed it while writing this test.

// ---- HandleGoogleLogin ------------------------------------------------------

func TestHandleGoogleLogin(t *testing.T) {
	db := setupAuthHandlerTestDB(t)

	t.Run("sets oauth_state cookie honoring CookieSecure", func(t *testing.T) {
		h := NewAuthHandler(db, newTestConfig(true))
		r := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
		w := httptest.NewRecorder()

		h.HandleGoogleLogin(w, r)

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
		w := httptest.NewRecorder()

		h.HandleGoogleLogin(w, r)

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
		w := httptest.NewRecorder()

		h.HandleGoogleLogin(w, r)

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
		w := httptest.NewRecorder()

		if validateState(w, r) {
			t.Error("expected validateState to return false with no cookie")
		}
		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Result().StatusCode)
		}
	})

	t.Run("rejects a request where state param doesn't match the cookie", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?state=wrong-value", nil)
		r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "correct-value"})
		w := httptest.NewRecorder()

		if validateState(w, r) {
			t.Error("expected validateState to return false on state mismatch")
		}
	})

	t.Run("accepts a matching state and clears the cookie", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?state=matching-value", nil)
		r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "matching-value"})
		w := httptest.NewRecorder()

		if !validateState(w, r) {
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
	w := httptest.NewRecorder()
	setSessionCookie(w, cfg, "raw-token-value")

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
	// setSessionCookie hardcodes Secure: true — unlike the oauth_state
	// cookie in HandleGoogleLogin, it does NOT consult cfg.CookieSecure.
	// This test documents that current behavior rather than asserting
	// it's correct: in local dev over plain HTTP with COOKIE_SECURE=false,
	// oauth_state would be set without Secure, but session_token still
	// would be — which browsers refuse to store over HTTP, silently
	// breaking login in that environment. Worth confirming whether that's
	// intentional.
	w := httptest.NewRecorder()
	cfg := newTestConfig(true)
	setSessionCookie(w, cfg, "token")

	for _, c := range w.Result().Cookies() {
		if c.Name == "session_token" && !c.Secure {
			t.Error("expected session_token's current implementation to always set Secure=true — if this now fails, the hardcoding was fixed and this test should be updated/removed")
		}
	}
}

func TestClearSessionCookie(t *testing.T) {
	w := httptest.NewRecorder()
	cfg := newTestConfig(true)
	clearSessionCookie(w, cfg)

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
		w := httptest.NewRecorder()

		h.HandleLogout(w, r)

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
		w := httptest.NewRecorder()

		h.HandleLogout(w, r)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("expected status %d even with no cookie, got %d", http.StatusOK, w.Result().StatusCode)
		}
	})
}

// ---- findOrCreateUser --------------------------------------------------------

func TestFindOrCreateUser(t *testing.T) {
	t.Run("creates a new user and linked oauth account", func(t *testing.T) {
		db := setupAuthHandlerTestDB(t)
		service := auth.UserAuthService{DB: db}

		info := auth.GoogleUserInfo{
			ID:            "google-id-123",
			Email:         "newuser@example.com",
			VerifiedEmail: true,
			Name:          "New User",
			Picture:       "https://example.com/avatar.png",
		}

		ctx := context.Background()
		user, err := service.FindOrCreateGoogleUser(ctx, info)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Email != info.Email {
			t.Errorf("expected Email %q, got %q", info.Email, user.Email)
		}
		if user.Username != info.Email {
			t.Errorf("expected Username %q, got %q", info.Email, user.Username)
		}
		if user.DisplayName != info.Name {
			t.Errorf("expected DisplayName %q, got %q", info.Name, user.DisplayName)
		}
		if user.AvatarURL != info.Picture {
			t.Errorf("expected AvatarURL %q, got %q", info.Picture, user.AvatarURL)
		}
		if !user.IsEmailVerified {
			t.Error("expected IsEmailVerified to be true")
		}

		var oauthAccount domain.OAuthAccount
		if err := db.Where("provider = ? AND provider_user_id = ?", "google", info.ID).First(&oauthAccount).Error; err != nil {
			t.Fatalf("expected an oauth account to be created: %v", err)
		}
		if oauthAccount.UserID != user.ID {
			t.Errorf("expected oauth account UserID %v, got %v", user.ID, oauthAccount.UserID)
		}
	})

	t.Run("returns the existing user for an already-linked account", func(t *testing.T) {
		db := setupAuthHandlerTestDB(t)

		existing := domain.User{Username: "existing@example.com", Email: "existing@example.com", PasswordHash: "x", DisplayName: "Existing User"}
		if err := db.Create(&existing).Error; err != nil {
			t.Fatalf("unexpected error creating user: %v", err)
		}
		link := domain.OAuthAccount{UserID: existing.ID, Provider: "google", ProviderUserID: "google-id-456"}
		if err := db.Create(&link).Error; err != nil {
			t.Fatalf("unexpected error creating oauth link: %v", err)
		}

		service := auth.UserAuthService{DB: db}
		info := auth.GoogleUserInfo{ID: "google-id-456", Email: "existing@example.com"}
		user, err := service.FindOrCreateGoogleUser(context.Background(), info)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.ID != existing.ID {
			t.Errorf("expected existing user ID %v, got %v", existing.ID, user.ID)
		}

		var count int64
		db.Model(&domain.User{}).Where("email = ?", "existing@example.com").Count(&count)
		if count != 1 {
			t.Errorf("expected exactly 1 user, got %d — a duplicate may have been created", count)
		}
	})

	t.Run("propagates unexpected DB errors instead of creating a user", func(t *testing.T) {
		db := setupAuthHandlerTestDB(t)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // force the initial lookup query to fail with something other than ErrRecordNotFound

		info := auth.GoogleUserInfo{ID: "google-id-789", Email: "shouldnotexist@example.com"}
		service := auth.UserAuthService{DB: db}
		_, err := service.FindOrCreateGoogleUser(ctx, info)
		//_, err := h.findOrCreateUser(ctx, info)
		if err == nil {
			t.Fatal("expected an error from a canceled context, got nil")
		}

		var count int64
		db.Model(&domain.User{}).Where("email = ?", "shouldnotexist@example.com").Count(&count)
		if count != 0 {
			t.Error("expected no user to be created when the lookup query itself failed")
		}
	})
}

// ---- fetchGoogleUserInfo ------------------------------------------------------

func TestFetchGoogleUserInfo(t *testing.T) {
	t.Run("decodes a successful response", func(t *testing.T) {
		want := googleUserInfo{ID: "abc123", Email: "fetch@example.com", VerifiedEmail: true, Name: "Fetch Test", Picture: "https://example.com/pic.png"}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/oauth2/v2/userinfo" {
				t.Errorf("unexpected request path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(want)
			if err != nil {
				return
			}
		}))
		defer server.Close()

		ctx := withHijackedHTTPClient(context.Background(), server.URL)
		token := &oauth2.Token{AccessToken: "test-token", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}
		cfg := &oauth2.Config{}

		got, err := fetchGoogleUserInfo(ctx, cfg, token)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("expected %+v, got %+v", want, got)
		}
	})

	t.Run("returns an error on invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte("not json"))
			if err != nil {
				return
			}
		}))
		defer server.Close()

		ctx := withHijackedHTTPClient(context.Background(), server.URL)
		token := &oauth2.Token{AccessToken: "test-token", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}
		cfg := &oauth2.Config{}

		if _, err := fetchGoogleUserInfo(ctx, cfg, token); err == nil {
			t.Error("expected an error decoding invalid JSON, got nil")
		}
	})

	t.Run("does not itself reject a non-2xx status", func(t *testing.T) {
		// fetchGoogleUserInfo only checks the transport-level error and
		// the JSON decode error — it never inspects resp.StatusCode. A
		// non-2xx response with a valid-JSON error body (typical for
		// Google's API) decodes without error into a mostly-empty
		// googleUserInfo instead of surfacing as a failure. This test
		// documents that; if you'd rather non-2xx be treated as an error,
		// that's a small addition (check resp.StatusCode before decoding)
		// and I can add it if you want.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, err := w.Write([]byte(`{"error":"invalid_token"}`))
			if err != nil {
				return
			}
		}))
		defer server.Close()

		ctx := withHijackedHTTPClient(context.Background(), server.URL)
		token := &oauth2.Token{AccessToken: "bad-token", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}
		cfg := &oauth2.Config{}

		_, err := fetchGoogleUserInfo(ctx, cfg, token)
		if err != nil {
			t.Errorf("expected no decode error for a 401 with a valid JSON body (current behavior), got %v", err)
		}
	})
}

// ---- HandleGoogleCallback (full flow) -----------------------------------------

func TestHandleGoogleCallback(t *testing.T) {
	newMockGoogleServer := func(t *testing.T, userInfo googleUserInfo) *httptest.Server {
		t.Helper()
		mux := http.NewServeMux()
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "mock-access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			if err != nil {
				return
			}
		})
		mux.HandleFunc("/oauth2/v2/userinfo", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(userInfo)
			if err != nil {
				return
			}
		})
		return httptest.NewServer(mux)
	}

	t.Run("full flow creates a user and issues a session", func(t *testing.T) {
		db := setupAuthHandlerTestDB(t)
		h := NewAuthHandler(db, newTestConfig(true))

		userInfo := googleUserInfo{ID: "callback-id-1", Email: "callback@example.com", VerifiedEmail: true, Name: "Callback User"}
		server := newMockGoogleServer(t, userInfo)
		defer server.Close()

		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=test-code&state=test-state", nil)
		r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
		r = r.WithContext(withHijackedHTTPClient(r.Context(), server.URL))
		w := httptest.NewRecorder()

		h.HandleGoogleCallback(w, r)

		resp := w.Result()
		if resp.StatusCode != http.StatusTemporaryRedirect {
			t.Fatalf("expected status %d, got %d (body: %s)", http.StatusTemporaryRedirect, resp.StatusCode, w.Body.String())
		}
		if resp.Header.Get("Location") != "/" {
			t.Errorf("expected redirect to /, got %q", resp.Header.Get("Location"))
		}

		var sessionCookie *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "session_token" {
				sessionCookie = c
			}
		}
		if sessionCookie == nil || sessionCookie.Value == "" {
			t.Fatal("expected a session_token cookie to be set")
		}

		session, err := auth.ValidateSession(context.Background(), db, sessionCookie.Value)
		if err != nil {
			t.Fatalf("expected the issued session to be valid: %v", err)
		}

		var user domain.User
		if err := db.First(&user, "id = ?", session.UserID).Error; err != nil {
			t.Fatalf("expected a user to exist for the session: %v", err)
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
		w := httptest.NewRecorder()

		h.HandleGoogleCallback(w, r)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Result().StatusCode)
		}
	})

	t.Run("rejects a callback with no code", func(t *testing.T) {
		db := setupAuthHandlerTestDB(t)
		h := NewAuthHandler(db, newTestConfig(true))

		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?state=test-state", nil)
		r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
		w := httptest.NewRecorder()

		h.HandleGoogleCallback(w, r)

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
		w := httptest.NewRecorder()

		h.HandleGoogleCallback(w, r)

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
			w := httptest.NewRecorder()
			h.HandleGoogleCallback(w, r)
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
