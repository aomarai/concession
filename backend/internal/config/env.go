package config

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/sethvargo/go-envconfig"
)

// CookieConfig holds the shared attributes for a cookie (name, path, domain,
// security flags, etc.). It is reused for both the session and oauth_state
// cookies so clearing helpers always mirror the original cookie's attributes.
type CookieConfig struct {
	Name     string
	Path     string
	Domain   string
	Secure   bool
	HTTPOnly bool
	SameSite http.SameSite
	MaxAge   time.Duration
}

// SameSiteFromString converts a string representation of SameSite to the
// corresponding http.SameSite constant. The comparison is case-insensitive to
// tolerate common env-var variations (e.g. "strict", "NONE", "lax").
func SameSiteFromString(s string) http.SameSite {
	switch strings.ToLower(s) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	case "lax":
		return http.SameSiteLaxMode
	default:
		// fall back to Lax for unknown values
		return http.SameSiteLaxMode
	}
}

type Config struct {
	DBDriver             string `env:"DB_DRIVER,default=sqlite"`
	DBHost               string `env:"DB_HOST"`
	DBUser               string `env:"DB_USER"`
	DBPassword           string `env:"DB_PASSWORD"`
	DBName               string `env:"DB_NAME"`
	DBPort               string `env:"DB_PORT"`
	DBPath               string `env:"DB_PATH"`
	Environment          string `env:"ENV,default=development"`
	JWTSecret            string `env:"JWT_SECRET"`
	Port                 string `env:"PORT"`
	CookieSecure         bool   `env:"COOKIE_SECURE,default=true"`
	CookieHTTPOnly       bool   `env:"COOKIE_HTTP_ONLY,default=true"`
	CookieSameSite       string `env:"COOKIE_SAME_SITE,default=Lax"`
	CookiePath           string `env:"COOKIE_PATH,default=/"`
	CookieDomain         string `env:"COOKIE_DOMAIN"`
	SessionCookieName    string `env:"SESSION_COOKIE_NAME,default=session_token"`
	SessionCookieMaxAge  int    `env:"SESSION_COOKIE_MAX_AGE,default=2592000"` // seconds (30 days)
	OAuthStateCookieName string `env:"OAUTH_STATE_COOKIE_NAME,default=oauth_state"`
	OAuthStateCookiePath string `env:"OAUTH_STATE_COOKIE_PATH,default=/"`
	GoogleClientID       string `env:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret   string `env:"GOOGLE_CLIENT_SECRET"`
	GoogleRedirectURL    string `env:"GOOGLE_REDIRECT_URL"`
}

// SessionCookie builds a CookieConfig from the session cookie config fields.
func (c *Config) SessionCookie() CookieConfig {
	return CookieConfig{
		Name:     c.SessionCookieName,
		Path:     c.CookiePath,
		Domain:   c.CookieDomain,
		Secure:   c.CookieSecure,
		HTTPOnly: c.CookieHTTPOnly,
		SameSite: SameSiteFromString(c.CookieSameSite),
		MaxAge:   time.Duration(c.SessionCookieMaxAge) * time.Second,
	}
}

// OAuthStateCookie builds a CookieConfig from the oauth_state cookie config fields.
func (c *Config) OAuthStateCookie() CookieConfig {
	return CookieConfig{
		Name:     c.OAuthStateCookieName,
		Path:     c.OAuthStateCookiePath,
		Domain:   c.CookieDomain,
		Secure:   c.CookieSecure,
		HTTPOnly: c.CookieHTTPOnly,
		SameSite: SameSiteFromString(c.CookieSameSite),
		MaxAge:   10 * time.Minute,
	}
}

func Load(ctx context.Context) (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}
	if err := envconfig.Process(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
