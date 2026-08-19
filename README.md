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

Copy the environment template and replace every `CHANGE_ME` value:

```powershell
Copy-Item deploy/.env.example deploy/.env
docker compose --env-file deploy/.env -f deploy/docker-compose.yml config
docker compose --env-file deploy/.env -f deploy/docker-compose.yml up -d --build
docker compose --env-file deploy/.env -f deploy/docker-compose.yml ps
```

Production exposes only Caddy on ports 80/443. PostgreSQL and the server stay on
private Compose networks. Applications use the stable domain in `LICENSE_DOMAIN`;
never embed a VPS IP address in a client.

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

## Integrate another app

`cli/licctl/dist/win-x64/licctl.exe` generates a ready integration kit for
.NET, Electron/Node, Python or C++. Run `licctl --help` for the command shape.
For signer rotation, first ship an app update whose manifest pins both current
and next key IDs. Cut the VPS over only after adoption; see `docs/operations.md`.

## Backup and migration

```powershell
# Preview without mutation
./scripts/Backup-LicenseHub.ps1 -DryRun

# Create a database dump plus SHA-256 manifest
./scripts/Backup-LicenseHub.ps1

# Destructive restore requires explicit -Force
./scripts/Restore-LicenseHub.ps1 -BackupDirectory .\backups\20260819T120000Z -Force

# Build a self-contained, checksummed migration bundle outside the repository
./scripts/Migrate-LicenseHubVps.ps1 -Destination D:\licensehub-migration -DryRun
```

The bundle contains the database backup, Compose/Caddy configuration and exact
server build context. Runtime configuration stays excluded and must be
provisioned independently on the destination VPS. Read `docs/operations.md`,
`docs/vps-migration.md`, `docs/release-verification.md` and
`release-manifest.json` before deployment or cutover.
