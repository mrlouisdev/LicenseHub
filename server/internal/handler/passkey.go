package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/tabloy/keygate/internal/model"
	"github.com/tabloy/keygate/internal/store"
	"github.com/tabloy/keygate/pkg/response"
)

const (
	maxPasskeyRequestBytes = 96 * 1024
	passkeyCeremonyTTL     = 5 * time.Minute
	recoveryCodeCount      = 10
)

type passkeyUser struct {
	user        *model.User
	credentials []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte   { return []byte(u.user.ID) }
func (u *passkeyUser) WebAuthnName() string { return u.user.Email }
func (u *passkeyUser) WebAuthnDisplayName() string {
	if strings.TrimSpace(u.user.Name) != "" {
		return u.user.Name
	}
	return u.user.Email
}
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (h *AuthHandler) loadPasskeyUser(c *gin.Context, userID string) (*passkeyUser, error) {
	user, err := h.Store.FindUserByID(c, userID)
	if err != nil {
		return nil, err
	}
	records, err := h.Store.ListPasskeyCredentials(c, userID)
	if err != nil {
		return nil, err
	}
	credentials := make([]webauthn.Credential, 0, len(records))
	for _, record := range records {
		credentials = append(credentials, record.Credential)
	}
	return &passkeyUser{user: user, credentials: credentials}, nil
}

func currentAdminUserID(c *gin.Context) (string, bool) {
	uid := c.GetString("user_id")
	isAdmin, _ := c.Get("is_admin")
	if uid == "" || isAdmin != true {
		response.Forbidden(c, "admin required")
		return "", false
	}
	return uid, true
}

func ceremonyDigest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (h *AuthHandler) savePasskeyCeremony(c *gin.Context, userID, purpose string, session *webauthn.SessionData) (string, error) {
	rawID := randomHex(32)
	data, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	err = h.Store.SaveWebAuthnSession(c, &model.WebAuthnSession{
		CeremonyIDHash: ceremonyDigest(rawID),
		UserID:         userID,
		Purpose:        purpose,
		SessionData:    data,
		ExpiresAt:      time.Now().Add(passkeyCeremonyTTL),
	})
	return rawID, err
}

func decodeStrictJSON(c *gin.Context, out any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPasskeyRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func requestWithJSONBody(original *http.Request, body json.RawMessage) *http.Request {
	clone := original.Clone(original.Context())
	clone.Body = io.NopCloser(strings.NewReader(string(body)))
	clone.ContentLength = int64(len(body))
	clone.Header = original.Header.Clone()
	clone.Header.Set("Content-Type", "application/json")
	return clone
}

// PasskeyRegisterBegin starts authenticated admin passkey enrollment. An OTP
// session may bootstrap the first passkey; adding another requires an existing
// passkey step-up so an email-only compromise cannot silently add persistence.
func (h *AuthHandler) PasskeyRegisterBegin(c *gin.Context) {
	uid, ok := currentAdminUserID(c)
	if !ok {
		return
	}
	if h.WebAuthn == nil {
		response.Err(c, http.StatusServiceUnavailable, "PASSKEY_UNAVAILABLE", "passkey authentication is unavailable")
		return
	}
	count, err := h.Store.CountPasskeyCredentials(c, uid)
	if err != nil {
		response.Internal(c)
		return
	}
	if count > 0 && c.GetString("auth_method") != "webauthn" {
		response.Err(c, http.StatusForbidden, "PASSKEY_STEP_UP_REQUIRED", "verify an existing passkey before adding another")
		return
	}
	user, err := h.loadPasskeyUser(c, uid)
	if err != nil {
		response.Internal(c)
		return
	}
	creation, session, err := h.WebAuthn.BeginRegistration(
		user,
		webauthn.WithExclusions(webauthn.Credentials(user.credentials).CredentialDescriptors()),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
	)
	if err != nil {
		response.Internal(c)
		return
	}
	ceremonyID, err := h.savePasskeyCeremony(c, uid, "register", session)
	if err != nil {
		response.Internal(c)
		return
	}
	response.OK(c, gin.H{"ceremony_id": ceremonyID, "options": creation})
}

type passkeyFinishRequest struct {
	CeremonyID string          `json:"ceremony_id"`
	Name       string          `json:"name,omitempty"`
	Credential json.RawMessage `json:"credential"`
}

func validatePasskeyName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		name = "Passkey"
	}
	if len(name) > 80 {
		return "", errors.New("passkey name is too long")
	}
	return name, nil
}

