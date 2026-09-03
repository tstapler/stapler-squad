# Research: Pitfalls for Session Pinning

Scope: known trouble spots specific to this codebase for "add a persisted boolean
flag to an existing high-traffic entity + RPC toggle + list re-render."

## 1. ent generate — `--feature sql/upsert` is required, and it does matter for a plain bool add

`session/ent/generate.go`'s `//go:generate` directive is:
```go
//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
```
Confirmed the flag is not upsert-specific in effect on the generated package as a whole —
`--feature sql/upsert` changes what ent emits into the generated client (adds
`OnConflict`/upsert builder methods used elsewhere, e.g. `UpsertRule`). Running plain
`ent generate` (no flag) regenerates the *entire* `session/ent/` package, including the
new `pinned` field's accessors, but silently drops the upsert builder methods other
schema types depend on — the build still compiles (Go doesn't know a method used to
exist), so this fails at the call site of `UpsertRule`-style methods, not at `go build`
of the ent package itself. **Action: always run the exact `go generate ./session/ent/`
or the documented command — never the bare `ent generate ./session/ent/schema` even
though it looks sufficient for "just add a bool field."**

Also required: `-mod=mod` (allows go.mod graph updates during generation).

## 2. Double-checked-locking cache-slot bug (`.claude/rules/go-double-checked-locking.md`) — does NOT apply to `pinned` as currently scoped

`session/git/worktree_git.go`'s `IsDirty`/`IsDirtyWithHint` (`worktree_git.go:203-227`) is
the canonical example: a TTL'd cache computed off a subprocess call, where the bug class
is "recompute under lock, then return the cache slot instead of the just-computed value."

`pinned` is a plain persisted boolean, not a lazily-computed/derived value — it has no
"read cache, else expensively compute" path. The existing precedent for boolean mutation
on `Instance` is `SetArchivedAt`/`ArchiveWithStop`/`SetArchivedAtIfNilAndStop`
(`session/instance_actor_setters.go:212-282`), which are all actor-routed
(`i.sendSyncErr(...)`), mutate `s.inst.mu`-protected state directly, rebuild the
snapshot, and return either nothing or a `bool` indicating whether *this* call's CAS won
— never a value re-read from the struct after the lock. **A `SetPinned(v bool)` built the
same way (unconditional `sendSyncErr` + direct field set, no CAS) does not have the bug
surface described in the rule at all — CAS-with-return-the-computed-value only becomes a
risk if a future revision adds a "pin with idempotency check" variant that returns
whether the value changed; if so, mirror `SetArchivedAtIfNil`'s pattern (return the
locally-observed `set` bool, never re-read `s.inst.Pinned` after unlocking).**

## 3. Registry applicability — requirements.md's reasoning holds, confirmed

- **Session-creation registry (7 touchpoints,** `.claude/rules/session-creation-registry.md`**): does NOT apply.**
  Verified: that registry governs `SessionType`/`CreateSessionRequest` — a *new session*
  being created in a new mode. Pin/unpin mutates an *existing* session's persisted state,
  structurally identical to `ArchiveSession`/`UnarchiveSession`, which are not part of
  that registry either. No proto `SessionType` enum work, no `Omnibar.tsx` `sessionType`
  union work, no `OmnibarCreationPanel.tsx` `SESSION_TYPES` entry.
- **Frontend testing registries (**`.claude/rules/feature-testing-registry.md`**): neither applies.**
  - `OmnibarAction` union (`web-app/src/lib/omnibar/actions/types.ts` + `dispatch.ts`) is
    for user-triggerable actions *dispatched through the omnibar* (navigate, create
    session, pause, etc.). Pin/unpin is a session-card/detail-header action, not an
    omnibar-typed input action — same category as archive/hide, which also do not have
    `OmnibarAction` entries.
  - `DetectorRegistry` (`detector.ts`) is for auto-detecting input *patterns typed into
    the omnibar* (URLs, shorthand). Pinning has no textual input pattern to detect.
  - Confirmed by absence: grepping `SessionService.ArchiveSession`/`UnarchiveSession`
    turns up no `OmnibarAction`/`Detector` registration anywhere — they're the direct
    structural precedent and they don't touch either registry.

## 4. `feature-registry.md` — what breaks in CI if per-feature JSON files aren't added

Two independent CI gates depend on the registry, found in `.github/workflows/`:
- `.github/workflows/registry-validation.yml:40-43` runs `make registry-generate` (fails
  if generation itself errors — unlikely for this feature).
- `.github/workflows/build.yml:225-228` is the actual enforcement: it runs
  `make registry-generate` then `git diff --exit-code docs/registry/features/`, printing
  `"Registry out of date — run: make registry-generate"` and failing the build if the
  working tree changed. **Concretely: if `PinSession`/`UnpinSession` RPC handlers land
  without a `// +api: session:pin` / `session:unpin` marker (or without hand-added
  per-feature JSON files under `docs/registry/features/backend/`), `make
  registry-generate` run in CI will produce new/changed files that aren't committed, and
  `build.yml`'s `git diff --exit-code` step fails the whole build.** Same mechanism for
  any new frontend feature file if a `// +feature:` marker is added to a new pinned-
  section component. Also feeds `docs/registry/coverage-gaps.json` — the feature-registry
  rule's own instruction to check the gap count doesn't grow is enforced by this same
  diff-clean requirement, since gaps.json is a generated aggregate.

