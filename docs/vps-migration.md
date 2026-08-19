# VPS migration runbook

Clients use a stable domain, never a VPS IP. Preserve the database, signing key
identity, encryption configuration, product IDs and license records.

## Planned cutover

1. At least one TTL period earlier, reduce DNS TTL to 300 seconds.
2. Run `Migrate-LicenseHubVps.ps1` to create a checksummed deployment bundle.
3. Copy the self-contained bundle (`deploy/`, `server/`, `backup/`) to the new
   VPS and independently provision `deploy/.env` from the secrets manager.
4. From `deploy/`, build the staged server context with
   `docker compose build --pull server`, then restore while isolated.
5. Test with a local DNS override: health, login, refresh, activation, deactivate.
6. Put the old server in a maintenance window and create a final backup.
7. Restore the final dump on the new server and change DNS.
8. Monitor errors and retain the old VPS, non-writing, for at least 72 hours.

## Rollback

Stop writes on the new server, point DNS back to the old server and reconcile any
new writes. Never allow both servers to accept mutations against independent
databases.

## Disaster recovery

Restore PostgreSQL and exact server configuration on a clean host, deploy the
pinned server version, then point the stable domain to it. Cached leases bridge
an outage only until signed `exp`; they do not replace database recovery.
