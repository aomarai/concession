// internal/auth/cookies.go
package auth

import (
	"net/http"
	"time"

	"github.com/aomarai/concession/internal/config"
)

// NewSessionCookie builds an http.Cookie for the session token.
// The caller decides whether to set it via http.SetCookie or gin.Context.SetCookie.
func NewSessionCookie(value string, cfg *config.Config) *http.Cookie {
	return &http.Cookie{
		Name:     "session_token",
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	}
}

// NewClearedSessionCookie builds an expired session cookie to clear the existing one.
func NewClearedSessionCookie(cfg *config.Config) *http.Cookie {
	c := NewSessionCookie("", cfg)
	c.MaxAge = -1
	c.Expires = time.Time{}
	return c
}
