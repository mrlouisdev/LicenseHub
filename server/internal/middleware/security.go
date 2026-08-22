package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// NoStore prevents proxies and local browser caches from retaining license
// keys, signed leases, activation state, or authenticated API responses.
func NoStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		c.Next()
	}
}

// MetricsAuth disables metrics when no production token is configured and
// otherwise accepts only a constant-time Bearer-token comparison.
func MetricsAuth(token string, allowWithoutToken bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if token == "" {
			if allowWithoutToken {
				c.Next()
				return
			}
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		authorization := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(authorization, prefix) {
			c.Header("WWW-Authenticate", "Bearer")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		candidate := strings.TrimPrefix(authorization, prefix)
		if len(candidate) != len(token) || subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) != 1 {
			c.Header("WWW-Authenticate", "Bearer")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}
