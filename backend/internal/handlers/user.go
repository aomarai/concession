package handlers

import (
	"net/http"

	"github.com/aomarai/concession/internal/account"
	"github.com/aomarai/concession/internal/config"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/logging"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserHandler struct {
	DB      *gorm.DB
	cfg     *config.Config
	account *account.Service
}

func NewUserHandler(db *gorm.DB, cfg *config.Config) *UserHandler {
	return &UserHandler{DB: db, cfg: cfg, account: account.NewService(db)}
}

// RegisterRoutes mounts the current-user endpoints on r (authenticated group).
func (u *UserHandler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/me", u.HandleGetMe)
	r.PATCH("/me", u.HandleUpdateMe)
	r.DELETE("/me", u.HandleDeleteMe)
}

func (u *UserHandler) HandleGetMe(c *gin.Context) {
	logger := logging.FromContext(c.Request.Context())

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var user domain.User
	if err := u.DB.WithContext(c.Request.Context()).First(&user, "id = ?", userID).Error; err != nil {
		logger.Error("failed to load user", "error", err, "user_id", userID)
		RespondError(c, http.StatusNotFound, "not_found", "Not found")
		return
	}

	c.JSON(http.StatusOK, user)
}

// HandleUpdateMe serves PATCH /me: change the display name and/or username.
func (u *UserHandler) HandleUpdateMe(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req struct {
		DisplayName *string `json:"display_name"`
		Username    *string `json:"username"`
	}
	if !bindJSON(c, &req) {
		return
	}
	user, err := u.account.UpdateProfile(c.Request.Context(), userID, account.ProfileUpdate{
		DisplayName: req.DisplayName, Username: req.Username,
	})
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

// HandleDeleteMe serves DELETE /me: permanently delete the account. The body
// must carry the username or e-mail address as confirmation.
func (u *UserHandler) HandleDeleteMe(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req struct {
		Confirm string `json:"confirm"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := u.account.Delete(c.Request.Context(), userID, req.Confirm); err != nil {
		RespondServiceError(c, err)
		return
	}
	logging.FromContext(c.Request.Context()).Info("account deleted", "user_id", userID)
	clearSessionCookie(c, u.cfg)
	c.Status(http.StatusNoContent)
}
