# LicenseHub client error codes

Client code branches on `error.code`, never on the human-readable message.

| HTTP | Code | Client action |
|---:|---|---|
| 400 | `BAD_REQUEST` | Stop retrying; log a redacted validation error. |
| 403 | `LICENSE_EXPIRED` | Disable gated features and show renewal UI. |
| 403 | `LICENSE_CANCELED` | Disable gated features after the paid period. |
| 403 | `LICENSE_SUSPENDED` | Disable gated features and direct the user to support. |
| 403 | `LICENSE_REVOKED` | Delete the cached lease and disable gated features. |
| 404 | `LICENSE_NOT_FOUND` | Treat as unavailable without revealing lifecycle detail. |
| 404 | `FEATURE_NOT_AVAILABLE` | Product type does not support device activation. |
| 409 | `ACTIVATION_LIMIT` | Show reset-device flow; do not loop retries. |
| 409 | `IDEMPOTENCY_CONFLICT` | Use a new key only for a genuinely new operation. |
| 429 | `LOCKED_OUT` | Honor retry delay; stop automatic activation attempts. |
| 429 | `RATE_LIMITED` | Use jittered exponential backoff. |
| 500 | `INTERNAL_ERROR` | Keep a valid cached lease and retry with bounded backoff. |

Public verification collapses wrong-product, revoked, suspended, expired and
unknown licenses into `LICENSE_NOT_FOUND` where possible. Detailed lifecycle
status belongs in authenticated portal/admin APIs.

