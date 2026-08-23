package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestSessionVersionRevokesAccessTokenImmediately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "test-jwt-secret-that-is-at-least-thirty-two-bytes"
	var currentVersion atomic.Int64
	currentVersion.Store(7)
	checker := func(_ context.Context, userID string, tokenVersion int64) (bool, bool) {
		return true, userID == "user-1" && tokenVersion == currentVersion.Load()
	}

	token, err := IssueJWTWithSession(secret, "user-1", "user@example.test", "User", true, time.Hour, 7, "otp")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	router := gin.New()
	router.GET("/protected", SessionAuthWithState(secret, checker), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if got := request().Code; got != http.StatusNoContent {
		t.Fatalf("fresh token status = %d", got)
	}
	currentVersion.Add(1) // same state transition performed by logout
	replayed := request()
	if replayed.Code != http.StatusUnauthorized {
		t.Fatalf("replayed token status = %d, want 401", replayed.Code)
	}
	if !strings.Contains(replayed.Body.String(), "SESSION_REVOKED") {
		t.Fatalf("unexpected replay response: %s", replayed.Body.String())
	}
}

func TestSessionAuthRejectsUnexpectedJWTAlgorithm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "test-jwt-secret-that-is-at-least-thirty-two-bytes"
	claims := Claims{
		UserID:         "user-1",
		SessionVersion: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "jti-fixture",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign fixture: %v", err)
	}
	router := gin.New()
	router.GET("/protected", SessionAuthWithState(secret, func(context.Context, string, int64) (bool, bool) {
		return true, true
	}), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected-algorithm token status = %d, want 401", w.Code)
	}
}