## 5. Concurrency: last-write-wins is the existing pattern, and it's adequate here

`ArchiveSession`/`UnarchiveSession` (`server/services/session_service.go:4283-4327`) do no
conflict detection: `FindLiveInstance` → mutate → `SaveInstances`. Two browser tabs
racing to archive/unarchive the same session get plain last-write-wins — the actor
(`sendSyncErr`) only guarantees the *in-memory* mutation is atomic/serialized per
`Instance`, not that concurrent RPCs produce a deterministic ordering across two HTTP
requests. There is no ETag/version field or optimistic-concurrency check anywhere in this
RPC family. **`PinSession`/`UnpinSession` should follow the identical LWW pattern** — no
new conflict-detection mechanism is warranted; anything else would be inconsistent with
every other boolean toggle in the codebase (`hidden`, `archived_at`, `auto_yes`,
`is_expanded`) and add real complexity for a UX action where LWW is harmless (worst case:
a rapid double-toggle from two tabs settles on whichever request's `SaveInstances` call
lands last, and a page reload/live update immediately shows the true state).

## 6. Migration/backfill: none needed, confirmed by existing `hidden` field precedent

DB schema application uses ent's `client.Schema.Create(ctx)` (`session/ent_repository.go:93`,
plain auto-migration, no manual `.sql` migration files in this repo). `hidden` — the
closest existing precedent, added the same way (`field.Bool("hidden").Default(false)`,
`session/ent/schema/session.go:102-104`) — required no backfill script anywhere in the
codebase (none found under any migration/backfill path). SQLite's `ALTER TABLE ... ADD
COLUMN ... DEFAULT 0` (what ent's auto-migrate emits for a new `Bool.Default(false)`
column) populates existing rows with the default at the DDL level — no explicit backfill
step needed for `pinned` either. **One thing to double check in implementation, not
found in this research pass: whether `ent_repository.go`'s hand-written `Create`/`Update`
helper functions (`sessionCreate.SetHidden(data.Hidden)` at `ent_repository.go:216`,
`sessionUpdate.SetHidden(data.Hidden)` at `ent_repository.go:444`) need a matching
`SetPinned(data.Pinned)` call added by hand** — ent's generated builders don't
auto-populate from the Go struct; every existing boolean field has an explicit
`Set<Field>()` call at both those call sites, and `pinned` will need the same two lines
or new instances will silently persist the SQL default instead of the constructor's
value.

## 7. Proto field number — requirements.md's "63 is highest" is now stale; correct next number is 72

`proto/session/v1/types.proto` currently goes up to field `71` (`workspace_key = 71`,
line 239), not `63` as requirements.md states (`archived_at = 63` was highest at some
earlier point in the repo's history, evidently — several fields have been added since:
`workflow_name=64`, `autonomous_turn=65`, `autonomous_max_turns=66`, `autonomous_outcome=67`,
`detected_status=68`, `detected_context=69`, `artifacts=70`, `workspace_key=71`).
**Correction for the plan phase: the new `pinned` field must use `72`, not `64`.** Flag
this back to requirements/plan — using a stale field number would either collide with an
already-assigned field or force a renumber later.

## 8. "Pin to top" UX pitfalls (general, not codebase-specific — flag for plan/UX phase)

- **Pin sprawl**: no server-side cap on pinned-session count anywhere precedented in this
  codebase (no analogous cap exists for `hidden` or `archived_at` either). If the pinned
  section is meant to stay small/high-signal, either accept unbounded sprawl (matches
  existing conventions — no other boolean flag in this schema is capped) or explicitly
  scope a limit as a requirements decision; there's no reference implementation in-repo
  to follow either way.
- **Pinned + archived conflict**: requirements.md marks "pinning archived sessions" as
  out-of-scope/TBD. Structurally, `pinned` and `archived_at` are independent booleans/
  fields on the same entity with no repo-level invariant enforcing exclusivity (compare:
  `hidden` and `archived_at` already coexist independently — a session can be both
  hidden and archived simultaneously, per the `if inst.Hidden && !req.Msg.IncludeHidden`
  / archived-filter checks being separate conditionals in `ListSessions`,
  `session_service.go:1140,1180`). Recommend the same approach: let them coexist as
  independent flags, and decide only the *rendering* rule (e.g. "pinned section excludes
  archived sessions unless `include_archived` is also set") rather than a hard schema
  constraint — consistent with how `hidden`/`archived_at` interact today.
- **Stale pinned sessions**: no existing precedent for "auto-expire" a persisted user
  preference flag in this codebase (`auto_yes`, `is_expanded`, `hidden` are all sticky
  until manually toggled) — an unpin-after-N-days affordance would be a new pattern, not
  a mirrored one. If wanted, treat as a separate, explicitly-scoped decision rather than
  default behavior.
