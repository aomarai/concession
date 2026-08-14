package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/logging"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserHandler struct {
	DB *gorm.DB
}

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{DB: db}
}

func (u *UserHandler) HandleGetMe(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	userID, ok := r.Context().Value(userIDKey).(uuid.UUID)
	if !ok {
		logger.Error("user_id missing from context on authenticated route")
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	var user domain.User
	if err := u.DB.WithContext(r.Context()).First(&user, "id = ?", userID).Error; err != nil {
		logger.Error("failed to load user", "error", err, "user_id", userID)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(user); err != nil {
		logger.Error("failed to encode user response", "error", err)
	}
}
