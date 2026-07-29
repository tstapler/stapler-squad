# Build vs. Buy — Duplicate Backlog Status

Scope reminder: this feature is a new `BacklogStatus` enum value (or link field), a
nullable string field for the linked-item ID, a validation guard, an MCP tool, list
filtering, and a UI badge/link. It is explicitly NOT a general "duplicate detection"
system — no fuzzy matching, no ML similarity search. Every recommendation below is
"do the small, boring, consistent thing," not "adopt a new dependency."

## 1. FSM library vs. existing hand-rolled transition table

**Verdict: keep the existing bespoke `map[BacklogStatus]map[BacklogStatus]bool` in
`session/backlog.go`. Do not add an FSM library.**

- Confirmed via `grep -i "fsm|stateless|statemachine" go.mod`: **no FSM library is
  currently a dependency** (no `looplab/fsm`, `qmuntal/stateless`, or similar).
- The existing state machine (`session/backlog.go:121-160`) is ~40 lines: a
  `validTransitions` map plus a single `CanTransitionBacklog(from, to)` guard
  function. It already handles 7 states (`idea`, `refining`, `ready`, `in_progress`,
  `review`, `done`, `archived`) with asymmetric transitions (e.g. `archived` can only
  go back to `idea`).
- Adding a "duplicate" status/link is additive to this same table (one more enum
  value and a couple of map entries, or a link field with no transition table at
  all if it's an orthogonal marker rather than a status). Pulling in an FSM library
  for one more enum value would:
  - add a new dependency for a problem already solved in-repo,
  - force callers to learn a second, unrelated state-machine API/DSL only for this
    one feature,
  - buy no meaningful capability (no need for hierarchical states, guards with side
    effects, event-driven callbacks, or persistence hooks — this is a plain
    admission-into-set check).
- **Recommendation: extend `validTransitions` and `CanTransitionBacklog` in place**,
  exactly as `archived` was added, and skip any FSM library evaluation beyond this
  confirmation.

## 2. Hosted/SaaS "duplicate management" service

**Verdict: no — dismissed immediately, not worth further evaluation.**

- stapler-squad is a self-hosted, single-tenant internal tool persisting to its own
  Postgres/SQLite via ent (`session/ent/`). There is no multi-tenant data boundary,
  no external customer-facing dedup workflow, and no volume of duplicate records
  that would justify an external service.
- The "duplicate" concept here is a single nullable string field (a link to another
  backlog item's ID) set explicitly by a human/agent after manual judgment — this is
  a foreign-key-shaped field, not a dataset requiring managed deduplication
  infrastructure (e.g. no address/entity-resolution SaaS like Dedupe.io, Melissa, or
  a vendor's record-linkage API would ever be reached for from inside this
  self-hosted app).
- No further build-vs-buy analysis needed on this axis.

## 3. Optimistic concurrency: ent generated code vs. hand-rolled locking

**Verdict: reuse ent's existing generated update/precondition machinery. Do not
invent a new locking scheme.**

Actual pattern found in `session/ent_repository_backlog.go` (both
`UpdateBacklogItem` ~line 183 and `TransitionBacklogItemStatus` ~line 274):

```go
current, err := r.client.BacklogItem.Get(ctx, parsedID)
// ...
if precondition != nil {
    if precondition.ExpectedStatus != "" && current.Status != precondition.ExpectedStatus {
        return nil, fmt.Errorf("%w: expected status %q, got %q", ErrPreconditionFailed, ...)
    }
    // ExpectedUpdatedAt check similarly
}
u := r.client.BacklogItem.UpdateOneID(parsedID)
// ... field setters ... u.Save(ctx)
```

Note: this is a **Get-then-compare-then-Update** pattern using
`BacklogItemPrecondition{ExpectedStatus, ExpectedUpdatedAt}`
(`session/repository.go:302-308`) and the sentinel `ErrPreconditionFailed`
(`session/repository.go:11-12`) — not literally a `.Where(StatusEQ(...))` predicate
chained onto the update builder with a rows-affected check. Both are legitimate
optimistic-concurrency shapes; the important point is that **this repo already has
one canonical, tested implementation of it**, and the new duplicate-link feature's
"is this transition/update still valid" check is exactly the same shape of problem
(check current state, only apply if still what the caller expected).

- **Recommendation:** the duplicate-link/status change should thread through the
  existing `BacklogItemUpdate` / `BacklogItemPrecondition` / `UpdateBacklogItem` (or
  `TransitionBacklogItemStatus`) path rather than adding a bespoke
  SELECT-then-manual-version-column scheme, a new mutex, or a hand-rolled
  compare-and-swap. ent's generated `UpdateOneID(...).Save(ctx)` plus the existing
  precondition struct is the right and only tool needed — there is no new algorithm
  here, just one more field/status value flowing through code that already exists.

## 4. Fork/model on existing precedent

**Verdict: model tightly on the `archived` status precedent in `session/backlog.go`,
not on the `one_off` session-type precedent.**

- `archived` (`session/backlog.go:18,125,129,134,146,148`) is the closer analog:
  it's a same-file addition to the `BacklogStatus` enum plus a couple of entries in
  `validTransitions`, with no proto/enum-across-layers fan-out. A "duplicate" status
  (or a duplicate-link field alongside existing statuses) is the same shape of
  change — contained to `session/backlog.go`, `session/repository.go` (field on the
  update struct), the ent schema, and then MCP tool / list filter / UI badge on top.
- The `one_off` session-type precedent (`.claude/rules/session-creation-registry.md`,
  `session/namegen/`) is a **different kind of feature** — it required touching 7
  registry touchpoints (proto enum, proto request field, Go handler switch, Go
  session-type constant, three frontend files) because session *type* has a much
  larger surface (worktree lifecycle, path resolution, creation UI). Backlog
  *status* does not have that fan-out — it's a single Go enum consumed by a
  transition-table guard, an ent field, and read/filtered by MCP tools and the UI
  list. Following the 7-touchpoint registry here would be over-engineering; it does
  not apply to this feature.
- **Recommendation:** use `archived`'s exact footprint as the template — same enum
  block, same map-literal transition entries, same "guard function checks map
  membership" pattern — and reuse the existing `BacklogItemPrecondition` plumbing
  (point 3) for anything touching status transitions.

## 5. Frontend contrast tooling for new status color tokens

**Verdict: reuse the existing bespoke contrast checker; no new library needed.**

- `web-app/package.json` has no contrast/color library dependency
  (`wcag-contrast`, `polished`, `chroma-js`, `color2k` — none present).
- The repo already has a **hand-rolled WCAG AA contrast checker**:
  `web-app/scripts/check-theme-contrast.ts`, run via `npm run check-contrast`. It
  implements relative-luminance/contrast-ratio math directly (no library) against
  hardcoded per-theme color tables and asserts `WCAG_AA_NORMAL = 4.5` /
  `WCAG_AA_LARGE = 3.0` ratios, failing the script (exit) if any pair is under
  threshold.
- Existing token additions in `web-app/src/styles/theme.css.ts` follow a
  consistent, already-established convention: **each token that was tuned for
  contrast carries an inline comment recording the before/after ratio**, e.g.:
  ```ts
  textTertiary: "#767676", /* was #9ca3af — 2.53:1 fails WCAG AA on white; #767676 = 4.55:1 ✅ */
  textMuted: "#00b32d", /* was #004d18 — 1.32:1 fails WCAG AA; #00b32d = 5.12:1 on #0a0a0a ✅ */
  ```
- **Recommendation:** when adding new duplicate-status badge/link color tokens
  across the themes, (a) add the new colors to
  `web-app/scripts/check-theme-contrast.ts`'s `ThemeColors`/`themes` tables so they
  get checked, (b) run `npm run check-contrast` before picking final values, and
  (c) annotate the chosen hex values in `theme.css.ts` with the same
  `/* was X — ratio fails; new = ratio ✅ */` comment style already used throughout
  the file. No new dependency (chroma-js, polished, etc.) is warranted — the
  existing lightweight script already does exactly what's needed and is already
  wired into the repo's conventions.

## Summary

Every axis points the same direction: **build small, and build it exactly like the
nearest existing precedent already in this repo.** No new dependency (FSM library,
contrast library, SaaS dedup service) is justified anywhere in this feature's scope.
The only "reuse existing machinery, don't hand-roll" call is ent's own generated
update/precondition path (point 3), which the repo already uses correctly for
backlog status transitions.
