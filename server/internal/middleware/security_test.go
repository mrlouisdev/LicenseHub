package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func metricsRouter(token string, allowWithoutToken bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/metrics", MetricsAuth(token, allowWithoutToken), func(c *gin.Context) {
		c.String(http.StatusOK, "metrics")
	})
	return r
}

func TestMetricsAuth(t *testing.T) {
	t.Run("production route is absent without token", func(t *testing.T) {
		w := httptest.NewRecorder()
		metricsRouter("", false).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("valid bearer token passes", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.Header.Set("Authorization", "Bearer "+stringsOf('a', 32))
		metricsRouter(stringsOf('a', 32), false).ServeHTTP(w, req)
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("status/cache = %d/%q", w.Code, w.Header().Get("Cache-Control"))
		}
	})

	t.Run("wrong bearer token fails", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.Header.Set("Authorization", "Bearer "+stringsOf('b', 32))
		metricsRouter(stringsOf('a', 32), false).ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
}

func stringsOf(value byte, count int) string {
	return string(makeBytes(value, count))
}

func makeBytes(value byte, count int) []byte {
	result := make([]byte, count)
	for i := range result {
		result[i] = value
	}
	return result
}
