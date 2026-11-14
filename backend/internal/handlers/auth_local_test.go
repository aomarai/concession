package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/auth"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func newLocalAuthHandler(t *testing.T) (*AuthHandler, *gorm.DB) {
	t.Helper()
	db := setupAuthHandlerTestDB(t)
	return NewAuthHandler(db, newTestConfig(false)), db
}

// callAuth invokes an auth handler with a JSON body and, when userID is not
// nil, an authenticated request context.
func callAuth(t *testing.T, handler gin.HandlerFunc, method, path, body string, userID *uuid.UUID) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("User-Agent", "test-agent")
	c, w := ginTestContext(t, r)
	if userID != nil {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), userIDKey, *userID))
	}
	handler(c)
	// The gin engine flushes the status at the end of the chain; calling the
	// handler directly means doing it here so a bare c.Status(204) is recorded.
	c.Writer.WriteHeaderNow()
	return w, c
}

func sessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "session_token" && cookie.Value != "" {
			return cookie
		}
	}
	t.Fatal("no session cookie was set")
	return nil
}

const registrationBody = `{"email":"Grace@Example.com","username":"grace.h","display_name":"Grace H","password":"correct horse battery"}`

func TestHandleRegisterCreatesAndSignsIn(t *testing.T) {
	h, db := newLocalAuthHandler(t)

	w, _ := callAuth(t, h.HandleRegister, http.MethodPost, "/api/v1/auth/register", registrationBody, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	sessionCookie(t, w)

	if strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), "correct horse") {
		t.Errorf("the response leaked the password: %s", w.Body.String())
	}
	var user domain.User
	if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
		t.Fatalf("response is not a user: %v", err)
	}
	if user.Email != "grace@example.com" || user.Username != "grace.h" || !user.HasPassword {
		t.Errorf("unexpected user: %+v", user)
	}

	// The starter watchlists are part of registration, not a later step.
	var lists int64
	db.Model(&domain.Watchlist{}).Where("owner_id = ?", user.ID).Count(&lists)
	if lists != 2 {
		t.Errorf("expected the two starter lists, got %d", lists)
	}
	// And the new password actually signs in.
	if _, err := auth.NewUserAuthService(db).AuthenticateLocal(context.Background(), "grace.h", "correct horse battery"); err != nil {
		t.Errorf("the registered password does not authenticate: %v", err)
	}
}

func TestHandleRegisterConflictAndValidation(t *testing.T) {
	h, _ := newLocalAuthHandler(t)
	if w, _ := callAuth(t, h.HandleRegister, http.MethodPost, "/api/v1/auth/register", registrationBody, nil); w.Code != http.StatusCreated {
		t.Fatalf("setup registration failed: %d", w.Code)
	}

	cases := []struct {
		name string
		body string
		code int
		err  string
	}{
		{"same e-mail", `{"email":"GRACE@example.com","username":"other.name","password":"correct horse battery"}`, http.StatusConflict, "conflict"},
		{"same username", `{"email":"someone@example.com","username":"grace.h","password":"correct horse battery"}`, http.StatusConflict, "conflict"},
		{"bad e-mail", `{"email":"nope","username":"new.name","password":"correct horse battery"}`, http.StatusBadRequest, "bad_request"},
		{"bad username", `{"email":"new@example.com","username":"x","password":"correct horse battery"}`, http.StatusBadRequest, "bad_request"},
		{"weak password", `{"email":"new@example.com","username":"new.name","password":"short"}`, http.StatusBadRequest, "bad_request"},
		{"not json", `{`, http.StatusBadRequest, "bad_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, _ := callAuth(t, h.HandleRegister, http.MethodPost, "/api/v1/auth/register", tc.body, nil)
			if w.Code != tc.code {
				t.Fatalf("expected %d, got %d: %s", tc.code, w.Code, w.Body.String())
			}
			var res ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatalf("error body is not JSON: %v", err)
			}
			if res.Error.Code != tc.err {
				t.Errorf("expected code %q, got %q", tc.err, res.Error.Code)
			}
		})
	}
}

