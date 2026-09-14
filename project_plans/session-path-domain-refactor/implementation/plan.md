# Implementation Plan: session-path-domain-refactor

**Feature**: Name the four session path concepts distinctly across Go and the wire, and correct the doc comments that describe them wrongly.
**Date**: 2026-09-13
**Status**: Ready for implementation (revised after adversarial review — 4 BLOCKERs resolved)
**ADRs**: `decisions/ADR-001-session-path-vocabulary.md`

---

## Vocabulary (ADR-001)

| # | Name | Meaning | Safe for |
|---|---|---|---|
| 1 | `RepoRoot` / `repo_root` | path the session was created against | repo identity, grouping |
| 2 | `WorktreeDir` / `worktree_dir` | this session's worktree, `""` if none | worktree-specific logic |
| 3 | `ActiveDir` / `active_dir` | `WorktreeDir` if set, else `RepoRoot`; disk-agnostic | **comparison, correlation, identity — the default** |
| 4 | `ExistingDir` / `existing_dir` | `ActiveDir` if it exists on disk, else `RepoRoot` | **opening files only** |

---

## Phase A (this PR) — Go vocabulary, `Session` wire fields, corrected docs

Additive. No existing field changes value; no consumer is migrated. Reviewable as plumbing.

### Epic A.1: `Workspace` names all four

#### Story A.1.1: One pure helper, four fields, no recursion
**As a** backend engineer, **I want** `Workspace` to name all four concepts, **so that** I pick the right one without reading `instance_worktree.go`.

**Acceptance Criteria**:
- `session/types.go`'s `Workspace` carries `RepoRoot`, `WorktreeDir`, `ActiveDir`, `ExistingDir`, each documenting what it is *not* safe for. `EffectivePath` is **removed**, and its three readers (`server/adapters/instance_adapter.go:65`, `server/services/workspace_service.go:146`, `:364`) move to `ExistingDir` in this PR.
- A new **pure, unexported** `func (i *Instance) activeDir() string` holds the concept-3 logic (`HasWorktree()` → non-empty `GetWorktreePath()` → else `GetPath()`).
- `GetEffectiveRootDir()` and `GetWorkingDirectory()` both `return i.activeDir()` — they must **not** call `Workspace()`.
  - *Why this is a hard constraint*: `Workspace()` does an `os.Stat` and a `log.Warn` on the fallback branch. These two accessors have 37 call sites including `cmd.Dir`, session start, and poll loops (`session/history_linker.go:291`, `session/retry_state.go:352`, `server/services/session_service.go:3566`…). Routing them through `Workspace()` would add a syscall and emit a warning per call for every paused session whose worktree `pause_session` removed — sustained log spam. It would also be unbounded recursion if `Workspace()` still computed `ActiveDir` by calling `GetEffectiveRootDir()`, which is what `instance_worktree.go:419` does today.
- `Workspace()` calls `activeDir()` too, and keeps its existing `ActiveDir != RepoRoot` guard around the `os.Stat` so directory sessions still cost zero syscalls.
- `Workspace()`'s doc comment stops claiming to be "the single source of truth for path resolution" and points at the table. `RepoRoot`'s comment stops claiming to be "the main checkout, not the worktree" — for a session created from a pre-existing worktree it *is* the worktree, and `Instance.MainRepoPath` holds the main checkout.

**Files**: `session/types.go`, `session/instance_worktree.go`, `server/adapters/instance_adapter.go`, `server/services/workspace_service.go`

*(A.1.1 and the former A.1.2 are deliberately one story: split across two subagents, the natural first implementation is `ActiveDir: i.GetEffectiveRootDir()`, and the second change then makes it recurse.)*

#### Story A.1.2: Pin the one intended behavior change
**Acceptance Criteria**:
- `GetWorkingDirectory()` today returns `""` when `HasWorktree()` is true but `GetWorktreePath()` is `""`; `GetEffectiveRootDir()` returns `RepoRoot` in that case. Converging on `activeDir()` picks the non-empty answer.
- A test pins this **before** the change (`GetWorkingDirectory` has no behavioral test today — its only mention in `instance_worktree_test.go` is `:290`, a race test that discards the result).

**Files**: `session/instance_worktree_test.go`

### Epic A.2: `Session` wire fields

#### Story A.2.1: Add four fields, fix two wrong comments
**Acceptance Criteria**:
- `Session` gains `repo_root = 91`, `worktree_dir = 92`, `active_dir = 93`, `existing_dir = 94`. Highest current field number is 90; no `reserved` ranges — verified.
- `path = 3` gets `[deprecated = true]` and its comment corrected: it says "Path to workspace repository root" and carries `Workspace().EffectivePath`.
- `working_dir = 4` gets its comment corrected — it says "Directory within repository to start in" and carries an absolute `GetWorkingDirectory()` — but is **not** deprecated: `SessionDetailView.tsx:496`/`:678` round-trips it through `UpdateSession` into `Instance.WorkingDir` (the relative subdir), so a replacement it cannot write would break the editor. The corrected comment states the read/write asymmetry and points at the follow-up item.
- `worktree_dir`'s comment explains why it differs from `git_worktree.worktree_path` (that submessage is gated on `i.started`, so it is nil for stopped sessions while `worktree_dir` is populated).
- `ReviewItem` is **not** touched — its `path`/`working_dir` carry different concepts than `Session`'s and their comments are already accurate (see ADR alternatives).

**Files**: `proto/session/v1/types.proto`

#### Story A.2.2: Populate from one `Workspace()` call
**Acceptance Criteria**:
- `InstanceToProto` populates all four from a single `inst.Workspace()`, so the syscall count per conversion is unchanged.
- `path` and `working_dir` keep their exact current values.
- Adapter tests assert each new field, including the worktree-missing-from-disk case where `active_dir` keeps the worktree path and `existing_dir` falls back — the divergence that caused the bug. `server/adapters/instance_adapter_test.go` has **zero** path assertions today.

