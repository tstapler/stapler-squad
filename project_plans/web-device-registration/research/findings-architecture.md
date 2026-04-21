# Findings: Architecture

## Summary

The existing auth subsystem (`server/auth/`) already implements all the WebAuthn primitives needed for this feature: ceremony management (`session.go`), credential persistence (`store.go`), QR PNG generation (`qrcode.go`), and a one-time token pattern (`setup.go`). The `beginRegistration`/`finishRegistration` handlers already accept either an auth session cookie **or** a `setup_token` query param as the gate for adding a new credential. The gap is: there is no authenticated server-side path to *generate* a new invite token on demand and return it alongside a QR PNG URL, no `/account` page in the Next.js app, no per-credential metadata (label, registered-at, last-used) that would make a credential list actionable, and no revoke-by-ID endpoint.

The recommended approach is **Option A**: a minimal new `/auth/invite/generate` endpoint that is authenticated, creates an in-memory invite token (structurally identical to a setup token but issued to an already-authenticated user), and returns the registration URL plus a pre-rendered QR PNG as a base64 data URI. This avoids touching the existing `SetupManager` bootstrap concern and keeps the two flows separate.

---

## Options Surveyed

### Option A — New `/auth/invite/generate` endpoint (authenticated, returns token + inline QR)

A new `InviteManager` (or a second token slot in `SetupManager`) stores a short-lived invite token. A `POST /auth/invite/generate` endpoint requires a valid auth session, generates the token, builds the registration URL (`https://<host>/login?setup_token=<token>`), and returns JSON containing the token, expiry timestamp, registration URL, and a base64-encoded QR PNG. The new device visits the URL, which loads `login/page.tsx` (already handles `setup_token` in the query string), and completes the existing begin/finish registration ceremony. No changes to the ceremony handlers are required.

### Option B — Extend `SetupManager` with an authenticated path

Add an `InviteActive` boolean to `SetupManager` that, when true, means the token was issued by an authenticated user rather than at bootstrap. The `isAuthorised` gate in the ceremony handlers already accepts `setup_token`, so nothing there changes. `POST /auth/setup/regenerate` (authenticated) calls `setup.Init()` to generate a fresh token and returns it.

The problem: `SetupManager` was designed for the bootstrap case — it holds exactly one token at a time, writes it to `setup-token.json`, is watched by `WatchFile`, and its `IsActive` state is surfaced in `GET /auth/status` via `setup_active` (consumed by the frontend to show a banner). Folding invite generation into this mechanism couples two distinct concerns: first-time bootstrap vs. adding a second device to an already-secured system.

### Option C — Client-side token generation (no new endpoint)

The browser generates a random UUID, embeds it in a URL, and displays a QR code. The server has no record of this token, so the registration ceremony gate (`isAuthorised`) cannot accept it without also weakening the check to "accept any plausible-looking token," which removes the TOFU guarantee. Not viable.

---

## Trade-off Matrix

| Axis | Option A (new endpoint) | Option B (extend SetupManager) | Option C (client-side) |
|---|---|---|---|
| **Auth requirement** | Full: requires valid session cookie | Full: same | None: no server gate |
| **Alignment with existing code** | Medium: adds `InviteManager`, new route, new token map | High: reuses `SetupManager` but conflates two concerns | Low: breaks auth model |
| **Frontend complexity** | Low: login/page.tsx already handles `setup_token`; only need to add `/account` page | Low: same | High: must also implement QR rendering in-browser with no server validation |
| **Security** | Strong: invite token is single-use, server-validated, time-bounded | Strong: same, but only one active token at a time (cannot queue two devices) | Broken: no server-side gate |

---

## Risk and Failure Modes

**Token replay after partial failure**: The existing `finishRegistration` handler calls `setup.Consume(token)` only on success. An invite token must follow the same pattern — consumed on `finishRegistration` success, not on `beginRegistration`. The `InviteManager.IsValid` (non-consuming) / `Consume` pattern from `SetupManager` should be reproduced exactly.

