# Release verification 0.1.0

## Passed locally

- Go server: unit tests, vet and final Windows build.
- Rust core: format, clippy with warnings denied, 10 tests and release DLL.
- Console: ESLint, 12 Vitest tests, web build and Tauri/NSIS build.
- Browser QA: first-run connection screen, explicit Demo mode, navigation,
  remote plain-HTTP rejection and zero browser warnings/errors.
- Protocol validator and PowerShell parser checks.
- .NET, Node/Electron, Python and C++ ABI/status/error smoke tests.
- `licctl` generated kits for all four supported stacks.
- Node/Python packages contain the final native DLL hash.

## Target-VPS acceptance still required

This workstation had no running Docker daemon or PostgreSQL instance. Before
cutover, execute on a staging VPS:

1. Start Compose and apply migrations.
2. Login by OTP and create one Product/Plan/License.
3. Activate machine A, confirm machine B is rejected, refresh, revoke and reset.
4. Back up, restore on an isolated stack and exercise rollback.
5. Build the migration bundle and boot it on a clean VPS.

## Release gates

- Windows files are not Authenticode-signed because no certificate was supplied.
- Signer rotation is staged: ship both current and next public-key pins before
  changing the VPS signer.
- License storage currently uses the upstream-compatible transition schema.
  Ciphertext and hash are populated while the compatibility column remains until
  the separately tested Phase-C migration is deployed.
