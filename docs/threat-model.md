# Threat model

## Assets and trust boundaries

- The VPS signing seed and encryption master key are server-only assets.
- PostgreSQL stores products, licenses, entitlements and activation state.
- Clients contain only product ID, endpoint and Ed25519 public keys.
- The license key is a bearer credential; shipped binaries cannot keep shared
  credentials confidential.
- A signed offline lease is an authorization snapshot valid until `exp`.

## Controls

| Threat | Control |
|---|---|
| Forged lease | Ed25519 signature; signing seed remains on VPS. |
| Edited lease | Verify signature over the exact encoded payload segment. |
| Lease copied to another product/device | Verify expected `product_id` and local `device_id`. |
| Two devices racing for one slot | Server transaction and row lock enforce activation count. |
| VPS outage | Verified lease continues only until `exp` (default 72h). |
| Key enumeration | Random keys, per-IP throttling and collapsed public errors. |
| Signing-key rotation | `kid` selects a pinned public-key ring with an overlap window. |
| Clock rollback | Persist highest trusted time and reject material rollback. |
| Backup tampering | Verify a SHA-256 manifest before restore. |

## Residual risks

Client-side checks can be patched by an attacker controlling the machine. Put
entitlement checks at high-value feature boundaries, not only at startup. Shorter
leases reduce revocation delay but increase VPS availability dependence. Device
identity may change after repair, so an audited reset workflow remains necessary.

## Client verification order

1. Split the lease into exactly two non-empty base64url segments.
2. Decode the signature and select the pinned public key by untrusted `kid`.
3. Verify Ed25519 over the first encoded segment exactly.
4. Decode JSON and validate `protocol/lease.schema.json`.
5. Require supported `version`, expected `product_id` and local `device_id`.
6. Require `iat <= trusted_now + allowed_skew` and `trusted_now < exp`.
7. Check entitlement at the protected feature boundary.

