# Production operations

## First deployment

1. Point `LICENSE_DOMAIN` DNS to the VPS and lower TTL before cutover.
2. Copy `deploy/.env.example` to `deploy/.env` and replace every placeholder.
3. Generate independent values for JWT, PostgreSQL, Ed25519 and encryption keys.
4. Validate Compose; confirm PostgreSQL has no published host port.
5. Start the stack and check the HTTPS `/health` endpoint.
6. Create the first admin, a test product and a one-device test license.
7. Activate, refresh, verify offline and deactivate on a clean test machine.

Never commit local environment files, database dumps or server key material.
Keep server configuration in a dedicated secrets manager and test recovery.

## Backup policy

- Run `Backup-LicenseHub.ps1` daily and before every upgrade/migration.
- Retain at least 7 daily, 4 weekly and 3 monthly backups.
- Copy backups off the VPS; storage on the same host is not disaster recovery.
- Restore-test a backup on an isolated host at least monthly.
- Record the server image digest and schema version with every deployment.

## Upgrade

1. Create and verify a backup.
2. Pin a version or commit; do not deploy a floating production tag.
3. Run migrations on one controlled instance.
4. Verify health, activation, refresh and admin login.
5. Keep the previous image and backup through the offline-lease window.

## Key handling

- The license signing value is a 32-byte Ed25519 seed encoded as 64 hex chars.
- The database/release encryption value is an independent 32-byte hex key.
- Rotation needs a public-key overlap longer than maximum lease TTL.
- Losing either value is not fixed by a database restore.

### License signing-key rotation

The client deliberately does not auto-trust `GET /v1/client/public-keys`: TLS
alone does not authorize a new signing identity. Client key rings are immutable
pins supplied by an authenticated application/SDK release.

Use a staged rotation:

1. Generate the next Ed25519 seed and derive its standard-base64 public key.
2. Release clients whose `public_keys` contains both the current and next `kid`.
3. Wait at least the maximum supported client-update interval plus the 72-hour
   lease TTL; verify adoption before switching the server signer.
4. Change `LICENSE_SIGNING_KEY` and `LICENSE_SIGNING_KEY_ID` to the next key.
5. Set `LICENSE_RETAINED_PUBLIC_KEYS` to a compact JSON object containing the
   previous `kid` and public key, for example `{"v1":"BASE64_PUBLIC_KEY"}`.
   Retained public keys let the server authenticate refresh/deactivate requests
   carrying an old signed lease; they do not grant signing authority.
6. Restart, verify that `/v1/client/public-keys` lists both IDs, and test an old
   lease refresh plus a new lease on a pre-staged client.
7. Retain the old public key through the longest lease/update-support window.

Do not rotate the server signer until supported deployed clients already pin the
next public key. A client that has only the old pin must reject leases signed by
an unexpected network-provided key; update that client through the normal signed
software-release channel first.
