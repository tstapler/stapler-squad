# Implementation Plan: Web Device Registration

**Status**: Ready for implementation
**Branch**: `stapler-squad-qr-code-in-https`
**ADRs**: `project_plans/web-device-registration/decisions/`
**Requirements**: `project_plans/web-device-registration/requirements.md`

---

## Summary

Allow an authenticated Stapler Squad web client to register additional passkey-enabled devices from the browser, without running any CLI command. An authenticated user generates a one-time QR-code invite from a new `/account` page. The new device scans the QR, imports the CA cert, and completes the existing WebAuthn registration ceremony via `/login?setup_token=<token>`. The same page lists all registered passkeys and allows revocation.

All backend changes are additive. No existing handler logic changes except the pre-existing race-condition bug fix in `finishRegistration`.

---

## Architecture Decisions (ADRs)

| ADR | Decision |
|-----|----------|
| ADR-001 | Separate `InviteManager` from `SetupManager` — bootstrap and ongoing enrollment are distinct concerns |
| ADR-002 | Inline QR PNG as base64 data URI in generate response — no separate QR endpoint |
| ADR-003 | Client-side TTL countdown from `ttl_seconds` — no server polling |
| ADR-004 | CA cert as mandatory Step 0 in modal — two QR codes matching CLI behavior |
| ADR-005 | `Origin` header check on invite generation — defense-in-depth beyond `SameSite=Strict` |

---

## Dependency Graph

```
Epic 1: Bug Fix (no dependencies — do first)
    └─ Task 1.1: Fix finishRegistration race condition

Epic 2: Backend Store Extensions
    └─ Task 2.1: Add DisplayName, CreatedAt, LastUsedAt to storedCredential
    └─ Task 2.2: Add ListCredentials() + last-credential guard to CredentialStore

Epic 3: InviteManager (depends on Epic 2)
    └─ Task 3.1: Implement server/auth/invite.go

Epic 4: New HTTP Endpoints (depends on Epics 2 + 3)
    └─ Task 4.1: POST /auth/invite/generate
    └─ Task 4.2: GET /auth/credentials
    └─ Task 4.3: POST /auth/credentials/{id}/revoke
    └─ Task 4.4: Update RegisterRoutes + isAuthorised + httpHandlers struct

Epic 5: Frontend API Client (depends on Epic 4)
    └─ Task 5.1: Add generateInvite, listCredentials, revokeCredential to passkey.ts
    └─ Task 5.2: Add routes.account to routes.ts

Epic 6: /account Page (depends on Epic 5)
    └─ Task 6.1: account/layout.tsx (auth guard)
    └─ Task 6.2: CredentialList component
    └─ Task 6.3: AddDeviceModal component (QR + countdown + CA cert steps)
    └─ Task 6.4: RevokeConfirmDialog component
    └─ Task 6.5: account/page.tsx (orchestration)

Epic 7: Navigation + Login Entry Point (depends on Epics 5 + 6)
    └─ Task 7.1: Header.tsx — add Account nav link
    └─ Task 7.2: login/page.tsx — add "Add another device" link
```

**Critical path**: 1.1 → 2.1 → 2.2 → 3.1 → 4.1-4.4 → 5.1-5.2 → 6.1-6.5 → 7.1-7.2

---

## Epic 1: Pre-existing Bug Fix

### Task 1.1 — Fix `finishRegistration` race condition

**Files**: `server/auth/handlers.go`
**Complexity**: Low (surgical change, high importance)

**Problem**: In `finishRegistration`, `h.setup.Consume(setupToken)` is called AFTER `h.wa.FinishRegistration(ceremonyKey, r)`. `FinishRegistration` calls `store.AddCredential` internally. Two devices that both called `beginRegistration` with the same invite token will both have valid ceremony keys. The first device to call `finishRegistration` consumes the token; the second device's `finishRegistration` runs `FinishRegistration` (which adds the credential) and then calls `Consume` which returns false — but the credential is already persisted and the handler still returns `{"ok": true}`.

**Fix**: Move both token consumption checks to before `FinishRegistration`. If consumption fails, return HTTP 401 before touching the credential store.

```go
// finishRegistration — corrected token consumption order
func (h *httpHandlers) finishRegistration(w http.ResponseWriter, r *http.Request) {
    // ... method + wa nil checks ...

    if h.store.HasCredentials() && !h.isAuthorised(r) {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }

    ceremonyKey := r.URL.Query().Get("ceremony_key")
    if ceremonyKey == "" {
        http.Error(w, "missing ceremony_key", http.StatusBadRequest)
        return
    }

    // Consume token BEFORE FinishRegistration to prevent concurrent double-registration.
    // isAuthorised uses IsValid (non-consuming); we must consume atomically here.
    setupToken := r.URL.Query().Get("setup_token")
    if setupToken != "" {
        // Try setup token first, then invite token.
        if !h.setup.Consume(setupToken) && !h.invites.Consume(setupToken) {
            http.Error(w, "setup token already used or expired", http.StatusUnauthorized)
            return
        }
    }

    token, err := h.wa.FinishRegistration(ceremonyKey, r)
    if err != nil {
        log.ErrorLog.Printf("auth: finish registration failed: %v", err)
        http.Error(w, fmt.Sprintf("registration failed: %v", err), http.StatusBadRequest)
        return
    }

    setAuthCookie(w, token)
    jsonResponse(w, map[string]interface{}{"ok": true})
}
```

