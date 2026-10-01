package handlers

import (
	"net/http"
	"strconv"

	"github.com/aomarai/concession/internal/notifications"
	"github.com/gin-gonic/gin"
)

type NotificationHandler struct {
	svc *notifications.Service
}

func NewNotificationHandler(svc *notifications.Service) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// RegisterRoutes mounts the notification endpoints on r (authenticated group).
func (h *NotificationHandler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/me/notifications", h.List)
	r.GET("/me/notifications/unread-count", h.UnreadCount)
	r.POST("/me/notifications/read-all", h.MarkAllRead)
	r.POST("/me/notifications/:id/read", h.MarkRead)
}

func (h *NotificationHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	page, perPage, ok := pageParams(c)
	if !ok {
		return
	}
	unreadOnly := false
	if raw := c.Query("unread"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "bad_request", "Invalid unread")
			return
		}
		unreadOnly = v
	}
	p, err := h.svc.List(c.Request.Context(), userID, unreadOnly, page, perPage)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	n, err := h.svc.UnreadCount(c.Request.Context(), userID)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"unread_count": n})
}

func (h *NotificationHandler) MarkRead(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), userID, id); err != nil {
		RespondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	n, err := h.svc.MarkAllRead(c.Request.Context(), userID)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"marked": n})
}
