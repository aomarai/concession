package handlers

import (
	"net/http"
	"strconv"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/reviews"
	"github.com/gin-gonic/gin"
)

type ReviewHandler struct {
	svc *reviews.Service
}

func NewReviewHandler(svc *reviews.Service) *ReviewHandler {
	return &ReviewHandler{svc: svc}
}

// RegisterRoutes mounts the review endpoints on r (authenticated group).
func (h *ReviewHandler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/movies/:tmdb_id/reviews", h.listForTitle(domain.ReviewableMovies))
	r.POST("/movies/:tmdb_id/reviews", h.create(domain.ReviewableMovies))
	r.GET("/shows/:tmdb_id/reviews", h.listForTitle(domain.ReviewableShows))
	r.POST("/shows/:tmdb_id/reviews", h.create(domain.ReviewableShows))
	r.GET("/me/reviews", h.listMine)
	r.GET("/reviews/:id", h.get)
	r.PATCH("/reviews/:id", h.update)
	r.DELETE("/reviews/:id", h.delete)
}

// pageParams reads the optional page and per_page query parameters.
func pageParams(c *gin.Context) (page, perPage int, ok bool) {
	for _, p := range []struct {
		name string
		dst  *int
	}{{"page", &page}, {"per_page", &perPage}} {
		raw := c.Query(p.name)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "bad_request", "Invalid "+p.name)
			return 0, 0, false
		}
		*p.dst = n
	}
	return page, perPage, true
}

func (h *ReviewHandler) create(kind domain.ReviewableItem) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := currentUserID(c)
		if !ok {
			return
		}
		tmdbID, ok := parseID(c, "tmdb_id")
		if !ok {
			return
		}
		var req struct {
			Rating  int    `json:"rating"`
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if !bindJSON(c, &req) {
			return
		}
		v, err := h.svc.Create(c.Request.Context(), userID, kind, tmdbID, reviews.Input{
			Rating: req.Rating, Title: req.Title, Content: req.Content,
		})
		if err != nil {
			RespondServiceError(c, err)
			return
		}
		c.JSON(http.StatusCreated, v)
	}
}

func (h *ReviewHandler) listForTitle(kind domain.ReviewableItem) gin.HandlerFunc {
	return func(c *gin.Context) {
		tmdbID, ok := parseID(c, "tmdb_id")
		if !ok {
			return
		}
		page, perPage, ok := pageParams(c)
		if !ok {
			return
		}
		p, err := h.svc.ListForTitle(c.Request.Context(), kind, tmdbID, page, perPage)
		if err != nil {
			RespondServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, p)
	}
}

func (h *ReviewHandler) listMine(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	page, perPage, ok := pageParams(c)
	if !ok {
		return
	}
	p, err := h.svc.ListMine(c.Request.Context(), userID, page, perPage)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *ReviewHandler) get(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	v, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *ReviewHandler) update(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req struct {
		Rating  *int    `json:"rating"`
		Title   *string `json:"title"`
		Content *string `json:"content"`
	}
	if !bindJSON(c, &req) {
		return
	}
	v, err := h.svc.Update(c.Request.Context(), userID, id, reviews.UpdateInput{
		Rating: req.Rating, Title: req.Title, Content: req.Content,
	})
	if err != nil {
		RespondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *ReviewHandler) delete(c *gin.Context) {
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