Note: `h.invites` does not yet exist at this point in the implementation sequence. Task 1.1 should add the structural change and stub `h.invites` as nil-safe, or defer the invite branch until Task 4.4 when `InviteManager` is wired in. The core fix — consuming the setup token before `FinishRegistration` — can land immediately.

**Acceptance criteria**:
- Two concurrent requests to `finishRegistration` with the same `setup_token` result in exactly one credential being registered.
- `go test ./server/...` passes with a new table-driven test covering the concurrent case.

---

## Epic 2: Backend Store Extensions

### Task 2.1 — Add credential metadata fields

**Files**: `server/auth/store.go`
**Complexity**: Low

Add three fields to `storedCredential`:

```go
type storedCredential struct {
    ID              []byte                 `json:"id"`
    PublicKey       []byte                 `json:"public_key"`
    AttestationType string                 `json:"attestation_type"`
    Authenticator   webauthn.Authenticator `json:"authenticator"`
    DisplayName     string                 `json:"display_name,omitempty"`
    CreatedAt       time.Time              `json:"created_at,omitempty"`
    LastUsedAt      *time.Time             `json:"last_used_at,omitempty"`
}
```

In `AddCredential`, populate `CreatedAt = time.Now()`. `DisplayName` is populated by the caller (invite label). `LastUsedAt` is set in `UpdateCredential`.

In `UpdateCredential`, add `cs.data.Credentials[i].LastUsedAt = &now` before calling `cs.save()`.

**Migration note**: Existing `passkeys.json` records missing these fields will deserialize with zero values (`"", zero-time, nil`). This is safe. No migration script needed.

**Acceptance criteria**:
- `AddCredential` sets `CreatedAt` to current time.
- `UpdateCredential` sets `LastUsedAt` to current time.
- Existing `passkeys.json` without these fields loads without error.
- `go test ./server/...` passes.

### Task 2.2 — Add `ListCredentials()` and last-credential guard

**Files**: `server/auth/store.go`
**Complexity**: Low

Add `ListCredentials()` that returns `[]storedCredential` (the internal type, not `[]webauthn.Credential`). This is safe to expose to handlers within the same package.

```go
// ListCredentials returns a copy of all stored credentials including metadata.
func (cs *CredentialStore) ListCredentials() []storedCredential {
    cs.mu.RLock()
    defer cs.mu.RUnlock()
    result := make([]storedCredential, len(cs.data.Credentials))
    copy(result, cs.data.Credentials)
    return result
}
```

Add a sentinel error and guard in `RemoveCredential`:

```go
// ErrLastCredential is returned when attempting to remove the only remaining credential.
var ErrLastCredential = errors.New("cannot remove the last passkey credential")

func (cs *CredentialStore) RemoveCredential(credID []byte) error {
    cs.mu.Lock()
    defer cs.mu.Unlock()

    for i, sc := range cs.data.Credentials {
        if bytesEqual(sc.ID, credID) {
            if len(cs.data.Credentials) == 1 {
                return ErrLastCredential
            }
            cs.data.Credentials = append(cs.data.Credentials[:i], cs.data.Credentials[i+1:]...)
            return cs.save()
        }
    }
    return fmt.Errorf("credential %x not found", credID)
}
```

The revoke HTTP handler will check for `ErrLastCredential` and call `sessions.RevokeAllSessions()` before returning success, so the last credential CAN be deleted — `ErrLastCredential` is a signal to the caller, not a hard refusal.

**Acceptance criteria**:
- `ListCredentials()` returns all stored credentials with metadata.
- `RemoveCredential` returns `ErrLastCredential` when removing the last entry.
- `go test ./server/...` passes.

---

## Epic 3: InviteManager

### Task 3.1 — Implement `server/auth/invite.go`

**Files**: `server/auth/invite.go` (new file, ~65 lines)
**Complexity**: Low

