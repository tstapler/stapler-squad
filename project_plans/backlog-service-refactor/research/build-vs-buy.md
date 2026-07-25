# Build vs. Buy: backlog-service-refactor

Evaluated against the 6 refactor items (P1–P6). All are pure Go structural changes; no new
runtime dependencies are warranted. The notes below explain which tools exist, which to use
for each phase, and which to skip.

---

## P1 — File split of `backlog_service.go` (2,570 lines)

**Verdict: manual split, no tooling needed.**

The split is a cut-by-responsibility operation (triage logic, review gate logic, CRUD/list
helpers, sync/import helpers). Go allows multiple files in the same package — you move
functions between files without changing import paths, so there is no symbol-rename problem
at all.

Workflow:
1. Create the new files (`backlog_service_triage.go`, `backlog_service_review.go`, etc.) in
   the same `package services` namespace.
2. Move function bodies with their `// +api:` markers intact.
3. `go build ./server/services` catches any missing declarations immediately.
4. `make quick-check` confirms lint/tests still pass.

No external tool adds value here; gorename/gopls rename are only needed when the symbol's
*name* changes, not when its file changes.

---

## P2 — Extract `mergeAcCriteria` with unit tests

**Verdict: build it; use native Go fuzzing for edge-case discovery.**

The function handles index-merge semantics over `[]AcCriterion` slices (merging proto
criteria into the stored JSON form with index assignments). This is a pure function with
well-defined inputs — the right test strategy is:

- **Table-driven tests** (idiomatic Go) for the 3–5 documented cases: empty input, single
  criterion, duplicate index collisions, gap-fill, and existing criteria preserved on partial
  update.
- **Native fuzzer** (`go test -run=^$ -fuzz=FuzzMergeAcCriteria -fuzztime=30s`) to explore
  off-nominal inputs (negative indices, large slices, unicode text). Go 1.18+ fuzzing
  requires zero new dependencies — it is part of the standard toolchain.

```go
func FuzzMergeAcCriteria(f *testing.F) {
    f.Add(`[{"index":1,"text":"AC1","status":"pending"}]`, `[{"index":0,"text":"new"}]`)
    f.Fuzz(func(t *testing.T, existing, incoming string) {
        // should not panic; index uniqueness invariant must hold
        merged, err := mergeAcCriteria(existing, incoming)
        if err != nil { return }
        seen := map[int]bool{}
        for _, c := range merged {
            if seen[c.Index] { t.Fatalf("duplicate index %d", c.Index) }
            seen[c.Index] = true
        }
    })
}
```

Third-party fuzz frameworks (`go-fuzz`, `dvyukov/gofuzz`) are superseded by native fuzzing
for new code.

**Table-test generators** (`gotests`, AI-generated stubs): `gotests` can scaffold the table
structure in one command — `gotests -w -only mergeAcCriteria ./server/services/` — but the
interesting index-merge cases must be written by hand. Worth running to get the boilerplate,
then fill in meaningful inputs.

---

## P3 — Extract `ReviewGateRunner` type

**Verdict: manual extraction; no tooling needed beyond `go build`.**

A `ReviewGateSpawner` interface already exists in `session/backlog_lifecycle.go` (lines
19–23 and 154). The extraction is naming a type that wraps the spawn → create-ItemSession →
capture-base-SHA sequence (currently inlined twice: in `SpawnSessionFromItem` and
`TriggerReReview`).

No automated refactoring tool handles "extract a sequence of statements into a named type
with methods" — that is a semantic operation. The workflow is:

1. Define `type ReviewGateRunner struct{ ... }` in `backlog_service_review.go`.
2. Move the repeated 10-step sequence into `ReviewGateRunner.Run(ctx, item) (*session.Instance, *ent.ItemSession, error)`.
3. `go build` catches missed call sites.
4. Existing `BacklogLifecycleListener` tests still pass without change.

---

## P4 — Add `BacklogItemSummary` DTO

**Verdict: use ent's built-in `.Select().Scan()` — no plugin, no library.**

ent v0.14.5 (in use here) supports partial-field projection natively:

```go
type BacklogItemSummary struct {
    ID        uuid.UUID `json:"id"`
    Title     string    `json:"title"`
    Status    string    `json:"status"`
    Priority  int       `json:"priority"`
    UpdatedAt time.Time `json:"updated_at"`
}

// In EntRepository:
func (r *EntRepository) ListBacklogItemSummaries(ctx context.Context, filter BacklogItemFilter) ([]BacklogItemSummary, error) {
    var out []BacklogItemSummary
    err := r.client.BacklogItem.Query().
        Select(
            backlogitem.FieldID,
            backlogitem.FieldTitle,
            backlogitem.FieldStatus,
            backlogitem.FieldPriority,
            backlogitem.FieldUpdatedAt,
        ).
        Where(...).
        Scan(ctx, &out)
    return out, err
}
```

The generated `backlogitem.FieldXxx` constants in `session/ent/backlogitem/backlogitem.go`
are already present for every column. No ent plugin or external pattern library is needed.

**Tradeoff to note**: `.Select().Scan()` does not work with `.With*()` eager-loading edges.
The summary query must not join `ItemSessions` or `Source`; those are only needed for the
full `BacklogItemData` representation. Keep the existing `ListBacklogItems` → `backlogItemToData`
path for the detail view; add `ListBacklogItemSummaries` → `BacklogItemSummary` as an
additive read path.

