# Requirements: session-path-domain-refactor

**Date**: 2026-09-12
**Type**: refactor (existing project)
**Backlog item**: `64c7d4c8-d14c-4006-909e-dadc1dda9ee4` (follow-up to `e2430349-8891-4067-a6c3-0d1383ff2fd4`)

## Problem Statement

`stapler-squad` tracks two distinct concepts about where a session's code lives, and the
domain does not name them distinctly enough to make the correct one obvious to a new
consumer:

1. **Stable, creation-time identity path** — the original/logical repo path an instance
   was created against (`session/instance_snapshot.go`'s `Instance.Path` / `GetPath()` /
   `Workspace().RepoRoot`). Used for repo-scoped identity/grouping — analogous to
   `WorkspaceKey`'s `owner/repo/mainRepoPath`.
2. **Resolved, worktree-aware current working path** — where the session's process is
   actually running right now (`Workspace().EffectivePath`, `GetEffectiveRootDir()`,
   `GetWorkingDirectory()`). Used for anything about "could this collide with another
   session's filesystem location" or "what directory do I read files from."

Verified against `main` @ `c412906c3` (2026-09-12) — **this is more nuanced than the
originating backlog item's description**:

- `server/adapters/instance_adapter.go:65` already populates the `Session` proto's
  `path` field (proto/session/v1/types.proto field 3) from `inst.Workspace().EffectivePath`
  — i.e., **already the resolved, worktree-aware value**, not the raw `i.Path`. This line
  predates today's session (traced via `git log -L`, oldest visible revision
  `e16ab45e1`, the IAC Epic 2 snapshot migration).
- A second, subtly different resolved value — `WorkingDir` (proto field, populated from
  `GetWorkingDirectory()`) — also ships on the wire. It computes almost the same thing as
  `EffectivePath` but skips `Workspace()`'s disk-existence fallback check. Two
  differently-named fields that both mean roughly "resolved path" is itself part of the
  ambiguity problem.
- Insights' `SessionsTable.tsx` reads `s.projectPath` — a **third**, differently-named
  field, from what appears to be a separate summary proto/message not going through
  `InstanceToProto`. Its resolution semantics (raw vs. resolved) are not yet confirmed.
- `WorkspacePeersPanel.tsx:82`'s raw `s.path === session.path` comparison — the subject
  of sibling backlog item `e2430349` — is **still unfixed on `main`** as of this
  session (confirmed by reading the file directly); that item's status is `idea`, not
  `done`, contrary to this item's originating note that a PR "likely" already exists.
  Because `session.path` is already `EffectivePath` (see above), whether that panel's
  bug reproduces as literally described (false collision between two isolated
  worktrees) or stems from a different cause (e.g. `HasWorktree()`/`GetWorktreePath()`
  not being populated for some session-creation path) is an open question for Phase 2
  research, not yet confirmed.
- The backend-only correct mechanism for the *repo-scoped* (not literal-path) question
  — "do these sessions share a repo identity, worktree siblings included" — is
  `session/workspace_peers.go`'s `WorkspacePeersBlockForPath` / `ListWorkspacePeers`,
  used today only for the initial-prompt nudge. It is not confirmed whether this is
  reachable from the web frontend via a public ConnectRPC endpoint (an MCP tool of the
  same name exists for agent sessions, which is a different consumer).

Net effect: the domain has at least **three or four differently-named, inconsistently
resolved "path" concepts** on the wire (`path`/`EffectivePath`, `working_dir`,
`projectPath`, plus sub-message `repo_path`/`worktree_path` pairs), no shared naming
convention distinguishing "identity" from "current location," and the raw `Instance.Path`
Go field remains directly readable by any future backend code with no structural
guard beyond an always-loaded convention doc (`.claude/rules/instance-lock-free-reads.md`,
which is scoped to lock-safety, not to the identity-vs-resolved distinction at all).
This is a naming/API-shape gap that has already produced one confirmed frontend bug
report and is a standing trap for the next one.

## Users / Consumers

- Backend Go code populating the `Session` proto and any sibling summary/list proto
  messages (Insights, workspace peers, backlog ship-status, etc.).
- Frontend React components reading session path-like fields: confirmed so far —
  `SessionRow.tsx`, `SessionCard.tsx`, `SessionDetailView.tsx`, `WorkspacePeersPanel.tsx`,
  `ResumeSessionModal.tsx`, `OmnibarSessionResult.tsx`, `ImportExternalSessionsPanel.tsx`,
  `Omnibar.tsx`, `WorkspaceSwitchModal.tsx`, `ReviewQueuePanel.tsx`, Insights'
  `SessionsTable.tsx`. Phase 2 research must produce the exhaustive list (this is a
  required research deliverable, not just these).
- MCP tool consumers (`mcp__stapler-squad__list_sessions`, `get_session`,
  `list_workspace_peers`, etc.) — external agent sessions reading the same proto fields.
- Downstream: anyone who added a proto field named generically "path" going forward.

## Success Metrics

- Every path-like concept in the domain (Go struct fields, proto fields, TS types) has a
  name that unambiguously signals "identity/creation-time" vs. "resolved/current
  location" — a reader should not need to check the backend implementation to know
  which one to use.
