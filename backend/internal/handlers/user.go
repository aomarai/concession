package handlers

import (
	"net/http"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/logging"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserHandler struct {
	DB *gorm.DB
}

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{DB: db}
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
