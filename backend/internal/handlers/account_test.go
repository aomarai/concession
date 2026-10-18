package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func accountModels() []any {
	return []any{
		&domain.User{}, &domain.OAuthAccount{}, &domain.Session{}, &domain.Movie{}, &domain.Show{}, &domain.Genre{},
		&domain.Review{}, &domain.Watchlist{}, &domain.WatchlistItem{}, &domain.Collaborator{},
		&domain.UserWatchProgress{}, &domain.Notification{}, &domain.Friendship{},
	}
}

// accountRouter mounts the /me routes behind the real session middleware.
func accountRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.NewDB(t, accountModels()...)
	cfg := newTestConfig(true)
	r := gin.New()
	g := r.Group("/api/v1")
	g.Use(NewAuthHandler(db, cfg).AuthMiddleware())
	NewUserHandler(db, cfg).RegisterRoutes(g)
	return r, db
}

func meRequest(r *gin.Engine, method, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/me", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestPatchMeChangesTheProfile(t *testing.T) {
	r, db := accountRouter(t)
	user, token := loginUser(t, db)

	w := meRequest(r, http.MethodPatch, token, `{"display_name":"  Ada L  ","username":"Ada_L"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}
	got := decode[domain.User](t, w)
	if got.ID != user.ID || got.DisplayName != "Ada L" || got.Username != "ada_l" || got.Email != user.Email {
		t.Errorf("response = %+v", got)
	}
	var stored domain.User
	db.First(&stored, "id = ?", user.ID)
	if stored.DisplayName != "Ada L" || stored.Username != "ada_l" {
		t.Errorf("stored = %+v", stored)
	}
	// GET /me shows the change.
	if me := decode[domain.User](t, meRequest(r, http.MethodGet, token, "")); me.Username != "ada_l" {
		t.Errorf("GET /me = %+v", me)
	}
}

func TestPatchMeErrors(t *testing.T) {
	r, db := accountRouter(t)
	_, token := loginUser(t, db)
	other, _ := loginUser2(t, db, "other@example.com")
	db.Model(&domain.User{}).Where("id = ?", other.ID).Update("username", "taken")

	for _, tc := range []struct {
		name, body string
		status     int
		code       string
	}{
		{"empty name", `{"display_name":"  "}`, http.StatusBadRequest, "bad_request"},
		{"bad username", `{"username":"a@b.com"}`, http.StatusBadRequest, "bad_request"},
		{"nothing to change", `{}`, http.StatusBadRequest, "bad_request"},
		{"not json", `nope`, http.StatusBadRequest, "bad_request"},
		{"taken", `{"username":"taken"}`, http.StatusConflict, "conflict"},
	} {
		assertErrorCode(t, meRequest(r, http.MethodPatch, token, tc.body), tc.status, tc.code)
	}
	// The message tells the person what to fix.
	w := meRequest(r, http.MethodPatch, token, `{"username":"x"}`)
	if !strings.Contains(w.Body.String(), "3 to 32") {
		t.Errorf("message = %s", w.Body)
	}
}

func TestPatchMeReportsDatabaseFailures(t *testing.T) {
	r, db := accountRouter(t)
	_, token := loginUser(t, db)
	testutil.FailOn(t, db, "update", "users")
	assertErrorCode(t, meRequest(r, http.MethodPatch, token, `{"display_name":"x"}`), http.StatusInternalServerError, "internal_error")
}

func TestDeleteMeRemovesTheAccountAndEndsTheSession(t *testing.T) {
	r, db := accountRouter(t)
	user, token := loginUser(t, db)
	other, _ := loginUser2(t, db, "other@example.com")
	mine := domain.Watchlist{OwnerID: user.ID, Title: "mine", Type: domain.WatchlistTypeMovie, Privacy: domain.PrivacyPrivate}
	theirs := domain.Watchlist{OwnerID: other.ID, Title: "theirs", Type: domain.WatchlistTypeMovie, Privacy: domain.PrivacyPrivate}
	db.Create(&mine)
	db.Create(&theirs)

	w := meRequest(r, http.MethodDelete, token, `{"confirm":"ME"}`) // the username is "me"; case does not matter
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}
	if !clearedSessionCookie(w) {
		t.Error("the session cookie should be cleared")
	}
	var users, lists int64
	db.Unscoped().Model(&domain.User{}).Where("id = ?", user.ID).Count(&users)
	db.Unscoped().Model(&domain.Watchlist{}).Where("id = ?", mine.ID).Count(&lists)
	if users != 0 || lists != 0 {
		t.Errorf("user rows %d, list rows %d: both should be gone", users, lists)
	}
	db.Model(&domain.Watchlist{}).Where("id = ?", theirs.ID).Count(&lists)
	if lists != 1 {
		t.Error("someone else's list must not be touched")
	}
	// The old session no longer works.
	assertErrorCode(t, meRequest(r, http.MethodGet, token, ""), http.StatusUnauthorized, "unauthorized")
}

func TestDeleteMeNeedsTheRightConfirmation(t *testing.T) {
	r, db := accountRouter(t)
	user, token := loginUser(t, db)
	for _, body := range []string{`{}`, `{"confirm":"someone else"}`, `nope`} {
		assertErrorCode(t, meRequest(r, http.MethodDelete, token, body), http.StatusBadRequest, "bad_request")
	}
	var n int64
	db.Model(&domain.User{}).Where("id = ?", user.ID).Count(&n)
	if n != 1 {
		t.Error("the account must survive a failed confirmation")
	}
}

func TestDeleteMeReportsDatabaseFailures(t *testing.T) {
	r, db := accountRouter(t)
	_, token := loginUser(t, db)
	// The session is valid, then the delete itself fails.
	testutil.FailOn(t, db, "delete", "users")
	assertErrorCode(t, meRequest(r, http.MethodDelete, token, `{"confirm":"me"}`), http.StatusInternalServerError, "internal_error")
}

func TestAccountRoutesNeedASession(t *testing.T) {
	r, _ := accountRouter(t)
	for _, m := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
		w := meRequest(r, m, "no-such-session", `{}`)
		assertErrorCode(t, w, http.StatusUnauthorized, "unauthorized")
	}
}

func TestAccountHandlersRefuseARequestWithoutAUser(t *testing.T) {
	// Mounted without the session middleware, so no user is in the context:
	// that is a wiring bug and must fail loudly, not act on a nil user.
	gin.SetMode(gin.TestMode)
	db := testutil.NewDB(t, accountModels()...)
	h := NewUserHandler(db, newTestConfig(true))
	r := gin.New()
	r.PATCH("/me", h.HandleUpdateMe)
	r.DELETE("/me", h.HandleDeleteMe)
	for _, m := range []string{http.MethodPatch, http.MethodDelete} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(m, "/me", strings.NewReader(`{"display_name":"x","confirm":"x"}`)))
		assertErrorCode(t, w, http.StatusInternalServerError, "internal_error")
	}
}
