package config

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSameSiteFromString(t *testing.T) {
	cases := map[string]http.SameSite{
		"strict": http.SameSiteStrictMode, "STRICT": http.SameSiteStrictMode,
		"none": http.SameSiteNoneMode, "None": http.SameSiteNoneMode,
		"lax": http.SameSiteLaxMode, "": http.SameSiteLaxMode, "bogus": http.SameSiteLaxMode,
	}
	for in, want := range cases {
		if got := sameSiteFromString(in); got != want {
			t.Errorf("sameSiteFromString(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := (&Config{CookieSameSite: "none", CookieSecure: false}).Validate(); err == nil {
		t.Error("expected SameSite=None without Secure to be rejected")
	}
	if err := (&Config{CookieSameSite: "none", CookieSecure: true}).Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := (&Config{CookieSameSite: "lax"}).Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCookieConfigs(t *testing.T) {
	c := &Config{
		SessionCookieName: "s", SessionCookieMaxAge: 99, CookiePath: "/p", CookieDomain: "d.example",
		CookieSecure: true, CookieHTTPOnly: true, CookieSameSite: "strict",
		OAuthStateCookieName: "o", OAuthStateCookiePath: "/auth",
	}
	s := c.SessionCookie()
	if s.Name != "s" || s.Path != "/p" || s.MaxAge != 99 || s.Domain != "d.example" || !s.Secure || !s.HTTPOnly || s.SameSite != http.SameSiteStrictMode {
		t.Errorf("session cookie config wrong: %+v", s)
	}
	o := c.OAuthStateCookie()
	if o.Name != "o" || o.Path != "/auth" || o.MaxAge != 600 {
		t.Errorf("oauth state cookie config wrong: %+v", o)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	unsetAllConfigEnv(t)
	t.Setenv("COOKIE_SAME_SITE", "none")
	t.Setenv("COOKIE_SECURE", "false")
	if _, err := Load(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid config") {
		t.Errorf("expected invalid config error, got %v", err)
	}
}

func TestLoadRejectsUnparsableValues(t *testing.T) {
	unsetAllConfigEnv(t)
	t.Setenv("SESSION_COOKIE_MAX_AGE", "not-a-number")
	if _, err := Load(context.Background()); err == nil {
		t.Error("expected parse error")
	}
}

func TestLoadCORSAndTMDB(t *testing.T) {
	unsetAllConfigEnv(t)
	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "http://localhost:5173" {
		t.Errorf("default CORS origins wrong: %v", cfg.CORSAllowedOrigins)
	}

	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.example,https://b.example")
	t.Setenv("TMDB_READ_ACCESS_TOKEN", "tok")
	cfg, err = Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CORSAllowedOrigins) != 2 || cfg.CORSAllowedOrigins[1] != "https://b.example" {
		t.Errorf("CORS origins not split: %v", cfg.CORSAllowedOrigins)
	}
	if cfg.TMDBReadAccessToken != "tok" {
		t.Errorf("token not read: %q", cfg.TMDBReadAccessToken)
	}
}
