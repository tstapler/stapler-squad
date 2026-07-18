# Backlog Service Refactor — Pitfalls Research

Codebase investigated: `/home/tstapler/Programming/stapler-squad`
Scope: `server/services/backlog_service.go` (2570 lines), `server/mcp/tools_backlog.go`,
`session/backlog_lifecycle.go`, `session/` package.

---

## P1 — Compile-time regression during file split (MEDIUM risk)

**Finding**: Splitting `backlog_service.go` across multiple files within the same
`package services` directory is safe — the Go compiler treats all `*.go` files in a
directory as a single compilation unit regardless of how many files they span. Callers
outside the package see no change; there is no "recompile all callers" issue.

**The real risk**: mid-PR breakage if you move a symbol to a new file before updating
every reference in the same PR. Because the package builds as a whole, a partial
move (symbol defined in the new file but not yet referenced by the right build tag,
or a typo in the new file that prevents the whole package from compiling) fails all
tests in `server/services` until the PR is complete. Strategy: keep each file-split
commit green by moving one cohesive group at a time and running `make build` after
each group.

**Package-level vars check**: `backlog_service.go` has no package-level `var` blocks
and no `init()` function. All mutable state lives on `*BacklogService` struct fields.
File-level constants (`headlessTriageUUIDPrefix`, `maxAutoReworkIterations`,
`defaultTriageCleanupTimeout`, etc.) can be split to any file without ordering issues.

**Verdict**: splitting within the same package is safe; the only risk is an incomplete
commit that leaves the package uncompilable. Mitigate with small, atomic commits.

---

## P5 — Import cycle when introducing `session/domain` sub-package (HIGH risk)

**Finding**: This is the single most dangerous structural risk. The existing sub-packages
under `session/` — `session/search`, `session/workspace`, `session/framebuffer` — all
import the parent `session` package (`github.com/tstapler/stapler-squad/session`). If
the proposed `session/domain` package follows the same pattern and imports `session`, no
cycle exists. But if `session` imports `session/domain` (to move types like
`BacklogItemData`, `AcCriterion`, `BacklogStatus`, `AcCriteriaJSON` there), while any of
those types reference types still living in `session`, the cycle is immediate and the
build fails.

**Concrete evidence**: `session/search/engine.go` and `session/workspace/types.go` both
import `session` and compile successfully today because `session` does NOT import them
back. The moment `session` imports `session/domain`, any file in `session/domain` that
references a `session.*` type creates `session → session/domain → session`.

**Domain types at risk**: `BacklogItemData` (in `session/repository.go`) references
`AcCriteriaJSON` (in `session/backlog.go`) and `BacklogStatus`. `AcCriterion` is also
in `session/backlog.go`. If moved to `session/domain`, the new package must be
self-contained — no imports of `session` itself.

**Pre-move detection**: Before moving any type, run:
```bash
go list -f '{{join .Imports "\n"}}' github.com/tstapler/stapler-squad/session
```
to see the current import set, then confirm the planned `session/domain` types don't
transitively depend on anything that stays in `session`. Use `go build ./session/domain`
immediately after creating the package to catch cycles before any callers are updated.

**Sub-package import direction rule**: `session/domain` may import nothing from
`github.com/tstapler/stapler-squad/session`. `session` may import `session/domain`.
Callers in `server/services` must update their import to `session/domain` for the types
they use. The existing pattern (`session/search`, `session/workspace`) proves the
opposite direction — sub-package imports parent — works, but that is the wrong
direction for a shared domain layer.

---

## P4 — ent partial hydration / nil panic from Select() (LOW risk, currently)

**Finding**: The codebase does NOT currently use ent's `Select(field...)` API to
partially hydrate `BacklogItem` or related entities. All queries in
`session/ent_repository_backlog.go` load full entities. The proposed
`BacklogItemSummary` struct (for list views) would introduce `Select()` for the first
time in this subsystem.

