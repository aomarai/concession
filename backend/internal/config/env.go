package config

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

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
	MaxAge   int // seconds
}

// sameSiteFromString converts a string representation of SameSite to the
// corresponding http.SameSite constant. The comparison is case-insensitive to
// tolerate common env-var variations (e.g. "strict", "NONE", "lax").
func sameSiteFromString(s string) http.SameSite {
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

// baseCookieConfig builds a CookieConfig with the shared attributes (domain,
// secure, httpOnly, sameSite) and the caller-provided name, path, and maxAge.
// This centralizes cookie attribute wiring so SessionCookie and
// OAuthStateCookie stay focused on their specific differences.
func (c *Config) baseCookieConfig(name, path string, maxAge int) CookieConfig {
	return CookieConfig{
		Name:     name,
		Path:     path,
		Domain:   c.CookieDomain,
		Secure:   c.CookieSecure,
		HTTPOnly: c.CookieHTTPOnly,
		SameSite: sameSiteFromString(c.CookieSameSite),
		MaxAge:   maxAge,
	}
}

// SessionCookie builds a CookieConfig for the session cookie.
func (c *Config) SessionCookie() CookieConfig {
	return c.baseCookieConfig(
		c.SessionCookieName,
		c.CookiePath,
		c.SessionCookieMaxAge,
	)
}

// OAuthStateCookie builds a CookieConfig for the oauth_state cookie.
func (c *Config) OAuthStateCookie() CookieConfig {
	return c.baseCookieConfig(
		c.OAuthStateCookieName,
		c.OAuthStateCookiePath,
		600, // 10 minutes in seconds
	)
}

// Validate checks the config for invariants that cannot be expressed via
// env tags alone. It returns an error describing the first problem found.
func (c *Config) Validate() error {
	// Browsers reject SameSite=None cookies that are not marked Secure.
	// Enforce this at config-load time so the misconfiguration is caught early.
	if sameSiteFromString(c.CookieSameSite) == http.SameSiteNoneMode && !c.CookieSecure {
		return errors.New("COOKIE_SAME_SITE=None requires COOKIE_SECURE=true (modern browsers reject insecure SameSite=None cookies)")
	}
	return nil
}

type Config struct {
	DBDriver             string `env:"DB_DRIVER"`
	DBHost               string `env:"DB_HOST"`
	DBUser               string `env:"DB_USER"`
	DBPassword           string `env:"DB_PASSWORD"`
	DBName               string `env:"DB_NAME"`
	DBPort               string `env:"DB_PORT"`
	DBPath               string `env:"DB_PATH"`
	Environment          string `env:"ENV"`
	JWTSecret            string `env:"JWT_SECRET"`
	Port                 string `env:"PORT"`
	CookieSecure         bool   `env:"COOKIE_SECURE"`
	CookieHTTPOnly       bool   `env:"COOKIE_HTTP_ONLY"`
	CookieSameSite       string `env:"COOKIE_SAME_SITE"`
	CookiePath           string `env:"COOKIE_PATH"`
	CookieDomain         string `env:"COOKIE_DOMAIN"`
	SessionCookieName    string `env:"SESSION_COOKIE_NAME"`
	SessionCookieMaxAge  int    `env:"SESSION_COOKIE_MAX_AGE"`
	OAuthStateCookieName string `env:"OAUTH_STATE_COOKIE_NAME"`
	OAuthStateCookiePath string `env:"OAUTH_STATE_COOKIE_PATH"`
	GoogleClientID       string `env:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret   string `env:"GOOGLE_CLIENT_SECRET"`
	GoogleRedirectURL    string `env:"GOOGLE_REDIRECT_URL"`
}

func Load(ctx context.Context) (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}
	if err := envconfig.Process(ctx, cfg); err != nil {
		return nil, err
	}

	// Apply defaults for fields that need them (envconfig doesn't support
	// envDefault tags, so we apply them manually after processing).
	applyDefaults(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

// applyDefaults sets default values for config fields when environment
// variables are not provided. This is necessary because go-envconfig doesn't
// support envDefault tags.
func applyDefaults(cfg *Config) {
	if _, ok := os.LookupEnv("DB_DRIVER"); !ok {
		cfg.DBDriver = "sqlite"
	}
	if _, ok := os.LookupEnv("ENV"); !ok {
		cfg.Environment = "development"
	}
	if _, ok := os.LookupEnv("COOKIE_SECURE"); !ok {
		cfg.CookieSecure = true
	}
	if _, ok := os.LookupEnv("COOKIE_HTTP_ONLY"); !ok {
		cfg.CookieHTTPOnly = true
	}
	if _, ok := os.LookupEnv("COOKIE_PATH"); !ok {
		cfg.CookiePath = "/"
	}
	if _, ok := os.LookupEnv("SESSION_COOKIE_NAME"); !ok {
		cfg.SessionCookieName = "session_token"
	}
	if _, ok := os.LookupEnv("SESSION_COOKIE_MAX_AGE"); !ok {
		cfg.SessionCookieMaxAge = 2592000 // 30 days in seconds
	}
	if _, ok := os.LookupEnv("OAUTH_STATE_COOKIE_NAME"); !ok {
		cfg.OAuthStateCookieName = "oauth_state"
	}
}
