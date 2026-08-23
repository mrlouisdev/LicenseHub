package model

import (
	"time"

	"github.com/uptrace/bun"
)

// PasskeyCredential stores the credential identifier in searchable form and
// the complete WebAuthn credential record as an encrypted envelope.
type PasskeyCredential struct {
	bun.BaseModel `bun:"table:passkey_credentials"`

	ID             string     `bun:",pk" json:"id"`
	UserID         string     `bun:",notnull" json:"-"`
	CredentialID   []byte     `bun:",notnull,unique" json:"-"`
	CredentialData []byte     `bun:",notnull" json:"-"`
	Name           string     `bun:",notnull,default:''" json:"name"`
	CreatedAt      time.Time  `bun:",nullzero,default:now()" json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
}

// WebAuthnSession is a short-lived, single-use server-side ceremony record.
// Only a digest of the opaque ceremony handle is persisted.
type WebAuthnSession struct {
	bun.BaseModel `bun:"table:webauthn_sessions"`

	CeremonyIDHash string    `bun:",pk"`
	UserID         string    `bun:",notnull"`
	Purpose        string    `bun:",notnull"`
	SessionData    []byte    `bun:",notnull,type:jsonb"`
	ExpiresAt      time.Time `bun:",notnull"`
	CreatedAt      time.Time `bun:",nullzero,default:now()"`
}

// PasskeyRecoveryCode is a one-time HMAC digest. Plain recovery codes are
// returned exactly once at enrollment and never written to storage or logs.
type PasskeyRecoveryCode struct {
	bun.BaseModel `bun:"table:passkey_recovery_codes"`

	ID        string     `bun:",pk"`
	UserID    string     `bun:",notnull"`
	CodeHash  string     `bun:",notnull,unique"`
	CreatedAt time.Time  `bun:",nullzero,default:now()"`
	UsedAt    *time.Time `json:"-"`
}