**No ent codegen plugin** for partial views exists or is recommended by the ent team. The
`.Select().Scan()` pattern IS the documented approach in the ent docs ("Select Specific
Fields" section).

---

## P5 — Create `session/domain` sub-package

**Verdict: manual move; use `go list` + `goda` for pre-flight cycle checks.**

### Moving types

The target types (likely `AcCriterion`, `AcCriteriaJSON`, `BacklogStatus`, and other pure
value types in `session/backlog.go`) must be moved, not just aliased. Since `session/` is
already a large package imported by `server/services/`, moving a type to `session/domain`
and importing it from both `session/` and `server/services/` is safe — but you must confirm
no cycle is introduced.

**gorename** (`golang.org/x/tools/cmd/gorename`) is available in module cache at
`~/.local/lib/go/pkg/mod/golang.org/x/tools@v0.21.0/cmd/gorename` but is considered a
legacy tool. It handles in-package renames only — it cannot move a type to a different
package. Skip it.

**gopls rename** is available at `~/.local/share/opencode/bin/gopls`. It can rename
symbols in-place and update all references, but again does not move across packages. Worth
using for any symbol rename within the new `domain` package after the move.

**Safe workflow for the cross-package move**:
1. Create `session/domain/` with the type definitions (copy, do not remove yet).
2. In `session/backlog.go`, replace the originals with type aliases pointing to the domain
   package: `type AcCriterion = domain.AcCriterion`. This keeps backward compatibility
   during the transition without a big-bang import update.
3. Update `server/services/backlog_service.go` to import from `session/domain` instead.
4. Once all callers import from `session/domain`, remove the aliases in `session/`.
5. `go build ./...` confirms no breakage; `make ci` confirms tests + lint pass.

### Import cycle detection

**`goda`** is installed at `/home/tstapler/.local/bin/goda`. Use it:

```bash
# Before creating session/domain/, dry-run what session/ currently depends on:
goda list "github.com/tstapler/stapler-squad/session:..."

# After creating session/domain/, confirm domain/ does not import session/:
goda reach "github.com/tstapler/stapler-squad/session/domain" "github.com/tstapler/stapler-squad/session"
# Should output nothing (no path exists)

# Confirm server/services still reaches session/domain (expected):
goda reach "github.com/tstapler/stapler-squad/server/services" "github.com/tstapler/stapler-squad/session/domain"
```

`go build ./...` will also catch cycles immediately at compile time. `goda reach` is more
useful for proactive "what if" analysis before you start, since `go build` only fails after
you've already written the import statement.

**`go mod graph`** is for module-level dependencies, not package-level cycles. Skip it for
this use case — it will not reveal intra-module import cycles.

**`goimports`** formats imports and adds missing ones but has no cycle-detection capability.
Not useful here.

---

## P6 — Interface boundary cleanup (no ent types in `server/services`)

**Verdict: use `depguard` — add a rule to the existing `.golangci.yml`.**

### Current state

`server/services/backlog_service.go` directly imports `session/ent` (line 21) and uses
`*ent.ItemSession` in internal iteration. The existing `.golangci.yml` has a `depguard`
rule (`no_server_in_core`) that prevents `session/` from importing `server/`, but nothing
prevents the reverse ent-leakage direction.

### Recommended approach: incremental `depguard` + `forbidigo`

The surgical approach is a two-step plan to avoid a big-bang rewrite:

**Step 1 — Add a `forbidigo` rule** on the specific ent error-checking helper that leaks
most pervasively:

```yaml
# In .golangci.yml linters.settings.forbidigo.forbid:
- pattern: 'ent\.IsNotFound'
  msg: "use errors.Is(err, session.ErrNotFound) in server/services — wrap ent errors at the storage layer"
```

Add a `.golangci.yml` exclusion for the storage-layer files that are *allowed* to use this
(`session/ent_repository_backlog.go`, `session/storage_backlog.go`). This flags the 14
existing `ent.IsNotFound` call sites in `backlog_service.go` immediately and forces the
error-wrapping to live in the data layer where it belongs.

**Step 2 — Add a `depguard` rule** once the ent types are fully wrapped:

```yaml
# In .golangci.yml linters.settings.depguard.rules:
no_ent_in_services:
  files:
    - "**/server/services/**/*.go"
    - "!**/*_test.go"
    - "!**/server/services/error_registry.go"      # DB error unwrapping lives here by design
  deny:
    - pkg: "github.com/tstapler/stapler-squad/session/ent"
      desc: "server/services must not import session/ent directly; ent types belong in the data layer"
```

**Why not `revive` or `gocritic`?** Both are style/code-smell linters. Neither can enforce
import-graph boundaries — that is exclusively `depguard`'s domain in this toolchain.

**Why not a custom golangci-lint plugin?** Writing a custom plugin requires building against
golangci-lint's plugin API (using `go build -buildmode=plugin`), which is fragile across Go
versions and overkill for what a `depguard` rule handles in 5 lines of YAML. No plugin is
warranted here.

---

## Summary Matrix

| Phase | Build vs. Buy | Primary tool | Effort |
|---|---|---|---|
| P1 — File split | Build (manual) | `go build`, editor | Low |
| P2 — Extract `mergeAcCriteria` | Build + `gotests` scaffold | `gotests`, native fuzzer | Low |
| P3 — Extract `ReviewGateRunner` | Build (manual) | `go build`, existing tests | Low |
| P4 — `BacklogItemSummary` DTO | Build (ent built-in) | ent `.Select().Scan()` | Low |
| P5 — `session/domain` package | Build (manual + `goda`) | `goda reach`, `go build` | Medium |
| P6 — Interface boundary | Buy (existing `depguard`) | `.golangci.yml` rule | Low |

---

## Top 3 Recommendations

1. **P6 first, as a guard rail.** Add the `forbidigo` rule on `ent.IsNotFound` to the
   `.golangci.yml` before writing any refactor code. This turns future regressions (new ent
   leakage) into CI failures immediately and forces the error-wrapping to stay in the data
   layer throughout the refactor.

2. **Use ent's native `.Select().Scan()` for P4 without any external library.** The
   `backlogitem.FieldXxx` constants are already generated. The only design decision is to
   keep the summary path (`ListBacklogItemSummaries`) strictly separate from the full-fetch
   path (`ListBacklogItems`) — mixing the two would break edge loading.

3. **Use `goda reach` as the pre-flight check before creating `session/domain`.** Run it
   once before cutting the new package to map the current import graph, then again after to
   confirm no cycle has been introduced. The type-alias transition strategy (`type
   AcCriterion = domain.AcCriterion` in `session/`) lets you migrate callers incrementally
   without a big-bang import update.