- Zero raw, unresolved `Instance.Path` reads bypass `Workspace()`/`GetPath()` in new or
  touched backend code (existing accessors already do this correctly; the goal is
  making the *wrong* choice structurally harder, not just documented).
- Every current consumer identified in the Phase 2 audit either: (a) is migrated to the
  new, unambiguous field name(s) in this change, or (b) is explicitly listed as a
  deferred follow-on with reasoning, per the phased-rollout requirement below — no
  consumer is silently left on an ambiguous/soon-to-be-deprecated field without a
  tracked follow-up.
- `WorkspacePeersPanel.tsx`'s specific bug (backlog `e2430349`) is not regressed by this
  refactor; if it is still open when this ships, this refactor's new naming/mechanism
  must make that fix's eventual implementation obviously correct (ideally the fix
  becomes trivial — swap to the now-clearly-named resolved field, or call the
  now-exposed `ListWorkspacePeers`-backed mechanism).
- `make ci` / `make ready` green on every PR produced by this work.

## Constraints

- **Shared, widely-consumed proto message** (`Session`) — no single breaking rename.
  Plan for backward compatibility / staged rollout: add new, clearly-named fields
  alongside the old ones, migrate consumers, then deprecate (and, in a later,
  out-of-scope change, remove) the old ones. Do not silently drop or repurpose an
  existing field name's meaning.
- Backend Go changes must respect `.claude/rules/instance-lock-free-reads.md` (reads via
  `Snapshot()`, not raw mutable fields) — any new snapshot fields follow the existing
  `InstanceSnapshot` pattern.
- `session/ent/*` generated code is never hand-edited or committed; proto `gen/` output
  is never committed — both regenerate via existing Make targets.
- Repo convention: Conventional Commits, PRs default to ready-for-review (not draft),
  `make ci`/`make ready` must pass before shipping each PR, `gh pr merge` needs
  `--repo owner/repo`.
- If splitting into multiple PRs (foundation/backend rename + migration, then
  per-consumer or per-surface frontend follow-ons), each split must be justified
  explicitly in the plan, not just asserted.
- A lint-rule enforcement mechanism (mirroring `tools/lint/norawghrequest` /
  `tools/lint/norawgitopen`) is a candidate but not mandated — Phase 3 planning decides
  proportionality based on the actual number of call sites found in Phase 2.

## Scope

### In Scope
- Auditing and enumerating every backend producer and frontend/MCP consumer of the
  current ambiguous path-like fields (`Session.path`, `Session.working_dir`, Insights'
  `projectPath`, any others research turns up).
- Deciding and documenting (ADR if warranted) the final naming scheme for
  identity-path vs. resolved-path across Go structs, proto messages, and TS-consumed
  fields.
- Backend changes: new/renamed snapshot and proto fields, populated correctly,
  old fields retained during migration per the backward-compat constraint.
- Migrating identified consumers to the new fields, in this change or as explicitly
  justified, tracked follow-on work (new backlog items or a documented plan section).
- Confirming (not necessarily fixing, unless trivial once the new naming lands) the
  actual current status/root cause of `WorkspacePeersPanel`'s bug (`e2430349`), and
  coordinating so this refactor doesn't collide with or regress that item's own fix
  when it lands.
- Regression coverage: unit tests for the resolution logic, and tests for each migrated
  consumer confirming it now reads the correct field.

### Out of Scope
- Re-implementing `e2430349`'s `WorkspacePeersPanel` fix itself — that is a separate,
  independently tracked backlog item. This work must not silently duplicate or conflict
  with it; if this refactor's PR(s) land first, note in the PR description what the
  `e2430349` fix should now use.
- Removing/deleting the old ambiguous proto fields — full removal is a later,
  independently-scoped breaking-change PR once every consumer (including any external
  MCP/agent consumers not controlled by this repo) has migrated.
- Any change to worktree detection/creation logic itself (`gitManager`, `DetectWorktree`,
  `MainRepoRoot`) — this work only renames/clarifies what already exists and fixes
  consumer call sites, not the underlying git-worktree mechanics.
- Building a full custom lint analyzer unless Phase 3 planning concludes the call-site
  count justifies it (see Constraints).

## Open Questions (for Phase 2 research to resolve)

1. Does `WorkspacePeersPanel`'s false-collision bug (`e2430349`) actually reproduce
   today given that `Session.path` is already `Workspace().EffectivePath`? If not, what
   *is* the actual root cause (a `HasWorktree()` detection gap, a stale snapshot, or
   something else)?
2. What backs Insights' `SessionsTable.tsx`'s `projectPath` field — is it the same
   raw/resolved distinction, a wholly separate summary type, and does it have its own
   producer code path to migrate?
3. Is `session/workspace_peers.go`'s `ListWorkspacePeers`/`WorkspacePeersBlockForPath`
   reachable via any public ConnectRPC endpoint the web frontend could call, or is it
   currently backend-internal + MCP-tool-only?
4. Full enumeration: every Go producer site, every proto message carrying a path-like
   field, every frontend/TS consumer, every MCP tool surfacing one of these fields.
5. How many total call sites would a custom lint rule need to cover, to judge whether
   one is proportionate (per Constraints)?