```go
package auth

import (
    "crypto/subtle"
    "sync"
    "time"
)

const inviteTokenTTL = 15 * time.Minute
const inviteMaxSlots = 5

type inviteEntry struct {
    Token     string
    Label     string
    ExpiresAt time.Time
}

// InviteManager manages short-lived web-generated invite tokens for adding
// new passkey-enabled devices. Separate from SetupManager (bootstrap concern).
// See ADR-001.
type InviteManager struct {
    mu      sync.Mutex
    entries map[string]inviteEntry // keyed by token
}

func NewInviteManager() *InviteManager {
    return &InviteManager{entries: make(map[string]inviteEntry)}
}

// Generate creates a new invite token with the given label and TTL.
// If the map exceeds inviteMaxSlots, expired entries are pruned first.
// Returns the token string.
func (m *InviteManager) Generate(label string) (string, time.Time, error) {
    token, err := randomHex(16)
    if err != nil {
        return "", time.Time{}, err
    }
    expiresAt := time.Now().Add(inviteTokenTTL)

    m.mu.Lock()
    defer m.mu.Unlock()
    m.pruneExpiredLocked()
    m.entries[token] = inviteEntry{Token: token, Label: label, ExpiresAt: expiresAt}
    return token, expiresAt, nil
}

// IsValid checks whether the candidate token is valid without consuming it.
// Used by isAuthorised for both begin and finish registration checks.
func (m *InviteManager) IsValid(candidate string) bool {
    m.mu.Lock()
    defer m.mu.Unlock()
    for _, e := range m.entries {
        if time.Now().Before(e.ExpiresAt) &&
            subtle.ConstantTimeCompare([]byte(e.Token), []byte(candidate)) == 1 {
            return true
        }
    }
    return false
}

// Consume removes a valid token atomically. Returns true if the token was
// found and consumed; false if not found or already expired.
// Call this before FinishRegistration (see ADR-001, Task 1.1).
func (m *InviteManager) Consume(candidate string) bool {
    m.mu.Lock()
    defer m.mu.Unlock()
    for key, e := range m.entries {
        if time.Now().Before(e.ExpiresAt) &&
            subtle.ConstantTimeCompare([]byte(e.Token), []byte(candidate)) == 1 {
            delete(m.entries, key)
            return true
        }
    }
    return false
}

// LabelFor returns the label associated with the given token, if it is valid.
func (m *InviteManager) LabelFor(candidate string) (string, bool) {
    m.mu.Lock()
    defer m.mu.Unlock()
    for _, e := range m.entries {
        if time.Now().Before(e.ExpiresAt) &&
            subtle.ConstantTimeCompare([]byte(e.Token), []byte(candidate)) == 1 {
            return e.Label, true
        }
    }
    return "", false
}

func (m *InviteManager) pruneExpiredLocked() {
    now := time.Now()
    for k, e := range m.entries {
        if now.After(e.ExpiresAt) {
            delete(m.entries, k)
        }
    }
}
```

**Acceptance criteria**:
- `Generate` returns a non-empty token and a future `ExpiresAt`.
- `IsValid` returns true for a valid token, false after `Consume` is called.
- `Consume` returns false if called twice with the same token.
- Expired tokens are not returned as valid.
- `go test ./server/...` passes with unit tests covering all four methods.

---

## Epic 4: New HTTP Endpoints

### Task 4.1 — `POST /auth/invite/generate`

**Files**: `server/auth/handlers.go`
**Complexity**: Medium

Handler method `(h *httpHandlers) generateInvite`. Authentication gate: `!h.isAuthorised(r)` → 401. CSRF gate: `!h.verifyOrigin(r)` → 403 (see ADR-005). TLS gate: `h.port == 0` or `h.caPath == ""` → 503 with `{"error": "TLS not enabled"}`.

Request body (JSON, all optional):
```json
{ "label": "iPhone 15" }
```
If `label` is empty, default to `"Passkey"`.

Response:
```json
{
  "token":              "<32 hex chars>",
  "registration_url":   "https://<primaryDomain>:<port>/login?setup_token=<token>",
  "qr_png_data_url":    "data:image/png;base64,<base64>",
  "ca_url":             "https://<primaryDomain>:<port>/auth/ca.pem",
  "ca_qr_png_data_url": "data:image/png;base64,<base64>",
  "expires_at":         "2026-04-21T14:35:00Z",
  "ttl_seconds":        900
}
```

Implementation steps:
1. Parse optional JSON body for `label`.
2. Call `h.verifyOrigin(r)`.
3. Call `h.isAuthorised(r)`.
4. Call `h.invites.Generate(label)` → `(token, expiresAt, err)`.
5. Build `registrationURL = fmt.Sprintf("https://%s:%d/login?setup_token=%s", h.primaryDomain, h.port, token)`.
6. Build `caURL = fmt.Sprintf("https://%s:%d/auth/ca.pem", h.primaryDomain, h.port)`.
7. Call `GenerateQRPNG(registrationURL)` and `GenerateQRPNG(caURL)`.
8. Base64-encode both PNGs.
9. Return JSON.

Add `verifyOrigin` helper on `httpHandlers`:
```go
func (h *httpHandlers) verifyOrigin(r *http.Request) bool {
    if isLocalhostRequest(r) {
        return true
    }
    expected := fmt.Sprintf("https://%s:%d", h.primaryDomain, h.port)
    origin := r.Header.Get("Origin")
    if origin != "" {
        return origin == expected
    }
    return strings.HasPrefix(r.Header.Get("Referer"), expected)
}
```

**Acceptance criteria**:
- Unauthenticated request returns 401.
- Wrong `Origin` header returns 403.
- No TLS configured returns 503.
- Valid authenticated request returns JSON with all six fields.
- `registration_url` contains the invite token as `setup_token` query param.
- Both QR PNG data URIs decode to valid 256×256 PNGs.

### Task 4.2 — `GET /auth/credentials`

**Files**: `server/auth/handlers.go`
**Complexity**: Low

Handler method `(h *httpHandlers) listCredentials`. Auth gate: `!h.isAuthorised(r)` → 401.

Response:
```json
{
  "credentials": [
    {
      "id":           "<hex-encoded credential ID>",
      "display_name": "iPhone 15",
      "created_at":   "2026-01-15T10:00:00Z",
      "last_used_at": "2026-04-20T09:12:00Z",
      "sign_count":   42
    }
  ]
}
```