**Single-token constraint**: If the user clicks "Add New Device" twice in rapid succession, the second call overwrites the first invite token. The new device that scanned the first QR will get a 401 on `beginRegistration`. Mitigation: `InviteManager` can hold a small map (up to 5 tokens) keyed by a random ID, each with its own expiry. The response includes the token so the client can track which QR belongs to which outstanding invite.

**Expiry during ceremony**: The WebAuthn ceremony itself has a 5-minute window (`ceremonySessionTTL`). The invite token TTL should be at least 15 minutes to give the user time to scan and complete the ceremony without racing against both clocks.

**No credential labels in `storedCredential`**: The current `store.go` struct has no `Label`, `CreatedAt`, or `LastUsedAt` fields. Listing credentials on `/account` without at least `CreatedAt` is unhelpful. Adding these fields to `storedCredential` and populating `CreatedAt` in `AddCredential` is a prerequisite. `LastUsedAt` can be set in `UpdateCredential` (called on every successful login).

**Revoke-by-ID**: `RemoveCredential(credID []byte)` exists in `store.go`. A `POST /auth/credentials/{id}/revoke` endpoint that requires an auth session is the only missing piece. The credential ID bytes should be hex-encoded in the URL to stay URL-safe.

**Race condition on revoke-last-credential**: If the user revokes the only registered credential while authenticated, the server must not immediately require authentication to reach any page, but subsequent requests that require auth will fail. The `/account` page should warn "Revoking your last passkey will lock you out of remote access" and require a confirmation dialog.

**HTTPS requirement**: WebAuthn credential creation requires a secure context. The invite URL must be an HTTPS URL. `server/auth/handlers.go` already uses `isLocalhostRequest` to bypass auth for loopback; the invite URL generation must use the HTTPS hostname from `serverInfo.https_url`, not `localhost`.

---

## Migration and Adoption Cost

**Backend**:
- Add `Label string`, `CreatedAt time.Time`, `LastUsedAt *time.Time` to `storedCredential` in `store.go`. Fields are additive and backward-compatible with existing `passkeys.json` files (missing fields zero-value on load).
- Add `InviteManager` (~60 lines, mirrors `SetupManager` but in-memory only, map-based for multiple concurrent invites).
- Add 3 new HTTP routes: `POST /auth/invite/generate`, `GET /auth/credentials`, `POST /auth/credentials/{id}/revoke`.
- Register routes in `RegisterRoutes`.

**Frontend**:
- New Next.js page: `web-app/src/app/account/page.tsx` + `account.css.ts`.
- New Next.js layout: `web-app/src/app/account/layout.tsx` (reuse existing pattern from `settings/layout.tsx`).
- 3 new React components: `CredentialList`, `AddDeviceModal` (QR + URL + countdown), `RevokeConfirmDialog`.
- Minimal change to `login/page.tsx`: add an "Add Another Device" link/button visible when `authenticated && hasCredentials`, pointing to `/account`.
- No changes to `passkey.ts` — `registerPasskey(setupToken)` already handles the invite flow.

**Total estimated new code**: ~200 lines Go, ~300 lines TSX/CSS.

---

## Operational Concerns

**Invite token lifetime**: 15-minute default (configurable via constant). Longer TTLs increase the window for a stolen URL to be used by an attacker with physical or network access. 15 minutes is a reasonable balance for LAN/Tailscale use.

**Credential count growth**: This is a single-user app. A list of 10 credentials renders trivially; no pagination needed.

**No email/push notification**: Stapler Squad has no notification subsystem. The operator must be present at the `/account` page while the new device scans. This is the expected UX for a self-hosted tool.

**Audit logging**: New device registrations and revocations should emit `log.InfoLog.Printf` entries (already the pattern in `webauthn.go`). No structured audit log is needed at this stage.

---

## Prior Art and Lessons Learned

**Existing `SetupManager` pattern**: The decision to use a non-consuming `IsValid` check in `beginRegistration` and a consuming `Consume` in `finishRegistration` (on success only) prevents the token from being exhausted by a failed or partial ceremony. The `InviteManager` must preserve this invariant.

**`config/page.tsx` has a "Register New Passkey" button**: This already calls `registerPasskey()` without a setup token, which works because `beginRegistration` allows re-registration from an authenticated session. The `/account` page should replace this button in the Config page, or at minimum link to it, to avoid two entry points for the same action.

