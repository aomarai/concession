package handlers

import (
	"errors"
	"net/http"

	"github.com/aomarai/concession/internal/auth"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/logging"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterRoutes mounts the authenticated password endpoints on r.
func (h *AuthHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/me/password", h.HandleSetPassword)
}

// startSession issues a session for user and sets the cookie. It reports
// whether it succeeded; on failure the error response is already written.
func (h *AuthHandler) startSession(c *gin.Context, user *domain.User) bool {
	logger := logging.FromContext(c.Request.Context())

	rawToken, err := auth.CreateSession(c.Request.Context(), h.DB, user.ID, c.Request.UserAgent(), c.Request.RemoteAddr)
	if err != nil {
		logger.Error("failed to create session", "error", err)
		RespondError(c, http.StatusInternalServerError, "internal_error", "Authentication failed")
		return false
	}
	setSessionCookie(c, h.cfg, rawToken)
	return true
}

// HandleRegister serves POST /api/v1/auth/register: create a local (password)
// account and sign it in. Registration is open, so it runs under the shared
// per-IP sign-in rate limit.
func (h *AuthHandler) HandleRegister(c *gin.Context) {
	logger := logging.FromContext(c.Request.Context())

	var req struct {
		Email       string `json:"email"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if !bindJSON(c, &req) {
		return
	}

	user, err := h.user.RegisterLocalUser(c.Request.Context(), auth.LocalUserRegistration{
		Email:       req.Email,
		Username:    req.Username,
		DisplayName: req.DisplayName,
		Password:    req.Password,
	})
	if err != nil {
		if errors.Is(err, svcerr.ErrInvalid) || errors.Is(err, svcerr.ErrDuplicate) {
			RespondServiceError(c, err)
			return
		}
		logger.Error("failed to register local account", "error", err)
		RespondError(c, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}

	if !h.startSession(c, user) {
		return
	}
	c.JSON(http.StatusCreated, user)
}

// HandleLogin serves POST /api/v1/auth/login with a password. The identifier is
// an e-mail address or a username. Every failure answers the same 401, so the
// response cannot be used to discover which accounts exist.
func (h *AuthHandler) HandleLogin(c *gin.Context) {
	logger := logging.FromContext(c.Request.Context())

	var req struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if !bindJSON(c, &req) {
		return
	}

	user, err := h.user.AuthenticateLocal(c.Request.Context(), req.Identifier, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			logger.Warn("local sign-in failed")
			RespondError(c, http.StatusUnauthorized, "unauthorized", "Incorrect e-mail, username or password")
			return
		}
		logger.Error("failed to authenticate local account", "error", err)
		RespondError(c, http.StatusInternalServerError, "internal_error", "Authentication failed")
		return
	}

	if !h.startSession(c, user) {
		return
	}
	c.JSON(http.StatusOK, user)
}

// HandleSetPassword serves POST /api/v1/me/password: set a local password, or
// change the existing one by supplying the current password.
func (h *AuthHandler) HandleSetPassword(c *gin.Context) {
	logger := logging.FromContext(c.Request.Context())

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !bindJSON(c, &req) {
		return
	}

	err := h.user.SetPassword(c.Request.Context(), userID, req.CurrentPassword, req.NewPassword)
	switch {
	case errors.Is(err, svcerr.ErrInvalid):
		RespondServiceError(c, err)
	case errors.Is(err, gorm.ErrRecordNotFound):
		RespondError(c, http.StatusNotFound, "not_found", "Not found")
	case err != nil:
		logger.Error("failed to set password", "error", err)
		RespondError(c, http.StatusInternalServerError, "internal_error", "Internal error")
	default:
		c.Status(http.StatusNoContent)
	}
}
