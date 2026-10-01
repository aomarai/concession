package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/auth"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

func authModels() []any { return []any{&domain.User{}, &domain.OAuthAccount{}, &domain.Session{}} }

func newMeRouter(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewAuthHandler(db, newTestConfig(true))
	r := gin.New()
	r.GET("/me", h.AuthMiddleware(), NewUserHandler(db).HandleGetMe)
	r.GET("/me-no-mw", NewUserHandler(db).HandleGetMe)
	return r
}

func loginUser(t *testing.T, db *gorm.DB) (domain.User, string) {
	t.Helper()
	u := domain.User{Username: "me", Email: "me@example.com", DisplayName: "Me"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	tok, err := auth.CreateSession(context.Background(), db, u.ID, "ua", "ip")
	if err != nil {
		t.Fatal(err)
	}
	return u, tok
}

func getMe(r *gin.Engine, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func clearedSessionCookie(w *httptest.ResponseRecorder) bool {
	for _, c := range w.Result().Cookies() {
		if c.Name == "session_token" && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

func TestAuthMiddlewareAndGetMe(t *testing.T) {
	db := testutil.NewDB(t, authModels()...)
	r := newMeRouter(t, db)
	user, token := loginUser(t, db)

	t.Run("missing cookie is 401 and clears the cookie", func(t *testing.T) {
		w := getMe(r, "/me", "")
		assertErrorCode(t, w, http.StatusUnauthorized, "unauthorized")
		if !clearedSessionCookie(w) {
			t.Error("expected session cookie to be cleared")
		}
	})

	t.Run("unknown token is 401", func(t *testing.T) {
		w := getMe(r, "/me", "not-a-real-token")
		assertErrorCode(t, w, http.StatusUnauthorized, "unauthorized")
		if !clearedSessionCookie(w) {
			t.Error("expected session cookie to be cleared")
		}
	})

	t.Run("revoked token is 401", func(t *testing.T) {
		_, revoked := loginUser2(t, db, "revoked@example.com")
		if err := auth.RevokeSession(context.Background(), db, revoked); err != nil {
			t.Fatal(err)
		}
		assertErrorCode(t, getMe(r, "/me", revoked), http.StatusUnauthorized, "unauthorized")
	})

	t.Run("valid session returns the user", func(t *testing.T) {
		w := getMe(r, "/me", token)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", w.Code, w.Body)
		}
		var got domain.User
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.ID != user.ID || got.Email != user.Email {
			t.Errorf("wrong user: %+v", got)
		}
	})

	t.Run("user deleted after login is 404", func(t *testing.T) {
		u2, tok2 := loginUser2(t, db, "gone@example.com")
		if err := db.Unscoped().Delete(&domain.User{}, u2.ID).Error; err != nil {
			t.Fatal(err)
		}
		assertErrorCode(t, getMe(r, "/me", tok2), http.StatusNotFound, "not_found")
	})

	t.Run("handler without middleware is 500", func(t *testing.T) {
		assertErrorCode(t, getMe(r, "/me-no-mw", ""), http.StatusInternalServerError, "internal_error")
	})
}

func loginUser2(t *testing.T, db *gorm.DB, email string) (domain.User, string) {
	t.Helper()
	u := domain.User{Username: email, Email: email, DisplayName: email}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	tok, err := auth.CreateSession(context.Background(), db, u.ID, "ua", "ip")
	if err != nil {
		t.Fatal(err)
	}
	return u, tok
}

func assertErrorCode(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status = %d, want %d (%s)", w.Code, status, w.Body)
	}
	var e ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || e.Error.Code != code || e.Error.Message == "" {
		t.Errorf("error body = %s, want code %q", w.Body, code)
	}
}

// ---- auth handler error paths ----------------------------------------------

func TestHandleGoogleLoginStateFailure(t *testing.T) {
	db := testutil.NewDB(t, authModels()...)
	h := NewAuthHandler(db, newTestConfig(true))
	h.newState = func() (string, error) { return "", errors.New("no entropy") }

	c, w := ginTestContext(t, httptest.NewRequest(http.MethodGet, "/auth/google/login", nil))
	h.HandleGoogleLogin(c)
	assertErrorCode(t, w, http.StatusInternalServerError, "internal_error")
	if len(w.Result().Cookies()) != 0 {
		t.Error("no state cookie should be set on failure")
	}
}

func callbackRequest(t *testing.T, serverURL string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=c&state=s", nil)
	r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "s"})
	r = r.WithContext(withHijackedHTTPClient(r.Context(), serverURL))
	return ginTestContext(t, r)
}

