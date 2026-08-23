# Production operations

## First deployment

1. Point `LICENSE_DOMAIN` DNS to the VPS and lower TTL before cutover.
2. Stage a checksummed source bundle with
   `Migrate-LicenseHubVps.ps1 -SkipBackup`, then copy it to the VPS.
3. On the VPS run `deploy/new-env.sh` once. It creates independent PostgreSQL,
   JWT, Ed25519, metrics and encryption values without printing them. Keep the
   resulting file mode `0600`.
4. Run `chmod 700 deploy/*.sh` and validate Compose; confirm PostgreSQL has no
   published host port.
5. Run `deploy/bootstrap.sh <admin-email>`. It initializes the first owner
   through the container-local interface before Caddy starts, removing the
   public first-owner race window.
6. Run the public verifier and check the HTTPS `/health` endpoint.
7. Confirm public `/metrics` returns 404 and the internal server route rejects
   a missing/invalid Bearer token.
8. Create the first admin, a test product and a one-device test license.
9. Activate machine A, reject machine B, refresh, verify entitlement allow/deny,
   revoke, and confirm the next refresh is denied.
10. Restart the stack and verify license/device/audit state persists.

### Existing reverse proxy

If another Compose project already owns ports 80/443, use
`deploy/docker-compose.integrated.yml` instead of starting the bundled Caddy.
Set `LICENSEHUB_ENV_FILE` to the protected environment file and
`EXTERNAL_EDGE_NETWORK` to the proxy's external Docker network. Route only the
license domain to `licensehub-server:9000`, keep `/metrics` as a public 404,
validate the proxy configuration, then reload it. PostgreSQL stays isolated.

`backup.sh` and `restore.sh` accept the integrated topology through
`COMPOSE_FILE` and `ENV_FILE`. Restore discovers whether the selected topology
contains Caddy, so it stops/restarts only services it owns.

Never commit local environment files, database dumps or server key material.
Keep server configuration in a dedicated secrets manager and test recovery.

## Metrics and edge isolation

- Caddy deliberately returns 404 for public `/metrics`; do not remove this edge
  rule to make monitoring easier.
- Scrape the server on the private Compose network and send
  `Authorization: Bearer <METRICS_TOKEN>`.
- An unset production token disables the server metrics route as 404. A weak
  configured token is a startup error.
- Never reuse JWT, signing, database, metrics or encryption secrets.
- Keep edge request limits and server-side 64 KiB client-contract limits; the
  edge limit also protects unrelated UI/API routes.

## Backup policy

- On Linux run `deploy/backup.sh`; on Windows administration hosts use
  `Backup-LicenseHub.ps1 -EnvironmentFile <protected-env>`. Both paths emit
  the same v2 bundle: database dump, encrypted recovery environment, image
  lock, manifest and checksums. Run it daily and before every upgrade/migration.
- Keep the age identity only on the recovery host; the backup directory contains
  no plaintext environment or signing material. `Restore-LicenseHub.ps1` and
  `deploy/restore.sh` reject incomplete or non-v2 bundles.
- Retain at least 7 daily, 4 weekly and 3 monthly backups.
- Copy backups off the VPS; storage on the same host is not disaster recovery.
- Restore-test a backup on an isolated host at least monthly.
- Record the server image digest and schema version with every deployment.

## Upgrade

1. Run `deploy/deploy.sh`; it creates and verifies a pre-deploy backup before
   rebuilding or running migrations. Keep the printed backup path.
2. Pin a version or commit; do not deploy a floating production tag.
3. Run migrations on one controlled instance.
4. Verify health, activation, refresh and admin login.
5. Verify the public metrics block, invalid-license negative path, revoke flow
   and restart persistence.
6. Keep the previous image and backup through the offline-lease window.

For a database rollback, review the target backup and run
`deploy/restore.sh <backup-directory> --force`. Restore validates the checksum
and archive, creates a second pre-restore safety dump, performs one transactional
restore, and rolls the previous database back automatically if restore or
startup fails. Run the public verifier after every restore.

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
