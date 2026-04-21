# Findings: Stack

**Feature**: Web Device Registration (invite generation, credential management, /account page)
**Date**: 2026-04-21
**Scope**: Code archaeology of `server/auth/` and related frontend files

---

## Summary

The existing auth package covers about 70% of what the feature needs. `SetupManager` already generates single-use time-boxed tokens, `CredentialStore` already has list and delete primitives, `GenerateQRPNG` is ready to reuse, and `beginRegistration`/`finishRegistration` already accept either a valid auth session or a valid setup token — meaning an authenticated user can already trigger a new-device registration ceremony without any backend changes to those ceremony handlers.

What is entirely absent:

1. An HTTP endpoint that generates a new setup token on demand (authenticated POST).
2. An HTTP endpoint that returns all registered credentials (authenticated GET).
3. An HTTP endpoint that revokes a credential by ID (authenticated DELETE).
4. An HTTP endpoint that serves a QR code PNG for a given URL (authenticated GET).
5. A `/account` page and route constant on the frontend.
6. A header nav link to `/account`.
7. Frontend components to display the invite (QR + link + countdown) and credential list.

The gap is entirely additive — no existing handler logic needs to change.

---

## Options Surveyed

### Option A — New `/auth/invite` endpoint wrapping SetupManager (recommended)

Add `POST /auth/invite` that validates the caller is authenticated (`isAuthorised`), calls `SetupManager.Init()` to generate a new in-memory token (reusing the existing 1-hour TTL and single-token model), and returns `{ token, expires_at, setup_url, ca_url }` as JSON.

Add `GET /auth/invite/qr?url=<encoded>` that proxies `GenerateQRPNG` and serves `image/png`.

Add `GET /auth/credentials` and `DELETE /auth/credentials/{id}` delegating to `CredentialStore.GetCredentials()` and `CredentialStore.RemoveCredential()`.

All four new routes registered inside the existing `RegisterRoutes` call — no new mux or server needed.

### Option B — Extend `/auth/register/begin` to embed token generation

When an already-authenticated client calls `beginRegistration`, the server generates and includes a fresh setup token in the response so the client can share it. The ceremony and invite flow are merged into one round-trip.

This couples token lifecycle to ceremony lifetime: a failed or abandoned ceremony leaves a valid but orphaned token. It also overloads an endpoint that currently serves a single clear purpose, and requires changing the existing `passkey.ts` `registerPasskey` client.

### Option C — Separate token system independent of SetupManager

Build a new `InviteManager` struct with its own storage, multiple concurrent tokens, expiry model, and file format. Provides maximum isolation but duplicates the core logic already proven in `SetupManager`.

---

## Trade-off Matrix

| Axis | Option A | Option B | Option C |
|---|---|---|---|
| Code reuse | High — all primitives reused as-is | High — no new endpoint | Low — duplicates SetupManager logic |
| Security isolation | Good — invite gated behind `isAuthorised` | Weak — token tied to ceremony lifetime | Good — independent system |
| Complexity | Low (~120 lines, 4 handlers) | Moderate — response shape change, client update needed | High — parallel token machinery |
| Consistency with existing patterns | Best — follows RegisterRoutes/isAuthorised exactly | Moderate — overloads existing endpoint | Poor — diverges entirely |

---

## Risk and Failure Modes

**Single-token model**: `SetupManager` holds exactly one token in memory. Calling `Init()` or `GenerateToFile()` replaces the previous token immediately. If an authenticated user generates an invite and then the CLI runs `print-qr-codes`, the web-generated token is silently invalidated (and vice versa). Clicking "regenerate" in the UI kills the previous invite. Acceptable for a single-operator system but must be communicated in the UI. [NEEDS_VERIFICATION: confirm one active invite at a time is acceptable.]

**Token lost on server restart**: `Init()` stores the token in memory only. If the server restarts within the invite's 1-hour window, the invite link breaks. `GenerateToFile()` writes to `setup-token.json` and is picked up by `WatchFile`, surviving restarts — consistent with the CLI's `print-qr-codes` behavior. The web endpoint should use `GenerateToFile` unless ephemeral invites are deliberately preferred. [NEEDS_VERIFICATION]

