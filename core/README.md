# License Core

Embeddable Rust license client with ABI v1 for .NET, Electron/Node, Python and C/C++ wrappers.

## Contract

- Lease format: `base64url(payload).base64url(signature)`. Ed25519 signs the encoded payload segment.
- Payload: `version`, `kid`, `license_id`, `product_id`, `device_id`, `entitlements`, `iat`, `exp`.
- Offline duration comes from signed `exp`; a 72-hour server lease uses `exp = iat + 259200`.
- Product/device binding is verified before cache persistence.
- Windows C ABI protects cache data with user-scoped DPAPI and atomic file replacement.
- Embedded applications only receive public verification keys.

## Rust API

Construct `LicenseClient::initialize` with a `LicenseTransport`, `SecureStore` and `Clock`. These interfaces enable platform secure stores and deterministic tests. `HttpTransport` implements the standard client endpoints.

## C ABI

Include `include/license_core.h`, verify the ABI version, then initialize with UTF-8 JSON:

```json
{
  "product_id": "app_a",
  "server_url": "https://license.example.com",
  "cache_dir": "C:\\Users\\me\\AppData\\Local\\Vendor\\app_a\\license",
  "public_keys": { "2026-01": "BASE64_ED25519_PUBLIC_KEY" },
  "clock_rollback_tolerance_seconds": 300,
  "request_timeout_seconds": 15
}
```

String functions use a two-call pattern: call with `(NULL, 0)` for required bytes including NUL, allocate, then call again. Negative returns are typed error codes; retrieve text through `license_last_error`.

```powershell
cd core
cargo test --workspace
cargo build --release -p license-core
```

