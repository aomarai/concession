package handlers

import (
	"net/http"

	"github.com/aomarai/concession/internal/config"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB        *gorm.DB
	JWTSecret []byte
}

func NewAuthHandler(db *gorm.DB, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		DB:        db,
		JWTSecret: []byte(jwtSecret),
	}
}

func (h *AuthHandler) HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGoogleOAuthConfig()
	url := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

//func (h *AuthHandler) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
//	TODO: Write this entire function
//}
