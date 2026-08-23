-- Authentication hardening: revocable access sessions and passkey persistence.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS session_version BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS passkey_credentials (
    id                  TEXT PRIMARY KEY,
    user_id             TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id       BYTEA NOT NULL UNIQUE,
    credential_data     BYTEA NOT NULL,
    name                TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at        TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_passkey_credentials_user
    ON passkey_credentials(user_id);

-- Ceremony state is server-side and single-use. Only a SHA-256 digest of the
-- opaque ceremony id returned to the browser is persisted.
CREATE TABLE IF NOT EXISTS webauthn_sessions (
    ceremony_id_hash TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose          TEXT NOT NULL CHECK (purpose IN ('register', 'login')),
    session_data     JSONB NOT NULL,
    expires_at       TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_webauthn_sessions_expiry
    ON webauthn_sessions(expires_at);

CREATE TABLE IF NOT EXISTS passkey_recovery_codes (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash   TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    used_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_passkey_recovery_codes_user
    ON passkey_recovery_codes(user_id);

CREATE TABLE IF NOT EXISTS audit_outbox (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    entity      TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    action      TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_outbox_created_at ON audit_outbox(created_at);

CREATE OR REPLACE FUNCTION licensehub_enqueue_audit_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE row_id TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN row_id := to_jsonb(OLD)->>'id';
    ELSE row_id := to_jsonb(NEW)->>'id';
    END IF;
    IF row_id IS NOT NULL AND row_id <> '' THEN
        INSERT INTO audit_outbox(entity, entity_id, action)
        VALUES (TG_TABLE_NAME, row_id, lower(TG_OP));
    END IF;
    RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;

CREATE TRIGGER lh_outbox_product AFTER INSERT OR UPDATE OR DELETE ON products
    FOR EACH ROW EXECUTE FUNCTION licensehub_enqueue_audit_mutation();
CREATE TRIGGER lh_outbox_plan AFTER INSERT OR UPDATE OR DELETE ON plans
    FOR EACH ROW EXECUTE FUNCTION licensehub_enqueue_audit_mutation();
CREATE TRIGGER lh_outbox_entitlement AFTER INSERT OR UPDATE OR DELETE ON entitlements
    FOR EACH ROW EXECUTE FUNCTION licensehub_enqueue_audit_mutation();
CREATE TRIGGER lh_outbox_activation AFTER INSERT OR UPDATE OR DELETE ON activations
    FOR EACH ROW EXECUTE FUNCTION licensehub_enqueue_audit_mutation();
CREATE TRIGGER lh_outbox_subscription AFTER INSERT OR UPDATE OR DELETE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION licensehub_enqueue_audit_mutation();
CREATE TRIGGER lh_outbox_user AFTER INSERT OR UPDATE OR DELETE ON users
    FOR EACH ROW EXECUTE FUNCTION licensehub_enqueue_audit_mutation();
CREATE TRIGGER lh_outbox_passkey AFTER INSERT OR UPDATE OR DELETE ON passkey_credentials
    FOR EACH ROW EXECUTE FUNCTION licensehub_enqueue_audit_mutation();
CREATE TRIGGER lh_outbox_release AFTER INSERT OR UPDATE OR DELETE ON releases
    FOR EACH ROW EXECUTE FUNCTION licensehub_enqueue_audit_mutation();
CREATE TRIGGER lh_outbox_addon AFTER INSERT OR UPDATE OR DELETE ON addons
    FOR EACH ROW EXECUTE FUNCTION licensehub_enqueue_audit_mutation();
CREATE TRIGGER lh_outbox_api_key AFTER INSERT OR UPDATE OR DELETE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION licensehub_enqueue_audit_mutation();
CREATE TRIGGER lh_outbox_license AFTER INSERT OR UPDATE OR DELETE ON licenses
    FOR EACH ROW EXECUTE FUNCTION licensehub_enqueue_audit_mutation();
CREATE TRIGGER lh_outbox_release_signer AFTER INSERT OR UPDATE OR DELETE ON release_signing_keys
    FOR EACH ROW EXECUTE FUNCTION licensehub_enqueue_audit_mutation();