Credential ID encoding: `hex.EncodeToString(sc.ID)`. This matches the existing Go log format (`%x`) and avoids base64url encoding issues in URL path params.

`last_used_at` is omitted from the JSON when nil (field is `*time.Time` with `omitempty`).

**Acceptance criteria**:
- Unauthenticated request returns 401.
- Response contains one entry per registered passkey.
- `id` is lowercase hex, decodable back to the credential bytes.
- `created_at` is populated for credentials registered after Task 2.1 lands.
- `go test ./server/...` passes.

### Task 4.3 — `POST /auth/credentials/{id}/revoke`

**Files**: `server/auth/handlers.go`
**Complexity**: Low-Medium

Handler method `(h *httpHandlers) revokeCredential`. Uses Go 1.25 `r.PathValue("id")` (confirmed available in `go.mod`).

```go
func (h *httpHandlers) revokeCredential(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    if !h.isAuthorised(r) {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }

    idHex := r.PathValue("id")
    credID, err := hex.DecodeString(idHex)
    if err != nil {
        http.Error(w, "invalid credential id", http.StatusBadRequest)
        return
    }

    err = h.store.RemoveCredential(credID)
    if errors.Is(err, ErrLastCredential) {
        // Last credential — revoke all sessions so no stale cookie grants access.
        h.sessions.RevokeAllSessions()
        if err2 := h.store.RemoveCredential(credID); err2 != nil {
            // Re-attempt with ErrLastCredential guard removed is not how this works.
            // The guard is in RemoveCredential itself; we need a force-remove path.
            // See implementation note below.
        }
        jsonResponse(w, map[string]interface{}{"ok": true, "last_credential": true})
        return
    }
    if err != nil {
        // Check for "not found" to return 404.
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    log.InfoLog.Printf("auth: credential %x revoked", credID)
    jsonResponse(w, map[string]interface{}{"ok": true, "last_credential": false})
}
```

**Implementation note on last-credential removal**: `RemoveCredential` returns `ErrLastCredential` to signal the caller should decide. The handler must handle the "allow deletion of last credential but also revoke all sessions" case. One approach: add `RemoveCredentialForce(credID []byte) error` that skips the last-credential guard. The revoke handler calls `RevokeAllSessions` first (invalidating any stale cookies) and then `RemoveCredentialForce`. This ordering ensures that even if `RemoveCredentialForce` fails, no stale session can access a now-ownerless server.

**Acceptance criteria**:
- Unauthenticated request returns 401.
- Invalid hex ID returns 400.
- Non-existent ID returns 404 (update `RemoveCredential` error message to be distinguishable from the "not found" case).
- Revoking the last credential calls `RevokeAllSessions()` and returns `{"ok":true,"last_credential":true}`.
- `go test ./server/...` passes.

### Task 4.4 — Wire `InviteManager` into `RegisterRoutes` and `isAuthorised`

**Files**: `server/auth/handlers.go`
**Complexity**: Low

Update `httpHandlers` struct:
```go
type httpHandlers struct {
    wa            *Handler
    sessions      *SessionManager
    store         *CredentialStore
    setup         *SetupManager
    invites       *InviteManager   // new
    caPath        string
    primaryDomain string
    port          int              // new — required for invite URL construction (ADR-005)
}
```

Update `RegisterRoutes` signature:
```go
func RegisterRoutes(
    mux *http.ServeMux,
    waHandler *Handler,
    sessions *SessionManager,
    store *CredentialStore,
    setup *SetupManager,
    invites *InviteManager,
    tlsCAPath, primaryDomain string,
    port int,
) {
```

Update `isAuthorised` to check invite tokens:
```go
func (h *httpHandlers) isAuthorised(r *http.Request) bool {
    if token, err := getAuthToken(r); err == nil {
        if h.sessions.ValidateAuthSession(token) {
            return true
        }
    }
    if setupToken := r.URL.Query().Get("setup_token"); setupToken != "" {
        if h.setup.IsValid(setupToken) {
            return true
        }
        if h.invites != nil && h.invites.IsValid(setupToken) {
            return true
        }
    }
    return false
}
```

Register new routes:
```go
mux.HandleFunc("POST /auth/invite/generate", h.generateInvite)
mux.HandleFunc("GET /auth/credentials", h.listCredentials)
mux.HandleFunc("POST /auth/credentials/{id}/revoke", h.revokeCredential)
```

Update the `RegisterRoutes` call site in `server/server.go` (or wherever `RegisterRoutes` is called) to pass `invites` and `port`.

**Acceptance criteria**:
- `make build` succeeds with updated `RegisterRoutes` signature.
- All three new routes are reachable.
- `isAuthorised` returns true for a valid invite token.
- Complete Task 1.1 fully now that `h.invites` exists.

---

## Epic 5: Frontend API Client

### Task 5.1 — Add invite and credential functions to `passkey.ts`

**Files**: `web-app/src/lib/auth/passkey.ts`
**Complexity**: Low

Add TypeScript interfaces and three functions:

```typescript
export interface InviteResponse {
  token: string;
  registration_url: string;
  qr_png_data_url: string;
  ca_url: string;
  ca_qr_png_data_url: string;
  expires_at: string;         // ISO 8601
  ttl_seconds: number;
}

export interface Credential {
  id: string;                 // hex-encoded
  display_name: string;
  created_at: string;         // ISO 8601
  last_used_at: string | null;
  sign_count: number;
}

export interface RevokeResult {
  ok: boolean;
  last_credential: boolean;
}

export async function generateInvite(label: string): Promise<InviteResponse> {
  const resp = await fetch(`${authBase()}/invite/generate`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify({ label }),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`generate invite failed: ${text}`);
  }
  return resp.json();
}

export async function listCredentials(): Promise<Credential[]> {
  const resp = await fetch(`${authBase()}/credentials`, {
    credentials: "include",
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`list credentials failed: ${text}`);
  }
  const data = await resp.json();
  return data.credentials as Credential[];
}

export async function revokeCredential(id: string): Promise<RevokeResult> {
  const resp = await fetch(`${authBase()}/credentials/${id}/revoke`, {
    method: "POST",
    credentials: "include",
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`revoke credential failed: ${text}`);
  }
  return resp.json();
}
```

**Acceptance criteria**:
- TypeScript compiles without errors (`cd web-app && npx tsc --noEmit`).
- Functions use `credentials: "include"` for all requests.
- `generateInvite` sends POST with JSON body.

### Task 5.2 — Add `account` route to `routes.ts`

**Files**: `web-app/src/lib/routes.ts`
**Complexity**: Trivial

Add one line:

```typescript
account: "/account",
```

After this change `routes.account` is `"/account"` — used by `Header.tsx` and `login/page.tsx`.

---

## Epic 6: `/account` Page

### Task 6.1 — `account/layout.tsx` (auth guard)

**Files**: `web-app/src/app/account/layout.tsx` (new file)
**Complexity**: Low

Mirror `settings/layout.tsx` with the addition of an auth redirect:

```typescript
"use client";
import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/contexts/AuthContext";
import type { Metadata } from "next";

export default function AccountLayout({ children }: { children: React.ReactNode }) {
  const { authenticated, loading, authEnabled } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (!loading && authEnabled && !authenticated) {
      router.replace("/login");
    }
  }, [loading, authEnabled, authenticated, router]);

  return <>{children}</>;
}
```

Note: `Metadata` export must be in a separate `metadata.ts` or removed if the layout is `"use client"`. Export metadata from `page.tsx` instead.

**Acceptance criteria**:
- Unauthenticated browser navigation to `/account` redirects to `/login`.
- Authenticated users reach the page without redirect.

### Task 6.2 — `CredentialList` component

**Files**:
- `web-app/src/app/account/components/CredentialList/CredentialList.tsx` (new)
- `web-app/src/app/account/components/CredentialList/CredentialList.css.ts` (new)

**Complexity**: Low-Medium

Props:
```typescript
interface Props {
  credentials: Credential[];
  onRevoke: (id: string) => void;
  loading: boolean;
}
```

Renders a list of credential rows. Each row shows:
- `display_name` (fallback: "Passkey")
- `created_at` formatted as human-readable date
- `last_used_at` formatted (or "Never" if null)
- `sign_count`
- "Revoke" button that calls `onRevoke(credential.id)`

Uses vanilla-extract styles (ADR-009). All styles go in `CredentialList.css.ts` using `style()` and `vars` from `../../styles/theme.css`.

Empty state: "No passkeys registered." (not reachable in practice since the user must be authenticated to reach this page).

**Acceptance criteria**:
- Renders correctly with one or more credentials.
- "Revoke" button calls `onRevoke` with the correct credential ID.
- All styles are in `CredentialList.css.ts` — no inline styles, no CSS modules.

### Task 6.3 — `AddDeviceModal` component

**Files**:
- `web-app/src/app/account/components/AddDeviceSection/AddDeviceSection.tsx` (new)
- `web-app/src/app/account/components/AddDeviceSection/AddDeviceModal.tsx` (new)
- `web-app/src/app/account/components/AddDeviceSection/AddDeviceModal.css.ts` (new)

**Complexity**: High (most complex component in this feature)

`AddDeviceSection` renders a button "Add New Device". On click, it calls `generateInvite(label)` and opens the modal with the response.

`AddDeviceModal` props:
```typescript
interface Props {
  invite: InviteResponse;
  onClose: () => void;
  onRegenerate: () => void;
}
```

Modal layout (two-column on desktop, stacked on mobile):

**Left column — Step 0: Install CA Certificate**
- Label QR code: `ca_qr_png_data_url`
- "Download CA Cert" button linking to `ca_url`
- Expandable platform instructions (accordion: iOS, Android, macOS/Windows)
- iOS instructions:
  1. Tap QR or "Download CA Cert" — file downloads in Safari.
  2. Settings → General → VPN & Device Management → tap profile → Install.
  3. Settings → General → About → Certificate Trust Settings → enable full trust.
- Android instructions:
  1. Tap QR or "Download CA Cert".
  2. Settings → Security & privacy → More security settings → Encryption & credentials → Install a certificate → CA Certificate.
- macOS/Windows: Download the `.pem` file and install into System keychain.

