package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/ratelimit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func limitedEngine(l *ratelimit.Limiter, key KeyFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", RateLimit(l, key), func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return r
}

func hit(r *gin.Engine, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = remote
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRateLimitAnswers429WithRetryAfter(t *testing.T) {
	r := limitedEngine(ratelimit.New(2, time.Minute), ClientIPKey)
	for i := 0; i < 2; i++ {
		if w := hit(r, "10.0.0.1:1111"); w.Code != http.StatusOK {
			t.Fatalf("request %d: %d", i+1, w.Code)
		}
	}
	w := hit(r, "10.0.0.1:2222") // same IP, different port: still the same client
	assertErrorCode(t, w, http.StatusTooManyRequests, "rate_limited")
	if secs, err := strconv.Atoi(w.Header().Get("Retry-After")); err != nil || secs < 1 || secs > 30 {
		t.Errorf("Retry-After = %q, want a whole number of seconds between 1 and 30", w.Header().Get("Retry-After"))
	}
	if w := hit(r, "10.0.0.2:1111"); w.Code != http.StatusOK {
		t.Errorf("another client has its own budget, got %d", w.Code)
	}
}

func TestRateLimitWithNoLimiterPassesThrough(t *testing.T) {
	r := limitedEngine(nil, ClientIPKey)
	for i := 0; i < 50; i++ {
		if w := hit(r, "10.0.0.1:1"); w.Code != http.StatusOK {
			t.Fatalf("request %d: %d", i+1, w.Code)
		}
	}
}

func TestUserKeyPrefersTheAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.RemoteAddr = "10.9.9.9:1"
	if got := UserKey(c); got != "10.9.9.9" {
		t.Errorf("without a user the key falls back to the client IP, got %q", got)
	}
	id := uuid.New()
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), userIDKey, id))
	if got := UserKey(c); got != id.String() {
		t.Errorf("UserKey = %q, want the user id", got)
	}
}

func TestFriendRequestsAreLimitedPerUser(t *testing.T) {
	a := newAPIWithLimits(t, testLimits{friendRequests: ratelimit.New(2, time.Hour)})
	me, other := a.newUser("me"), a.newUser("other")
	for _, name := range []string{"t1", "t2", "t3"} {
		a.newUser(name)
	}
	a.expect(a.do(me, http.MethodPost, "/friends", map[string]any{"user": "t1"}), http.StatusCreated, "")
	a.expect(a.do(me, http.MethodPost, "/friends", map[string]any{"user": "t2"}), http.StatusCreated, "")
	a.expect(a.do(me, http.MethodPost, "/friends", map[string]any{"user": "t3"}), http.StatusTooManyRequests, "rate_limited")

	// Reading, answering requests and other people are unaffected.
	a.expect(a.do(me, http.MethodGet, "/friends", nil), http.StatusOK, "")
	a.expect(a.do(other, http.MethodPost, "/friends", map[string]any{"user": "t1"}), http.StatusCreated, "")
}

func TestInvitesAreLimitedPerUser(t *testing.T) {
	a := newAPIWithLimits(t, testLimits{invites: ratelimit.New(1, time.Hour)})
	owner := a.newUser("owner")
	a.newUser("p1")
	a.newUser("p2")
	list := a.createList(owner, "movie")
	path := "/watchlists/" + list.ID.String() + "/collaborators"
	a.expect(a.do(owner, http.MethodPost, path, map[string]any{"user": "p1", "role": "editor"}), http.StatusCreated, "")
	a.expect(a.do(owner, http.MethodPost, path, map[string]any{"user": "p2", "role": "editor"}), http.StatusTooManyRequests, "rate_limited")
	a.expect(a.do(owner, http.MethodGet, path, nil), http.StatusOK, "")
}

func TestOpenStreamsAreCappedPerUser(t *testing.T) {
	a := newAPIWithLimits(t, testLimits{streams: ratelimit.NewGate(1)})
	srv := httptest.NewServer(a.router)
	t.Cleanup(srv.Close)
	user, other := a.newUser("u"), a.newUser("o")
	list := a.createList(user, "movie")
	otherList := a.createList(other, "movie")

	first := a.openStream(srv, user, list.ID)
	first.expect("ready")

	second := a.openStream(srv, user, list.ID)
	if second.resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second stream status = %d, want 429", second.resp.StatusCode)
	}
	body, _ := io.ReadAll(second.resp.Body)
	if !strings.Contains(string(body), "rate_limited") {
		t.Errorf("body = %s", body)
	}
	if a.hub.Subscribers(list.ID) != 1 {
		t.Error("a refused stream must not subscribe")
	}

	// Another user is not affected.
	a.openStream(srv, other, otherList.ID).expect("ready")

	// Closing the first stream frees the slot.
	first.cancel()
	deadline := time.Now().Add(3 * time.Second)
	for {
		s := a.openStream(srv, user, list.ID)
		if s.resp.StatusCode == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the slot was never released")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWithMiddlewareRunsTheMiddlewareBeforeTheHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var order []string
	mw := func(*gin.Context) { order = append(order, "middleware") }
	h := func(c *gin.Context) { order = append(order, "handler"); c.Status(http.StatusNoContent) }

	run := func(chain []gin.HandlerFunc) []string {
		order = nil
		r := gin.New()
		r.GET("/x", chain...)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d: the handler did not run", w.Code)
		}
		return order
	}
	if got := run(withMiddleware(nil, h)); strings.Join(got, ",") != "handler" {
		t.Errorf("without middleware ran %v", got)
	}
	if got := run(withMiddleware(mw, h)); strings.Join(got, ",") != "middleware,handler" {
		t.Errorf("with middleware ran %v", got)
	}
}

func TestClientIPKeyGroupsIPv6ClientsByTheir64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := func(remote string) string {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Request.RemoteAddr = remote
		return ClientIPKey(c)
	}
	// One household or host usually owns a whole /64, so rotating the low
	// bits must not buy a fresh budget.
	if a, b := key("[2001:db8:1:2::1]:80"), key("[2001:db8:1:2:ffff:abcd::9]:80"); a != b {
		t.Errorf("same /64 gave different keys %q and %q", a, b)
	}
	if a, b := key("[2001:db8:1:2::1]:80"), key("[2001:db8:1:3::1]:80"); a == b {
		t.Errorf("different /64s share the key %q", a)
	}
	if got := key("192.0.2.7:80"); got != "192.0.2.7" {
		t.Errorf("IPv4 key = %q, want the address itself", got)
	}
	if got := key("garbage"); got != "" && got != "garbage" {
		t.Errorf("an unparsable peer should pass through unchanged, got %q", got)
	}
}