**`setup_token` in URL query string**: The existing `login/page.tsx` already reads `searchParams.get("setup_token")` and passes it through the `registerPasskey(setupToken)` call. The invite URL format `https://<host>/login?setup_token=<token>` requires zero changes to the login page for the new-device flow. [TRAINING_ONLY — verify that Next.js `useSearchParams` works correctly in this context under the app router]

**WebAuthn RPID is derived from request Host**: `webauthnForHost` in `webauthn.go` selects the WebAuthn instance by matching the request `Host` header against configured RPIDs. The invite URL must use a hostname that matches a configured RPID; otherwise `beginRegistration` will return a "no valid rpID found" error on the new device. This means the HTTPS URL used in the QR code must be the same origin that the server is configured to accept as a valid WebAuthn RPID origin.

---

## Open Questions

1. Should `InviteManager` hold multiple concurrent invites (map) or a single token (like `SetupManager`)? A map of up to 5 is recommended; single-token means a second "Add Device" click silently invalidates the first QR.
2. Should the `/account` page replace or supplement the passkey section currently in `config/page.tsx`? Recommendation: move all passkey management to `/account` and remove it from `config/page.tsx` to avoid duplication.
3. Are credential labels user-supplied, auto-generated from User-Agent/platform, or both? User-Agent sniffing is fragile; a short text input at invite-generation time (e.g., "iPhone 15") is more reliable and associates the label with the invite rather than requiring the new device to supply it post-registration.
4. Should `LastUsedAt` be tracked? It requires `UpdateCredential` to set a timestamp on every login, adding a disk write per auth. For a single-user tool this is acceptable and aids security review ("this device has not been used in 90 days").
5. What happens if the server has no `https_url` (HTTP-only mode)? WebAuthn registration will fail on non-localhost origins. The "Add New Device" button should be disabled with an explanatory message when `serverInfo.tls_enabled` is false.

---

## Recommendation

Use **Option A** — a dedicated `InviteManager` and `POST /auth/invite/generate` endpoint.

### New HTTP Endpoints

**1. `POST /auth/invite/generate`**
- Auth: requires valid session cookie (`isAuthorised` check)
- Request body: `{ "label": "iPhone 15" }` (optional, defaults to empty string)
- Response:
  ```json
  {
    "invite_id":        "a3f9...",
    "token":            "b7c2...",
    "registration_url": "https://onyx.staplerhome.internal:8444/login?setup_token=b7c2...",
    "qr_png_data_url":  "data:image/png;base64,iVBOR...",
    "expires_at":       "2026-04-21T14:35:00Z",
    "ttl_seconds":      900
  }
  ```
- The server generates a 32-byte random hex token, stores it in `InviteManager` with a 15-minute expiry, calls `GenerateQRPNG(registrationURL)`, base64-encodes the PNG bytes, and returns everything in one response. No separate QR endpoint is needed.
- The `registration_url` is constructed from the server's configured HTTPS origin (must match a WebAuthn RPID). If no HTTPS URL is configured, return HTTP 503 with `{ "error": "TLS not enabled" }`.

**2. `GET /auth/credentials`**
- Auth: requires valid session cookie
- Response:
  ```json
  {
    "credentials": [
      {
        "id":           "a1b2c3...",
        "label":        "MacBook Pro",
        "created_at":   "2026-01-15T10:00:00Z",
        "last_used_at": "2026-04-20T09:12:00Z",
        "sign_count":   42
      }
    ]
  }
  ```
- Requires adding `Label string`, `CreatedAt time.Time`, `LastUsedAt *time.Time` to `storedCredential` in `store.go`.
- Credential `id` in the response is `hex.EncodeToString(sc.ID)`.

**3. `POST /auth/credentials/{id}/revoke`**
- Auth: requires valid session cookie
- Path param `id`: hex-encoded credential ID
- Request body: empty
- Response: `{ "ok": true }`
- Calls `store.RemoveCredential(credID)` then, if the revoked credential was the last one, calls `sessions.RevokeAllSessions()` (force re-auth of all clients).
- Returns 400 if `id` is malformed hex, 404 if credential not found.
- The route pattern `/auth/credentials/{id}/revoke` requires Go 1.22+ `http.ServeMux` path parameters. [TRAINING_ONLY — verify Go version in `go.mod`]