**Right column — Step 1: Register Device**
- Label QR code: `qr_png_data_url`
- Copyable URL field showing `registration_url` with a "Copy" button
- TTL countdown (from ADR-003):
  ```tsx
  const [secondsLeft, setSecondsLeft] = useState(() =>
    Math.max(0, Math.floor((new Date(invite.expires_at).getTime() - Date.now()) / 1000))
  );
  useEffect(() => {
    if (secondsLeft <= 0) return;
    const id = setInterval(() =>
      setSecondsLeft(s => { if (s <= 1) { clearInterval(id); return 0; } return s - 1; }), 1000
    );
    return () => clearInterval(id);
  }, []);
  ```
  When `secondsLeft === 0`: show "Invite expired" banner, disable copy button, show "Generate New Invite" button.
- "Generate New Invite" button with warning text: "Generating a new invite invalidates the current one."

**Warning banner**: Display at the top of the modal — "Do not share this QR code or URL. It grants access to your Stapler Squad instance."

All styles in `AddDeviceModal.css.ts`. Use `vars` from the theme contract.

**Acceptance criteria**:
- Modal shows both QR codes.
- Countdown decrements every second.
- "Copy" copies `registration_url` to clipboard.
- When `secondsLeft === 0`, copy is disabled and expired message shows.
- Platform instructions expand on click.
- "Generate New Invite" calls `onRegenerate`.
- "Close" calls `onClose`.
- All styles are in `.css.ts` — no inline styles.

### Task 6.4 — `RevokeConfirmDialog` component

**Files**:
- `web-app/src/app/account/components/RevokeConfirmDialog/RevokeConfirmDialog.tsx` (new)
- `web-app/src/app/account/components/RevokeConfirmDialog/RevokeConfirmDialog.css.ts` (new)

**Complexity**: Low

A confirmation dialog shown before revoking a credential. Props:
```typescript
interface Props {
  credentialName: string;
  isLastCredential: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}
```

When `isLastCredential` is true, show a red warning: "Revoking your last passkey will immediately log out all devices including this one. You will need CLI access to re-register."

Standard confirm/cancel buttons. All styles in `RevokeConfirmDialog.css.ts`.

**Acceptance criteria**:
- Warning text appears only when `isLastCredential` is true.
- "Confirm" calls `onConfirm`, "Cancel" calls `onCancel`.

### Task 6.5 — `account/page.tsx` (orchestration)

**Files**:
- `web-app/src/app/account/page.tsx` (new)
- `web-app/src/app/account/account.css.ts` (new)

**Complexity**: Medium

`AccountPage` fetches credentials on mount and after any revoke or modal close. Manages:
- `credentials: Credential[]`
- `loading: boolean`
- `error: string | null`
- `invite: InviteResponse | null`
- `revokeTarget: Credential | null`
- `isLastCredentialForRevoke: boolean`

Flow:
1. On mount: call `listCredentials()` → populate `credentials`.
2. "Add New Device" button: show label input (optional), call `generateInvite(label)` → open `AddDeviceModal`.
3. Modal "Close": close modal, call `listCredentials()` to refresh (the new device may have registered).
4. Modal "Generate New Invite": call `generateInvite(label)` again → replace `invite` in state.
5. `CredentialList` "Revoke" button: check if it's the last credential (`credentials.length === 1`), set `revokeTarget`, open `RevokeConfirmDialog`.
6. `RevokeConfirmDialog` "Confirm": call `revokeCredential(revokeTarget.id)`, refresh credentials, close dialog. If `last_credential: true` in response, the user will be redirected to `/login` by the auth guard on next navigation (sessions revoked server-side).

Page layout:
- `<h1>Account</h1>`
- Section: "Your Passkeys" — `<CredentialList />`
- Section: "Add a New Device" — `<AddDeviceSection />`

All page-level styles in `account.css.ts`.

**Acceptance criteria**:
- Page loads and shows credential list.
- Adding a device shows the modal with QR codes.
- Revoking a credential refreshes the list.
- Revoking the last credential shows the warning and after confirm, redirects to `/login`.

---

## Epic 7: Navigation and Login Entry Point

### Task 7.1 — Add "Account" nav link to `Header.tsx`

**Files**: `web-app/src/components/layout/Header.tsx`
**Complexity**: Low

The `Header` component already uses `useAuth` indirectly (via `routes`). Add an "Account" nav link after the "Settings" link, visible only when `authenticated` is true.

Import `useAuth`:
```tsx
import { useAuth } from "@/lib/contexts/AuthContext";
```

Inside `Header`:
```tsx
const { authenticated } = useAuth();
```

Add after the Settings `AppLink`:
```tsx
{authenticated && (
  <AppLink
    href={routes.account}
    className={`${styles.navLink} ${pathname === routes.account ? styles.active : ""}`}
    onClick={handleNavLinkClick}
  >
    Account
  </AppLink>
)}
```

**Acceptance criteria**:
- "Account" link appears in the nav when `authenticated` is true.
- Link is absent when not authenticated.
- Link is active-styled when on `/account`.
- `make restart-web` builds without errors.

### Task 7.2 — Add "Add another device" to `login/page.tsx`

**Files**: `web-app/src/app/login/page.tsx`
**Complexity**: Low

