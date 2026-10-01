package handlers

import (
	"net/http"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/logging"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

	userID, ok := c.Request.Context().Value(userIDKey).(uuid.UUID)
	if !ok {
		logger.Error("user_id missing from context on authenticated route")
		RespondError(c, http.StatusInternalServerError, "internal_error", "Internal error")
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
