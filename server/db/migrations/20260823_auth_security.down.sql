DROP FUNCTION IF EXISTS licensehub_enqueue_audit_mutation() CASCADE;
DROP TABLE IF EXISTS audit_outbox;
DROP TABLE IF EXISTS passkey_recovery_codes;
DROP TABLE IF EXISTS webauthn_sessions;
DROP TABLE IF EXISTS passkey_credentials;
ALTER TABLE users DROP COLUMN IF EXISTS session_version;
