package handlers

import (
	"errors"
	"net/http"

	"github.com/aomarai/concession/internal/catalog"
	"github.com/aomarai/concession/internal/logging"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/tmdb"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ErrorResponse is the JSON shape returned for every API error.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody carries a stable machine-readable code and a human message.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RespondError aborts the request with a consistently shaped JSON error.
func RespondError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

// RespondServiceError maps an error returned by a service to an HTTP response.
func RespondServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, svcerr.ErrInvalid):
		RespondError(c, http.StatusBadRequest, "bad_request", svcerr.Message(err))
	case errors.Is(err, svcerr.ErrNotFound):
		RespondError(c, http.StatusNotFound, "not_found", svcerr.MessageOr(err, "Not found"))
	case errors.Is(err, tmdb.ErrNotFound):
		RespondError(c, http.StatusNotFound, "not_found", "Title not found")
	case errors.Is(err, svcerr.ErrForbidden):
		RespondError(c, http.StatusForbidden, "forbidden", "You do not have permission to do that")
	case errors.Is(err, svcerr.ErrDuplicate):
		RespondError(c, http.StatusConflict, "conflict", svcerr.MessageOr(err, "Already exists"))
	case errors.Is(err, catalog.ErrUpstream):
		logging.FromContext(c.Request.Context()).Error("TMDB request failed", "error", err)
		RespondError(c, http.StatusBadGateway, "upstream_error", "Could not load title data")
	default:
		logging.FromContext(c.Request.Context()).Error("request failed", "error", err)
		RespondError(c, http.StatusInternalServerError, "internal_error", "Internal error")
	}
}

// currentUserID returns the authenticated user's ID, responding 500 if the
// route was registered without the auth middleware.
func currentUserID(c *gin.Context) (uuid.UUID, bool) {
	id, ok := c.Request.Context().Value(userIDKey).(uuid.UUID)
	if !ok {
		logging.FromContext(c.Request.Context()).Error("user_id missing from context on authenticated route")
		RespondError(c, http.StatusInternalServerError, "internal_error", "Internal error")
	}
	return id, ok
}

// maxBodyBytes caps JSON request bodies.
const maxBodyBytes = 1 << 20

// bindJSON decodes the request body into v, responding 400 on failure.
func bindJSON(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	if err := c.ShouldBindJSON(v); err != nil {
		RespondError(c, http.StatusBadRequest, "bad_request", "Invalid request body")
		return false
	}
	return true
}

// parseUUIDParam reads a UUID path parameter, responding 400 if malformed.
func parseUUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "bad_request", "Invalid "+name)
		return uuid.Nil, false
	}
	return id, true
}