**Revoking the last credential**: `RemoveCredential` has no guard against deleting all credentials. After revocation the server reaches `HasCredentials() == false`, making the next registration unauthenticated. The endpoint should refuse to delete the last credential, or at minimum invalidate all auth sessions (`RevokeAllSessions`) when the last one is removed so no stale session cookie grants access to an ownerless server.

**CSRF on invite generation**: `POST /auth/invite` is called from the browser. The `SameSite=Strict` cookie already provides baseline CSRF protection — consistent with how `logout` and `register/begin` are handled today. No separate CSRF token is needed.

**Credential ID encoding**: `CredentialStore` uses `[]byte` for IDs and logs them as `%x` (hex). The WebAuthn browser API delivers credential IDs as base64url strings. The list/revoke HTTP API should use base64url to match browser conventions and avoid double-encoding errors. [NEEDS_VERIFICATION: explicit decision needed before implementation.]

**`storedCredential` has no display name or creation timestamp**: The requirements call for "display name / creation date" in the credential list. Neither field exists in the current struct. Adding them is a forward-compatible JSON change (zero values for existing credentials). This is the only schema migration required.

---

## Migration and Adoption Cost

**Backend (`server/auth/`)**:
- `store.go`: Add `DisplayName string` and `CreatedAt time.Time` to `storedCredential`. Populate `CreatedAt = time.Now()` in `AddCredential`. Add `ListCredentials()` method returning internal structs (so display metadata is accessible). Add last-credential guard in `RemoveCredential`.
- `handlers.go`: Add four new handler methods; register them in `RegisterRoutes`. No changes to existing handlers.
- `setup.go`, `qrcode.go`, `webauthn.go`, `session.go`: No changes.

**Frontend**:
- `web-app/src/lib/routes.ts`: Add `account: "/account"`.
- `web-app/src/lib/auth/passkey.ts`: Add `generateInvite()`, `listCredentials()`, `revokeCredential(id)`.
- `web-app/src/app/account/page.tsx`: New page (does not exist today).
- `web-app/src/components/layout/Header.tsx`: Add "Account" nav link, gated on `authenticated` from `useAuth()`.
- All new component styles in `.css.ts` per ADR-009.

**Estimated new code**: ~250–350 lines Go, ~400–500 lines TSX/TS.

**Breaking changes**: None. All changes are additive.

---

## Operational Concerns

**Single-operator model**: All passkeys belong to `ownerUserID = "stapler-squad-owner"` (see `user.go`). No user namespacing needed. The account page is always "my devices."

**Localhost bypass**: `isLocalhostRequest` grants `authenticated: true` unconditionally for loopback clients. The new handlers must gate on `isAuthorised` (not on the status response), which independently checks the session token — this is already the pattern used by all protected handlers and is correct.

**Auth middleware**: The `/account` route is a browser-navigable page. The middleware in `server/middleware/auth.go` redirects unauthenticated browser requests to `/login` by default. No exemption list change is needed.

**Port availability in `httpHandlers`**: `generateInvite` needs to construct `setup_url` and `ca_url` including the port number. Currently `httpHandlers` stores `primaryDomain` but not the port. The port must either be passed at construction time or derived from the request's `Host` header. [NEEDS_VERIFICATION: confirm approach before implementation.]

**CA cert QR on invite**: The `print-qr-codes` CLI generates two QR codes: CA cert URL and registration URL. The invite endpoint should return both so the new device can install the self-signed cert. `caPath` and `primaryDomain` are already on `httpHandlers`; only the port needs to be added.

**`RevokeAllSessions` after last-credential revoke**: `SessionManager.RevokeAllSessions()` exists and is already the right primitive. If the user removes their last passkey, calling this ensures no stale session cookie grants access to a now-ownerless server.

---

## Prior Art and Lessons Learned

**`print-qr-codes` CLI** (main.go:550–618): Uses `GenerateToFile` (file-backed, survives restart), generates `ca_url` and `setup_url` for each detected hostname, prints two QR codes. The web invite endpoint should mirror this two-QR pattern.

**Bootstrap flow** (main.go:880–907): On first start with no credentials, uses `Init()` (memory-only) and prints QRs to stderr. The CLI uses `GenerateToFile` for invites (persistence guarantee) but `Init()` for the automatic bootstrap. The web endpoint should match the CLI's `GenerateToFile` approach since persistence across restarts is desirable for a user-facing flow.

