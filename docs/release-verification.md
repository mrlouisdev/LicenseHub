# Release verification

This document separates the historical `0.1.0` target-VPS acceptance from the
current hardened branch. The historical acceptance record remains useful
evidence for the stable domain, but it does not prove that the hardened branch
has been deployed to production.

## Passed locally

- Go server: unit tests, vet and final Windows build.
- Rust core: format, clippy with warnings denied, 13 tests and release DLL.
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
  capabilities, resource limits and rotated JSON logs.
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

## Current hardened-branch gates

- Local Go, Rust, .NET, four-stack `licctl` fixtures and frontend verification
  must pass from the checked-out commit before a deployment is staged.
- A production claim requires a fresh backup, staged deploy evidence, public
  HTTPS lifecycle verification, restart/revoke/logout replay checks, and a
  direct-origin/edge check for the current image digest.
- Until those checks are recorded for the current commit, production status is
  `NOT DEPLOYED` even when the historical `0.1.0` acceptance record is present.

## Release gates

- Windows files are not Authenticode-signed because no certificate was supplied.
- Signer rotation is staged: ship both current and next public-key pins before
  changing the VPS signer.
- License storage currently uses the upstream-compatible transition schema.
  Ciphertext and hash are populated while the compatibility column remains until
  the separately tested Phase-C migration is deployed.
