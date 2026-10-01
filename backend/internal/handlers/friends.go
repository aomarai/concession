package handlers

import (
	"context"
	"net/http"

	"github.com/aomarai/concession/internal/friends"
	"github.com/aomarai/concession/internal/userref"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type FriendHandler struct {
	svc *friends.Service
}

func NewFriendHandler(svc *friends.Service) *FriendHandler { return &FriendHandler{svc: svc} }

// RegisterRoutes mounts the friends endpoints on r (authenticated group).
func (h *FriendHandler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/friends", h.List)
	r.POST("/friends", h.Send)
	r.DELETE("/friends/:user_id", h.Remove)
	r.GET("/friends/requests", h.ListRequests)
	r.POST("/friends/requests/:id/accept", h.Accept)
	r.POST("/friends/requests/:id/decline", h.Decline)
}

func (h *FriendHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	list, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"friends": list})
}

func (h *FriendHandler) Send(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req struct {
		User   string    `json:"user"`    // username or e-mail
		UserID uuid.UUID `json:"user_id"` // alternative: a user ID
	}
	if !bindJSON(c, &req) {
		return
	}
	entry, err := h.svc.Send(c.Request.Context(), userID, userref.Ref{Identifier: req.User, ID: req.UserID})
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, entry)
}

func (h *FriendHandler) Remove(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	other, ok := parseUUIDParam(c, "user_id")
	if !ok {
		return
	}
	if err := h.svc.Remove(c.Request.Context(), userID, other); err != nil {
		RespondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *FriendHandler) ListRequests(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	reqs, err := h.svc.ListRequests(c.Request.Context(), userID)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, reqs)
}

func (h *FriendHandler) Accept(c *gin.Context) { h.respond(c, h.svc.Accept) }

func (h *FriendHandler) Decline(c *gin.Context) { h.respond(c, h.svc.Decline) }

func (h *FriendHandler) respond(c *gin.Context, act func(ctx context.Context, me, requestID uuid.UUID) error) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if err := act(c.Request.Context(), userID, id); err != nil {
		RespondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
