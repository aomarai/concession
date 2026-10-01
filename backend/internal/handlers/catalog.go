package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/aomarai/concession/internal/catalog"
	"github.com/aomarai/concession/internal/logging"
	"github.com/aomarai/concession/internal/tmdb"
	"github.com/gin-gonic/gin"
)

type CatalogHandler struct {
	svc *catalog.Service
}

func NewCatalogHandler(svc *catalog.Service) *CatalogHandler {
	return &CatalogHandler{svc: svc}
}

// RegisterRoutes mounts the catalog endpoints on r.
func (h *CatalogHandler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/search", h.Search)
	r.GET("/movies/:tmdb_id", h.GetMovie)
	r.GET("/shows/:tmdb_id", h.GetShow)
	r.GET("/shows/:tmdb_id/seasons/:season", h.GetSeason)
}

func (h *CatalogHandler) fail(c *gin.Context, err error) {
	if errors.Is(err, tmdb.ErrNotFound) {
		RespondError(c, http.StatusNotFound, "not_found", "Title not found")
		return
	}
	logging.FromContext(c.Request.Context()).Error("catalog request failed", "error", err)
	RespondError(c, http.StatusBadGateway, "upstream_error", "Could not load title data")
}

func parseID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		RespondError(c, http.StatusBadRequest, "bad_request", "Invalid "+name)
		return 0, false
	}
	return id, true
}

func (h *CatalogHandler) Search(c *gin.Context) {
	page := 1
	if p := c.Query("page"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			RespondError(c, http.StatusBadRequest, "bad_request", "Invalid page")
			return
		}
		page = n
	}
	res, err := h.svc.Search(c.Request.Context(), c.Query("q"), page)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *CatalogHandler) GetMovie(c *gin.Context) {
	id, ok := parseID(c, "tmdb_id")
	if !ok {
		return
	}
	movie, err := h.svc.EnsureMovie(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, movie)
}

func (h *CatalogHandler) GetShow(c *gin.Context) {
	id, ok := parseID(c, "tmdb_id")
	if !ok {
		return
	}
	show, err := h.svc.EnsureShow(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, show)
}

func (h *CatalogHandler) GetSeason(c *gin.Context) {
	id, ok := parseID(c, "tmdb_id")
	if !ok {
		return
	}
	n, err := strconv.Atoi(c.Param("season"))
	if err != nil || n < 0 {
		RespondError(c, http.StatusBadRequest, "bad_request", "Invalid season")
		return
	}
	season, err := h.svc.EnsureSeason(c.Request.Context(), id, n)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, season)
}
