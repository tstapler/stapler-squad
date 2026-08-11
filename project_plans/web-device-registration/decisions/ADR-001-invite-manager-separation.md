# ADR-001: Separate InviteManager from SetupManager

**Status**: Accepted
**Date**: 2026-04-21
**Feature**: Web Device Registration

---

## Context

Two separate flows require one-time short-lived tokens to gate WebAuthn registration:

1. **Bootstrap (first device)**: The `SetupManager` in `server/auth/setup.go` generates a single in-memory token at server startup when no passkeys exist. It is also written to `setup-token.json` by the `print-qr-codes` CLI command and watched via `fsnotify.WatchFile`. Its liveness is surfaced in `GET /auth/status` as `"setup_active": true`, which the frontend uses to show the initial setup banner.

2. **Web invite (additional devices)**: An already-authenticated user needs to generate a short-lived invite token from the browser so a new device can complete the WebAuthn registration ceremony (`/login?setup_token=<token>`).

The question is whether to reuse `SetupManager` for web invites by adding an authenticated endpoint that calls `setup.GenerateToFile()` or `setup.Init()`, or to introduce a separate `InviteManager`.

---

## Decision

Introduce a separate `InviteManager` in `server/auth/invite.go`. The `SetupManager` is unchanged and remains exclusively responsible for the first-boot bootstrap concern.

---

## Rationale

**Conflating the two concerns breaks `setup_active` semantics.** `SetupManager.IsActive()` is returned as `setup_active` in `GET /auth/status`. The frontend interprets `setup_active: true` as "this server has never been configured; a first-time setup is in progress." If an authenticated user calls an endpoint that calls `setup.Init()` to generate a web invite, `setup_active` becomes `true` on a fully-configured server. Any UI component that guards on `setup_active` would incorrectly enter setup mode.

**File watch coupling.** `SetupManager.WatchFile` watches `setup-token.json` and reloads the token whenever the CLI rewrites it. A web-generated invite token must not write to `setup-token.json` — that file is owned by the CLI workflow. If a web invite used `GenerateToFile`, a subsequent `print-qr-codes` run would silently overwrite the web invite. The two token channels would contend on a single file.

**Invite TTL is different.** The CLI bootstrap uses a 1-hour TTL (`setupTokenTTL = time.Hour` in `setup.go`). Web invites should use 15 minutes to reduce the window between invite generation and unauthorized use. Sharing `SetupManager` would require conditional TTL logic, complicating the single-concern manager.

**Cost is low.** `InviteManager` is approximately 60 lines of Go. It reuses `randomHex(16)` (already in `session.go`), `crypto/subtle.ConstantTimeCompare`, and `sync.Mutex` — no new dependencies. The existing `SetupManager` is a clear template.

**The invite token is validated by `isAuthorised`.** `isAuthorised` in `handlers.go` currently checks `h.setup.IsValid(token)`. The new invite flow must also be validated by `isAuthorised`. Rather than extending `isAuthorised` to check both managers (coupling them in the handler), the `beginRegistration`/`finishRegistration` handlers can accept invite tokens via the same `?setup_token=` query param by having `InviteManager.IsValid` checked alongside `SetupManager.IsValid` in the `isAuthorised` method. This keeps the ceremony handlers unchanged.

---

## Consequences

**Accepted costs:**
- Two token systems to maintain. However, their lifecycles and responsibilities are genuinely distinct, and the implementation overlap is limited to the `IsValid`/`Consume` pattern.
- `isAuthorised` in `handlers.go` must check both `h.setup.IsValid(token)` and `h.invites.IsValid(token)`, adding one conditional. The `httpHandlers` struct gains an `invites *InviteManager` field.
- `RegisterRoutes` signature gains `invites *InviteManager` parameter.

**Not accepted:**
- Extending `SetupManager` with an `InviteActive` flag or a second token slot. This merges two distinct concerns and corrupts the `setup_active` status field.
- Client-side token generation. Without server-side token tracking there is no revocation and no single-use enforcement.

---

## Alternatives Rejected

**Option B — Extend `SetupManager` with authenticated path**: Rejected. Merging the two token flows makes `setup_active` semantically ambiguous and couples the CLI file-watch mechanism to the web invite path.

**Option C — Client-side token**: Rejected. Removes the server-side authentication gate, eliminating single-use enforcement and the TOFU registration guarantee.
