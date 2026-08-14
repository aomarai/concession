package logging

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// RequestLoggerMiddleware seeds each request's context with a logger
// enriched with a request ID, method, and path, so every handler and
// downstream function can log with consistent correlation fields.
func RequestLoggerMiddleware(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := uuid.NewString()

			// Let clients/tools correlate this response back to server logs.
			w.Header().Set("X-Request-ID", reqID)

			logger := base.With(
				"request_id", reqID,
				"method", r.Method,
				"path", r.URL.Path,
			)

			logger.Info("request started")

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			ctx := WithLogger(r.Context(), logger)
			next.ServeHTTP(rec, r.WithContext(ctx))

			logger.Info("request completed",
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}
