package middleware

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/auth"
	"github.com/harshit/food-ordering-app/internal/handlers"
	"github.com/harshit/food-ordering-app/internal/models"
)

// StaffAuth validates a staff JWT (audience "staff") and loads
//   c.Get("staff_id") (int64), c.Get("org_id") (int64, may be 0 for super_admin),
//   c.Get("role")     (string).
func StaffAuth(db *gorm.DB, jwtCfg auth.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			handlers.RespondError(c, http.StatusUnauthorized, "unauthorized", "missing bearer token")
			return
		}
		claims, err := auth.Parse(jwtCfg, token)
		if err != nil {
			handlers.RespondError(c, http.StatusUnauthorized, "invalid_token", err.Error())
			return
		}
		if claims.Aud != auth.AudStaff {
			handlers.RespondError(c, http.StatusUnauthorized, "wrong_audience", "token is not a staff token")
			return
		}

		var session models.AuthSession
		err = db.Where("jwt_id = ?", claims.ID).First(&session).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				handlers.RespondError(c, http.StatusUnauthorized, "session_not_found", "no active session")
				return
			}
			handlers.RespondError(c, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		if session.RevokedAt != nil {
			handlers.RespondError(c, http.StatusUnauthorized, "session_revoked", "session has been revoked")
			return
		}
		if session.SessionExpiresAt == nil || session.SessionExpiresAt.Before(time.Now()) {
			handlers.RespondError(c, http.StatusUnauthorized, "session_expired", "session has expired")
			return
		}
		if session.StaffID == nil {
			handlers.RespondError(c, http.StatusUnauthorized, "session_invalid", "session has no associated staff user")
			return
		}

		c.Set("staff_id", *session.StaffID)
		c.Set("org_id", claims.OrgID)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// RequireAdmin — gate endpoints that require manager / super_admin (full branch privileges).
// staff role is rejected with 403.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != models.RoleSuperAdmin && role != models.RoleManager {
			handlers.RespondError(c, http.StatusForbidden, "admin_only", "this action requires admin or manager role")
			return
		}
		c.Next()
	}
}

// RequireSuperAdmin — gate endpoints reserved for the platform-level super_admin.
// manager and staff are rejected with 403.
func RequireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != models.RoleSuperAdmin {
			handlers.RespondError(c, http.StatusForbidden, "super_admin_only", "this action requires the platform super admin role")
			return
		}
		c.Next()
	}
}
