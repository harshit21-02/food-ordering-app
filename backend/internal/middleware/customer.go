package middleware

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/harshit/food-ordering-app/internal/auth"
	"github.com/harshit/food-ordering-app/internal/handlers"
	"github.com/harshit/food-ordering-app/internal/models"
)

// CustomerAuth is a Gin middleware that:
//   1. Extracts a Bearer token from the Authorization header.
//   2. Parses and validates the JWT (must have aud_kind="customer").
//   3. Looks up the matching auth_sessions row by jwt_id and ensures it's
//      not revoked and not expired.
//   4. Stores customer_id (int64) on the gin context under key "customer_id".
//
// On any failure it aborts with a 401 and the standard error envelope.
func CustomerAuth(db *gorm.DB, jwtCfg auth.JWTConfig) gin.HandlerFunc {
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
		if claims.Aud != auth.AudCustomer {
			handlers.RespondError(c, http.StatusUnauthorized, "wrong_audience", "token is not a customer token")
			return
		}

		var session models.AuthSession
		err = db.Where("jwt_id = ?", claims.ID).First(&session).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				handlers.RespondError(c, http.StatusUnauthorized, "session_not_found", "no active session for this token")
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
		if session.CustomerID == nil {
			handlers.RespondError(c, http.StatusUnauthorized, "session_invalid", "session has no associated customer")
			return
		}

		c.Set("customer_id", *session.CustomerID)
		c.Next()
	}
}

func bearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
