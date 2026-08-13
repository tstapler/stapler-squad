# Build vs. Buy: Session Pinning

## Summary

Bespoke in-house implementation, copy-pattern from the existing `hidden`/`archived_at` boolean fields. No library or SaaS applies. Verdict: **build**, following existing precedent exactly.

## 1. Existing OSS library/framework

**Verdict: Not recommended (no library warranted).**

- No "pinning" library exists for Go/ent or React — this is inherently a one-boolean-field domain concern (`pinned bool` on the `Session` ent entity), identical in shape to `hidden` (`session/ent/schema/session.go:102`) and `is_expanded` (`session/ent/schema/session.go:59`).
- Checked `web-app/package.json` for anything that might already offer a "pinned section" primitive: `@reduxjs/toolkit` (`^2.11.2`), `react-redux` (`^9.2.0`), `@tanstack/react-virtual` (`^3.13.25`), `react-virtuoso` (`^4.18.7`), `react-arborist` (`^3.4.3`). None of these provide list-partitioning/pinning semantics — they're state-management, virtualization, and tree-view libraries respectively. Redux Toolkit could host the pin state as part of existing session slices, but that's just "use the state layer already in place," not a reusable pinning primitive.
- Go `go.mod` (module `github.com/tstapler/stapler-squad`, `go 1.26.3`) has nothing pin-related, and doesn't need to — the `slices` package used for the partition step is Go stdlib as of 1.21, no dependency required.
- Pulling in a dependency for a single boolean flag + list split would be over-engineering for this codebase's scale and violates the repo's own interface-pollution guidance against speculative abstraction (`.claude/rules/interface-pollution-checklist.md`).

## 2. SaaS/managed API

**Verdict: Not applicable.**

stapler-squad is a local, single-user session manager (Go server on `localhost:8543` + React SPA, per-user ent-backed storage). There is no multi-tenant or cross-device sync concern that a managed API would solve, and the requirements doc explicitly scopes out cross-workspace pin sync. Dismissed.

## 3. LLM-generated implementation vs. battle-tested library (algorithm choice)

**Verdict: Boring stdlib approach — recommended.**

There is no non-trivial algorithm here:
- **Go**: partitioning pinned vs. unpinned sessions before existing grouping/sort logic runs is a single filter pass — `slices` stdlib (`slices.SortStableFunc`, or a plain `for` loop with two `append`s) is sufficient. No custom sort algorithm, heap, or priority-queue structure is justified for what's ultimately "split a slice by a boolean field."
- **TypeScript**: `Array.prototype.filter` to split the pinned section out, feeding the existing grouped/sorted list unchanged for the remainder. `.sort()` is not even needed since the requirements explicitly put pin *ordering* out of scope (FR: "pin ordering/reordering within the pinned section" is out of scope) — pinned sessions likely just inherit their existing relative order or fall back to the same sort key already used for the main list.
- Recommend implementing the partition as a plain function colocated with the existing session-list grouping/sort utility rather than a new "PinnedListView" abstraction.

## 4. Fork or adapt — existing in-repo pattern to copy

**Verdict: Recommended — copy the `hidden` field + `ArchiveSession`/`UnarchiveSession` RPC pair pattern wholesale.**

This is the closest and most directly reusable precedent in the codebase. Exact files/locations to copy-pattern from:

- **ent schema field**: `session/ent/schema/session.go:102` — `field.Bool("hidden")` (and `is_expanded` at line 59) show the exact idiom for adding a persisted boolean to the `Session` entity. Add `field.Bool("pinned")` alongside these, then regenerate with `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` per `.claude/rules/ent-schema-generation.md`.
- **Proto field**: `proto/session/v1/types.proto:192` — `bool hidden = 57;` is the pattern for exposing the boolean on the `Session` message; add `bool pinned = <next available field number>;` near it. `archived_at` (`types.proto:209`) and its dedicated RPC pair below show the alternative "separate RPC pair" pattern if a plain field-on-`UpdateSession` isn't preferred.
- **RPC pair pattern**: `server/services/session_service.go:4285` (`ArchiveSession`) and `:4311` (`UnarchiveSession`) are the canonical soft-toggle RPC handlers — each validates `SessionId`, resolves the live instance via `FindLiveInstance`, mutates state, and persists via `s.storage.SaveInstances`. A `PinSession`/`UnpinSession` pair (or a single `SetSessionPinned(bool)`) should mirror this shape exactly, including the `// +api: session:pin` marker convention (see `.claude/rules/feature-registry.md`) and RPC declarations alongside `ArchiveSession`/`UnarchiveSession` in `proto/session/v1/session.proto:396-410`.
- **Frontend list rendering**: locate the existing grouping/sort entry point used by the session list (grouping strategies: Category, Tag, Branch, Path, Program, Status, Session Type, None — see `.claude/docs/tag-organization.md`) and add a pinned-section pre-filter ahead of it, following the same component/hook wiring already used for `hidden`/`archived` visibility toggles.
- **Context menu / detail header toggle**: the existing archive/unarchive toggle UI (wherever `ArchiveSession`/`UnarchiveSession` are called from the session card context menu and detail header) is the template for wiring a pin/unpin toggle in the same two locations, satisfying FR6.

This is a "copy the shape, rename the field" job — no fresh design needed for schema, RPC, or persistence; the only genuinely new logic is the pinned-section list partition, which is trivial per section 3.
