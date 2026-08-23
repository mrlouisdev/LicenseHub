package store_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/tabloy/keygate/internal/crypto"
	"github.com/tabloy/keygate/internal/model"
)

func TestConsumeOTPCodeTwentyConcurrentExactlyOneSessionWinner(t *testing.T) {
	s := setupTestDB(t)
	defer s.Close()
	ctx := context.Background()
	email := fmt.Sprintf("otp-race-%d@example.test", time.Now().UnixNano())
	const digest = "fixture-keyed-otp-digest"
	otp := &model.OTPCode{Email: email, CodeHash: digest, ExpiresAt: time.Now().Add(time.Minute)}
	if err := s.CreateOTPCode(ctx, otp); err != nil {
		t.Fatalf("create OTP: %v", err)
	}
	defer s.DB.NewRaw("DELETE FROM otp_codes WHERE email = ?", email).Exec(ctx) //nolint:errcheck

	const concurrency = 20
	start := make(chan struct{})
	var wg sync.WaitGroup
	var winners atomic.Int32
	var failures atomic.Int32
	wg.Add(concurrency)
	for range concurrency {
		go func() {
			defer wg.Done()
			<-start
			_, matched, err := s.ConsumeOTPCode(ctx, email, digest)
			if err != nil {
				failures.Add(1)
				return
			}
			if matched {
				winners.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("concurrent verification returned %d storage errors", failures.Load())
	}
	if winners.Load() != 1 {
		t.Fatalf("correct OTP had %d winners; want exactly one", winners.Load())
	}
	var state struct {
		Used     bool `bun:"used"`
		Attempts int  `bun:"attempts"`
	}
	if err := s.DB.NewRaw("SELECT used, attempts FROM otp_codes WHERE id = ?", otp.ID).Scan(ctx, &state); err != nil {
		t.Fatalf("read OTP state: %v", err)
	}
	if !state.Used || state.Attempts != 1 {
		t.Fatalf("OTP state = used:%v attempts:%d, want true/1", state.Used, state.Attempts)
	}
}

func TestWebAuthnCeremonyIsSingleUse(t *testing.T) {
	s := setupTestDB(t)
	defer s.Close()
	ctx := context.Background()
	user := &model.User{Email: fmt.Sprintf("ceremony-%d@example.test", time.Now().UnixNano())}
	if err := s.UpsertUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	user, _ = s.FindUserByEmail(ctx, user.Email)
	row := &model.WebAuthnSession{
		CeremonyIDHash: fmt.Sprintf("fixture-%d", time.Now().UnixNano()),
		UserID:         user.ID, Purpose: "login", SessionData: []byte(`{"challenge":"fixture"}`),
		ExpiresAt: time.Now().Add(time.Minute),
	}
	if err := s.SaveWebAuthnSession(ctx, row); err != nil {
		t.Fatalf("save ceremony: %v", err)
	}
	if _, err := s.ConsumeWebAuthnSession(ctx, row.CeremonyIDHash, user.ID, "login"); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, err := s.ConsumeWebAuthnSession(ctx, row.CeremonyIDHash, user.ID, "login"); err == nil {
		t.Fatal("replayed ceremony was accepted")
	}
}

func TestPasskeyRecoveryIsEncryptedAtomicAndAudited(t *testing.T) {
	s := setupTestDB(t)
	defer s.Close()
	ctx := context.Background()
	s.PasskeyCredentialAEAD = crypto.MustDeriveAEAD([]byte("01234567890123456789012345678901"), "passkey-test")
	user := &model.User{Email: fmt.Sprintf("passkey-%d@example.test", time.Now().UnixNano()), Role: model.RoleAdmin}
	if err := s.UpsertUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	user, _ = s.FindUserByEmail(ctx, user.Email)
	credential := &webauthn.Credential{ID: []byte("credential-fixture"), PublicKey: []byte("public-key-fixture")}
	hashes := make([]string, 10)
	for i := range hashes {
		hashes[i] = fmt.Sprintf("recovery-hash-%d-%d", time.Now().UnixNano(), i)
	}
	installed, err := s.CreatePasskeyCredential(ctx, user.ID, "Hardware key", "127.0.0.1", credential, hashes)
	if err != nil {
		t.Fatalf("create passkey: %v", err)
	}
	if !installed {
		t.Fatal("first passkey did not install recovery codes")
	}
	var envelope []byte
	if err := s.DB.NewRaw("SELECT credential_data FROM passkey_credentials WHERE user_id = ?", user.ID).Scan(ctx, &envelope); err != nil {
		t.Fatalf("read credential envelope: %v", err)
	}
	if strings.Contains(string(envelope), "public-key-fixture") {
		t.Fatal("credential public key record was stored without encryption")
	}
	records, err := s.ListPasskeyCredentials(ctx, user.ID)
	if err != nil || len(records) != 1 || string(records[0].Credential.ID) != "credential-fixture" {
		t.Fatalf("encrypted credential round-trip failed: records=%d err=%v", len(records), err)
	}
	if err := s.RecoverPasskeys(ctx, user.ID, hashes[0], "127.0.0.1"); err != nil {
		t.Fatalf("recover passkeys: %v", err)
	}
	if count, err := s.CountPasskeyCredentials(ctx, user.ID); err != nil || count != 0 {
		t.Fatalf("passkeys after recovery = %d, err=%v", count, err)
	}
	var state struct {
		SessionVersion int64 `bun:"session_version"`
		AuditCount     int   `bun:"audit_count"`
	}
	if err := s.DB.NewRaw(`
		SELECT u.session_version,
		       (SELECT count(*) FROM audit_logs WHERE entity = 'passkey' AND entity_id = u.id AND action = 'recovered') AS audit_count
		FROM users u WHERE u.id = ?`, user.ID).Scan(ctx, &state); err != nil {
		t.Fatalf("read recovery state: %v", err)
	}
	if state.SessionVersion != 1 || state.AuditCount != 1 {
		t.Fatalf("recovery state = version:%d audit:%d, want 1/1", state.SessionVersion, state.AuditCount)
	}
}

func TestAuditedLogoutRollsBackMutationWhenAuditWriteFails(t *testing.T) {
	s := setupTestDB(t)
	defer s.Close()
	ctx := context.Background()
	user := &model.User{Email: fmt.Sprintf("audit-rollback-%d@example.test", time.Now().UnixNano())}
	if err := s.UpsertUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	user, _ = s.FindUserByEmail(ctx, user.Email)
	if err := s.CreateRefreshToken(ctx, user.ID, "fixture-refresh-hash-"+user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	trigger := "fail_audit_" + strings.ReplaceAll(user.ID, "-", "_")
	function := trigger + "_fn"
	defer s.DB.NewRaw("DROP TRIGGER IF EXISTS " + trigger + " ON audit_logs; DROP FUNCTION IF EXISTS " + function + "()").Exec(ctx) //nolint:errcheck
	if _, err := s.DB.NewRaw(fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'forced audit failure'; END $$;
		CREATE TRIGGER %s BEFORE INSERT ON audit_logs
		FOR EACH ROW WHEN (NEW.entity_id = '%s') EXECUTE FUNCTION %s();`,
		function, trigger, user.ID, function)).Exec(ctx); err != nil {
		t.Fatalf("install audit failure trigger: %v", err)
	}
	if err := s.RevokeUserSessionsWithAudit(ctx, user.ID, "127.0.0.1"); err == nil {
		t.Fatal("forced audit failure did not fail the mutation")
	}
	var state struct {
		SessionVersion int64 `bun:"session_version"`
		RefreshCount   int   `bun:"refresh_count"`
	}
	if err := s.DB.NewRaw(`
		SELECT session_version,
		       (SELECT count(*) FROM refresh_tokens WHERE user_id = users.id) AS refresh_count
		FROM users WHERE id = ?`, user.ID).Scan(ctx, &state); err != nil {
		t.Fatalf("read rollback state: %v", err)
	}
	if state.SessionVersion != 0 || state.RefreshCount != 1 {
		t.Fatalf("mutation escaped failed audit: version=%d refresh=%d", state.SessionVersion, state.RefreshCount)
	}
}

func TestTransactionalAuditOutboxRollsBackAndFlushes(t *testing.T) {
	s := setupTestDB(t)
	defer s.Close()
	ctx := context.Background()
	productID := fmt.Sprintf("audit-outbox-%d", time.Now().UnixNano())
	trigger := "fail_outbox_" + strings.ReplaceAll(productID, "-", "_")
	function := trigger + "_fn"
	defer s.DB.NewRaw("DROP TRIGGER IF EXISTS " + trigger + " ON audit_outbox; DROP FUNCTION IF EXISTS " + function + "()").Exec(ctx) //nolint:errcheck
	if _, err := s.DB.NewRaw(fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'forced outbox failure'; END $$;
		CREATE TRIGGER %s BEFORE INSERT ON audit_outbox
		FOR EACH ROW WHEN (NEW.entity_id = '%s') EXECUTE FUNCTION %s();`,
		function, trigger, productID, function)).Exec(ctx); err != nil {
		t.Fatalf("install outbox failure trigger: %v", err)
	}
	product := &model.Product{ID: productID, Name: "Outbox rollback", Slug: productID, Type: "desktop"}
	if err := s.CreateProduct(ctx, product); err == nil {
		t.Fatal("forced outbox failure did not fail product mutation")
	}
	if count, err := s.DB.NewSelect().Model((*model.Product)(nil)).Where("id = ?", productID).Count(ctx); err != nil || count != 0 {
		t.Fatalf("product escaped failed outbox transaction: count=%d err=%v", count, err)
	}
	if _, err := s.DB.NewRaw("DROP TRIGGER " + trigger + " ON audit_outbox; DROP FUNCTION " + function + "()").Exec(ctx); err != nil {
		t.Fatalf("remove outbox failure trigger: %v", err)
	}
	if err := s.CreateProduct(ctx, product); err != nil {
		t.Fatalf("create product after removing failure: %v", err)
	}
	if _, err := s.FlushAuditOutbox(ctx, 1000); err != nil {
		t.Fatalf("flush outbox: %v", err)
	}
	var count int
	if err := s.DB.NewRaw(`SELECT count(*) FROM audit_logs
		WHERE entity = 'products' AND entity_id = ? AND action = 'db_insert'
		AND changes->>'source' = 'transactional_outbox'`, productID).Scan(ctx, &count); err != nil {
		t.Fatalf("read flushed audit row: %v", err)
	}
	if count != 1 {
		t.Fatalf("flushed audit rows = %d, want 1", count)
	}
}