**What ent does on unselected fields**: When `Select()` is used, unselected fields take
their zero value. For string fields this is `""`. For pointer fields it is `nil`. For
edge fields (relations loaded via `With*()`), the edge slice is `nil`. Accessing a
nil edge slice is safe (range over nil is a no-op), but calling `.Unwrap()` or
accessing edges off an edge struct that wasn't loaded panics.

**Known ent hazard**: `ent.BacklogItem.Edges.Sessions` is nil if `WithItemSessions()`
was not included in the query. Functions like `backlogItemToProto` in
`backlog_service.go` (line 466) call `item.Edges.Sessions` unconditionally. If the
partially-hydrated `BacklogItem` ever flows into `backlogItemToProto`, it panics.

**Safe pattern**: Do not pass a partially-hydrated `*ent.BacklogItem` to any function
that assumes full hydration. Introduce a typed distinction:
- `*ent.BacklogItem` — full entity, all edges loaded (existing callers)
- `BacklogItemSummary` — a separate Go struct with only the subset of fields, populated
  manually from a `Select()` query

Never widen a `Select()` query's result into a `*ent.BacklogItem` and pass it to
existing functions. Keep the summary type separate and concrete.

**Detection**: Before shipping, grep for any place a `Select()`-built query result
is passed to a function expecting `*ent.BacklogItem`:
```bash
grep -n "\.Select(" session/ent_repository_backlog.go
```
Currently returns zero hits — the baseline is clean.

---

## P6 — ent error type leakage across the Storage interface boundary (HIGH risk)

**Finding**: `ent.IsNotFound(err)` appears 14 times in `backlog_service.go` and
throughout `session/ent_repository_backlog.go`. This is a concrete ent ORM call leaking
through the `session.Storage` abstraction layer. The service package imports
`"github.com/tstapler/stapler-squad/session/ent"` explicitly because of it.

**Current state is inconsistent**: Some call sites already use the correct sentinel:
```
// backlog_service.go line 677
if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
```
Others use only `ent.IsNotFound(err)` with no sentinel fallback (lines 889, 1023,
1145, 1167). The Storage/EntRepository layer in `ent_repository_backlog.go` wraps
`ent.IsNotFound` correctly in some places (lines 130, 198, 276, etc.) but the
service reaches past the abstraction anyway.

**The correct abstraction**: `session.ErrNotFound` (`var ErrNotFound = errors.New("not
found")` in `session/repository.go`) is the intended sentinel. The ent repository
layer should translate `ent.IsNotFound` → `session.ErrNotFound` before returning
errors to callers. The service layer should only use `errors.Is(err, session.ErrNotFound)`.

**Risk during refactor**: If any of the 6 planned refactor items move functions from
`server/services` to a new file or package, the `ent.IsNotFound` calls must be audited.
If any extracted code moves to `session/` (e.g. `ReviewGateRunner`) it must NOT import
`session/ent` for error checking — it should use `session.ErrNotFound` only.

**Remediation before split**: In each function being extracted, replace every
`ent.IsNotFound(err)` with `errors.Is(err, session.ErrNotFound)` and verify the
repository layer already wraps the ent error. This lets the `session/ent` import be
dropped from `backlog_service.go` entirely after the refactor.

---

## P-MUTEX — Goroutine and mutex ownership (SAFE for file split, DANGEROUS for package split)

**Finding**: `BacklogService` holds two concurrency primitives:
- `worktreeMu sync.Mutex` (line 105) — used in `SpawnSessionFromItem` (line 1330) and
  `AttachSessionToItem` (line 1626). Both functions live in `backlog_service.go` and
  reference `s.worktreeMu` via the receiver.
- `triageSem chan struct{}` and `shutdownCtx`/`shutdownCancel` — used exclusively in
  `TriggerTriage` (lines 1798–1835) and `Shutdown` (line 206).

**File split (same package)**: Zero risk. All files in `package services` share the
same `BacklogService` type definition. `s.worktreeMu.Lock()` in one file and the
struct field definition in another file compile as a single unit.

