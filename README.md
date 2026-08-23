# LicenseHub

LicenseHub is a reusable licensing platform for multiple desktop applications. A
single VPS manages products, licenses and device activations; applications use a
shared client contract and verify short-lived Ed25519 leases offline.

## Components

- `server/` — Go API and web dashboard.
- `console/` — operator console.
- `core/` — universal client core.
- `protocol/` — versioned API and signed-lease contract.
- `deploy/` — production Docker Compose and Caddy configuration.
- `scripts/` — backup, restore and VPS migration helpers.
- `docs/` — threat model and operations runbooks.

## Install the management app

```powershell
Start-Process '.\console\src-tauri\target\release\bundle\nsis\LicenseHub Console_0.1.0_x64-setup.exe'
```

First launch opens **Connection settings**. Choose a LicenseHub VPS or explicit
Demo mode. Production mode is selected at runtime, never baked into the build.

## Start the VPS stack

On a Linux VPS, use the checked deployment scripts in this order:

1. `new-env.sh` once for a new installation.
2. `bootstrap.sh` to create the owner before Caddy is exposed.
3. `verify.sh` for the public edge gate.

For local Compose development, copy the environment template and replace every
`CHANGE_ME` value:

```powershell
Copy-Item deploy/.env.example deploy/.env
docker compose --env-file deploy/.env -f deploy/docker-compose.yml config
docker compose --env-file deploy/.env -f deploy/docker-compose.yml up -d --build
docker compose --env-file deploy/.env -f deploy/docker-compose.yml ps
```

Production exposes only Caddy on ports 80/443. PostgreSQL and the server stay on
private Compose networks. Applications use the stable domain in `LICENSE_DOMAIN`;
never embed a VPS IP address in a client.

For a VPS whose existing reverse proxy already owns 80/443, use
`deploy/docker-compose.integrated.yml`, set `EXTERNAL_EDGE_NETWORK`, and route
the domain to `licensehub-server:9000`. This preserves unrelated sites and keeps
PostgreSQL private; see `docs/operations.md` for cutover and rollback.

Production startup fails closed unless the signing seed, JWT secret and
`RELEASE_KEY_ENCRYPTION_KEY` are valid. `/metrics` is blocked at the public
edge and requires a separate 32+ character Bearer `METRICS_TOKEN` on the
internal server route. The Compose stack also uses read-only application
filesystems, dropped Linux capabilities, process/memory/CPU limits and bounded
JSON logs.

## Client contract

```text
GET  /v1/client/public-keys
POST /v1/client/activate
POST /v1/client/refresh
POST /v1/client/deactivate
```

Activate and refresh return a signed `lease`. Its format is
`base64url(JSON).base64url(Ed25519_signature)` and the signature covers the first
encoded segment exactly. Read `protocol/openapi.yaml` and
`protocol/lease.schema.json` for the wire contract.

Activation sends `{product_id, license_key, device_id, label?}`. Refresh and
deactivation send `{product_id, device_id, lease}` so the raw license key is not
retained after activation. The public-key endpoint returns `{keys: {kid: base64}}`.
Client request bodies, HTTP responses and signed leases are size-bounded before
JSON parsing. SDKs reject malformed key IDs, unexpected signing identities,
oversized claims and unsafe timeout/clock-skew configuration.

## Integrate another app

`scripts/Build-LicctlPortable.ps1` creates
`artifacts/licctl-portable-win-x64.zip`: a self-contained CLI distribution with
the four adapters and native runtime. Extract it anywhere; `licctl.exe` can then
generate a ready integration kit without a LicenseHub source checkout. Verify
the archive with `scripts/Verify-LicctlPortable.ps1`. Run `licctl --help` for
the command shape.
The complete copy/add/doctor/verify/remove flow is documented in
`docs/integration-quickstart.md`; the four-stack acceptance test is
`tests/integration/Run-LicctlIntegration.ps1`.
For signer rotation, first ship an app update whose manifest pins both current
and next key IDs. Cut the VPS over only after adoption; see `docs/operations.md`.

Each protected app follows the same lifecycle: activate once, persist the
device-bound signed lease, refresh while online, check the named entitlement at
the feature boundary, and deactivate on an intentional device transfer.
Revocation takes effect at the next refresh or lease expiry; shorten
`LICENSE_LEASE_TTL` where faster revocation matters more than offline uptime.

## Backup and migration

```powershell
# Preview without mutation
./scripts/Backup-LicenseHub.ps1 -EnvironmentFile .\deploy\.env -DryRun

# Create an encrypted database + recovery-material backup on Linux
./deploy/backup.sh ./backups

# Destructive restore requires explicit --force and keeps a safety dump
./deploy/restore.sh ./backups/20260823T031500Z --force

# Build a self-contained, checksummed migration bundle outside the repository
./scripts/Migrate-LicenseHubVps.ps1 -Destination D:\licensehub-migration -DryRun

# Verify every staged file plus source/deployment metadata before transfer
./scripts/Verify-MigrationBundle.ps1 -Bundle D:\licensehub-migration
```

The bundle contains the database backup, Compose/Caddy configuration and exact
server build context. Runtime configuration stays excluded and must be
provisioned independently on the destination VPS. Read `docs/operations.md`,
`docs/vps-migration.md`, `docs/release-verification.md` and
`release-manifest.json` before deployment or cutover.

Use `-SkipBackup` to build a source-only bundle when the authoritative database
lives on the VPS. Linux operations use `deploy/new-env.sh`, `deploy/bootstrap.sh`,
`deploy/deploy.sh`, `deploy/backup.sh`, `deploy/restore.sh` and
`deploy/recover-host.sh`. `deploy/install-operations.sh` installs a daily
encrypted backup timer and five-minute public acceptance monitor. Pulled images
are locked in `deploy/images.lock`; production secrets are mounted as Docker
secrets and read through `*_FILE` variables.