func recoveryCodeHash(secret, userID, rawCode string) string {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(rawCode), "-", ""), " ", ""))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("licensehub:passkey-recovery:v1\x00"))
	_, _ = mac.Write([]byte(userID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil))
}

func generateRecoveryCodes(secret, userID string) ([]string, []string) {
	codes := make([]string, 0, recoveryCodeCount)
	hashes := make([]string, 0, recoveryCodeCount)
	for range recoveryCodeCount {
		raw := strings.ToUpper(randomHex(10))
		code := fmt.Sprintf("LHRC-%s-%s-%s-%s-%s", raw[0:4], raw[4:8], raw[8:12], raw[12:16], raw[16:20])
		codes = append(codes, code)
		hashes = append(hashes, recoveryCodeHash(secret, userID, code))
	}
	return codes, hashes
}

func (h *AuthHandler) PasskeyRegisterFinish(c *gin.Context) {
	uid, ok := currentAdminUserID(c)
	if !ok {
		return
	}
	var req passkeyFinishRequest
	if err := decodeStrictJSON(c, &req); err != nil || req.CeremonyID == "" || len(req.Credential) == 0 {
		response.BadRequest(c, "ceremony_id and credential are required")
		return
	}
	name, err := validatePasskeyName(req.Name)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	sessionJSON, err := h.Store.ConsumeWebAuthnSession(c, ceremonyDigest(req.CeremonyID), uid, "register")
	if err != nil {
		response.Err(c, http.StatusUnauthorized, "PASSKEY_CEREMONY_INVALID", "passkey ceremony is invalid, expired, or already used")
		return
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(sessionJSON, &session); err != nil {
		response.Internal(c)
		return
	}
	user, err := h.loadPasskeyUser(c, uid)
	if err != nil {
		response.Internal(c)
		return
	}
	credential, err := h.WebAuthn.FinishRegistration(user, session, requestWithJSONBody(c.Request, req.Credential))
	if err != nil {
		response.Err(c, http.StatusUnauthorized, "PASSKEY_VERIFICATION_FAILED", "passkey registration could not be verified")
		return
	}
	recoveryCodes, recoveryHashes := generateRecoveryCodes(h.Config.JWTSecret, uid)
	recoveryInstalled, err := h.Store.CreatePasskeyCredential(c, uid, name, c.ClientIP(), credential, recoveryHashes)
	if err != nil {
		response.Internal(c)
		return
	}
	if err := h.issueSession(c, user.user, "webauthn"); err != nil {
		response.Internal(c)
		return
	}
	data := gin.H{"status": "enrolled", "name": name, "recovery_codes_issued": recoveryInstalled}
	if recoveryInstalled {
		data["recovery_codes"] = recoveryCodes
		data["recovery_notice"] = "store these one-time codes offline; they are never shown again"
	}
	response.OK(c, data)
}

func (h *AuthHandler) PasskeyAssertionBegin(c *gin.Context) {
	uid, ok := currentAdminUserID(c)
	if !ok {
		return
	}
	if h.WebAuthn == nil {
		response.Err(c, http.StatusServiceUnavailable, "PASSKEY_UNAVAILABLE", "passkey authentication is unavailable")
		return
	}
	user, err := h.loadPasskeyUser(c, uid)
	if err != nil {
		response.Internal(c)
		return
	}
	if len(user.credentials) == 0 {
		response.Conflict(c, "PASSKEY_NOT_ENROLLED", "enroll a passkey first", nil)
		return
	}
	assertion, session, err := h.WebAuthn.BeginLogin(user)
	if err != nil {
		response.Internal(c)
		return
	}
	ceremonyID, err := h.savePasskeyCeremony(c, uid, "login", session)
	if err != nil {
		response.Internal(c)
		return
	}
	response.OK(c, gin.H{"ceremony_id": ceremonyID, "options": assertion})
}

func (h *AuthHandler) PasskeyAssertionFinish(c *gin.Context) {
	uid, ok := currentAdminUserID(c)
	if !ok {
		return
	}
	var req passkeyFinishRequest
	if err := decodeStrictJSON(c, &req); err != nil || req.CeremonyID == "" || len(req.Credential) == 0 {
		response.BadRequest(c, "ceremony_id and credential are required")
		return
	}
	sessionJSON, err := h.Store.ConsumeWebAuthnSession(c, ceremonyDigest(req.CeremonyID), uid, "login")
	if err != nil {
		response.Err(c, http.StatusUnauthorized, "PASSKEY_CEREMONY_INVALID", "passkey ceremony is invalid, expired, or already used")
		return
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(sessionJSON, &session); err != nil {
		response.Internal(c)
		return
	}
	user, err := h.loadPasskeyUser(c, uid)
	if err != nil {
		response.Internal(c)
		return
	}
	credential, err := h.WebAuthn.FinishLogin(user, session, requestWithJSONBody(c.Request, req.Credential))
	if err != nil {
		response.Err(c, http.StatusUnauthorized, "PASSKEY_VERIFICATION_FAILED", "passkey assertion could not be verified")
		return
	}
	if err := h.Store.UpdatePasskeyCredentialWithAudit(c, uid, c.ClientIP(), credential); err != nil {
		response.Internal(c)
		return
	}
	if err := h.issueSession(c, user.user, "webauthn"); err != nil {
		response.Internal(c)
		return
	}
	h.Store.Audit(c, &model.AuditLog{
		Entity: "session", EntityID: uid, Action: "passkey_step_up",
		ActorType: "webauthn", ActorID: uid, IPAddress: c.ClientIP(),
	})
	response.OK(c, gin.H{"status": "verified"})
}

func (h *AuthHandler) PasskeyList(c *gin.Context) {
	uid, ok := currentAdminUserID(c)
	if !ok {
		return
	}
	records, err := h.Store.ListPasskeyCredentials(c, uid)
	if err != nil {
		response.Internal(c)
		return
	}
	items := make([]gin.H, 0, len(records))
	for _, item := range records {
		items = append(items, gin.H{
			"id": item.Record.ID, "name": item.Record.Name,
			"created_at": item.Record.CreatedAt, "last_used_at": item.Record.LastUsedAt,
		})
	}
	response.OK(c, gin.H{"passkeys": items, "step_up_verified": c.GetString("auth_method") == "webauthn"})
}

func (h *AuthHandler) PasskeyDelete(c *gin.Context) {
	uid, ok := currentAdminUserID(c)
	if !ok {
		return
	}
	if c.GetString("auth_method") != "webauthn" {
		response.Err(c, http.StatusForbidden, "PASSKEY_STEP_UP_REQUIRED", "verify a passkey before removing one")
		return
	}
	if err := h.Store.DeletePasskeyCredentialWithAudit(c, uid, c.Param("id"), c.ClientIP()); err != nil {
		switch {
		case errors.Is(err, store.ErrLastPasskey):
			response.Conflict(c, "LAST_PASSKEY", "the last passkey can only be reset with a recovery code", nil)
		case errors.Is(err, sql.ErrNoRows):
			response.NotFound(c, "passkey not found")
		default:
			response.Internal(c)
		}
		return
	}
	response.OK(c, gin.H{"status": "deleted"})
}

func (h *AuthHandler) PasskeyRecover(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := decodeStrictJSON(c, &req); err != nil || strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Code) == "" {
		response.BadRequest(c, "email and recovery code are required")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	user, err := h.Store.FindUserByEmail(c, email)
	if err != nil || user == nil || !user.IsAdmin() {
		_ = recoveryCodeHash(h.Config.JWTSecret, "unknown", req.Code)
		response.Unauthorized(c, "invalid recovery code")
		return
	}
	hash := recoveryCodeHash(h.Config.JWTSecret, user.ID, req.Code)
	if err := h.Store.RecoverPasskeys(c, user.ID, hash, c.ClientIP()); err != nil {
		response.Unauthorized(c, "invalid recovery code")
		return
	}
	user, err = h.Store.FindUserByID(c, user.ID)
	if err != nil {
		response.Internal(c)
		return
	}
	if err := h.issueSession(c, user, "recovery"); err != nil {
		response.Internal(c)
		return
	}
	response.OK(c, gin.H{"status": "recovered", "next": "enroll a new passkey immediately"})
}
