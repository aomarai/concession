package handlers

import (
	"math"
	"net"
	"net/http"
	"strconv"

	"github.com/aomarai/concession/internal/ratelimit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// KeyFunc picks what a request is counted against.
type KeyFunc func(c *gin.Context) string

// ClientIPKey counts per client IP. Behind a reverse proxy this is only the
// real client when the proxy is trusted (see TRUSTED_PROXIES). IPv6 clients
// are counted per /64, since one host or household owns the whole prefix and
// could otherwise rotate through addresses for free budgets.
func ClientIPKey(c *gin.Context) string {
	ip := c.ClientIP()
	if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() == nil {
		return parsed.Mask(net.CIDRMask(64, 128)).String()
	}
	return ip
}

// UserKey counts per authenticated user, falling back to the client IP.
func UserKey(c *gin.Context) string {
	if id, ok := c.Request.Context().Value(userIDKey).(uuid.UUID); ok {
		return id.String()
	}
	return c.ClientIP()
}

// RateLimit answers 429 with a Retry-After header once a key has used up its
// budget. A nil limiter (limit switched off) lets everything through.
func RateLimit(l *ratelimit.Limiter, key KeyFunc) gin.HandlerFunc {
	if l == nil {
		return func(*gin.Context) {}
	}
	return func(c *gin.Context) {
		ok, retry := l.Allow(key(c))
		if ok {
			return // no c.Next() needed: gin runs the rest of the chain unless the request is aborted
		}
		// A denied request always has a positive wait, so this is at least 1.
		secs := max(1, int(math.Ceil(retry.Seconds())))
		c.Header("Retry-After", strconv.Itoa(secs))
		RespondError(c, http.StatusTooManyRequests, "rate_limited", "Too many requests. Try again later.")
	}
}

// withMiddleware returns the handler chain for a route: the optional
// middleware (nil means none) followed by the handler.
func withMiddleware(mw gin.HandlerFunc, h gin.HandlerFunc) []gin.HandlerFunc {
	if mw == nil {
		return []gin.HandlerFunc{h}
	}
	return []gin.HandlerFunc{mw, h}
}