**Files**: `server/adapters/instance_adapter.go`, `server/adapters/instance_adapter_test.go`

### Epic A.3: Documentation

#### Story A.3.1: Extend the rule doc
**Acceptance Criteria**:
- `.claude/rules/instance-lock-free-reads.md` gains the identity-vs-resolved distinction. Today it covers only lock safety, which is why PR #801 could cite it by name in support of a wrong mechanism.
- No lint analyzer in this PR — see ADR alternatives.

**Files**: `.claude/rules/instance-lock-free-reads.md`

---

## Deferred, with reasoning (requirements' "no consumer silently left behind")

None is silently dropped. Filed 2026-09-13:

| Item | Covers |
|---|---|
| `a9e7edc4-0055-45d1-8397-626f2bfe3d90` | Phase B — frontend + MCP consumer migration, incl. replacing PR #801's unfalsifiable regression fixture and the stopped-session case its fix still misses |
| `7cfdb43e-a640-4f08-a7e2-2f5d7bd24775` | `unfinished_work_service.go` and `tokens/association.go` keying an index by identity path and querying it by resolved path |
| `29bd6922-ce09-41ed-ab04-a9238f4b1a9f` | `working_dir` reading absolute and writing relative |

| Deferred | Why |
|---|---|
| **Frontend migration** (~30 files) to `activeDir`/`existingDir`, incl. replacing `WorkspacePeersPanel`'s `effectiveSessionPath()` helper and its stale comment | Phase B. Separate PR: mechanical but wide, and the `jscpd` gate (0.1% used of a 0.12% absolute threshold — measured) needs any shared comparison logic extracted, not copied. |
| **PR #801's regression fixture is unfalsifiable** — `WorkspacePeersPanel.test.tsx` builds a session with `path` = repo root *and* a distinct `gitWorktree.worktreePath`, a state `instance_adapter.go:65` never produces | Phase B replaces the fixture, not just the field. |
| **MCP `SessionSummary.Path` / `SessionDetail.WorkingDir`** read raw `inst.Path`/`inst.WorkingDir` (`server/mcp/tools_discovery.go:58`, `:70`) and mean *identity*, while ConnectRPC `Session.path` means *resolved* — same name, two meanings, two protocols | Changing MCP JSON values is consumer-visible for agents outside this repo's deploy. Own item. |
| **`ReviewItem.path` vs `Session.path`** carry different concepts under identical names and comments; `review-queue/page.tsx:30` converts one into the other | Needs plumbing through the `session.ReviewItem` Go struct and both `server/dependencies.go` enrichment sites. |
| **`working_dir` read/write asymmetry** (absolute out, relative in) corrupts on round-trip | Real bug, needs a product decision on whether to split the field. |
| **`server/dependencies.go:292`, `:1103`** and `session/instance_workspace.go:309` (`i.Path = targetWorktree.Path`, no snapshot republish) are live `instance-lock-free-reads.md` violations | Lock-safety, not vocabulary. Belongs with the analyzer item. |
| **`norawinstancepath` analyzer** | ~90 sites across 34 production files; enforces lock safety, not this vocabulary. See ADR alternatives. |
| **`unfinished_work_service.go:117-125`** indexes by identity path, looks up by resolved path — worktree sessions never correlate. **`session/tokens/association.go:83`** same shape — worktree sessions get orphaned token records | Genuine behavior bugs found during the audit, out of scope for an additive rename. |

## Closed questions (requirements' Open Questions)

1. **Does the `WorkspacePeersPanel` bug reproduce given `Session.path` is already `EffectivePath`?** Yes, by a different mechanism than PR #801 claimed: `EffectivePath`'s disk-existence fallback collapses two cleaned-up worktree sessions onto the same repo root. Additionally `gitWorktree` is nil for stopped sessions, so #801's fix does not cover them.
2. **What backs Insights' `projectPath`?** `proto/session/v1/insights.proto:72`, fed from `tokens.ParseResult.ProjectPath` — parsed from Claude's own JSONL directory naming, never from an `*Instance`. **Not migratable**; it is a third kind of path, not either of these two.
3. **Is `ListWorkspacePeers` reachable from the web frontend?** No — backend-internal plus the `list_workspace_peers` MCP tool. No `WorkspacePeer` message exists in any `.proto`. `WorkspacePeersPanel.tsx` is an independent client-side implementation deriving peers from `WatchSessions`, which is why the two can and do disagree.
4. **Full enumeration** — done; drives the deferral table above.
5. **Lint-rule proportionality** — ~90 raw-read sites across 34 production files. Proportionate *for the lock-safety rule*, but that is not this ADR's invariant. Deferred with the rule it serves.

---

## Explicitly not touched

- Worktree detection/creation (`gitManager`, `DetectWorktree`, `MainRepoRoot`).
- `session/ent/schema/session.go`'s `path` / `working_dir` columns. **ent is the live session store** — `Storage` is a facade over `EntRepository` (`session/storage.go:260`), and `session/migrate.go` is a one-shot JSON→ent importer. Renaming a `field.String("path").NotEmpty()` column needs a data migration. Go identifiers may be renamed; schema field names stay.
- `InstanceData`'s JSON tags, for the same reason at the legacy-JSON layer.
- Deleting the deprecated `path` field — Phase C.

---

## Verification

`make build`, `make test` (affected packages), `make lint`, `make registry-diff`.
Registry baseline measured before any change: **267 committed / 266 generated, 0.37%
divergence, validation passed** — pre-existing, so it must still read 0.37% afterwards.
