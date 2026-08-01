package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/aomarai/concession/internal/auth"
	"github.com/aomarai/concession/internal/config"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/logging"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB *gorm.DB
}

func NewAuthHandler(db *gorm.DB) *AuthHandler {
	return &AuthHandler{DB: db}
}

func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (h *AuthHandler) HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	state, err := generateRandomToken()
	if err != nil {
		logger.Error("failed to generate oauth state", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(10 * time.Minute),
	})

	cfg := config.GetGoogleOAuthConfig()
	url := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

type googleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func (h *AuthHandler) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	// 1. Verify state to prevent CSRF
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || r.URL.Query().Get("state") != stateCookie.Value {
		logger.Warn("oauth state mismatch or missing")
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "oauth_state", Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		logger.Warn("missing oauth code")
		http.Error(w, "Missing code", http.StatusBadRequest)
		return
	}

	// 2. Exchange code for a Google access token
	cfg := config.GetGoogleOAuthConfig()
	token, err := cfg.Exchange(r.Context(), code)
	if err != nil {
		logger.Error("oauth code exchange failed", "error", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	// 3. Fetch the user's Google profile
	client := cfg.Client(r.Context(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		logger.Error("failed to fetch google userinfo", "error", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Error("failed to close body", "error", err)
			http.Error(w, "Authentication failed", http.StatusInternalServerError)
		}
	}(resp.Body)

	var info googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		logger.Error("failed to decode google userinfo", "error", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	// 4. Find or create the local user + oauth link
	user, err := h.findOrCreateUser(r.Context(), info)
	if err != nil {
		logger.Error("failed to resolve user", "error", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	// 5. Issue a session
	rawToken, err := auth.CreateSession(r.Context(), h.DB, user.ID, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		logger.Error("failed to create session", "error", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (h *AuthHandler) findOrCreateUser(ctx context.Context, info googleUserInfo) (*domain.User, error) {
	var oauthAccount domain.OAuthAccount
	err := h.DB.WithContext(ctx).
		Where("provider = ? AND provider_user_id = ?", "google", info.ID).
		First(&oauthAccount).Error

	if err == nil {
		var user domain.User
		if err := h.DB.WithContext(ctx).First(&user, "id = ?", oauthAccount.UserID).Error; err != nil {
			return nil, err
		}
		return &user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// No linked account yet — create user (or attach to an existing one by email later if you want that flow)
	user := domain.User{
		Username:        info.Email,
		Email:           info.Email,
		IsEmailVerified: info.VerifiedEmail,
		DisplayName:     info.Name,
		AvatarURL:       info.Picture,
	}
	if err := h.DB.WithContext(ctx).Create(&user).Error; err != nil {
		return nil, err
	}

	oauthAccount = domain.OAuthAccount{
		UserID:         user.ID,
		Provider:       "google",
		ProviderUserID: info.ID,
	}
	if err := h.DB.WithContext(ctx).Create(&oauthAccount).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	cookie, err := r.Cookie("session_token")
	if err == nil {
		if revokeErr := auth.RevokeSession(r.Context(), h.DB, cookie.Value); revokeErr != nil {
			logger.Error("failed to revoke session", "error", revokeErr)
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	w.WriteHeader(http.StatusOK)
}
