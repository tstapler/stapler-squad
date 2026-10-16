# Research: Stack (proto/codegen, ent, lint-analyzer pattern, deprecation conventions)

Scope note: this is a pure refactor of existing tooling/conventions — no new
dependency evaluation. All findings verified against `main` @ `c412906c3` /
this worktree, 2026-09-12.

## 1. Proto/codegen tooling

- Codegen is `buf` driven: `buf.gen.yaml` (repo root) + `buf.yaml` define the
  plugin pipeline; `proto/session/v1/types.proto` (1714 lines) is the schema
  source, `Session` message starts at `types.proto:9`.
- **Regen command**: `buf generate proto`, wrapped by the `proto-gen` Make
  target (`Makefile:535-550`). It's a no-op unless triggered: it stamp-checks
  `.proto-gen.stamp` against `find proto -name '*.proto' -newer ...` plus the
  presence of `gen/proto/go/session/v1/session.pb.go` and
  `web-app/src/gen/session/v1/session_pb.ts`. After editing a `.proto` file,
  either run `make proto-gen` directly or just `make build`/`make test`/
  `make lint` (all depend on it) — `touch .proto-gen.stamp` is not needed by
  hand, the timestamp check handles it.
- Output dirs (`PROTO_OUT_DIRS := gen/proto/go web-app/src/gen`, Makefile:52)
  are **both gitignored** — confirmed convention per repo CLAUDE.md ("proto
  `gen/` output is never committed") and matches `.gitignore` intent already
  documented; every CI job that needs them runs `proto-gen` first via the
  Make dependency chain, so nothing needs manual regeneration in a PR.
- **Field-add safety**: `Session` is a flat proto3 message with explicit,
  manually-assigned field numbers (currently up to 90, with several numeric
  gaps — e.g. 76-89 unused, next likely being 91+ for new work; skim the
  full field list before picking a number to avoid collision). Proto3
  field-add semantics are additive-safe by design: a new field number with a
  new name added to `Session` is not a wire-breaking change for existing
  readers (unknown/added fields are simply absent in old binaries, default
  zero-value in new ones reading old wire data) — no special repo-specific
  caveat found beyond "don't reuse a retired field number" (not an issue
  here since nothing is being removed). This matches the requirements doc's
  stated constraint: add new fields alongside old ones, no renumbering.
- The existing `GitWorktree` sub-message (`types.proto:635-651`) already
  models the identity-vs-resolved split correctly and cleanly:
  `repo_path` (field 1, "Path to original repository") vs. `worktree_path`
  (field 2, "Path to worktree directory"). This is a ready-made naming
  precedent to reuse at the top-level `Session` message instead of
  inventing new terminology — e.g. something like `repo_path` (identity) /
  `resolved_path` or `effective_path` (current location) would be
  consistent with existing sibling-message naming rather than novel.

## 2. ent ORM schema — path state IS persisted, not purely in-memory

Contrary to a plausible assumption in the requirements doc, path-like state
is **not** purely an in-memory `Instance`/snapshot concern — it round-trips
through ent:

- `session/ent/schema/session.go:28-31` — the `Session` ent entity has its
  own `field.String("path").NotEmpty()` and `field.String("working_dir").Optional()`,
  directly mirroring the two ambiguous proto fields. This is the DB-persisted
  form of `Instance.Path`/whatever backs `working_dir` — any rename/addition
  at the proto or `InstanceSnapshot` layer has a third layer (ent schema +
  migration) to keep in sync if the new field is meant to be durable across
  restarts, not just derived at read time.
- `session/ent/schema/worktree.go:17-20` — the `Worktree` ent entity
  separately persists `repo_path` (`NotEmpty`) and `worktree_path`
  (`NotEmpty`), mirroring the proto `GitWorktree` sub-message's already-clear
  naming. This confirms the identity/resolved split is already precedented
  at the persistence layer too, just not at the top-level `Session` row.
- No `session/ent/generate.go` file exists in this repo (despite being named
  in root `CLAUDE.md`'s prose) — the authoritative regen command lives
  directly in `Makefile`'s `ent-gen` target (`Makefile` ~line 555):
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`.
  Confirmed via the Makefile source directly, matching CLAUDE.md's stated
  command (missing `--feature sql/upsert` breaks `UpsertRule`-family
  methods). Gated by `.ent-gen.stamp`; edit `session/ent/schema/*.go` only,
  never the generated output (gitignored, same policy as `gen/proto/go`).
- Planning implication for Phase 3: **decide up front whether new
  identity/resolved fields need their own ent columns** (durable, survives
  restart) or can stay derived-at-read-time from existing persisted `path`/
  `working_dir` plus in-memory `gitManager` state (no ent schema change,
  simpler migration). The requirements doc's `Instance.Path`/`Workspace()`
  description suggests the resolved value is already computed at read time
  from `gitManager`, not stored — so a schema change may only be needed if
  the *identity* path's storage representation itself is renamed at the ent
  layer too (optional; the proto/Go rename doesn't strictly require an ent
  column rename since ent field names are internal and don't have to match
  proto field names character-for-character).

## 3. "Shadow the primitive" custom-analyzer pattern — cost characterization

Two existing precedents, both `golang.org/x/tools/go/analysis`-based,
compiled into one multichecker binary:

| Analyzer | Purpose | Files | Lines (analyzer.go + test) |
|---|---|---|---|
| `tools/lint/norawgitopen` | Flag direct `git.PlainOpen`/`PlainOpenWithOptions` calls outside `session/git.OpenRepo` | `analyzer.go`, `analyzer_test.go`, `testdata/src/.../fake.go` | 101 + 14 + 18 = 133 |
| `tools/lint/norawghrequest` | Flag direct `http.NewRequest(WithContext)` against GitHub base URLs outside the approved request constructors | `analyzer.go`, `analyzer_test.go`, `testdata/.../fixture.go` | 298 + 14 + 85 = 397 |

- Both share `tools/lint/internal/nolintcomment` (an escape-hatch helper for
  `//nolint:<name>` comments) rather than reimplementing it — a third
  analyzer for `Instance.Path` would do the same, so its true added cost is
  closer to the `norawgitopen` end (~100-150 lines) than the `norawghrequest`
  end (which is bigger because its "is this a GitHub base URL" check
  resolves/matches multiple constructor functions and host-derivation
  helpers, not just one selector name).
- Wiring cost beyond the analyzer package itself: one line each in
  `tools/lint/cmd/linter/main.go` (`Analyzer` added to the multichecker
  list, confirmed at `main.go:39-46` — all 7 current analyzers listed
  there: `entfullscan`, `hotpolllog`, `nocommandpattern`, `norawexec`,
  `norawghrequest`, `norawgitopen`, `silenttransition`, `tmuxsocketscope`
  — the Makefile's `lint-custom` doc-comment string is stale/incomplete,
  omitting `norawghrequest`/`tmuxsocketscope`, but the actual `main.go`
  registration list is authoritative and complete). No `.golangci.yml`
  entry needed — `lint-custom` is a fully separate binary invoked directly
  by `make lint-custom` (a dependency of `make lint`), not a golangci-lint
  plugin.
- **Structural fit for this refactor's specific case**: `norawgitopen` is
  the closer analogue in *shape* (it flags an unwrapped call to a specific
  function/selector, resolved via `go/types`, with a package-suffix
  exemption list and a `//nolint` escape hatch) but not in *target* — this
  refactor's hazard is a raw **struct field read** (`i.Path`), not a call
  to an external function. A field-read analyzer needs a different check:
  walk `*ast.SelectorExpr` nodes, use `pass.TypesInfo.Selections` to confirm
  the receiver type is `*session.Instance` (or the concrete struct) and the
  selected field name is in a denylist (`Path`, and whichever new raw field
  the domain refactor adds), then exempt selector expressions that occur
  lexically inside the accessor methods themselves
  (`GetPath`/`Snapshot`/`Workspace`/etc.) and inside
  `instance_actor_setters.go`'s locked-write functions — conceptually
  identical difficulty to the existing two, no novel `go/analysis` technique
  required, but this is a **new field-access check bespoke to this repo's
  pattern, not a drop-in variant** of either existing analyzer's core logic.
  Given the existing `.claude/rules/instance-lock-free-reads.md` doc-comment
  convention already targets exactly this hazard (raw `Instance` field
  reads) via a glob-scoped auto-loaded rule file rather than a compiled
  analyzer, Phase 3 should weigh: doc-comment-rule (near-zero build cost,
  relies on the agent reading it) vs. ~100-150 line compiled analyzer
  (build-time enforcement, zero reliance on convention-following) purely on
  the call-site count a later research pass finds — this dimension does not
  itself count call sites.

## 4. Deprecation conventions — established precedent on all three layers

The repo already has a live, consistent convention to follow; nothing new
needs to be invented for this refactor's "keep old fields, mark superseded"
requirement:

- **Proto enum values**: `[deprecated = true]` field option paired with a
  `// Deprecated: use X instead` comment, e.g.
  `types.proto:437-442` (`SESSION_STATUS_RUNNING`/`READY`/`LOADING`, each
  annotated `[deprecated = true]` with wire value preserved for compat) and
  `backlog.proto:735` (`// Deprecated: use item instead. Left for wire
  compatibility.`). No example yet of `[deprecated = true]` on a plain
  message *field* (only on enum values) — precedent still applies directly:
  proto3 supports the same `[deprecated = true]` field option on message
  fields, unused so far in this repo but the natural next case.
- **Go**: standard `// Deprecated: <replacement guidance>` doc-comment
  directly above the symbol (godoc convention), e.g.
  `session/instance.go:77-81`, `session/load_options.go:33-73`,
  `session/interfaces.go:29`. Consistently phrased as "Deprecated: use X
  instead" / "Deprecated: for new code, use X".
- **TypeScript**: JSDoc `@deprecated` tag with a one-line reason, e.g.
  `web-app/src/lib/hooks/useApprovals.ts:10-17`
  (`@deprecated Polling is now controlled centrally by ApprovalsProvider.`).
- Recommendation: apply the same three-layer pattern to the superseded
  `Session.path`/`working_dir` proto fields (add `[deprecated = true]` +
  `// Deprecated: use <new_field> instead` comment), the Go
  `InstanceSnapshot` fields/accessors that back them, and any TS type
  fields once the frontend migration lands — consistent with, not a
  departure from, existing style.

## Decision-relevant takeaways for Phase 3 planning

- Codegen mechanics are cheap and safe: adding fields to `Session` is a
  routine `buf generate proto` + Make-target-driven regen, no breaking-change
  risk from proto3 field-add semantics alone.
- The refactor is **not purely in-memory** — `session/ent/schema/session.go`
  persists `path`/`working_dir` directly, so Phase 3 must decide whether new
  identity/resolved naming requires an ent schema change too, or whether the
  existing ent columns can stay as internal storage names while only the
  proto/Go/TS-facing names change.
- A custom lint analyzer for "raw `Instance.Path` read" is buildable at
  roughly `norawgitopen`'s cost (~100-150 lines, one multichecker
  registration line) but is a field-selector check, not a call-wrapper
  check — a new (if straightforward) analyzer shape, not a copy-paste of
  either existing one. Given `.claude/rules/instance-lock-free-reads.md`
  already exists as a doc-based control for this exact hazard, proportionality
  should be judged against how much that rule is actually failing in
  practice (call-site count from a later research pass), not against build
  cost alone.
- Deprecation tooling/convention is a solved problem here: `[deprecated =
  true]` + `// Deprecated:` comment (proto/Go) and `@deprecated` JSDoc (TS)
  are all already in active use elsewhere in this codebase — apply the same
  pattern rather than inventing new deprecation markup.
