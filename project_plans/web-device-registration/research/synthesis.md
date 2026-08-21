# Research Synthesis: Web Device Registration

**Decision Required**: How to allow an already-authenticated Stapler Squad user to register a new passkey-enabled device entirely from the web UI, without CLI access.

## Context

The existing system requires running `ssq print-qr-codes` to generate a one-time setup token and QR code. This is fine for initial setup but blocks users who primarily access the app remotely (phone, secondary laptop) from enrolling new devices. The system is single-user, self-hosted, with a self-signed TLS CA.

---

## Options Considered

| Option | Summary | Key Trade-off |
|--------|---------|---------------|
| **A — New `/auth/invite/generate` + InviteManager** | Dedicated authenticated endpoint returns token, QR data URI, and registration URL in one response. Separate `InviteManager` from bootstrap `SetupManager`. | More code isolation; two token systems, but concerns are genuinely different |
| **B — Extend SetupManager with authenticated path** | Reuse existing single-token `SetupManager`; add an authenticated `POST /auth/setup/regenerate` endpoint | Less code, but conflates first-boot bootstrap with ongoing device management; `setup_active` status flag leaks into wrong contexts |
| **C — Client-side token generation** | Browser generates token; no server involvement | Breaks the server-side authentication gate; not viable |

---

## Dominant Trade-off

**Code reuse vs. concern isolation.** Option B maximizes reuse but merges two distinct concepts (unauthenticated bootstrap vs. authenticated multi-device enrollment) into one mechanism. Option A adds ~60 lines of `InviteManager` in exchange for clear separation. Given that `SetupManager`'s `setup_active` flag is already surfaced in `GET /auth/status` and consumed by the UI, conflating bootstrap state with invite state would cause confusing UI behavior. Option A is the better long-term design.

---

## Recommendation

**Choose: Option A — dedicated `InviteManager` + `POST /auth/invite/generate`**

**Because**: The existing `SetupManager` bootstrap concern is deliberately separate from ongoing device management. Merging them forces `setup_active` in `/auth/status` to mean two different things (first-boot vs. invite pending), which would require UI changes anyway. The `InviteManager` adds ~60 lines of Go and reuses all the same primitives (`GenerateQRPNG`, `setup_token` ceremony gate, `time.Now().Add(TTL)`). The overall implementation is ~250–350 lines Go + ~400–500 lines TSX — a modest, bounded scope.

**Accept these costs**: Two token systems to maintain (`SetupManager` for bootstrap, `InviteManager` for web invites). The invite token is in-memory only by default (lost on restart); if disk persistence is desired it must be added explicitly.

**Reject these alternatives**:
- **Option B**: Rejected because `SetupManager.GenerateToFile` writes `setup-token.json` watched by `WatchFile` and surfaced as `setup_active` in status — folding invite generation in would make `setup_active: true` appear during normal multi-device enrollment, confusing users who have not yet set up their first device.
- **Option C**: Rejected because it removes the server-side token validation gate, allowing any URL with a plausible-looking token to initiate a registration ceremony.

---

## Complete Implementation Plan

### Backend changes (all additive)

#### `server/auth/store.go`
- Add fields to `storedCredential`: `DisplayName string`, `CreatedAt time.Time`, `LastUsedAt *time.Time`
- Populate `CreatedAt = time.Now()` in `AddCredential`
- Add `ListCredentials() []storedCredential`
- Add last-credential guard in `RemoveCredential`: return a sentinel `ErrLastCredential` error so callers can decide whether to also revoke all sessions

#### `server/auth/invite.go` (new file, ~60 lines)
```go
type inviteEntry struct {
    Token     string
    ExpiresAt time.Time
}
type InviteManager struct {
    mu      sync.Mutex
    entries map[string]inviteEntry  // keyed by token
    maxSize int                     // default 5
}
func (m *InviteManager) Generate(ttl time.Duration) string   // returns token
func (m *InviteManager) IsValid(token string) bool           // non-consuming
func (m *InviteManager) Consume(token string) bool           // consumes on success
func (m *InviteManager) Cleanup()                            // remove expired entries
```

#### `server/auth/handlers.go`
Add `invites *InviteManager` field to `httpHandlers`. Add `port int` field (passed at `RegisterRoutes` call time for URL construction). Add four new methods:

1. **`POST /auth/invite/generate`** — auth-gated; calls `invites.Generate(15*time.Minute)`; constructs `registration_url` = `https://<primaryDomain>:<port>/login?setup_token=<token>`; calls `GenerateQRPNG(registrationURL)`; base64-encodes PNG; returns JSON `{token, registration_url, qr_png_data_url, expires_at, ttl_seconds}`. Returns HTTP 503 if TLS not enabled.

2. **`GET /auth/credentials`** — auth-gated; calls `store.ListCredentials()`; serializes IDs as `hex.EncodeToString`; returns JSON credential list with label, created_at, last_used_at, sign_count.