In `LoginContent`, the existing `useEffect` redirects authenticated users to `/`. Before that redirect fires, a user who is already authenticated and has credentials might arrive at the login page (e.g., via a bookmark or a direct link). They should see an "Add another device" link pointing to `/account` rather than immediately being redirected.

The simplest approach: in the `isSetup` false branch (where the "Sign in with Passkey" button is shown), add a secondary link:

```tsx
{authenticated && hasCredentials && (
  <p className={styles.hint} style={{ marginTop: "1rem" }}>
    Already signed in?{" "}
    <a href={routes.account} className={styles.link}>
      Add another device
    </a>
  </p>
)}
```

Note: the existing `useEffect` redirects `authenticated` users to `/`. The "Add another device" link will only be visible in the brief window before that redirect — or if the user navigates directly to `/login` while already having a valid session and auth is enabled. The more reliable entry point is the `Header.tsx` Account link (Task 7.1). This task provides a secondary discovery path.

**Acceptance criteria**:
- "Add another device" link appears on `/login` when `authenticated && hasCredentials`.
- Link navigates to `/account`.

---

## Integration Checkpoints

### Checkpoint A — Backend complete (after Epic 4)
Verify with `curl` or a browser devtools console:
```bash
# From an authenticated browser session, open devtools console:
fetch('/auth/invite/generate', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  credentials: 'include',
  body: JSON.stringify({label: 'Test Device'})
}).then(r => r.json()).then(console.log)
```
Expected: JSON with `token`, `registration_url`, two QR data URIs, `expires_at`, `ttl_seconds`.

```bash
fetch('/auth/credentials', {credentials: 'include'}).then(r => r.json()).then(console.log)
```
Expected: JSON with `credentials` array.

### Checkpoint B — End-to-end invite flow (after Epic 6)
1. Open `/account` on the desktop browser (authenticated).
2. Click "Add New Device", enter label "Test Phone".
3. Verify both QR codes appear and countdown starts.
4. On a second browser tab (or mobile device that already trusts the CA), navigate to the `registration_url`.
5. Complete the passkey registration ceremony.
6. Return to `/account` desktop tab, close and reopen the modal — the new credential should appear in the list.

### Checkpoint C — Revoke flow (after Epic 6)
1. On `/account`, click "Revoke" on a non-last credential.
2. Confirm in the dialog.
3. Credential disappears from list.
4. Repeat for the last credential — verify the "last passkey" warning appears and the user is logged out after confirming.

---

## Known Issues

### BUG-001: Double-registration race condition in `finishRegistration` [SEVERITY: High]

**Description**: Two devices calling `beginRegistration` with the same setup/invite token both receive valid ceremony challenges. The first to call `finishRegistration` consumes the token. The second device's call passes the `isAuthorised` check (token not yet consumed at that point) but `Consume` returns false. Currently the handler ignores a false return from `Consume` and still calls `setAuthCookie`, allowing two credentials to be registered with one token.

**Evidence**: `server/auth/handlers.go` lines 157–159 — `h.setup.Consume(setupToken)` return value is silently discarded; `FinishRegistration` has already persisted the credential by this point.

**Fix**: Task 1.1 — move `Consume` call before `FinishRegistration`.

**Files Affected**:
- `server/auth/handlers.go` (fix)
- `server/auth/invite.go` (must not repeat the same pattern)

---

### BUG-002: Stale session not invalidated on credential revocation [SEVERITY: Medium]

**Description**: `RemoveCredential` removes the stored passkey but does not invalidate any active `authSession` tokens. A device whose passkey was revoked remains authenticated for up to 30 days (the `authTokenTTL`).

**Mitigation (implemented)**: Task 4.3 calls `sessions.RevokeAllSessions()` when the last credential is revoked. For non-last revocations, the residual session lifetime (up to 30 days) is accepted as an acceptable limitation for a single-user tool. The UI warns that "Revoking a passkey does not immediately log out the associated device" in the `RevokeConfirmDialog` for non-last credentials.

**Full fix (SHOULD implement, not in this plan)**: Add `credentialID []byte` to `authSession`; implement `RevokeSessionsByCredential(credID []byte)` in `SessionManager`. This requires a `passkeys-sessions.json` migration. Deferred to a follow-up task.

**Files Affected**:
- `server/auth/session.go` (future fix)
- `server/auth/handlers.go` (revoke handler)

---

### BUG-003: `setup_active: true` leaks after web invite generation if SetupManager is used [SEVERITY: Medium]

**Description**: If the `SetupManager` bootstrap flow and the web invite flow are merged (Option B from ADR-001), any web invite generation would set `setup_active: true` on a fully-configured server. This would cause the frontend to display the initial setup banner on configured servers.

**Mitigation**: ADR-001 — `InviteManager` is separate from `SetupManager`. `setup_active` in `GET /auth/status` reflects only `SetupManager.IsActive()` and is unaffected by web invites.

**Files Affected**: N/A (architecture decision prevents the bug).

---

### BUG-004: iOS CA cert download does not auto-trigger install [SEVERITY: High for UX]

**Description**: On iOS 17+, downloading a `.pem` file from Safari triggers a download, not a profile install dialog. Users must manually navigate to Settings → General → VPN & Device Management to install the cert. Without explicit instructions, device registration will fail with a TLS error on the new device.

