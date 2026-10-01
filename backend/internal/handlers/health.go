package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// HealthHandler reports liveness and database connectivity.
func HealthHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err == nil {
			err = sqlDB.PingContext(c.Request.Context())
		}
		if err != nil {
			RespondError(c, http.StatusServiceUnavailable, "unhealthy", "database unreachable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
