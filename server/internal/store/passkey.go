package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/uptrace/bun"

	"github.com/tabloy/keygate/internal/model"
)

var ErrLastPasskey = errors.New("cannot remove the last passkey")

type PasskeyWithCredential struct {
	Record     *model.PasskeyCredential
	Credential webauthn.Credential
}

func (s *Store) encodePasskeyCredential(id string, credential *webauthn.Credential) ([]byte, error) {
	raw, err := json.Marshal(credential)
	if err != nil {
		return nil, fmt.Errorf("marshal WebAuthn credential: %w", err)
	}
	if s.PasskeyCredentialAEAD == nil {
		return raw, nil
	}
	return s.PasskeyCredentialAEAD.Encrypt(raw, []byte("passkey:"+id))
}

func (s *Store) decodePasskeyCredential(record *model.PasskeyCredential) (webauthn.Credential, error) {
	raw := record.CredentialData
	if s.PasskeyCredentialAEAD != nil {
		plaintext, err := s.PasskeyCredentialAEAD.Decrypt(raw, []byte("passkey:"+record.ID))
		if err != nil {
			return webauthn.Credential{}, err
		}
		raw = plaintext
		defer clear(plaintext)
	}
	var credential webauthn.Credential
	if err := json.Unmarshal(raw, &credential); err != nil {
		return webauthn.Credential{}, fmt.Errorf("decode WebAuthn credential: %w", err)
	}
	return credential, nil
}

func (s *Store) SaveWebAuthnSession(ctx context.Context, row *model.WebAuthnSession) error {
	if row == nil || row.CeremonyIDHash == "" || row.UserID == "" {
		return errors.New("invalid WebAuthn session")
	}
	_, err := s.DB.NewInsert().Model(row).Exec(ctx)
	return err
}

// ConsumeWebAuthnSession deletes and returns a live ceremony in one statement.
// A replay therefore fails even if two finish requests race.
func (s *Store) ConsumeWebAuthnSession(ctx context.Context, digest, userID, purpose string) ([]byte, error) {
	var sessionData []byte
	err := s.DB.NewRaw(`
		DELETE FROM webauthn_sessions
		WHERE ceremony_id_hash = ? AND user_id = ? AND purpose = ? AND expires_at > now()
		RETURNING session_data`, digest, userID, purpose).Scan(ctx, &sessionData)
	if err != nil {
		return nil, err
	}
	return sessionData, nil
}

func (s *Store) CleanExpiredWebAuthnSessions(ctx context.Context) {
	_, _ = s.DB.NewRaw("DELETE FROM webauthn_sessions WHERE expires_at <= now()").Exec(ctx)
}

func (s *Store) ListPasskeyCredentials(ctx context.Context, userID string) ([]PasskeyWithCredential, error) {
	var rows []*model.PasskeyCredential
	if err := s.DB.NewSelect().Model(&rows).
		Where("user_id = ?", userID).
		OrderExpr("created_at ASC").
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]PasskeyWithCredential, 0, len(rows))
	for _, row := range rows {
		credential, err := s.decodePasskeyCredential(row)
		if err != nil {
			return nil, fmt.Errorf("passkey %s: %w", row.ID, err)
		}
		out = append(out, PasskeyWithCredential{Record: row, Credential: credential})
	}
	return out, nil
}

func (s *Store) CountPasskeyCredentials(ctx context.Context, userID string) (int, error) {
	return s.DB.NewSelect().Model((*model.PasskeyCredential)(nil)).Where("user_id = ?", userID).Count(ctx)
}

