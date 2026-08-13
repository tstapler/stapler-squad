# Requirements: Session Pinning

Source: backlog item `9959d36a-01e7-4dce-92bd-74ee87b2c99d` (migrated from
[TylerStaplerAtFanatics/stapler-squad#171](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/171)).

## Problem

Users can't mark a session as "important" and keep it visible regardless of
recency/status/sort order. herdr-web's agent-pins feature (`web/src/agentPins.ts`)
is the reference: pinned panes surface at the top of the sidebar. This is distinct
from the existing pinned-*repos* concept in the unfinished-work scanner — this is
per-session, server-owned, survives reloads and browser switches.

## Scope

In scope: pin/unpin toggle on a session, server-persisted boolean, pinned
sessions rendered in a dedicated top section of the session list.

Out of scope: pin ordering/reordering within the pinned section (pins sort by
existing list order, e.g. `pinned_at` desc, not drag-to-reorder), pinning
archived sessions (archiving should probably clear/ignore pin — decide in
research), cross-workspace pin sync.

## Functional Requirements

1. A session can be pinned and unpinned by the user from the UI.
2. Pin state is stored server-side (ent-backed `Session` entity, matching how
   `hidden`, `auto_yes`, `is_expanded` already work) — not localStorage.
3. `ListSessions` (and single-session reads) return the pin state so the
   frontend never has to guess.
4. The session list renders a "Pinned" section above the normal grouped/sorted
   list, containing all pinned sessions regardless of their status/recency.
5. Pin state persists across page reloads and across browser/device (server-owned).
6. Toggling pin is available from the session card context menu and/or the
   session detail header (mirrors existing action-menu affordances such as
   archive/hide).

## Acceptance Criteria

- [ ] Sessions can be pinned/unpinned from the UI.
- [ ] Pinned sessions appear in a dedicated section in the session list, at the top.
- [ ] Pin state persists across browser reloads.
- [ ] Pin state is server-owned (not localStorage).
- [ ] Pinning/unpinning is exposed via RPC (`PinSession`/`UnpinSession` or a
      single `SetSessionPinned`) and covered by a backend test.
- [ ] Frontend pin toggle and pinned-section rendering covered by a Jest/RTL test.
- [ ] `docs/registry/features/` updated per `.claude/rules/feature-registry.md`
      (new RPC + new UI affordance) with an e2e test per
      `.claude/rules/e2e-test-conventions.md`.

## Constraints / Conventions to honor (from repo instructions)

- ent schema change → regenerate with
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  (`.claude/rules/ent-schema-generation.md`).
- Proto change → `make proto-gen`; new `Session` field uses the next free
  field number. **Correction from research (features.md, pitfalls.md):** the
  true highest field in use is `workspace_key = 71`, not `archived_at = 63` —
  so `pinned` must be field `72`.
- New RPC/UI feature → per-feature JSON files under `docs/registry/features/`
  plus `make registry-generate` (`.claude/rules/feature-registry.md`).
- New user-facing feature → e2e test in `tests/e2e/`, `@feature` header,
  `data-testid`/ARIA locators only, no `waitForTimeout`
  (`.claude/rules/e2e-test-conventions.md`).
- This is a mutation on existing sessions, not a new session-creation mode —
  the 7-touchpoint session-creation registry
  (`.claude/rules/session-creation-registry.md`) does not apply.
- Follow `.claude/rules/interface-pollution-checklist.md`: no speculative
  `PinStore` interface, no getter/setter ceremony — a plain boolean field and
  direct ent calls, matching the existing `hidden`/`ArchiveSession` pattern.

## Existing precedent to mirror

- `session/ent/schema/session.go` — `hidden`, `auto_yes`, `is_expanded` are
  all plain `field.Bool(...).Default(false)` on the same entity; `pinned`
  follows the same shape.
- `ArchiveSession` / `UnarchiveSession` RPC pair
  (`proto/session/v1/session.proto`, `server/services/session_service.go:4285`)
  is the closest existing "toggle a persisted session flag via RPC" precedent
  for `PinSession`/`UnpinSession`.
- `session/instance_actor_setters.go` (e.g. `SetArchivedAtIfNil`) is the
  precedent for how a boolean/timestamp mutation is threaded through the
  actor-owned `Instance` state safely.
