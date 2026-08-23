# LicenseHub integration quickstart

This is the repeatable path for adding LicenseHub to a clean application. The
kit is stack-agnostic; the project directory determines the adapter.

## Create a product kit

Build and verify the portable CLI distribution once per release:

```powershell
.\scripts\Build-LicctlPortable.ps1
.\scripts\Verify-LicctlPortable.ps1 -Archive .\artifacts\licctl-portable-win-x64.zip
```

Extract that archive on any Windows administration machine. It contains
`licctl.exe`, all four adapters, their native runtime and a SHA-256 manifest;
it does not depend on the LicenseHub source checkout.

```powershell
licctl init --product my-app --profile .\product.profile.json --out .\licensehub-kit
Compress-Archive .\licensehub-kit\* .\licensehub-kit.zip
```

The profile contains the HTTPS server URL, public-key ring and safe timeout /
clock settings. It must never contain a private signing seed or license key.

## Add, verify and run

```powershell
licctl add --project . --kit .\licensehub-kit.zip
licctl doctor --project . --json
licctl verify --project .
licctl verify --project . --live --activation-stdin --entitlement my-app.pro
```

The first `add` writes only `.licensehub/` vendor/runtime files, a generated
adapter and the smallest project hook. It records SHA-256 hashes and backups in
`.licensehub/install.json`. A second identical `add` prints `NOOP` and does not
rewrite a clean project.

Use the adapter lifecycle `status → activate → refresh → require_entitlement →
deactivate`. Persist the device-bound lease in the SDK cache; do not retain the
raw license key after activation. Gate premium features with
`require_entitlement`, and refresh before lease expiry.

Supported project roots:

- .NET: one top-level `.csproj`.
- Electron: `package.json`, with generated main/preload/renderer IPC bridge.
- Python: `pyproject.toml`, `requirements.txt`, or a top-level `.py` file.
- C++: `CMakeLists.txt` containing a literal `add_executable` target.

## Remove or migrate

```powershell
licctl remove --project .
```

Removal refuses to overwrite a file modified after installation. It restores
recorded backups and removes only files owned by the manifest.

On a new VPS, provision the same product profile with the new HTTPS domain,
retain the old public key during signing-key rotation, then run `doctor` and
`verify --live`. Client code stays unchanged because it reads the profile,
never an origin IP.

## Security rules

- Keep the server behind HTTPS; localhost is the only development exception.
- Pin public keys by key ID and fingerprint; never trust network-delivered keys
  silently.
- Do not put license keys, JWTs, recovery codes or private signing material in
  source control, logs, package metadata or command-line arguments.
- Treat a failed `doctor` hash check as a stop-and-investigate event.
