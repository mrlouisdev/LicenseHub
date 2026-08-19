# LicenseHub server operations

## Defaults

- New desktop and hybrid plans use `max_activations = 1`.
- `ActivateWithinLimit` locks the license row before counting and inserting;
  the database also enforces unique `(license_id, identifier)` pairs.
- Signed offline leases default to 72 hours with `LICENSE_LEASE_TTL=72h`.
- Leases carry `kid`; `/api/v1/license/pubkey` keeps its legacy fields and adds
  a `keys` array for key-ring capable clients.

## Universal client compatibility

The upstream routes remain available. The universal core uses unwrapped JSON:

- `GET /v1/client/public-keys` → `{ "keys": { "kid": "BASE64..." } }`
  in the exact standard-base64 format consumed by the core manifest.
- `POST /v1/client/activate` with `product_id`, `license_key`, `device_id` →
  `{ "lease": "..." }` plus status metadata.
- `POST /v1/client/refresh` with `product_id`, `device_id`, `lease` →
  `{ "lease": "..." }`.
- `POST /v1/client/deactivate` with `product_id`, `device_id`, `lease`.

Lease payloads use canonical claims: `version`, `kid`, `license_id`,
`product_id`, `device_id`, `entitlements`, `iat`, and `exp`. Refresh and
deactivate verify the presented lease before looking up or changing activation.

## Rotation

1. Add the retiring key to `LICENSE_RETAINED_PUBLIC_KEYS` as a standard-base64
   Ed25519 public key, for example `{"old-2025":"BASE64..."}`.
2. Change `LICENSE_SIGNING_KEY` and `LICENSE_SIGNING_KEY_ID` together.
3. Restart and verify `/v1/client/public-keys` contains old and active IDs.
4. Retain old keys for at least `LICENSE_LEASE_TTL`; refresh/deactivate accepts
   old signed leases and every refreshed lease is signed with the active key.

The retained-key JSON rejects duplicate IDs, malformed base64, non-32-byte
keys, invalid IDs, trailing data, and any entry repeating the active ID.

## Existing plans

The migration changes the column default only. Set an existing plan's
`max_activations` to `1` through the admin API/UI to opt it in.

## Device-proof boundary

The MVP binds leases to `identifier + product_id` and atomically limits slots.
A challenge/proof endpoint is deferred until the universal client provides
secure device-key storage, nonce expiry, replay handling, persisted device public
keys, and proof verification on every refresh. Shipping only half of that flow
would create a false trust boundary.

## Verification

```powershell
$go = '..\.toolchains\go\bin\go.exe'
& $go test ./...
& $go build ./cmd/server
```
