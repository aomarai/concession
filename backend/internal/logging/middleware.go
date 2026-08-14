package logging

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GinRequestLoggerMiddleware seeds each request's context with a logger
// enriched with a request ID, method, and path, so every handler and
// downstream function can log with consistent correlation fields.
func GinRequestLoggerMiddleware(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		reqID := uuid.NewString()

		// Let clients/tools correlate this response back to server logs.
		c.Header("X-Request-ID", reqID)

		logger := base.With(
			"request_id", reqID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
		)

		// Inject the enriched logger into the request context so downstream
		// handlers and services can retrieve it via logging.FromContext.
		ctx := WithLogger(c.Request.Context(), logger)
		c.Request = c.Request.WithContext(ctx)

		logger.Info("request started")

		c.Next()

		logger.Info("request completed",
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}
