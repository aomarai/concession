package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/aomarai/concession/internal/config"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
)

var errEntropy = errors.New("no entropy")

func failRandom(t *testing.T) {
	t.Helper()
	orig := randRead
	randRead = func([]byte) (int, error) { return 0, errEntropy }
	t.Cleanup(func() { randRead = orig })
}

func TestGenerateRandomTokenEntropyFailure(t *testing.T) {
	failRandom(t)
	if _, err := GenerateRandomToken(8); !errors.Is(err, errEntropy) {
		t.Errorf("expected entropy error, got %v", err)
	}
	if _, err := GenerateSessionToken(); !errors.Is(err, errEntropy) {
		t.Errorf("expected entropy error, got %v", err)
	}
	db := testutil.NewDB(t, &domain.Session{})
	if _, err := CreateSession(context.Background(), db, uuid.New(), "ua", "ip"); !errors.Is(err, errEntropy) {
		t.Errorf("expected entropy error from CreateSession, got %v", err)
	}
}

func TestCreateSessionDBFailure(t *testing.T) {
	db := testutil.NewDB(t, &domain.Session{})
	testutil.FailOn(t, db, "create", "sessions")
	if tok, err := CreateSession(context.Background(), db, uuid.New(), "ua", "ip"); !errors.Is(err, testutil.ErrInjected) || tok != "" {
		t.Errorf("expected injected error, got %q, %v", tok, err)
	}
}

func TestCookieConstructors(t *testing.T) {
	cfg := &config.Config{
		SessionCookieName: "s", SessionCookieMaxAge: 60, CookiePath: "/",
		CookieDomain: "example.com", CookieSecure: true, CookieHTTPOnly: true, CookieSameSite: "strict",
		OAuthStateCookieName: "o", OAuthStateCookiePath: "/auth",
	}

	c := NewSessionCookie("tok", cfg)
	if c.Name != "s" || c.Value != "tok" || c.MaxAge != 60 || c.Domain != "example.com" || !c.Secure || !c.HttpOnly {
		t.Errorf("session cookie wrong: %+v", c)
	}
	cleared := NewClearedSessionCookie(cfg)
	if cleared.Name != "s" || cleared.Value != "" || cleared.MaxAge != -1 || cleared.Path != "/" || cleared.Domain != "example.com" {
		t.Errorf("cleared session cookie wrong: %+v", cleared)
	}
	st := NewClearedOAuthStateCookie(cfg)
	if st.Name != "o" || st.MaxAge != -1 || st.Path != "/auth" || !st.Secure {
		t.Errorf("cleared state cookie wrong: %+v", st)
	}
}
