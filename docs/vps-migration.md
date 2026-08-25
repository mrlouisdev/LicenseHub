# VPS migration runbook

Clients use a stable domain, never a VPS IP. Preserve the database, signing key
identity, encryption configuration, product IDs and license records.

## Planned cutover

1. At least one TTL period earlier, reduce DNS TTL to 300 seconds.
2. Run `Migrate-LicenseHubVps.ps1` to create a checksummed migration bundle.
   The bundle includes the pinned image registry, restore/recovery/monitor
   scripts and a v2 encrypted backup when `-SkipBackup` is not used.
   Run `Verify-MigrationBundle.ps1 -Bundle <path>` before transfer. It rejects
   missing, modified, path-escaping or unchecksummed files and reports the exact
   source commit plus dirty-worktree state captured in `migration-metadata.json`.
   Linux scripts, Compose files, Docker build inputs and migration SQL are
   normalized to LF in the bundle even when it is staged from Windows.
   For a clean initial/source-only bundle where no local database exists, use
   `-SkipBackup`; take the authoritative backup on the source VPS instead.
3. Copy the self-contained bundle (`deploy/`, `server/`, `backup/`) to the new
   VPS and independently provision `deploy/.env` from the secrets manager.
4. Run `chmod 700 deploy/*.sh`. Generate a new environment only for a genuinely
   new installation; migrations must reuse the exact source configuration
   because changing signing/encryption keys breaks clients and encrypted rows.
5. From `deploy/`, run `./deploy.sh`. It makes a pre-deploy database backup,
   atomically materializes mode-0400 container-readable secret files, builds
   the staged server context and refuses to expose an uninitialized
   installation. Run it as root so the server and PostgreSQL secret files get
   their configured container UID ownership. Use `./bootstrap.sh <admin-email>`
   only for first install.
   When an existing proxy owns 80/443, select
   `docker-compose.integrated.yml`, set `EXTERNAL_EDGE_NETWORK`, and cut over
   only the intended virtual-host block after private health passes.
6. Test with a local DNS override: health, login, refresh, activation, deactivate.
7. Put the old server in a maintenance window and create a final backup.
8. Restore the final dump on the new server and change DNS.
9. Run the public verifier, monitor errors and retain the old VPS, non-writing,
   for at least 72 hours.

## Rollback

Stop writes on the new server, point DNS back to the old server and reconcile any
new writes. Never allow both servers to accept mutations against independent
databases.

For an in-place database rollback, use `deploy/restore.sh <backup> --force`.
The restore script verifies checksums, takes a safety dump and uses one
transaction; public verification remains a separate explicit gate.

## Disaster recovery

Restore PostgreSQL and exact server configuration on a clean host, deploy the
pinned server version, then point the stable domain to it. Cached leases bridge
an outage only until signed `exp`; they do not replace database recovery.