### React Component Hierarchy for `/account`

```
web-app/src/app/account/
  layout.tsx                  (auth guard: redirect to /login if !authenticated)
  page.tsx                    (AccountPage — orchestrates data fetching)
  account.css.ts
  components/
    CredentialList.tsx         (renders list of credentials with add/revoke actions)
    CredentialRow.tsx          (single credential: label, dates, sign count, revoke button)
    AddDeviceSection.tsx       (contains "Add New Device" button and modal trigger)
    AddDeviceModal.tsx         (QR image, copyable URL, countdown timer, dismiss)
    RevokeConfirmDialog.tsx    (confirmation before deleting, warns on last credential)
```

`AccountPage` fetches `GET /auth/credentials` on mount and re-fetches after any revoke or after `AddDeviceModal` is dismissed (in case the new device completed registration). The auth guard in `layout.tsx` mirrors the redirect logic already in `login/page.tsx`.

### QR PNG Delivery

Inline data URI returned directly from `POST /auth/invite/generate`. Rationale: avoids a second round-trip, no need to store the PNG server-side, the PNG is ~1–3 KB base64 which is negligible in JSON. A separate `/auth/invite/{id}/qr.png` endpoint would add complexity with no benefit in this single-page modal context.

### Expiry Countdown Implementation

Use the `ttl_seconds` value from the generate response (not polling). On mount of `AddDeviceModal`, start a `setInterval` that decrements a local state counter every second. When the counter reaches 0, show an "Invite expired — generate a new one" message and disable the copy button. This avoids polling and is accurate within plus or minus one second. No server-side push is needed.

```tsx
const [secondsLeft, setSecondsLeft] = useState(data.ttl_seconds);
useEffect(() => {
  if (secondsLeft <= 0) return;
  const id = setInterval(() =>
    setSecondsLeft(s => {
      if (s <= 1) { clearInterval(id); return 0; }
      return s - 1;
    }), 1000);
  return () => clearInterval(id);
}, []);
```

### Changes to `login/page.tsx`

Minimal: add a visible "Add another device" link when `authenticated && hasCredentials`, pointing to `/account`. This gives an easy re-entry point without duplicating any logic. The existing `setup_token` flow in `LoginContent` already handles the new device's side of the ceremony — no code changes are needed to the ceremony logic in `login/page.tsx`.

---

## Pending Web Searches

1. `site:pkg.go.dev go 1.22 http ServeMux path parameters` — confirm Go version in `go.mod` supports `{id}` path parameter syntax
2. `@simplewebauthn/browser startRegistration optionsJSON` — confirm the exact field name accepted by the installed version (currently the code uses `optionsJSON: options.publicKey`)
3. `go-webauthn webauthn UpdateCredential sign count hook` — confirm `UpdateCredential` is the right place to update `LastUsedAt` or if a separate post-login callback exists
4. `WebAuthn secure context requirement same-origin QR scan` — confirm that scanning a QR on a mobile device and completing WebAuthn at the same HTTPS origin as the desktop session satisfies all secure context requirements

---

## Web Search Results

### Go 1.22 ServeMux path parameters (query 1)
- Confirmed: Go 1.22 `net/http.ServeMux` natively supports `{id}` path parameter syntax.
- Extract with `r.PathValue("id")`. Wildcards must be full path segments (preceded by `/`).
- Pattern `DELETE /auth/credentials/{id}` is valid Go 1.22 syntax.
- Source: https://go.dev/blog/routing-enhancements

### go-webauthn UpdateCredential (query 3)
- The library provides `WebAuthn.ValidateDiscoverableLogin` and `UpdateCredential` is the application's responsibility to call after a successful authentication. There is no post-login callback hook in the library — the handler must explicitly call `store.UpdateCredential(cred)` after `FinishLogin`.
- For `LastUsedAt`, the application must set it in the stored credential struct before calling `store.UpdateCredential`.
- Source: https://pkg.go.dev/github.com/go-webauthn/webauthn/webauthn
