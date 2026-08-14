package handlers

import (
	"context"

	"github.com/aomarai/concession/internal/auth"
	"github.com/aomarai/concession/internal/logging"
	"github.com/gin-gonic/gin"
)

type ctxKey string

const userIDKey ctxKey = "user_id"

// AuthMiddleware is a Gin middleware that validates the session cookie and
// injects the authenticated user ID into the request context.
func (h *AuthHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		logger := logging.FromContext(c.Request.Context())

		cookie, err := c.Cookie("session_token")
		if err != nil {
			logger.Warn("missing session cookie")
			clearSessionCookie(c, h.cfg) // Clear existing/stale session cookie to avoid repeated 401s
			c.AbortWithStatusJSON(401, gin.H{"error": "Unauthorized"})
			return
		}

		session, err := auth.ValidateSession(c.Request.Context(), h.DB, cookie)
		if err != nil {
			logger.Warn("invalid session", "error", err)
			clearSessionCookie(c, h.cfg)
			c.AbortWithStatusJSON(401, gin.H{"error": "Unauthorized"})
			return
		}

		logger = logger.With("user_id", session.UserID)

		ctx := context.WithValue(c.Request.Context(), userIDKey, session.UserID)
		ctx = logging.WithLogger(ctx, logger)

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