**Package split (if any)**: Extracting `ReviewGateRunner` from `session/backlog_lifecycle.go`
as a new struct is safe as long as the mutex lives on `BacklogLifecycleListener` (which
it already does, as `mu sync.RWMutex` in that struct). The proposed extraction should
keep `ReviewGateRunner` as a standalone struct with its own lock, not sharing
`BacklogLifecycleListener`'s mutex.

---

## P-TESTS — Test helpers that reference package internals (CONCRETE RISK)

**Finding**: All four backlog test files (`backlog_service_test.go`,
`backlog_triage_harness_test.go`, `backlog_github_rpc_test.go`,
`backlog_service_encryption_test.go`) use `package services` (not `package services_test`).
They directly call unexported functions:

- `itemSessionToProto` — called at lines 1078 and 1116 of `backlog_service_test.go`
- `itemSourceBackend` interface — used in `backlog_service_encryption_test.go` line 18
  to construct a `BacklogService` literal with unexported field access

**Risk**: If `itemSessionToProto` moves to a new file `backlog_query.go`, the tests
continue to work because the package is unchanged. But if it moves to a new package
(e.g. as an exported converter), the test file can no longer call it without an import.

**Risk for `itemSourceBackend` interface**: This interface is currently unexported.
`backlog_service_encryption_test.go` constructs `&BacklogService{sourceBackend: backend}`
using a struct literal with named fields — this works only because the test is in
`package services`. If `BacklogService` moves to any new package or if `sourceBackend`
field access is removed, this test breaks.

**Mitigation**: Before moving any unexported symbol that tests call directly, check
with:
```bash
grep -rn "itemSessionToProto\|backlogItemToProto\|itemSourceBackend\|acCriteriaToJSON\|slugify\|triageShortTitle" \
  server/services/*_test.go
```
Either export the symbol (if it belongs in the API surface), keep it in the original
package, or move the test that uses it alongside it.

---

## P-GOWORK — Go module / go.work considerations (NO RISK)

**Finding**: No `go.work` file exists at the repo root. This is a standard single-module
Go project (`module github.com/tstapler/stapler-squad`, Go 1.25.0). Creating
`session/domain` as a sub-directory with its own `package domain` declaration is a
standard sub-package, not a separate module. No `go.mod` is needed in `session/domain`.
The only consideration is that `internal/` enforcement does not apply here — sub-packages
of `session/` are visible to any package in the module.

---

## P-INIT — Package-level vars and init() (NO RISK)

**Finding**: `backlog_service.go` has no `var` blocks and no `init()` function.
Package-level state is limited to `const` declarations (lines 31–54), which are
inlined by the compiler and carry no ordering risk across files. The struct-level
concurrency state (`shutdownCtx`, `triageSem`, `worktreeMu`) is initialized only in
`NewBacklogService()` (line 154) via a constructor, not via `init()`.

---

## Summary: Priority Ordering

| ID | Pitfall | Severity | File split safe? | Package split safe? |
|----|---------|----------|-----------------|---------------------|
| P5 | Import cycle: `session/domain` sub-package | HIGH | N/A | Only if `session/domain` never imports `session` back |
| P6 | `ent.IsNotFound` leakage past Storage boundary | HIGH | Safe | Breaks if code moves outside `server/services` without cleanup |
| P-TESTS | White-box tests calling unexported helpers | MEDIUM | Safe | Breaks if symbols move to new package |
| P1 | Mid-PR build breakage during file split | MEDIUM | Manageable | N/A |
| P4 | ent partial hydration nil panic | LOW (future) | Safe | Safe (no Select() today) |
| P-MUTEX | Mutex ownership after extraction | LOW | Safe | Safe if `ReviewGateRunner` gets own lock |
| P-GOWORK | go.work / module considerations | NONE | — | — |
| P-INIT | init() / package-level var ordering | NONE | — | — |