**Mitigation**: ADR-004 — `AddDeviceModal` includes step-by-step iOS instructions as Step 0.

**Files Affected**: `AddDeviceModal.tsx` (instructions must be correct and visible).

---

### BUG-005: Port not available in `httpHandlers` for invite URL construction [SEVERITY: Medium]

**Description**: `RegisterRoutes` currently does not accept a `port` parameter. `httpHandlers.primaryDomain` is the bare hostname without port. `POST /auth/invite/generate` must construct `https://<host>:<port>/login?setup_token=<token>` — without `port`, the URL is malformed for non-standard ports (e.g., `8443` or `8444`).

**Fix**: Task 4.4 — add `port int` field to `httpHandlers` and `port int` parameter to `RegisterRoutes`. Update the call site in `server/server.go`.

**Files Affected**:
- `server/auth/handlers.go`
- `server/server.go` (or wherever `RegisterRoutes` is called)

---

### BUG-006: `finishRegistration` does not redirect to strip `setup_token` from URL [SEVERITY: Low]

**Description**: After a successful `finishRegistration`, the browser URL retains `?setup_token=<token>`. This exposes the token in browser history and any subsequent `Referer` headers.

**Fix**: In `registerPasskey` (`passkey.ts`), after `finishRegistration` returns 200, the caller already calls `router.replace("/")` in `login/page.tsx`, which strips the query string. This is effectively already handled. Verify the `router.replace("/")` call happens unconditionally after `registerPasskey` resolves.

**Files Affected**: `web-app/src/app/login/page.tsx` (verify existing behavior is correct).

---

### BUG-007: `InviteManager.IsValid` iterates all entries on every auth check [SEVERITY: Low]

**Description**: `isAuthorised` is called on every request to the registration ceremony endpoints. `IsValid` iterates the entire `entries` map. With `inviteMaxSlots = 5`, this is a constant-time operation in practice, but it is O(n) rather than O(1).

**Mitigation**: With a maximum of 5 slots, the impact is negligible. If `inviteMaxSlots` is ever increased significantly, index by token for O(1) lookup. Not worth optimizing for the current design.

**Files Affected**: `server/auth/invite.go`.

---

## Context Preparation for Implementation

Before starting each epic, load these files into context:

### Epic 1 (Bug Fix)
- `server/auth/handlers.go` — full file
- `server/auth/setup.go` — `Consume` method

### Epic 2 (Store Extensions)
- `server/auth/store.go` — full file
- `server/auth/session.go` — `UpdateCredential` call site

### Epic 3 (InviteManager)
- `server/auth/setup.go` — as template for `IsValid`/`Consume` pattern
- `server/auth/session.go` — for `randomHex` function reference

### Epics 4.1-4.4 (HTTP Endpoints)
- `server/auth/handlers.go` — full file
- `server/auth/store.go` — `ListCredentials`, `RemoveCredential`, `ErrLastCredential`
- `server/auth/invite.go` — full new file
- `server/auth/qrcode.go` — `GenerateQRPNG` signature
- `server/server.go` — `RegisterRoutes` call site (to update signature)

### Epics 5-6 (Frontend)
- `web-app/src/lib/auth/passkey.ts` — full file
- `web-app/src/lib/routes.ts` — full file
- `web-app/src/app/login/page.tsx` — full file
- `web-app/src/app/settings/layout.tsx` — auth guard pattern
- `web-app/src/styles/theme.css.ts` — vanilla-extract token contract
- `web-app/src/lib/contexts/AuthContext.tsx` — `useAuth` hook interface

### Epic 7 (Navigation)
- `web-app/src/components/layout/Header.tsx` — full file
- `web-app/src/lib/routes.ts` — full file

---

## Testing Checklist

### Unit Tests (Go)
- [ ] `InviteManager.Generate` returns non-empty token and future expiry
- [ ] `InviteManager.IsValid` returns false after expiry
- [ ] `InviteManager.Consume` is single-use (second call returns false)
- [ ] `InviteManager.IsValid` and `Consume` are concurrent-safe (run with `-race`)
- [ ] `CredentialStore.ListCredentials` returns entries with `CreatedAt` set
- [ ] `CredentialStore.RemoveCredential` returns `ErrLastCredential` when removing the only entry
- [ ] `finishRegistration` concurrent test: two goroutines, one invite token, only one credential registered
- [ ] `POST /auth/invite/generate`: 401 without auth, 403 with wrong Origin, 503 without TLS, 200 with valid auth
- [ ] `GET /auth/credentials`: 401 without auth, 200 with credential list
- [ ] `POST /auth/credentials/{id}/revoke`: 401 without auth, 400 with invalid hex, 200 with valid ID

### Frontend Tests (Jest)
- [ ] `AddDeviceModal` renders both QR images
- [ ] `AddDeviceModal` countdown reaches 0 and shows expired state
- [ ] `RevokeConfirmDialog` shows last-credential warning when `isLastCredential` is true
- [ ] `CredentialList` renders empty state and populated state

### End-to-End (Playwright)
- [ ] Authenticated user navigates to `/account` successfully
- [ ] Unauthenticated navigation to `/account` redirects to `/login`
- [ ] "Account" link appears in header when authenticated
- [ ] Invite modal opens and displays countdown
