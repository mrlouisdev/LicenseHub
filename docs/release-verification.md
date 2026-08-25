# Release verification

This document separates the historical `0.1.0` target-VPS acceptance from the
current production-hardened `0.2.1` release at commit `b84eb74`.

## Passed locally

- Go server: unit tests, vet and final Windows build.
- Rust core: format, clippy with warnings denied, 14 tests and release DLL.
- Console: ESLint, 12 Vitest tests, web build and Tauri/NSIS build.
- Browser QA: first-run connection screen, explicit Demo mode, navigation,
  remote plain-HTTP rejection and zero browser warnings/errors.
- Protocol validator and PowerShell parser checks.
- .NET, Node/Electron, Python and C++ ABI/status/error smoke tests.
- `licctl` generated kits for all four supported stacks.
- Node/Python packages contain the final native DLL hash.
- Production configuration fails closed without release-key encryption.
- Client contracts reject oversized/ambiguous JSON, HTTP responses and leases.
- Metrics are token-protected in the server and blocked at the public Caddy edge.
- Compose configuration validates with read-only app filesystems, dropped
  capabilities, resource limits, rotated JSON logs and file-backed secrets that
  remain readable by the non-root application and PostgreSQL users.
- Linux VPS scripts cover secret generation, non-public bootstrap, pre-deploy
  backup, transactional restore and public edge verification.

## Historical target-VPS acceptance (0.1.0)

Production acceptance passed on `license.zmmo.shop` against the checked
source-only bundle deployed through an existing Caddy edge:

- Private build, migration and health checks passed with PostgreSQL unexposed.
- Public health, key-ring, strict JSON, security-header and `/metrics`-404 gates
  passed over HTTPS.
- Machine A activated; machine B was rejected by the one-device limit.
- Lease refresh, entitlement allow/deny and quota enforcement passed.
- Refresh survived a server restart; revocation denied the next refresh.
- Created/activated/revoked audit evidence was present.
- A checksummed dump was restored transactionally after a database mutation;
  the original value and revoked license state were recovered.
- The existing `license.hubflow.store` API and unrelated Compose services stayed
  healthy across the domain-only cutover.

The VPS retains a mode-0600 machine-readable acceptance record and two rollback
layers: the pre-cutover HubFlow dump/config snapshot and LicenseHub's verified
backup plus pre-restore safety dump.

## Current production acceptance (0.2.1)

- All six protected GitHub checks passed for the deployed source line.
- The release archive hash and Linux line endings were verified before staging.
- A fresh encrypted pre-deploy backup completed before the `0.2.1` image build.
- The non-root image started against the historical migration checksums without
  weakening SQL tamper detection; the new auth-security migration applied.
- Public HTTPS health, public key ring, strict JSON, security headers and
  metrics-404 gates passed through Cloudflare.
- Direct-origin HTTP and HTTPS requests using the license hostname returned 403;
  no host listener exposes the application port.
- Gmail SMTP OTP delivery, OTP consumption, owner login and logout replay
  rejection passed. Login/logout audit rows committed and the audit outbox was
  empty after flushing.
- Server restart preserved data and public acceptance. The five-minute monitor
  and daily encrypted-backup timers are active; manual runs returned success.
- First owner passkey enrollment remains an explicit user-presence gesture in a
  supported browser. The backend and UI gate are deployed and regression-tested,
  but this account-level ceremony cannot be claimed until the owner completes it.

## Release gates

- Windows files are not Authenticode-signed because no certificate was supplied.
- Signer rotation is staged: ship both current and next public-key pins before
  changing the VPS signer.
- License storage currently uses the upstream-compatible transition schema.
  Ciphertext and hash are populated while the compatibility column remains until
  the separately tested Phase-C migration is deployed.
