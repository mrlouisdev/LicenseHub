# LicenseHub Console

React + TypeScript administrative console for LicenseHub, delivered as both a web build and a Tauri Windows application.

## Run locally

```powershell
npm install
npm run dev
```

## Runtime connection

The first launch opens **Connection settings**. Enter the public origin of a LicenseHub VPS, verify the server version, then sign in with the administrator email and one-time code. HTTPS is required except for localhost development endpoints.

Production is selected at runtime; it is not baked into the build. Only the validated server origin is stored locally. Demo mode is an explicit first-run choice and uses only the in-memory mock adapter.

Pages depend only on `ConsoleApi` in `src/types.ts`. `src/api/index.ts` switches between disconnected, production HTTP, and explicit Demo modes.

The Windows application uses a native HTTP bridge with a cookie jar that exists only for the running process. Browser builds use credentialed `fetch`. Neither transport writes authentication material to web storage.

## Server route mapping

| Console operation | Server route |
|---|---|
| Products | `GET/POST /api/v1/admin/products` |
| Default device policy | `GET/POST /api/v1/admin/plans` (`max_activations`) |
| Licenses | `GET/POST /api/v1/admin/licenses` |
| Revoke license | `POST /api/v1/admin/licenses/:id/revoke` |
| Device discovery | license list then `GET /api/v1/admin/licenses/:id` |
| Reset device | `DELETE /api/v1/admin/activations/:id` |
| Signing keys | `GET /api/v1/admin/products/:id/signing-keys` |
| Rotate signing key | `POST /api/v1/admin/products/:id/signing-key/rotate` |
| Audit events | `GET /api/v1/admin/audit-logs` |
| Dashboard totals | `GET /api/v1/admin/stats` |

Backend gaps return `UNSUPPORTED_OPERATION`: integration-kit export and backup management have no matching server route. The 72-hour lease is server-wide configuration, not a per-product field. Entitlements are plan-scoped; requested features are recorded in license notes instead of mutating a shared plan.

## Windows desktop

The same frontend is wrapped by Tauri under `src-tauri/`:

```powershell
npm run desktop:dev
npm run desktop:build
```

No build-time VPS URL is required. The target server can be changed from **Connection**; logout clears the saved origin and the in-memory session.

## Verification

```powershell
npm run lint
npm run test
npm run build
```
