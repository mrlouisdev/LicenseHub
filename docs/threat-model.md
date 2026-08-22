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
| Oversized client input/response | Strict bounded JSON decoders, capped HTTP readers and bounded lease claims. |
| Public operational telemetry | Caddy returns 404; internal route requires a constant-time Bearer-token check. |
| Secret or key material at rest | Production requires an independent AES-256-GCM master key. |
| Slow HTTP resource exhaustion | Header/read/write/idle timeouts, header cap and edge body limit. |
| Container breakout/blast radius | Read-only app filesystems, dropped capabilities, resource limits and private networks. |
| Backup tampering | Verify a SHA-256 manifest before restore. |
| First-owner takeover during bootstrap | Keep Caddy stopped; initialize through the container-local interface before opening 80/443. |
| Failed or partial database restore | Archive preflight, safety dump and single-transaction restore with automatic rollback. |

## Residual risks

Client-side checks can be patched by an attacker controlling the machine. Put
entitlement checks at high-value feature boundaries, not only at startup. Shorter
leases reduce revocation delay but increase VPS availability dependence. Device
identity may change after repair, so an audited reset workflow remains necessary.

## Client verification order

1. Split the lease into exactly two non-empty base64url segments.
2. Reject oversized segments and decode only the bounded untrusted `kid`.
3. Validate `kid` syntax and select a locally pinned public key.
4. Verify Ed25519 over the first encoded segment exactly.
5. Decode bounded JSON and validate `protocol/lease.schema.json`.
6. Require supported `version`, expected `product_id` and local `device_id`.
7. Require `iat <= trusted_now + allowed_skew` and `trusted_now < exp`.
8. Check entitlement at the protected feature boundary.
