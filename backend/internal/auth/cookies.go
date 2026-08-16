package auth

import (
	"net/http"

	"github.com/aomarai/concession/internal/config"
)

// NewSessionCookie creates a session cookie using the configured cookie settings.
func NewSessionCookie(rawToken string, cfg *config.Config) *http.Cookie {
	cookieCfg := cfg.SessionCookie()
	return &http.Cookie{
		Name:     cookieCfg.Name,
		Value:    rawToken,
		Path:     cookieCfg.Path,
		MaxAge:   cookieCfg.MaxAge,
		Secure:   cookieCfg.Secure,
		HttpOnly: cookieCfg.HTTPOnly,
		SameSite: cookieCfg.SameSite,
		Domain:   cookieCfg.Domain,
	}
}

// NewClearedSessionCookie creates a cookie that clears the session cookie by
// setting MaxAge to -1. It mirrors the original session cookie's attributes so
// browsers treat the clearing cookie identically to the one that was set.
func NewClearedSessionCookie(cfg *config.Config) *http.Cookie {
	cookieCfg := cfg.SessionCookie()
	return &http.Cookie{
		Name:     cookieCfg.Name,
		Value:    "",
		Path:     cookieCfg.Path,
		MaxAge:   -1,
		Secure:   cookieCfg.Secure,
		HttpOnly: cookieCfg.HTTPOnly,
		SameSite: cookieCfg.SameSite,
		Domain:   cookieCfg.Domain,
	}
}

// NewClearedOAuthStateCookie creates a cookie that clears the oauth_state
// cookie by setting MaxAge to -1. It mirrors the original oauth_state cookie's
// attributes so browsers treat the clearing cookie identically to the one that
// was set.
func NewClearedOAuthStateCookie(cfg *config.Config) *http.Cookie {
	cookieCfg := cfg.OAuthStateCookie()
	return &http.Cookie{
		Name:     cookieCfg.Name,
		Value:    "",
		Path:     cookieCfg.Path,
		MaxAge:   -1,
		Secure:   cookieCfg.Secure,
		HttpOnly: cookieCfg.HTTPOnly,
		SameSite: cookieCfg.SameSite,
		Domain:   cookieCfg.Domain,
	}
}