**`isAuthorised` in handlers.go:284–296**: Already accepts session cookie/Bearer token or setup token query param, and is a private method on `httpHandlers`. New handlers must be methods on `httpHandlers` (already the pattern) to access it.

**`beginRegistration`/`finishRegistration` auth gate** (handlers.go:108, 138): `if h.store.HasCredentials() && !h.isAuthorised(r)` — this already allows an authenticated user to register additional devices by presenting a valid session cookie. The frontend only needs to call these existing endpoints with `credentials: "include"` and no `setup_token` param.

**`storedCredential` struct accessibility**: The struct is unexported (lowercase) but its fields are JSON-serialized. Adding fields is safe and backward-compatible.

---

## Open Questions

1. **`Init()` vs `GenerateToFile()` for web invites**: Should the web `POST /auth/invite` call `Init()` (memory-only, simpler, lost on restart) or `GenerateToFile()` (disk-backed, survives restart, consistent with CLI)? Recommendation: `GenerateToFile()`. [NEEDS_VERIFICATION]

2. **One active invite at a time**: Acceptable that generating a new web invite invalidates the CLI-generated invite and vice versa? [NEEDS_VERIFICATION]

3. **CA cert QR on invite page**: Show both CA cert QR and registration QR (matching CLI), or assume visiting device already has the cert? (Requirements list this as an unresolved open question.) [NEEDS_VERIFICATION]

4. **Credential ID encoding in HTTP API**: Hex (matching existing Go log output) or base64url (matching WebAuthn browser API)? Recommendation: base64url. [NEEDS_VERIFICATION]

5. **Last-credential guard behavior**: Refuse the `DELETE` if it would leave zero credentials? Or allow it but forcibly revoke all sessions? Or both? [NEEDS_VERIFICATION]

6. **Credential display name source**: Options: (a) accept `display_name` in `POST /auth/register/begin` body and store it; (b) derive from AAGUID (requires lookup table); (c) show generic "Passkey (registered YYYY-MM-DD)" using only `CreatedAt`. Option (c) requires only one new field in `storedCredential`. [NEEDS_VERIFICATION]

7. **Port in `httpHandlers`**: Confirm whether `primaryDomain` already includes the port or whether the port needs to be passed separately at `RegisterRoutes` call time. (Code inspection shows `primaryDomain` is set to the hostname only, e.g. `"onyx.staplerhome.internal"`, with no port.) [NEEDS_VERIFICATION]

---

## Recommendation

Implement **Option A**. The concrete changes are:

**`server/auth/store.go`**
- Add `DisplayName string` and `CreatedAt time.Time` to `storedCredential` (JSON `omitempty`).
- Populate `CreatedAt = time.Now()` in `AddCredential`.
- Add `ListCredentials() []storedCredential` method.
- Add guard in `RemoveCredential`: if removing the last credential, return a sentinel error so callers can decide (revoke sessions, show UI warning).

**`server/auth/handlers.go`**
- Add to `httpHandlers`: a `port int` field (populated via updated `RegisterRoutes` signature).
- Add `generateInvite` (POST `/auth/invite`): auth-gated, calls `setup.GenerateToFile(path)`, returns `{token, expires_at, setup_url, ca_url}`.
- Add `inviteQR` (GET `/auth/invite/qr?url=...`): auth-gated, calls `auth.GenerateQRPNG(url)`, serves `image/png`.
- Add `listCredentials` (GET `/auth/credentials`): auth-gated, calls `store.ListCredentials()`, serializes IDs as base64url.
- Add `revokeCredential` (DELETE `/auth/credentials/{id}`): auth-gated, decodes base64url ID, calls `store.RemoveCredential(id)`, calls `sessions.RevokeAllSessions()` if the last credential was removed.

**Frontend**
- `routes.ts`: add `account: "/account"`.
- `passkey.ts`: add `generateInvite()`, `listCredentials()`, `revokeCredential(id: string)`.
- New `web-app/src/app/account/page.tsx`: two sections — "Add a device" (QR + URL + countdown + regenerate) and "Your passkeys" (list with revoke).
- `Header.tsx`: add "Account" nav link after "Settings", visible only when `authenticated`.
- All styles in `.css.ts` per ADR-009.

**No changes needed** to `setup.go`, `qrcode.go`, `webauthn.go`, `session.go`, `user.go`, or any existing handler.
