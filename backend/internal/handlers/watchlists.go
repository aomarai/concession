package handlers

import (
	"net/http"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/watchlist"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type WatchlistHandler struct {
	svc *watchlist.Service
}

func NewWatchlistHandler(svc *watchlist.Service) *WatchlistHandler {
	return &WatchlistHandler{svc: svc}
}

// RegisterRoutes mounts the watchlist endpoints on r (authenticated group).
func (h *WatchlistHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/watchlists", h.Create)
	r.GET("/watchlists", h.List)
	r.GET("/watchlists/:id", h.Get)
	r.PATCH("/watchlists/:id", h.Update)
	r.DELETE("/watchlists/:id", h.Delete)
	r.POST("/watchlists/:id/items", h.AddItem)
	r.PUT("/watchlists/:id/items/order", h.Reorder)
	r.PATCH("/watchlists/:id/items/:item_id", h.UpdateItem)
	r.DELETE("/watchlists/:id/items/:item_id", h.RemoveItem)
}

func (h *WatchlistHandler) Create(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req struct {
		Title       string               `json:"title"`
		Description string               `json:"description"`
		Privacy     domain.PrivacyLevel  `json:"privacy"`
		Type        domain.WatchlistType `json:"type"`
	}
	if !bindJSON(c, &req) {
		return
	}
	w, err := h.svc.Create(c.Request.Context(), userID, watchlist.CreateInput{
		Title: req.Title, Description: req.Description, Privacy: req.Privacy, Type: req.Type,
	})
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, w)
}

func (h *WatchlistHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	lists, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"watchlists": lists})
}

func (h *WatchlistHandler) Get(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	d, err := h.svc.Get(c.Request.Context(), userID, id)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *WatchlistHandler) Update(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req struct {
		Title       *string              `json:"title"`
		Description *string              `json:"description"`
		Privacy     *domain.PrivacyLevel `json:"privacy"`
	}
	if !bindJSON(c, &req) {
		return
	}
	w, err := h.svc.Update(c.Request.Context(), userID, id, watchlist.UpdateInput{
		Title: req.Title, Description: req.Description, Privacy: req.Privacy,
	})
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, w)
}

func (h *WatchlistHandler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, id); err != nil {
		RespondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *WatchlistHandler) AddItem(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req struct {
		TMDBID int64  `json:"tmdb_id"`
		Notes  string `json:"notes"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.TMDBID <= 0 {
		RespondError(c, http.StatusBadRequest, "bad_request", "tmdb_id is required")
		return
	}
	item, err := h.svc.AddItem(c.Request.Context(), userID, id, req.TMDBID, req.Notes)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *WatchlistHandler) UpdateItem(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	itemID, ok := parseUUIDParam(c, "item_id")
	if !ok {
		return
	}
	var req struct {
		Notes string `json:"notes"`
	}
	if !bindJSON(c, &req) {
		return
	}
	item, err := h.svc.UpdateItemNotes(c.Request.Context(), userID, id, itemID, req.Notes)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *WatchlistHandler) RemoveItem(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	itemID, ok := parseUUIDParam(c, "item_id")
	if !ok {
		return
	}
	if err := h.svc.RemoveItem(c.Request.Context(), userID, id, itemID); err != nil {
		RespondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *WatchlistHandler) Reorder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req struct {
		ItemIDs []uuid.UUID `json:"item_ids"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.Reorder(c.Request.Context(), userID, id, req.ItemIDs); err != nil {
		RespondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
