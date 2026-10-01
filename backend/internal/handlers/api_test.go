package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/catalog"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/friends"
	"github.com/aomarai/concession/internal/notifications"
	"github.com/aomarai/concession/internal/progress"
	"github.com/aomarai/concession/internal/reviews"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/aomarai/concession/internal/tmdb"
	"github.com/aomarai/concession/internal/watchlist"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// fakeCatalog stores a placeholder title per TMDB ID. IDs 404 and 502 simulate
// TMDB answering "not found" and being unreachable.
type fakeCatalog struct{ db *gorm.DB }

func (f fakeCatalog) fail(id int64) error {
	switch id {
	case 404:
		return tmdb.ErrNotFound
	case 502:
		return fmt.Errorf("%w: boom", catalog.ErrUpstream)
	}
	return nil
}

func (f fakeCatalog) EnsureMovie(_ context.Context, id int64) (*domain.Movie, error) {
	if err := f.fail(id); err != nil {
		return nil, err
	}
	m := domain.Movie{TMDBID: id, Title: fmt.Sprintf("Movie %d", id)}
	return &m, f.db.Where("tmdb_id = ?", id).FirstOrCreate(&m).Error
}

func (f fakeCatalog) EnsureShow(_ context.Context, id int64) (*domain.Show, error) {
	if err := f.fail(id); err != nil {
		return nil, err
	}
	s := domain.Show{TMDBID: &id, Name: fmt.Sprintf("Show %d", id)}
	return &s, f.db.Where("tmdb_id = ?", id).FirstOrCreate(&s).Error
}

type api struct {
	t      *testing.T
	db     *gorm.DB
	router *gin.Engine
}

// newAPI mounts the watchlist and progress routes behind a stub auth
// middleware: the X-Test-User header becomes the authenticated user, and a
// request without it reaches the handler unauthenticated.
func newAPI(t *testing.T) *api {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.NewDB(t, &domain.User{}, &domain.Movie{}, &domain.Show{}, &domain.Genre{}, &domain.Review{},
		&domain.Watchlist{}, &domain.WatchlistItem{}, &domain.Collaborator{}, &domain.UserWatchProgress{},
		&domain.Notification{}, &domain.Friendship{})
	cat := fakeCatalog{db}
	r := gin.New()
	g := r.Group("/api/v1", func(c *gin.Context) {
		if u := c.GetHeader("X-Test-User"); u != "" {
			id, _ := uuid.Parse(u)
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), userIDKey, id))
		}
	})
	notifs := notifications.NewService(db)
	wl := watchlist.NewService(db, cat)
	wl.Notifier = notifs
	fs := friends.NewService(db)
	fs.Notifier = notifs
	NewNotificationHandler(notifs).RegisterRoutes(g)
	NewFriendHandler(fs).RegisterRoutes(g)
	NewWatchlistHandler(wl).RegisterRoutes(g)
	NewCollaborationHandler(wl).RegisterRoutes(g)
	NewProgressHandler(progress.NewService(db, cat)).RegisterRoutes(g)
	NewReviewHandler(reviews.NewService(db, cat)).RegisterRoutes(g)
	return &api{t: t, db: db, router: r}
}

func (a *api) do(user uuid.UUID, method, path string, body any) *httptest.ResponseRecorder {
	a.t.Helper()
	var rd *bytes.Reader
	switch b := body.(type) {
	case nil:
		rd = bytes.NewReader(nil)
	case string:
		rd = bytes.NewReader([]byte(b))
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			a.t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	if user != uuid.Nil {
		req.Header.Set("X-Test-User", user.String())
	}
	w := httptest.NewRecorder()
	a.router.ServeHTTP(w, req)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", w.Body, err)
	}
	return v
}

func (a *api) expect(w *httptest.ResponseRecorder, status int, code string) {
	a.t.Helper()
	if code == "" {
		if w.Code != status {
			a.t.Fatalf("status = %d, want %d (%s)", w.Code, status, w.Body)
		}
		return
	}
	assertErrorCode(a.t, w, status, code)
}

func TestRespondServiceErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{svcErr("notfound"), 404, "not_found"},
		{tmdb.ErrNotFound, 404, "not_found"},
		{svcErr("forbidden"), 403, "forbidden"},
		{svcErr("duplicate"), 409, "conflict"},
		{svcErr("invalid"), 400, "bad_request"},
		{fmt.Errorf("%w: x", catalog.ErrUpstream), 502, "upstream_error"},
		{fmt.Errorf("db exploded"), 500, "internal_error"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		RespondServiceError(c, tc.err)
		assertErrorCode(t, w, tc.status, tc.code)
	}
}

func TestBindJSONRejectsBadBodies(t *testing.T) {
	a := newAPI(t)
	u := uuid.New()
	for name, body := range map[string]string{
		"malformed":  "{",
		"wrong type": `{"title": 5}`,
		"too large":  `{"title":"` + strings.Repeat("x", maxBodyBytes) + `"}`,
	} {
		a.expect(a.do(u, http.MethodPost, "/watchlists", body), 400, "bad_request")
		_ = name
	}
}

func svcErr(kind string) error {
	switch kind {
	case "notfound":
		return fmt.Errorf("wrap: %w", svcerr.ErrNotFound)
	case "forbidden":
		return svcerr.ErrForbidden
	case "duplicate":
		return svcerr.ErrDuplicate
	}
	return errors.Join(svcerr.Invalid("nope"))
}
