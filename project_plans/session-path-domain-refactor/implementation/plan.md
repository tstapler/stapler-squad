# Implementation Plan: session-path-domain-refactor

**Feature**: Give the four session path concepts distinct, self-documenting names across Go, proto, and TypeScript, and make the wrong choice structurally hard.
**Date**: 2026-09-13
**Status**: Draft — call-site enumeration pending audit
**ADRs**: `decisions/ADR-001-session-path-vocabulary.md`

---

## Vocabulary (from ADR-001)

| # | Name | Meaning | Safe for |
|---|---|---|---|
| 1 | `RepoRoot` / `repo_root_path` | repo root the session was created against | repo identity, grouping |
| 2 | `WorktreeDir` / `worktree_dir` | this session's worktree, `""` if none | worktree-specific logic |
| 3 | `SessionDir` / `session_dir` | `WorktreeDir` if set, else `RepoRoot`; disk-agnostic | **comparison, correlation, identity — the default** |
| 4 | `ReadableDir` / `readable_dir` | `SessionDir` if it exists on disk, else `RepoRoot` | **opening files only** |

---

## Phase split

**Phase A (PR 1)** — backend vocabulary, wire fields, enforcement. Additive only: no
existing field changes meaning or stops being populated, so nothing consumer-facing
breaks and the PR is reviewable as pure plumbing.

**Phase B (PR 2)** — frontend + MCP consumer migration onto the new fields. Mechanical
per-file swaps, reviewable as an enumerable list.

**Phase C (out of scope)** — delete the deprecated `path`/`working_dir` fields, once
external MCP/agent consumers have migrated. Separate breaking-change PR.

This mirrors the repo's own IAC Epic 1 → Epic 2 precedent (`447caed00` additive
infrastructure, then `e16ab45e1` consumer migration), for the same reason: a missed
dual-populate site and a consumer migrated to a not-yet-existing field are two different
review mistakes, and bundling them hides both.

---

## Phase A

### Epic A.1: `Workspace` carries all four concepts

**Goal**: one struct, four named fields, each documenting what it is *not* safe for.

#### Story A.1.1: Extend the `Workspace` value type
**As a** backend engineer, **I want** `Workspace` to name all four path concepts, **so that** I pick the right one without reading `instance_worktree.go`.

**Acceptance Criteria**:
- `session/types.go`'s `Workspace` gains `WorktreeDir`, `SessionDir`, `ReadableDir`.
- `EffectivePath` remains, marked `// Deprecated: use ReadableDir` and populated with the identical value — no behavior change.
- Each field's doc comment states its disk-existence semantics and the misuse it guards against; `SessionDir` says it is the default for comparison, `ReadableDir` says two sessions can share one without being co-located.
- `Workspace()`'s own doc comment stops claiming to be "the single source of truth for path resolution" and instead points at the table.

**Files**: `session/types.go`, `session/instance_worktree.go`

##### Task A.1.1a: Add the three fields + doc comments (~4 min)
##### Task A.1.1b: Populate them in `Workspace()`; keep `EffectivePath` as an alias of `ReadableDir` (~3 min)

#### Story A.1.2: Collapse the `GetEffectiveRootDir` / `GetWorkingDirectory` duplication
**As a** maintainer, **I want** one implementation of concept #3, **so that** the two accessors cannot drift.

**Acceptance Criteria**:
- Both accessors return `Workspace().SessionDir`; the near-duplicate bodies (identical but for an empty-string guard) are gone.
- Both carry `// Deprecated: use Workspace().SessionDir` and keep their current return values — verified by the existing tests at `session/instance_worktree_test.go:183` and `:218` passing unchanged.
- `Workspace()` no longer calls `GetEffectiveRootDir()` (inverted dependency), avoiding recursion.

**Files**: `session/instance_worktree.go`

### Epic A.2: Wire fields

#### Story A.2.1: New proto fields on `Session` and `ReviewItem`
**Acceptance Criteria**:
- `Session` gains `repo_root_path = 91`, `worktree_dir = 92`, `session_dir = 93`, `readable_dir = 94` (highest current field number is 90 — verified).
- `ReviewItem` gains the same four at `23`–`26` (highest current is 22 — verified).
- `path` and `working_dir` on both messages get `[deprecated = true]` plus a `// Deprecated: use session_dir` / `use readable_dir` comment, and their **wrong** doc comments are corrected to state what they actually carry.
- New fields carry doc comments matching the ADR table.
- `make proto-gen` regenerates; `gen/` output stays uncommitted.

**Files**: `proto/session/v1/types.proto`

