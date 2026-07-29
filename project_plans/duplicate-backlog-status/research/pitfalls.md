# Pitfalls Research: Duplicate Backlog Status

Project: `duplicate-backlog-status`. Research scope: known pitfalls/risks specific to this
feature's exact shape, grounded in the current codebase (pre-implementation — none of
`duplicate_of_id`, `BacklogStatusDuplicate`, or `mark_duplicate` exist yet).

---

## 1. Self-referential FK-as-string-field pitfalls

`duplicate_of_id` is specified (FR2) as `field.String("duplicate_of_id").Optional()` on
`session/ent/schema/backlog_item.go` — a plain string, not an ent `edge.To(...)` targeting
`BacklogItem` itself. Confirmed by reading the current schema: the only self-referencing
mechanism available (`edge.To`/`edge.From`) is used for `item_sessions`, `sessions`,
`status_events`, and `source`, but none of those model a self-referential edge, and the
plan doesn't propose one either — a plain string was a deliberate choice (proto/backend
symmetry, avoids ent recursive-edge complexity for what's just a display-time lookup).

Risks this creates vs. a real edge:
- **No cascade-on-delete.** Ent edges support `.Comment()`/annotations for cascade
  behavior on the *edge* relationship; a plain string field gets none of that. If the
  canonical item is ever hard-deleted (there is no `DeleteBacklogItem` visible in
  `ent_repository_backlog.go` today, only soft-archive via `ArchiveBacklogItem`/status
  transition — so hard delete may not even exist as an operation), any duplicate's
  `duplicate_of_id` becomes a dangling pointer with no DB-level signal.
- **No DB-level referential integrity.** A raw string column has no FK constraint, so
  nothing at the SQL layer prevents `duplicate_of_id` from pointing at a nonexistent
  UUID, a malformed value, or (absent the FR3 guard) an id from a different domain
  entity entirely. All integrity enforcement is pushed to `TransitionGuard` at
  write-time only — there is no ongoing invariant enforced by the schema itself.
- **Orphaned references after archival.** Archiving a canonical item does not currently
  touch anything that points at it (`ArchiveBacklogItem` in `ent_repository_backlog.go`
  only sets `archived_at`/`status`/`user_modified_status_at` on the row being archived).
  So a canonical item can be archived (or, per FR1, could even itself be marked
  `duplicate` of some third item, since `TransitionGuard`'s existence check only forbids
  self-reference and nonexistence — it does not forbid the target being archived or
  itself a duplicate) while duplicates still point at it.

**AC11's mitigation is confirmed as the accepted, deliberate approach**: app-level
tolerance of dangling/stale references (client-side resolve → "item not found" text on
the web UI), not DB constraints. This is consistent with the Non-Goals section
("Merging content ... only the link is structural") and is a reasonable scope choice for
a first-class-status-not-full-relational-model feature.

**Subtlety worth flagging (not a blocker, but a real gap the ACs are silent on):**
- Nothing prevents chains (A duplicate-of B, B duplicate-of C) or cycles (A duplicate-of
  B, B duplicate-of A) since the guard only checks direct self-reference
  (`item_id == duplicate_of_id`) and existence, not transitive cycles. A 2-cycle is
  actually preventable for free once both transitions require the *current* status to
  be an active (non-duplicate-terminal) status — B can't be set duplicate-of A if B is
  already the target of A's `duplicate_of_id` only insofar as that's never checked; the
  guard as specified would allow it. This is low-severity (a UI/display oddity, not a
  data-loss risk) but worth one sentence in the plan as an accepted limitation, same
  tier as "no unlink-on-archive."