// CreatePasskeyCredential serializes enrollment per user and writes the
// credential, optional first-enrollment recovery codes, and audit row in one
// transaction. Recovery hashes are installed only when this is the user's
// first passkey.
func (s *Store) CreatePasskeyCredential(ctx context.Context, userID, name, ip string, credential *webauthn.Credential, recoveryHashes []string) (recoveryInstalled bool, err error) {
	if credential == nil || len(credential.ID) == 0 {
		return false, errors.New("invalid WebAuthn credential")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck

	var lockedUserID string
	if err := tx.NewRaw("SELECT id FROM users WHERE id = ? FOR UPDATE", userID).Scan(ctx, &lockedUserID); err != nil {
		return false, err
	}
	count, err := tx.NewSelect().Model((*model.PasskeyCredential)(nil)).Where("user_id = ?", userID).Count(ctx)
	if err != nil {
		return false, err
	}
	row := &model.PasskeyCredential{
		ID:           newID(),
		UserID:       userID,
		CredentialID: append([]byte(nil), credential.ID...),
		Name:         name,
	}
	row.CredentialData, err = s.encodePasskeyCredential(row.ID, credential)
	if err != nil {
		return false, err
	}
	if _, err := tx.NewInsert().Model(row).Exec(ctx); err != nil {
		return false, err
	}

	if count == 0 {
		if len(recoveryHashes) < 8 {
			return false, errors.New("first passkey enrollment requires recovery codes")
		}
		if _, err := tx.NewDelete().Model((*model.PasskeyRecoveryCode)(nil)).Where("user_id = ?", userID).Exec(ctx); err != nil {
			return false, err
		}
		for _, hash := range recoveryHashes {
			code := &model.PasskeyRecoveryCode{ID: newID(), UserID: userID, CodeHash: hash}
			if _, err := tx.NewInsert().Model(code).Exec(ctx); err != nil {
				return false, err
			}
		}
		recoveryInstalled = true
	}

	audit := &model.AuditLog{
		ID: newID(), Entity: "passkey", EntityID: row.ID, Action: "enrolled",
		ActorType: "user", ActorID: userID, IPAddress: ip,
		Changes: map[string]any{"name": name, "first_passkey": count == 0},
	}
	if _, err := tx.NewInsert().Model(audit).Exec(ctx); err != nil {
		return false, fmt.Errorf("write passkey audit: %w", err)
	}
	return recoveryInstalled, tx.Commit()
}

func (s *Store) UpdatePasskeyCredentialWithAudit(ctx context.Context, userID, ip string, credential *webauthn.Credential) error {
	if credential == nil || len(credential.ID) == 0 {
		return errors.New("invalid WebAuthn credential")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	row := new(model.PasskeyCredential)
	if err := tx.NewSelect().Model(row).
		Where("user_id = ? AND credential_id = ?", userID, credential.ID).
		For("UPDATE").
		Scan(ctx); err != nil {
		return err
	}
	envelope, err := s.encodePasskeyCredential(row.ID, credential)
	if err != nil {
		return err
	}
	now := time.Now()
	res, err := tx.NewUpdate().Model((*model.PasskeyCredential)(nil)).
		Set("credential_data = ?", envelope).
		Set("last_used_at = ?", now).
		Where("id = ? AND user_id = ?", row.ID, userID).
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	audit := &model.AuditLog{
		ID: newID(), Entity: "passkey", EntityID: row.ID, Action: "asserted",
		ActorType: "webauthn", ActorID: userID, IPAddress: ip,
		Changes: map[string]any{"sign_count": credential.Authenticator.SignCount},
	}
	if _, err := tx.NewInsert().Model(audit).Exec(ctx); err != nil {
		return fmt.Errorf("write passkey assertion audit: %w", err)
	}
	return tx.Commit()
}

func (s *Store) DeletePasskeyCredentialWithAudit(ctx context.Context, userID, credentialRowID, ip string) error {
	return s.RunAuditedMutation(ctx, &model.AuditLog{
		Entity: "passkey", EntityID: credentialRowID, Action: "deleted",
		ActorType: "user", ActorID: userID, IPAddress: ip,
	}, func(ctx context.Context, tx bun.Tx) error {
		var lockedUserID string
		if err := tx.NewRaw("SELECT id FROM users WHERE id = ? FOR UPDATE", userID).Scan(ctx, &lockedUserID); err != nil {
			return err
		}
		count, err := tx.NewSelect().Model((*model.PasskeyCredential)(nil)).Where("user_id = ?", userID).Count(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastPasskey
		}
		res, err := tx.NewDelete().Model((*model.PasskeyCredential)(nil)).
			Where("id = ? AND user_id = ?", credentialRowID, userID).Exec(ctx)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
}

// RecoverPasskeys consumes a one-time recovery code and resets every existing
// passkey/session for the user. The audit row is committed atomically with the
// reset, so recovery cannot succeed without durable evidence.
func (s *Store) RecoverPasskeys(ctx context.Context, userID, recoveryHash, ip string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	code := new(model.PasskeyRecoveryCode)
	if err := tx.NewSelect().Model(code).
		Where("user_id = ? AND code_hash = ? AND used_at IS NULL", userID, recoveryHash).
		For("UPDATE").Scan(ctx); err != nil {
		return err
	}
	if _, err := tx.NewDelete().Model((*model.PasskeyCredential)(nil)).Where("user_id = ?", userID).Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.NewDelete().Model((*model.PasskeyRecoveryCode)(nil)).Where("user_id = ?", userID).Exec(ctx); err != nil {
		return err
	}
	res, err := tx.NewRaw("UPDATE users SET session_version = session_version + 1, updated_at = now() WHERE id = ?", userID).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	audit := &model.AuditLog{
		ID: newID(), Entity: "passkey", EntityID: userID, Action: "recovered",
		ActorType: "recovery_code", ActorID: userID, IPAddress: ip,
		Changes: map[string]any{"all_passkeys_revoked": true, "all_sessions_revoked": true},
	}
	if _, err := tx.NewInsert().Model(audit).Exec(ctx); err != nil {
		return fmt.Errorf("write recovery audit: %w", err)
	}
	return tx.Commit()
}