#### Story A.2.2: Dual-populate in the adapters
**Acceptance Criteria**:
- `InstanceToProto` populates all four new fields from one `inst.Workspace()` call.
- `path`/`working_dir` keep their exact current values — no consumer sees a change.
- The `ReviewItem` producer does the same.
- Adapter-level tests assert each new field's value, including the case where the worktree is gone from disk (`session_dir` keeps the worktree path, `readable_dir` falls back to the repo root) — the exact divergence that caused the `WorkspacePeersPanel` bug. `server/adapters/instance_adapter_test.go` currently has **zero** assertions on any path field.

**Files**: `server/adapters/instance_adapter.go`, `server/adapters/instance_adapter_test.go`

#### Story A.2.3: MCP surface
**Acceptance Criteria**:
- `server/mcp` session summary/detail types expose the new vocabulary.
- Raw `inst.Path` / `inst.WorkingDir` reads in the MCP adapters go through `Snapshot()`/`Workspace()`, per `.claude/rules/instance-lock-free-reads.md` — these are live violations of that rule, independently found while tracing this refactor.
- Existing JSON keys keep their names and values.

**Files**: `server/mcp/types.go`, `server/mcp/tools_discovery.go`

### Epic A.3: Enforcement

#### Story A.3.1: `norawinstancepath` analyzer
**As a** reviewer, **I want** a raw `Instance.Path` read to fail the build, **so that** the rule stops depending on whether the author read a doc.

**Acceptance Criteria**:
- New analyzer under `tools/lint/norawinstancepath`, registered in `tools/lint/cmd/linter/main.go` and thereby in `make lint-custom` → `make lint`.
- Flags `*ast.SelectorExpr` reads of `Path` (and the other snapshot-backed mutable fields it covers) where `pass.TypesInfo.Selections` resolves the receiver to `*session.Instance`.
- Exempts: writes (assignment LHS), the accessor methods themselves, `instance_actor_setters.go`'s locked writers, `buildSnapshot`, and `DetectAndPopulateWorktreeInfo`'s deliberately-raw read (which documents why it must stay raw).
- Honours `//nolint:norawinstancepath` via the shared `tools/lint/internal/nolintcomment` helper.
- `analyzer_test.go` + `testdata` fixture covering a flagged read, an exempt accessor, an exempt write, and a `//nolint` suppression.
- Sized against `tools/lint/norawgitopen` (101 lines + 14 test) rather than `norawghrequest` (298 + 14).

**Gate**: build this only if the audit shows enough raw-read call sites to justify it. If the count is low, fix the sites and extend the existing rule doc instead, and record the decision in the plan rather than silently dropping it.

**Files**: `tools/lint/norawinstancepath/*`, `tools/lint/cmd/linter/main.go`

#### Story A.3.2: Update the rule doc
**Acceptance Criteria**:
- `.claude/rules/instance-lock-free-reads.md` gains the identity-vs-resolved distinction; today it covers only lock safety, which is why it could be cited in support of a wrong explanation in PR #801.
- Names the analyzer as the enforcement mechanism, matching how `norawghrequest.md` documents its own.

**Files**: `.claude/rules/instance-lock-free-reads.md`

---

## Phase B (PR 2)

Migrate each consumer from `path`/`working_dir` to `session_dir`/`readable_dir`/`repo_root_path`. Per-file list filled from the audit.

Two non-mechanical items known in advance:

- `WorkspacePeersPanel.tsx`'s `effectiveSessionPath()` helper (added by PR #801) becomes `s.sessionDir` — a single field read. Its explanatory comment, which states a mechanism the code does not have, is replaced.
- Any other comparison/fallback logic is routed through one shared TS helper rather than copy-pasted, to stay under the `jscpd` absolute threshold (0.12%, no diff scoping — confirm current headroom with `pnpm run lint:duplicates` before the PR).

---

## Explicitly not touched

- Worktree detection/creation (`gitManager`, `DetectWorktree`, `MainRepoRoot`) — per the requirements' constraint. The `SessionDir`/`ReadableDir` split names existing behavior; it does not change when a fallback fires.
- `InstanceData`'s JSON tags (`json:"path"`, `json:"working_dir"`) — `sessions.json` has no schema version or migration path, so a tag rename silently zero-values the field for every existing session on next load. Go identifiers may be renamed; tags stay.
- `session/ent/schema/*` — pending audit confirmation of whether these columns are live.
- Deleting the deprecated proto fields — Phase C.

---

## Verification

`make build`, `make test` (affected packages), `make lint`, `make registry-diff`.
The registry is keyed on `// +api:`/`// +feature:` marker comments and RPC/component
names, not proto field names, so no regeneration is expected — `make registry-diff`
confirms rather than assumes.