func TestHandleRegisterInternalError(t *testing.T) {
	h, db := newLocalAuthHandler(t)
	testutil.FailOn(t, db, "query", "users")

	w, _ := callAuth(t, h.HandleRegister, http.MethodPost, "/api/v1/auth/register", registrationBody, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRegisterSessionFailure(t *testing.T) {
	h, db := newLocalAuthHandler(t)
	testutil.FailOn(t, db, "create", "sessions")

	w, _ := callAuth(t, h.HandleRegister, http.MethodPost, "/api/v1/auth/register", registrationBody, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleLogin(t *testing.T) {
	h, _ := newLocalAuthHandler(t)
	if w, _ := callAuth(t, h.HandleRegister, http.MethodPost, "/api/v1/auth/register", registrationBody, nil); w.Code != http.StatusCreated {
		t.Fatalf("setup registration failed: %d", w.Code)
	}

	// Either identifier works, in any case.
	for _, identifier := range []string{"grace@example.com", "GRACE.H"} {
		w, _ := callAuth(t, h.HandleLogin, http.MethodPost, "/api/v1/auth/login",
			`{"identifier":"`+identifier+`","password":"correct horse battery"}`, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for %q, got %d: %s", identifier, w.Code, w.Body.String())
		}
		sessionCookie(t, w)
	}

	failures := []string{
		`{"identifier":"grace.h","password":"wrong password here"}`,
		`{"identifier":"nobody@example.com","password":"correct horse battery"}`,
		`{"identifier":"","password":""}`,
	}
	for _, body := range failures {
		w, _ := callAuth(t, h.HandleLogin, http.MethodPost, "/api/v1/auth/login", body, nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for %s, got %d: %s", body, w.Code, w.Body.String())
		}
		var res ErrorResponse
		json.Unmarshal(w.Body.Bytes(), &res)
		if res.Error.Code != "unauthorized" {
			t.Errorf("expected code unauthorized, got %q", res.Error.Code)
		}
	}

	w, _ := callAuth(t, h.HandleLogin, http.MethodPost, "/api/v1/auth/login", `{`, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for a malformed body, got %d", w.Code)
	}
}

func TestHandleLoginInternalError(t *testing.T) {
	h, db := newLocalAuthHandler(t)
	testutil.FailOn(t, db, "query", "users")

	w, _ := callAuth(t, h.HandleLogin, http.MethodPost, "/api/v1/auth/login",
		`{"identifier":"grace.h","password":"correct horse battery"}`, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleLoginSessionFailure(t *testing.T) {
	h, db := newLocalAuthHandler(t)
	if w, _ := callAuth(t, h.HandleRegister, http.MethodPost, "/api/v1/auth/register", registrationBody, nil); w.Code != http.StatusCreated {
		t.Fatalf("setup registration failed: %d", w.Code)
	}
	testutil.FailOn(t, db, "create", "sessions")

	w, _ := callAuth(t, h.HandleLogin, http.MethodPost, "/api/v1/auth/login",
		`{"identifier":"grace.h","password":"correct horse battery"}`, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when the session cannot be created, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSetPassword(t *testing.T) {
	h, db := newLocalAuthHandler(t)
	svc := auth.NewUserAuthService(db)

	// A Google-only account: no current password needed.
	googleUser, err := svc.FindOrCreateGoogleUser(context.Background(), auth.GoogleUserInfo{
		ID: "g-1", Email: "g@example.com", VerifiedEmail: true, Name: "Gee",
	})
	if err != nil {
		t.Fatal(err)
	}

	w, _ := callAuth(t, h.HandleSetPassword, http.MethodPost, "/api/v1/me/password",
		`{"new_password":"a brand new password"}`, &googleUser.ID)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := svc.AuthenticateLocal(context.Background(), "g@example.com", "a brand new password"); err != nil {
		t.Fatalf("the new password does not work: %v", err)
	}

	// Changing it again requires the current one.
	w, _ = callAuth(t, h.HandleSetPassword, http.MethodPost, "/api/v1/me/password",
		`{"current_password":"a brand new password","new_password":"yet another password"}`, &googleUser.ID)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for a correct current password, got %d: %s", w.Code, w.Body.String())
	}

	w, _ = callAuth(t, h.HandleSetPassword, http.MethodPost, "/api/v1/me/password",
		`{"current_password":"stale password","new_password":"another password 1"}`, &googleUser.ID)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a wrong current password, got %d: %s", w.Code, w.Body.String())
	}
	var res ErrorResponse
	json.Unmarshal(w.Body.Bytes(), &res)
	if res.Error.Code != "bad_request" {
		t.Errorf("expected code bad_request, got %q", res.Error.Code)
	}

	w, _ = callAuth(t, h.HandleSetPassword, http.MethodPost, "/api/v1/me/password",
		`{"current_password":"yet another password","new_password":"short"}`, &googleUser.ID)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for a weak new password, got %d", w.Code)
	}

	w, _ = callAuth(t, h.HandleSetPassword, http.MethodPost, "/api/v1/me/password", `{`, &googleUser.ID)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for a malformed body, got %d", w.Code)
	}

	// An unknown user is a 404; the session was valid but the account is gone.
	missing := uuid.New()
	w, _ = callAuth(t, h.HandleSetPassword, http.MethodPost, "/api/v1/me/password",
		`{"new_password":"some other password"}`, &missing)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for an unknown user, got %d", w.Code)
	}

	// No user ID in the context means the route was registered without the auth
	// middleware, which is a wiring bug: 500, not 401.
	w, _ = callAuth(t, h.HandleSetPassword, http.MethodPost, "/api/v1/me/password",
		`{"new_password":"some other password"}`, nil)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 without the auth middleware, got %d", w.Code)
	}
}

func TestHandleSetPasswordInternalError(t *testing.T) {
	h, db := newLocalAuthHandler(t)
	user := domain.User{Username: "someone", Email: "s@example.com", DisplayName: "Some"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	testutil.FailOn(t, db, "update", "users")

	w, _ := callAuth(t, h.HandleSetPassword, http.MethodPost, "/api/v1/me/password",
		`{"new_password":"a brand new password"}`, &user.ID)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}
