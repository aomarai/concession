package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/aomarai/concession/internal/auth"
	"github.com/aomarai/concession/internal/config"
	"github.com/aomarai/concession/internal/logging"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB   *gorm.DB
	cfg  *config.Config
	user *auth.UserAuthService
}

func NewAuthHandler(db *gorm.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		DB:   db,
		cfg:  cfg,
		user: auth.NewUserAuthService(db),
	}
}

func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (h *AuthHandler) HandleGoogleLogin(c *gin.Context) {
	logger := logging.FromContext(c.Request.Context())

	state, err := auth.GenerateRandomToken(32)
	if err != nil {
		logger.Error("failed to generate oauth state", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal error"})
		return
	}

	cookie := &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(10 * time.Minute),
	}
	c.Header("Set-Cookie", cookie.String())

	cfg := config.GetGoogleOAuthConfig(h.cfg)
	url := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

type googleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func (h *AuthHandler) HandleGoogleCallback(c *gin.Context) {
	logger := logging.FromContext(c.Request.Context())

	// 1. Verify state to prevent CSRF
	if !validateState(c) {
		return
	}

	code := c.Query("code")
	if code == "" {
		logger.Warn("missing oauth code")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Missing code"})
		return
	}

	// 2. Exchange code for a Google access token
	cfg := config.GetGoogleOAuthConfig(h.cfg)
	token, err := cfg.Exchange(c.Request.Context(), code) // 👈 Save the returned token
	if err != nil {
		logger.Error("oauth code exchange failed", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Authentication failed"})
		return
	}

	// 3. Fetch the user's Google profile
	info, err := fetchGoogleUserInfo(c.Request.Context(), cfg, token)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Authentication failed"})
		return
	}

	// 4. Find or create the local user + oauth link
	user, err := h.user.FindOrCreateGoogleUser(c.Request.Context(), auth.GoogleUserInfo(info))
	if err != nil {
		logger.Error("failed to resolve user", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Authentication failed"})
		return
	}

	// 5. Issue a session
	rawToken, err := auth.CreateSession(c.Request.Context(), h.DB, user.ID, c.Request.UserAgent(), c.Request.RemoteAddr)
	if err != nil {
		logger.Error("failed to create session", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Authentication failed"})
		return
	}

	setSessionCookie(c, h.cfg, rawToken)
	c.Redirect(http.StatusTemporaryRedirect, "/")
}

func (h *AuthHandler) HandleLogout(c *gin.Context) {
	logger := logging.FromContext(c.Request.Context())

	cookie, err := c.Cookie("session_token")
	if err == nil {
		if revokeErr := auth.RevokeSession(c.Request.Context(), h.DB, cookie); revokeErr != nil {
			logger.Error("failed to revoke session", "error", revokeErr)
		}
	}
	clearSessionCookie(c, h.cfg)
	c.Status(http.StatusOK)
}

// clearSessionCookie clears an invalid/expired session cookie to avoid repeated errors
// and to keep auth behavior predictable.
func clearSessionCookie(c *gin.Context, cfg *config.Config) {
	cookie := auth.NewClearedSessionCookie(cfg)
	c.Header("Set-Cookie", cookie.String())
}

func setSessionCookie(c *gin.Context, cfg *config.Config, rawToken string) {
	cookie := auth.NewSessionCookie(rawToken, cfg)
	c.Header("Set-Cookie", cookie.String())
}

func validateState(c *gin.Context) bool {
	ctxLogger := logging.FromContext(c.Request.Context())
	stateCookie, err := c.Cookie("oauth_state")
	if err != nil || c.Query("state") != stateCookie {
		ctxLogger.Warn("oauth state mismatch or missing")
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return false
	}
	c.Header("Set-Cookie", "oauth_state=; Max-Age=0; Path=/; HttpOnly")
	return true
}

func fetchGoogleUserInfo(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (googleUserInfo, error) {
	ctxLogger := logging.FromContext(ctx)
	client := cfg.Client(ctx, token)

	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		ctxLogger.Error("failed to fetch google userinfo", "error", err)
		return googleUserInfo{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			ctxLogger.Error("failed to close body", "error", err)
		}
	}()

	var info googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		ctxLogger.Error("failed to decode google userinfo", "error", err)
		return googleUserInfo{}, err
	}
	return info, nil
}
