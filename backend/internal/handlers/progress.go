package handlers

import (
	"net/http"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/progress"
	"github.com/gin-gonic/gin"
)

type ProgressHandler struct {
	svc *progress.Service
}

func NewProgressHandler(svc *progress.Service) *ProgressHandler {
	return &ProgressHandler{svc: svc}
}

// RegisterRoutes mounts the progress endpoints on r (authenticated group).
func (h *ProgressHandler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/me/progress", h.List)
	r.GET("/me/progress/:kind/:tmdb_id", h.Get)
	r.PUT("/me/progress/:kind/:tmdb_id", h.Set)
	r.DELETE("/me/progress/:kind/:tmdb_id", h.Delete)
}

// target parses the :kind and :tmdb_id path parameters.
func progressTarget(c *gin.Context) (domain.ItemType, int64, bool) {
	kind, ok := progress.ParseKind(c.Param("kind"))
	if !ok {
		RespondError(c, http.StatusBadRequest, "bad_request", "kind must be movies or shows")
		return "", 0, false
	}
	tmdbID, ok := parseID(c, "tmdb_id")
	if !ok {
		return "", 0, false
	}
	return kind, tmdbID, true
}

func (h *ProgressHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	f := progress.Filter{Status: domain.WatchStatus(c.Query("status"))}
	if k := c.Query("kind"); k != "" {
		kind, ok := progress.ParseKind(k)
		if !ok {
			RespondError(c, http.StatusBadRequest, "bad_request", "kind must be movies or shows")
			return
		}
		f.Kind = kind
	}
	views, err := h.svc.List(c.Request.Context(), userID, f)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"progress": views})
}

func (h *ProgressHandler) Get(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	kind, tmdbID, ok := progressTarget(c)
	if !ok {
		return
	}
	v, err := h.svc.Get(c.Request.Context(), userID, kind, tmdbID)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *ProgressHandler) Set(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	kind, tmdbID, ok := progressTarget(c)
	if !ok {
		return
	}
	var req struct {
		Status        domain.WatchStatus `json:"status"`
		LastSeasonNum uint32             `json:"last_season_num"`
		LastEpisode   uint32             `json:"last_episode_num"`
	}
	if !bindJSON(c, &req) {
		return
	}
	v, err := h.svc.Set(c.Request.Context(), userID, kind, tmdbID, progress.SetInput{
		Status: req.Status, Season: req.LastSeasonNum, Episode: req.LastEpisode,
	})
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *ProgressHandler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	kind, tmdbID, ok := progressTarget(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, kind, tmdbID); err != nil {
		RespondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
