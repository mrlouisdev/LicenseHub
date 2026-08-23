package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tabloy/keygate/internal/middleware"
	"github.com/tabloy/keygate/internal/model"
	"github.com/tabloy/keygate/pkg/response"
)

// OTPSend handles POST /api/v1/auth/otp/send
func (h *AuthHandler) OTPSend(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "valid email is required")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	// Never mint a code that cannot be delivered in production. In
	// particular, do not log or return codes as a fallback.
	if h.Email == nil || !h.Email.IsConfigured() {
		response.Err(c, http.StatusServiceUnavailable, "SMTP_UNAVAILABLE", "email authentication is temporarily unavailable")
		return
	}

	// Rate limit: max 3 OTP requests per email per 10 minutes
	count, err := h.Store.CountRecentOTPCodes(c, email)
	if err != nil {
		response.Internal(c)
		return
	}
	if count >= 3 {
		response.Err(c, 429, "RATE_LIMITED", "too many code requests, try again later")
		return
	}

	code := generateOTPCode()
	codeHash := hashOTPCode(h.Config.JWTSecret, code)

	otp := &model.OTPCode{
		Email:     email,
		CodeHash:  codeHash,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if err := h.Store.CreateOTPCode(c, otp); err != nil {
		response.Internal(c)
		return
	}

	if err := h.Email.SendOTPCode(email, code); err != nil {
		_ = h.Store.MarkOTPUsed(c, otp.ID)
		response.Err(c, http.StatusServiceUnavailable, "SMTP_UNAVAILABLE", "email authentication is temporarily unavailable")
		return
	}

	response.OK(c, gin.H{"status": "sent"})
}

// OTPVerify handles POST /api/v1/auth/otp/verify
func (h *AuthHandler) OTPVerify(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Code  string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "email and code are required")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	code := strings.TrimSpace(req.Code)

	// Hash unconditionally so unknown-email and known-email requests perform
	// the same application-side work. The store locks and consumes atomically.
	otp, matched, err := h.Store.ConsumeOTPCode(c, email, hashOTPCode(h.Config.JWTSecret, code))
	if err != nil {
		response.Internal(c)
		return
	}
	if !matched || otp == nil {
		middleware.RecordBruteForceFailure(c, "ip:"+c.ClientIP())
		remaining := 0
		if otp != nil {
			remaining = 5 - otp.Attempts
		}
		if remaining <= 0 {
			response.Unauthorized(c, "too many attempts, request a new code")
		} else {
			response.Unauthorized(c, "invalid or expired code")
		}
		return
	}

	middleware.RecordBruteForceSuccess(c, "ip:"+c.ClientIP())

	// Upsert user (create on first login)
	user := &model.User{Email: email}
	if err := h.Store.UpsertUser(c, user); err != nil {
		response.Internal(c)
		return
	}
	user, err = h.Store.FindUserByEmail(c, email)
	if err != nil {
		response.Internal(c)
		return
	}

	// Auto-promote if email is in ADMIN_EMAILS
	if h.Config.IsAdminEmail(user.Email) && user.Role == model.RoleUser {
		_ = h.Store.SetUserRole(c, user.ID, model.RoleAdmin)
		user.Role = model.RoleAdmin
	}

	// Welcome email for new users
	if h.Email != nil && time.Since(user.CreatedAt) < time.Minute {
		h.Email.SendWelcome(user.Email, user.Name)
	}

	if err := h.issueSession(c, user, "otp"); err != nil {
		response.Internal(c)
		return
	}

	h.Store.Audit(c, &model.AuditLog{
		Entity: "session", EntityID: user.ID, Action: "login",
		ActorType: "otp", ActorID: user.ID, IPAddress: c.ClientIP(),
		Changes: map[string]any{"email": user.Email},
	})

	response.OK(c, gin.H{
		"status": "ok", "email": user.Email, "name": user.Name,
		"is_admin": user.IsAdmin(), "role": user.Role,
	})
}

func generateOTPCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	return fmt.Sprintf("%06d", n.Int64())
}

func hashOTPCode(secret, code string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("licensehub:otp:v1\x00"))
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}