3. **`POST /auth/credentials/{id}/revoke`** — auth-gated; hex-decodes `{id}`; calls `store.RemoveCredential`; if `ErrLastCredential`, calls `sessions.RevokeAllSessions()` and returns 200 with `{"ok":true,"last_credential":true}`; otherwise returns `{"ok":true}`.

4. **`GET /auth/invite/status`** — auth-gated; returns whether an active invite exists and its remaining TTL (for UI polling / recovery after page reload).

#### `server/auth/handlers.go` — existing finishRegistration bug fix (MUST)
**Move `setup.Consume(setupToken)` before `wa.FinishRegistration()`** to prevent the concurrent double-registration race. Currently Consume is called after the credential is stored — two devices using one token can both register. Fix: validate-and-consume the token atomically before writing the credential.

The same fix must apply to the invite token: `invites.Consume(token)` before `FinishRegistration`.

#### `server/auth/handlers.go` — CSRF hardening on invite generation
Add `Origin` header verification on `POST /auth/invite/generate`. The existing `SameSite=Strict` cookie is not sufficient per OWASP 2024 — add defense-in-depth by checking `r.Header.Get("Origin")` matches the expected HTTPS origin.

### Frontend changes

#### `web-app/src/app/account/` (new route)
```
account/
  layout.tsx          (auth guard: redirect to /login if !authenticated)
  page.tsx            (AccountPage: orchestrates data fetching)
  account.css.ts
  components/
    CredentialList/
      CredentialList.tsx
      CredentialList.css.ts
    AddDeviceSection/
      AddDeviceSection.tsx   (button + modal trigger)
      AddDeviceModal.tsx     (QR img, copyable URL, TTL countdown, CA cert step)
      AddDeviceModal.css.ts
    RevokeConfirmDialog/
      RevokeConfirmDialog.tsx
      RevokeConfirmDialog.css.ts
```

**`AddDeviceModal`** must include a CA cert installation step as step 0, with platform-specific instructions:
- iOS: "Download CA cert → Settings → General → VPN & Device Management → [cert] → Install. Then: Settings → General → About → Certificate Trust Settings → enable."
- Android: "Download CA cert → Settings → Security → Encryption & credentials → Install a certificate → CA Certificate."
- macOS/Windows: download and double-click the `.pem` file.

This is **not optional** — without CA cert import, Safari/Chrome on the new device will reject the HTTPS connection before the registration page loads.

#### `web-app/src/app/login/page.tsx`
Add: when `authenticated && hasCredentials`, show an "Add another device" link pointing to `/account`. This satisfies the requirement that authenticated users can navigate to device management from the login page.

#### `web-app/src/components/layout/Header.tsx`
Add "Account" nav link (or lock icon) visible only when `authenticated`, pointing to `/account`.

#### `web-app/src/lib/auth/passkey.ts`
Add:
- `generateInvite(): Promise<InviteResponse>`
- `listCredentials(): Promise<Credential[]>`
- `revokeCredential(id: string): Promise<RevokeResult>`

### CA cert QR decision
Include a second QR code for the CA cert download URL (`https://<host>:<port>/auth/ca.pem`) in `AddDeviceModal` — matching what the CLI does. This allows the new device to import the cert by scanning, before visiting the registration URL.

---

## Critical Bugs to Fix During Implementation

1. **Double-registration race** (MUST fix): `finishRegistration` stores the credential before consuming the setup token. Move `Consume` before `FinishRegistration`. Same pattern must apply to invite tokens in `InviteManager`.

2. **Session not invalidated on credential revoke**: When the last passkey is revoked, call `sessions.RevokeAllSessions()`. For non-last revocations, existing sessions from the revoked device remain valid for up to 30 days — acceptable for a single-user tool, but the UI should warn about this.

3. **Redirect after registration**: After `finishRegistration` succeeds, redirect to a clean URL (e.g., `/`) to strip `?setup_token=...` from browser history. This reduces token exposure window.

---

## Open Questions Before Committing

- [ ] One active invite at a time (like `SetupManager`) or map of 5? — affects `InviteManager` design; single-token is simpler and acceptable for a single operator
- [ ] Use `GenerateToFile` for web invites (disk-persisted, survives restart) or in-memory only? — in-memory is fine; 15-minute TTL is short enough that losing the invite on restart is acceptable
- [ ] Credential `DisplayName` source: user-supplied at invite time, or auto-generated "Passkey (2026-04-21)"? — recommend user-supplied label at invite generation time; show it in invite modal and store on registration

---

## Sources

- [`findings-stack.md`](findings-stack.md) — code archaeology of `server/auth/`, what exists and what's missing
- [`findings-architecture.md`](findings-architecture.md) — new endpoint design, React component hierarchy, QR delivery, countdown implementation
- [`findings-features.md`](findings-features.md) — prior art (Tailscale, Vaultwarden, FIDO2 hybrid transport); QR+link UX confirmed as best practice
- [`findings-pitfalls.md`](findings-pitfalls.md) — CSRF, token replay race (found as existing bug), CA cert bootstrapping (iOS steps required), credential orphan on revoke
