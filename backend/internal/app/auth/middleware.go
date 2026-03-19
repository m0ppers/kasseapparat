package auth

import (
	"log/slog"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	sqliteRepo "github.com/potibm/kasseapparat/internal/app/repository/sqlite"
)

func UserMiddleware(repo sqliteRepo.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		slog.Debug("Running auth middleware", "session", session.Get("user_id"))
		// user_id must be set by the auth implementation after successful authentication
		if session.Get("user_id") != nil {
			user, err := repo.GetUserByID(session.Get("user_id").(int))
			if err != nil {
				slog.Error("Error getting user from auth middleware", "error", err)
				// 500 error
				c.AbortWithStatusJSON(500, gin.H{"error": "Internal Server Error"})
				return
			}
			c.Set("user", user)
		}
		c.Next()
	}
}

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists || user == nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "Unauthorized"})
			return
		}
		c.Next()
	}
}
