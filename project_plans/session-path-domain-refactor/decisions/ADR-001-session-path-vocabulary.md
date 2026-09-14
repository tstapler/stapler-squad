# ADR-001: Session path vocabulary — four named concepts, not two

**Date**: 2026-09-13
**Status**: Accepted
**Project**: session-path-domain-refactor
**Backlog**: `64c7d4c8-d14c-4006-909e-dadc1dda9ee4`

## Context

The originating backlog item frames this as a two-concept problem: a "logical repo
path" vs. a "resolved worktree path." Reading the code at `main` (`fbdf80522`) shows
the domain actually carries **four** distinct semantics, and that the two proto fields
on the wire are named almost exactly backwards from what they contain.

Verified accessors (`session/instance_worktree.go`, `session/types.go`):

| # | Accessor | Definition | Disk-existence checked? |
|---|---|---|---|
| 1 | `GetPath()` = `Workspace().RepoRoot` | `Snapshot().Path` — repo root, set at creation | n/a |
| 2 | `gitManager.GetWorktreePath()` | the session's declared worktree dir, `""` if none | no |
| 3 | `GetEffectiveRootDir()` (and its near-twin `GetWorkingDirectory()`) | worktree dir if declared, else repo root | **no** |
| 4 | `Workspace().EffectivePath` | #3, but falls back to repo root when the directory is **gone from disk** | **yes** |

#3 and #4 differ only when a worktree directory has been deleted while its session
metadata survives — which is a normal, supported state here: `pause_session`
deliberately removes the worktree and keeps the branch.

Both behaviors are already deliberate and individually tested
(`session/instance_worktree_test.go:183`, `:202`, `:218`), each with a named consumer
in its doc comment:

- #3 must stay **disk-agnostic** because `HistoryLinker` correlates sessions by the
  nominal worktree path string.
- #4 must **fall back** because filesystem readers like `FileService.ListFiles`
  otherwise surface a bare `directory not found: .`.

Neither is wrong. The defect is that nothing in either name says which is which, and
`Workspace()`'s doc comment — "the single source of truth for path resolution" —
actively invites callers to treat #4 as the universal answer.

### The failure this already caused

`server/adapters/instance_adapter.go:65-66` puts both on the wire:

```go
Path:       inst.Workspace().EffectivePath,  // #4 — disk-checked, lossy
WorkingDir: inst.GetWorkingDirectory(),      // #3 — disk-agnostic
```

So `Session.path` — the field a reader would assume is the stable repo path — is the
lossy, disk-dependent one, and `Session.working_dir` is the reliable identity value.

Both fields' doc comments in `proto/session/v1/types.proto` describe something other
than what the adapter puts on the wire:

```proto
// Path to workspace repository root.
string path = 3;          // actually Workspace().EffectivePath — the worktree dir when present

// Directory within repository to start in.
string working_dir = 4;   // actually GetWorkingDirectory() — an absolute session dir,
                          // not the relative subdirectory Instance.WorkingDir this describes
```

A frontend developer reading only the `.proto` would pick `path` for "which repo is
this session in" and get the worktree directory, and would never consider `working_dir`
for "where is this session working" — the opposite of correct in both cases.

`WorkspacePeersPanel.tsx` compared `s.path === session.path` to answer "are these two
sessions in the same directory." When two sessions' worktrees are both cleaned up,
#4 collapses both to the same repo root and the panel falsely reports a collision.
PR #801 fixed it with `s.gitWorktree?.worktreePath || s.path` — which works, but its
commit message explains the bug as "path is the original repo path set once at session
creation and never updated for worktree sessions." That is not what the code does.
`Session.working_dir`, already on the wire, would have fixed the same bug.

Two careful readings of the same field reached opposite conclusions about its meaning,
and the shipped fix is justified by a wrong mechanism. That is the strongest available
evidence that this is a naming problem, not a logic problem.

## Decision

Adopt one vocabulary across Go, proto, and TypeScript. Each name states both what the
value is and, where it matters, what it is *not* safe for.

| Concept | Name | Meaning |
|---|---|---|
| 1 | `RepoRoot` / `repo_root_path` | the repository root this session was created against; repo-scoped identity and grouping |
| 2 | `WorktreeDir` / `worktree_dir` | this session's dedicated git worktree, empty when it works directly in the repo root |
| 3 | `SessionDir` / `session_dir` | where this session works: `WorktreeDir` if set, else `RepoRoot`. Disk-agnostic and therefore **stable** — the correct key for comparing, correlating, or identifying sessions. The default answer. |
| 4 | `ReadableDir` / `readable_dir` | `SessionDir` when it exists on disk, else `RepoRoot`. **Only** for opening files. Two sessions can share a `ReadableDir` without being in the same place. |

`Workspace` becomes the one struct carrying all four. `EffectivePath` is retained as a
deprecated alias for `ReadableDir` so no behavior changes while callers migrate.

Rules that follow from the names:

- Comparing two sessions' locations → `SessionDir`. Never `ReadableDir`.
- Reading or writing files → `ReadableDir`.
- "Same repo?" → `RepoRoot`, or the existing `WorkspaceKey()`.

## Consequences

**Positive**

- `SessionDir` is the default and is safe for the comparison use case that has now
  produced one shipped bug. `ReadableDir`'s name makes its narrow purpose explicit.
- The `#3`/`#4` distinction becomes visible instead of buried in a `Workspace()` doc
  comment, so the next `WorkspacePeersPanel` doesn't have to rediscover it.
- `GetEffectiveRootDir()` and `GetWorkingDirectory()`, currently near-duplicate
  implementations of #3 differing only by an empty-string guard, collapse onto one
  definition.
- Additive on the wire: `path` and `working_dir` keep their numbers, meanings, and
  values throughout the migration.

**Negative**

- Four names where the backlog item expected two. Justified by four genuinely distinct
  behaviors, each with an existing test and a named consumer — reducing to two would
  require deleting a behavior something depends on.
- Three proto fields now carry overlapping values during the dual-populate window.
  Mitigated by deprecating `path`/`working_dir` in the same change that adds the
  replacements, so a reader always sees which to use.

## Alternatives considered

**Two concepts, as the backlog item framed it.** Rejected: it forces #3 and #4 to share
a name, which is precisely the collision that caused the bug. Picking one and deleting
the other breaks either `HistoryLinker` (needs disk-agnostic) or `ListFiles` (needs the
fallback).

**Distinct Go types — `type RepoRoot string`, `type SessionDir string`** (per the
`type-driven-design` skill). Rejected as disproportionate. It would make misuse a
compile error, but every one of these values crosses a proto boundary (`string`), a JSON
persistence boundary (`string`), and `os`/`filepath` calls (`string`), so the conversions
would outnumber the protected assignments, and the wire/disk layers — where the actual
consumers live — get no protection at all. Named struct fields plus a lint rule cover
the same hazard where it occurs. Revisit if a future change keeps these values inside Go
long enough for the types to pay for themselves.

**Documentation only** — fix the wrong `working_dir` comment and extend
`.claude/rules/instance-lock-free-reads.md`. Rejected as the sole remedy: that rule
already existed and was cited *by name* in PR #801's commit message, in support of a
wrong explanation. Convention-only enforcement has now measurably failed on this exact
hazard. The docs are still updated, but alongside the rename and the lint rule, not
instead of them.

**Renaming `path`/`working_dir` in place.** Rejected — external MCP/agent consumers are
not under this repo's deploy control. Additive fields plus `[deprecated = true]`,
matching the repo's existing precedent (`types.proto:437-442`), and removal as a later,
separately-scoped change.