func TestHandleGoogleCallbackFailures(t *testing.T) {
	info := googleUserInfo{ID: "gid", Email: "cb@example.com", VerifiedEmail: true, Name: "CB"}

	t.Run("token exchange fails", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer srv.Close()
		h := NewAuthHandler(testutil.NewDB(t, authModels()...), newTestConfig(true))
		c, w := callbackRequest(t, srv.URL)
		h.HandleGoogleCallback(c)
		assertErrorCode(t, w, http.StatusInternalServerError, "internal_error")
	})

	t.Run("userinfo is not JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/token" {
				_, _ = io.WriteString(w, `{"access_token":"a","token_type":"Bearer"}`)
				return
			}
			_, _ = io.WriteString(w, "not json")
		}))
		defer srv.Close()
		h := NewAuthHandler(testutil.NewDB(t, authModels()...), newTestConfig(true))
		c, w := callbackRequest(t, srv.URL)
		h.HandleGoogleCallback(c)
		assertErrorCode(t, w, http.StatusInternalServerError, "internal_error")
	})

	t.Run("user lookup fails", func(t *testing.T) {
		srv := newMockGoogleServer(t, info)
		defer srv.Close()
		db := testutil.NewDB(t, authModels()...)
		testutil.FailOn(t, db, "query", "o_auth_accounts")
		h := NewAuthHandler(db, newTestConfig(true))
		c, w := callbackRequest(t, srv.URL)
		h.HandleGoogleCallback(c)
		assertErrorCode(t, w, http.StatusInternalServerError, "internal_error")
	})

	t.Run("session creation fails", func(t *testing.T) {
		srv := newMockGoogleServer(t, info)
		defer srv.Close()
		db := testutil.NewDB(t, authModels()...)
		testutil.FailOn(t, db, "create", "sessions")
		h := NewAuthHandler(db, newTestConfig(true))
		c, w := callbackRequest(t, srv.URL)
		h.HandleGoogleCallback(c)
		assertErrorCode(t, w, http.StatusInternalServerError, "internal_error")
		for _, ck := range w.Result().Cookies() {
			if ck.Name == "session_token" {
				t.Error("session cookie must not be set when session creation fails")
			}
		}
	})

	t.Run("missing code", func(t *testing.T) {
		h := NewAuthHandler(testutil.NewDB(t, authModels()...), newTestConfig(true))
		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback?state=s", nil)
		r.AddCookie(&http.Cookie{Name: "oauth_state", Value: "s"})
		c, w := ginTestContext(t, r)
		h.HandleGoogleCallback(c)
		assertErrorCode(t, w, http.StatusBadRequest, "bad_request")
	})
}

func TestHandleLogoutRevokeFailureStillClearsCookie(t *testing.T) {
	db := testutil.NewDB(t, authModels()...)
	testutil.FailOn(t, db, "update", "sessions")
	h := NewAuthHandler(db, newTestConfig(true))

	r := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	r.AddCookie(&http.Cookie{Name: "session_token", Value: "tok"})
	c, w := ginTestContext(t, r)
	h.HandleLogout(c)
	if !clearedSessionCookie(w) {
		t.Error("cookie should be cleared even if revoke fails")
	}
}

// ---- fetchGoogleUserInfo ------------------------------------------------------

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type failingCloser struct{ io.Reader }

func (failingCloser) Close() error { return errors.New("close failed") }

func TestFetchGoogleUserInfo(t *testing.T) {
	cfg := newTestConfig(true)
	oc := &oauth2.Config{ClientID: cfg.GoogleClientID}
	tok := &oauth2.Token{AccessToken: "a", TokenType: "Bearer"}
	ctxWith := func(rt roundTripFunc) context.Context {
		return context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: rt})
	}

	t.Run("transport error", func(t *testing.T) {
		ctx := ctxWith(func(*http.Request) (*http.Response, error) { return nil, errors.New("boom") })
		if _, err := fetchGoogleUserInfo(ctx, oc, tok); err == nil {
			t.Error("expected error")
		}
	})

	t.Run("body close error is logged but not fatal", func(t *testing.T) {
		ctx := ctxWith(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: failingCloser{strings.NewReader(`{"id":"1","email":"x@y.z"}`)}}, nil
		})
		info, err := fetchGoogleUserInfo(ctx, oc, tok)
		if err != nil || info.ID != "1" {
			t.Errorf("got %+v, %v", info, err)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		ctx := ctxWith(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{"))}, nil
		})
		if _, err := fetchGoogleUserInfo(ctx, oc, tok); err == nil {
			t.Error("expected decode error")
		}
	})
}

func TestHealthHandlerUnhealthyBodyShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.NewDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	r := gin.New()
	r.GET("/healthz", HealthHandler(db))
	assertErrorCode(t, getMe(r, "/healthz", ""), http.StatusServiceUnavailable, "unhealthy")
}
