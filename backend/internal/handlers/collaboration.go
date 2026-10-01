package handlers

import (
	"context"
	"net/http"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/watchlist"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CollaborationHandler serves invitations, membership, roles and share links
// for watchlists.
type CollaborationHandler struct {
	svc *watchlist.Service
}

func NewCollaborationHandler(svc *watchlist.Service) *CollaborationHandler {
	return &CollaborationHandler{svc: svc}
}

// RegisterRoutes mounts the collaboration endpoints on r (authenticated group).
func (h *CollaborationHandler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/watchlists/:id/collaborators", h.ListMembers)
	r.POST("/watchlists/:id/collaborators", h.Invite)
	r.PATCH("/watchlists/:id/collaborators/:user_id", h.SetRole)
	r.DELETE("/watchlists/:id/collaborators/:user_id", h.Remove)
	r.POST("/watchlists/:id/share-token", h.RotateShareToken)
	r.GET("/shared/:token", h.GetShared)
	r.GET("/me/invites", h.ListInvites)
	r.POST("/invites/:id/accept", h.Accept)
	r.POST("/invites/:id/decline", h.Decline)
}

func (h *CollaborationHandler) ListMembers(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	m, err := h.svc.ListMembers(c.Request.Context(), userID, id)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *CollaborationHandler) Invite(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req struct {
		User string                  `json:"user"`
		Role domain.CollaboratorRole `json:"role"`
	}
	if !bindJSON(c, &req) {
		return
	}
	m, err := h.svc.InviteUser(c.Request.Context(), userID, id, req.User, req.Role)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, m)
}

func (h *CollaborationHandler) SetRole(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	target, ok := parseUUIDParam(c, "user_id")
	if !ok {
		return
	}
	var req struct {
		Role domain.CollaboratorRole `json:"role"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.SetRole(c.Request.Context(), userID, id, target, req.Role); err != nil {
		RespondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CollaborationHandler) Remove(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	target, ok := parseUUIDParam(c, "user_id")
	if !ok {
		return
	}
	if err := h.svc.RemoveMember(c.Request.Context(), userID, id, target); err != nil {
		RespondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CollaborationHandler) RotateShareToken(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	token, err := h.svc.RotateShareToken(c.Request.Context(), userID, id)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"share_token": token})
}

func (h *CollaborationHandler) GetShared(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	d, err := h.svc.GetShared(c.Request.Context(), userID, c.Param("token"))
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *CollaborationHandler) ListInvites(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	invites, err := h.svc.ListInvites(c.Request.Context(), userID)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"invites": invites})
}

func (h *CollaborationHandler) Accept(c *gin.Context) { h.respondToInvite(c, h.svc.AcceptInvite) }

func (h *CollaborationHandler) Decline(c *gin.Context) { h.respondToInvite(c, h.svc.DeclineInvite) }

func (h *CollaborationHandler) respondToInvite(c *gin.Context, act func(ctx context.Context, userID, inviteID uuid.UUID) error) {
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
