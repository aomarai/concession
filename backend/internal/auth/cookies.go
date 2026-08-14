// internal/auth/cookies.go
package auth

import (
	"net/http"
	"time"

	"github.com/aomarai/concession/internal/config"
)

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

func NewClearedSessionCookie(cfg *config.Config) *http.Cookie {
	c := NewSessionCookie("", cfg)
	c.MaxAge = -1
	c.Expires = time.Time{}
	return c
}