- The ACs don't call for archiving/re-statusing a canonical item to notify or unlink its
  duplicates, and this is explicitly out of scope per the Non-Goals wording ("only the
  link is structural, not a data merge"). Recommend the plan state this explicitly as an
  **accepted scope limitation**, not silently omit it — future backlog item candidate:
  "warn when archiving/duplicating an item that is itself a canonical target of other
  duplicates."

---

## 2. State machine / concurrency pitfalls

Confirmed by reading `session/ent_repository_backlog.go`:

- `UpdateBacklogItem` (line 183) and `TransitionBacklogItemStatus` (line 274) have the
  **identical** read-then-blind-write structure: both do `r.client.BacklogItem.Get(ctx,
  parsedID)` to fetch `current`, optionally check `precondition.ExpectedStatus`/
  `ExpectedUpdatedAt` against the *in-memory* `current` value, and then call
  `r.client.BacklogItem.UpdateOneID(parsedID)....Save(ctx)` with **no `.Where(...)`**
  tying the write back to the read. Both have the exact same TOCTOU race: a concurrent
  writer can land a change between the `Get` and the `Save`, and the second `Save` wins
  unconditionally, silently discarding the interleaved change.
- FR4/AC7 only mandate adding the `StatusEQ(current.Status)` optimistic-concurrency
  `.Where()` predicate to `TransitionBacklogItemStatus`. `UpdateBacklogItem` is not
  mentioned in any AC.

**Recommendation: stay scoped to AC7 / `TransitionBacklogItemStatus` only.** This
matches the literal wording of every AC (1–13), none of which touch `UpdateBacklogItem`,
and the requirements.md "Constraints" section explicitly frames the fix as "FR4 closes
this generally as part of adding the atomic status+duplicate_of_id write, not just for
the new field" — i.e. general **within the scope of the one method being changed**, not
a mandate to fix every method with the same shape. Silently fixing `UpdateBacklogItem`
in the same PR would be scope creep against a literal-AC-driven plan (risk: touches
code paths with no test coverage requirement in FR8, and no reviewer expectation of that
diff being in this PR). Silently *ignoring* it (not mentioning it at all) would also be
wrong — the same TOCTOU pattern already exists and will keep existing after this PR
ships, so a future edit to `UpdateBacklogItem` could easily race with a `duplicate`
transition landing via `TransitionBacklogItemStatus` on the same row (e.g. someone
calls `UpdateBacklogItem` to edit `notes` at the same moment `mark_duplicate` calls
`TransitionBacklogItemStatus` — the notes update has no status precondition, so it will
succeed and won't itself be racy against the *status* field, but this is exactly the
class of bug the new pattern is meant to prevent, just left unfixed in a sibling method).

**Action**: note the twin gap in `UpdateBacklogItem` explicitly in the plan doc as a
follow-up/backlog item (e.g. "apply the same `StatusEQ`-style optimistic-concurrency
predicate to `UpdateBacklogItem` for the fields it can race on — separate backlog item,
out of scope here"), rather than fixing it inline or leaving it unmentioned.

---

## 3. Ent codegen pitfalls (`--feature sql/upsert`)

Confirmed: `session/ent/generate.go` currently contains only:
```go
//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
```
And `session/ent_repository.go:1082` (`UpsertRule`) plus `session/repository.go:108` and
`session/storage.go:429` depend on `UpsertRule`/upsert-shaped methods existing on the
generated client.

**Why omitting the flag breaks things, and the failure mode:**
`--feature sql/upsert` is an ent codegen feature flag that generates the
`OnConflict(...)`/upsert builder methods (`ClientOption`s like `.OnConflict()`,
`UpsertOne`, bulk `UpsertBulk`, and per-entity `UpsertRule`-style convenience wrappers if
hand-written on top of them) as part of the generated `session/ent/*.go` output. If
codegen is run **without** the flag, the generator simply does not emit those upsert
builder types/methods at all — it's not that they're emitted broken, they don't exist in
the generated package.

**Failure mode is a compile error, not a silent runtime failure**: any hand-written code
in `session/ent_repository.go` (`UpsertRule` and similar) that calls into the missing
generated upsert API (e.g. `.OnConflict(...)`) will fail `go build`/`go vet` with
"undefined: ent.XxxUpsertOne" or "undefined method" style errors, because the symbols
genuinely don't exist in the freshly (re)generated `session/ent` package. Since
`session/ent/*.go` is gitignored and regenerated fresh in CI (confirmed:
`.github/workflows/lint.yml` line 62-63 runs
`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
as an explicit CI step before build/test), running the wrong (flagless) command locally
during implementation would make the local build fail loudly and immediately the moment
`UpsertRule` (or the new `duplicate_of_id` write path, if it ever used upsert semantics)
is compiled — there is no scenario where the flag is silently dropped and the bug ships,
because CI regenerates independently using the correct flagged command regardless of
what a contributor ran locally. The risk is purely a **local dev-loop friction/confusion
cost** (a contributor who forgets the flag gets a confusing "undefined" compile error
disconnected from the actual cause — "I didn't touch UpsertRule, why is it broken?"),
not a merge/production risk.

**Implication for this feature**: the new `duplicate_of_id` field itself doesn't need
upsert semantics (it's a plain optional string set via `SetDuplicateOfID(...)` on a
normal `UpdateOneID` builder, same as every other field in `UpdateBacklogItem`), so this
pitfall mostly matters as a "don't break the unrelated `UpsertRule` machinery while
regenerating for schema work" warning, not something specific to the new field's write
path.

---

## 4. Proto/codegen drift pitfalls

Checked `.github/workflows/*.yml` for `proto-gen`/`buf generate`/`generate-proto`:

- `.github/workflows/lint.yml` — runs `buf generate proto` (line 60) then
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  (line 63) as explicit steps before any build/lint runs, i.e. lint job **regenerates
  from source** rather than trusting checked-in output (nothing is checked in for either
  generator — both `session/ent/*.go` (excl. `schema/`) and proto bindings are
  gitignored, confirmed in requirements.md's Constraints section).
- `.github/workflows/build.yml` — has a dedicated job "Generate & Build Web UI" (line
  44) that runs proto generation once, uploads the generated files as an artifact
  (`generated-files`), and the downstream Test/Build-matrix jobs *download* that same
  artifact rather than regenerating themselves — so the whole build/test pipeline is
  fed by exactly one generation pass per CI run, always from the PR's actual proto
  source, never from a stale checked-in copy (there is no stale checked-in copy since
  it's gitignored).
- `.github/workflows/release.yml` and `benchmark.yml` also invoke `buf generate` /
  `buf generate proto` directly.

**Conclusion: there IS a CI safety net, and it's a strong one** — because generated
proto/ent output is gitignored (not committed), CI has no choice but to regenerate from
source every run; there is no "checked-in generated code that could drift from source
and go unnoticed" failure mode at all for this repo, unlike repos where generated code
is committed and CI only diff-checks it. The only drift risk is between a contributor's
**local** generated output (stale, or generated with a wrong flag/version) and what they
believe they tested — but that's a local-dev-loop concern that self-corrects the moment
CI runs, not a merge-time risk. No action needed beyond normal "regenerate locally before
testing" discipline already documented in CLAUDE.md and requirements.md.

---

## 5. Web UI pitfalls: 6-theme status badge tokens

Confirmed via `web-app/src/styles/theme.css.ts` and `theme-contract.css.ts`:

**The 6 themes are enumerated as `createTheme(vars, {...})` calls:**
`lightTheme` (line 55), `darkTheme` (149), `matrixTheme` (246), `cyberpunk77Theme` (351),
`wh40kTheme` (456), `cleanTheme` (561) — all in `web-app/src/styles/theme.css.ts`.

**Mechanism ensuring a new token gets a value in all 6 themes — confirmed to be a real
compile-time guard, not manual discipline, IF the token is added the same way existing
tokens are:** `theme-contract.css.ts`'s `vars = createThemeContract({...})` declares the
full token shape (e.g. the existing `statusBadge: { approvalBg: null, approvalFg: null,
... }` block at line 93, or top-level `color: { ... }` keys). Each `createTheme(vars,
{...})` call in `theme.css.ts` must supply a value for every **non-null** key declared
in the contract shape — vanilla-extract's `createTheme` is typed against the contract's
shape, so TypeScript raises a compile error if any of the 6 theme objects is missing a
key that the contract declares with a placeholder (`null` in the contract means "this
slot may be omitted / is supplied via `assignVars` elsewhere," a non-null placeholder
string means "required"). Adding a new `statusBadge.duplicateBg`/`duplicateFg`/
`duplicateBorder` (or similar) to the contract and setting a real value in only 5 of the
6 `createTheme(...)` calls in `theme.css.ts` **will fail `tsc`/`next build`** with a
missing-property error — this is the correct mechanism and it's automatic, *provided the
new tokens go into `theme-contract.css.ts` the same way the existing `statusBadge` block
does.*

**Real gap found, separate from the token-contract mechanism**: the CSS token coverage
is enforced by TypeScript, but **which CSS class a given status string maps to in the
badge component is NOT enforced by anything.** `web-app/src/components/backlog/
BacklogItemBadge.tsx` (lines 14-24) maps status → CSS class via:
```ts
const STATUS_CLASS: Record<string, string> = {
  idea: styles.statusIdea, refining: styles.statusRefining, ready: styles.statusReady,
  in_progress: styles.statusInProgress, review: styles.statusReview, done: styles.statusDone,
  archived: styles.statusArchived,
};
const getStatusClass = (s: string): string => STATUS_CLASS[s] ?? styles.statusArchived;
```
`STATUS_CLASS` is typed `Record<string, string>` (a loose string-keyed map, not a
discriminated union keyed by a `BacklogStatus` literal type), and the fallback for any
unrecognized status key is **`styles.statusArchived`**. This means: if the
`duplicate: styles.statusDuplicate` entry is *not* added to `STATUS_CLASS`, the badge
will render with the archived style, silently — no TypeScript error (the map accepts any
string key), no lint error, no runtime warning. The only thing that would catch this
omission is the FR8/AC12-mandated Jest test asserting badge rendering for every status
including `duplicate` — which makes that specific test load-bearing, not just
nice-to-have. Recommend the plan/implementation explicitly call out
`STATUS_CLASS['duplicate']` as a required edit (not just "add CSS token to the contract")
and treat the badge-rendering test as a hard gate, since the failure mode here is "looks
fine, silently wrong," the same class of bug ADR-009/the CSS architecture rules exist to
prevent for hex/`var()` misuse but don't cover this particular string-map fallback
pattern.

Also confirmed: `web-app/scripts/check-css-vars.mjs` (the `lint:css-vars` script) only
scans `.module.css` files for undefined `var(--xxx)` references — it does **not** touch
`.css.ts` files at all, so it provides zero coverage for vanilla-extract token misuse;
the only guard for `.css.ts` correctness is TypeScript itself plus `stylelint` (which
also targets `.css`, not `.css.ts`). This is consistent with ADR-009's intent (vanilla-
extract's own type system replaces the need for the custom lint script on new code) but
worth stating explicitly so the fix doesn't rely on `npm run lint:css`/`lint:css-vars`
catching a missing-theme-value mistake in `theme.css.ts` — only `tsc`/`next build` will.

---

## 6. MCP tool pitfalls: `mark_duplicate` not-found vs. infra-error disambiguation

Confirmed pattern from `server/mcp/tools_backlog.go`'s existing handlers (`getBacklogItem`
line 71-89, `reportProgress`, `requestReview`, `submitReviewVerdict`, `submitTriageResult`):
every handler that fetches an entity distinguishes not-found from infra errors via
`errors.Is(err, session.ErrNotFound)`:
```go
if errors.Is(err, session.ErrNotFound) {
    return errResult(ErrItemNotFound, fmt.Sprintf("backlog item %q not found", itemID), ""), nil
}
return errResult(ErrInternalError, fmt.Sprintf("get backlog item: %v", err), ""), nil
```
This works because `EntRepository` methods (confirmed in `ent_repository_backlog.go`)
wrap not-found conditions as `fmt.Errorf("%w: ...", ErrNotFound, ...)` using `ent.IsNotFound(err)`
checks internally, so the repository layer already normalizes ent's not-found sentinel
into the package-level `session.ErrNotFound` that MCP handlers check with `errors.Is`.

**Pitfalls for `mark_duplicate` specifically, given it has *two* IDs to validate
(`item_id` and `duplicate_of_id`), not one:**
- **Must disambiguate which ID was not found, not just "not found."** A single generic
  "not found" error message (as `getBacklogItem` uses, since it only has one ID) would
  be ambiguous for `mark_duplicate` — the caller needs to know whether `item_id` or
  `duplicate_of_id` was the bad one, both to fix their own tool call and per AC8's
  "not-found vs. infra-error disambiguation" wording (which is about not-found-vs-infra,
  but a sloppy implementation could accidentally conflate "duplicate_of_id not found"
  with "internal error" if it only wraps the *guard's* sentinel errors and not a genuine
  lookup failure — see next point).
- **`item_id == duplicate_of_id` (self-reference) must be a `TransitionGuard`
  validation error (user input error), not a not-found or infra error.** Per FR3, this
  produces one of three *new sentinel errors* (empty/self-ref/nonexistent), which are
  business-rule violations, not "not found" — they should map to `ErrInvalidArgument`
  (matching the pattern other handlers use for bad user input, e.g.
  `errResult(ErrInvalidArgument, ...)` throughout the file for validation failures), not
  `ErrItemNotFound` or `ErrInternalError`. The FR3 sentinel for "references a nonexistent
  item" is a special case: semantically it's a not-found (the target doesn't exist), but
  it's discovered as a *guard* rejection (a business rule violation on the transition
  itself, using data supplied by the caller), not a repository lookup failure on the
  primary entity — this needs a clear decision in the plan for which MCP error code it
  maps to (`ErrItemNotFound` for consistency with "an id doesn't exist" semantics, vs.
  `ErrInvalidArgument` for consistency with "this is a guard/validation rejection").
  Recommend `ErrItemNotFound` (or a more specific `ErrInvalidArgument` with a hint
  clarifying which field), since the underlying cause — the referenced id doesn't
  exist — is the same shape of problem as `getBacklogItem`'s not-found case, and a bad
  `duplicate_of_id` is exactly as much "user gave a bad id" as a bad `item_id` would be.
- **Risk of leaking DB errors as user errors, or vice versa, specifically at the
  existence-check step for `duplicate_of_id`.** FR4 says the *service layer*
  (`BacklogService.TransitionBacklogItemStatus`) "looks up the target item ... to feed
  `TransitionGuard` existence-checking" — i.e. a second `Get`-style repository call is
  needed for `duplicate_of_id`, separate from the lookup already done for `item_id`.
  Whatever error that second lookup returns must go through the same
  `errors.Is(err, session.ErrNotFound)` → guard-sentinel-or-`ErrItemNotFound` path,
  **not** be treated as `ErrInternalError` by default just because it's an "extra"
  lookup the existing single-entity handlers don't have to do. Concretely: if the
  implementation writes a naive `if err != nil { return errResult(ErrInternalError,
  ...) }` for the `duplicate_of_id` lookup (forgetting the `errors.Is` check that every
  other handler in the file remembers to do for its *primary* id), a plain bad
  `duplicate_of_id` from a caller would incorrectly surface as an internal/infra error
  instead of a clear "duplicate_of_id not found" user error — this is the specific
  regression risk AC8 is guarding against, and it's easy to introduce precisely because
  `duplicate_of_id`'s existence check is a *new*, second lookup bolted onto a handler
  pattern that every existing tool in the file only had to apply once.
- **Best-effort note append (FR5) must not be conflated with the transition's own
  error handling.** The note-append failure path is explicitly required to not fail the
  overall tool call — this must be implemented as a separate try/log-and-continue step
  *after* the transition has already succeeded and been reported, structurally isolated
  from the `errResult(...)` return paths used for the transition/guard failures, or a
  bug could cause a successful transition to be reported as a failure (or vice versa)
  if the two error-handling paths are merged.

---

## Summary of concrete file/line references for implementers

| Area | File | Lines/Symbol |
|---|---|---|
| Twin TOCTOU gap | `session/ent_repository_backlog.go` | `UpdateBacklogItem` (183), `TransitionBacklogItemStatus` (274) |
| Ent generate directive | `session/ent/generate.go` | `//go:generate ... --feature sql/upsert ./schema` |
| Upsert dependents | `session/ent_repository.go:1082`, `session/repository.go:108`, `session/storage.go:429` | `UpsertRule` |
| CI proto/ent regen (safety net) | `.github/workflows/lint.yml` | lines 60, 63 |
| CI proto regen (build path) | `.github/workflows/build.yml` | "Generate & Build Web UI" job (~line 44), artifact `generated-files` |
| Theme contract | `web-app/src/styles/theme-contract.css.ts` | `statusBadge` block (93-113) — pattern to extend |
| 6 themes | `web-app/src/styles/theme.css.ts` | `lightTheme`(55) `darkTheme`(149) `matrixTheme`(246) `cyberpunk77Theme`(351) `wh40kTheme`(456) `cleanTheme`(561) |
| Badge status→class map (silent-fallback risk) | `web-app/src/components/backlog/BacklogItemBadge.tsx` | `STATUS_CLASS` (14-22), `getStatusClass` (24) |
| CSS var lint (does NOT cover .css.ts) | `web-app/scripts/check-css-vars.mjs` | scans only `*.module.css` |
| Existing not-found/infra disambiguation pattern | `server/mcp/tools_backlog.go` | `getBacklogItem` (71-89), repeated in `reportProgress`/`requestReview`/`submitReviewVerdict`/`submitTriageResult` |
| Ent schema (no self-edge today) | `session/ent/schema/backlog_item.go` | `Edges()` (73), `Indexes()` (86) |
