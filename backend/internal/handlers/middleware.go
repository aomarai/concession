package handlers

import (
	"context"
	"net/http"

	"github.com/aomarai/concession/internal/auth"
	"github.com/aomarai/concession/internal/logging"
)

type ctxKey string

const userIDKey ctxKey = "user_id"

func (h *AuthHandler) AuthenticateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.FromContext(r.Context())

		cookie, err := r.Cookie("session_token")
		if err != nil {
			logger.Warn("missing session cookie")
			clearSessionCookie(w) // Clear existing/stale session cookie to avoid repeated 401s
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		session, err := auth.ValidateSession(r.Context(), h.DB, cookie.Value)
		if err != nil {
			logger.Warn("invalid session", "error", err)
			clearSessionCookie(w)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		logger = logger.With("user_id", session.UserID)

		ctx := context.WithValue(r.Context(), userIDKey, session.UserID)
		ctx = logging.WithLogger(ctx, logger)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
