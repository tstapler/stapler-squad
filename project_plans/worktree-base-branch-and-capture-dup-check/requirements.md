# Requirements: new_worktree base-branch resolution & terminal output capture

## Source

Backlog item `4d856751-bfe9-4566-99ca-2c483b23bd06`: "new_worktree sessions branch from
dirty local HEAD instead of origin/<default>, and terminal output capture returns empty
for trivial commands."

## Original description (as filed)

### Issue 1 — `new_worktree` branches from source repo's ambient HEAD, not `origin/<default>`

`create_session(session_type=new_worktree)` created the new worktree's branch from
whatever branch the source repo's working directory happened to have checked out,
instead of `origin/<default-branch>`. In the reporter's repro, the source repo was
checked out on an unrelated in-progress branch ~20 commits ahead of `origin/main`;
the new worktree silently inherited all of it.

Expected: `new_worktree` should branch from `origin/<default-branch>` (fetching
first) by default, or at minimum warn/require an explicit base-branch parameter when
ambient HEAD diverges from the resolved default branch.

### Issue 2 — `run_command`/`read_session_output` return empty output for trivial commands

After creating a `new_worktree` session, `run_command echo alive-check` and
`read_session_output` both returned zero lines, with no error, even minutes after
creation and even on a freshly recreated clean session. No way to distinguish
"session still initializing" from "something is broken" using MCP tools alone.

## CRITICAL PRE-EXISTING-FIX FINDING (established during this triage, 2026-09-24)

Both issues described above are **already fixed and merged to `main`**, two days
before this triage ran:

- PR [#833](https://github.com/tstapler/stapler-squad/pull/833) — "fix(session):
  resolve new_worktree base branch from origin, wire scrollback capture" — merged
  2026-09-21, commit `1cc66c53d`.
- PR [#837](https://github.com/tstapler/stapler-squad/pull/837) — same title as
  *this* backlog item verbatim, explicitly linked to a **different** backlog item
  id, `c7466f05-3d19-4d25-a822-9ea1ac7a6faa` (see the PR body's "Backlog item:"
  link) — merged 2026-09-22, commit `ccbc1dba6`.

Both commits are present in this triage worktree's own `git log` (`git log --oneline
-5` shows `dd1848f9b` at HEAD, with `1cc66c53d` and `ccbc1dba6` in its ancestry — this
worktree branched from a `main` that already has the fix).

Verified in the current tree, not just from the PR description:

- `session/instance_worktree.go`'s `newWorktreeFromResolvedBase` resolves the base
  via `git.ResolveWorktreeBaseCommit` (origin's fetched default-branch tip, falling
  back to a local candidate, and to ambient HEAD only for a genuinely unborn repo),
  and sets `Instance.CreationWarning` via `git.FormatAmbientDivergenceWarning` when
  ambient HEAD diverges from the resolved base — exactly Issue 1's requested
  behavior ("branch from origin/default, warn on divergence").
  `CreationWarning` is threaded through `InstanceSnapshot` → MCP's
  `SessionDetail.creation_warning` (`server/mcp/tools_discovery.go:75`,
  `server/mcp/types.go:70`).
- `server/services/connectrpc_websocket.go` calls `scrollbackManager.AppendOutput`
  from both live output-delivery paths — `forwardOneControlModeFrame` (line ~1666)
  and `forwardCapturePaneOutput` (line ~3244) — closing exactly the gap Issue 2
  described (`AppendOutput` previously called only from tests). A new
  `SESSION_NOT_READY` MCP error code (`server/mcp/types.go:154`,
  `server/mcp/tools_terminal.go`'s `sessionNotReadyResult`) is returned by
  `read_session_output`/`run_command` while scrollback hasn't advanced yet,
  distinguishing "still initializing" from "ran and produced nothing."
- `go test ./session/git/... -run "TestAmbientHEAD|TestResolveRemoteWorktreeBaseCommit|TestResolveWorktreeBaseCommit"`
  — all 9 tests pass in this tree (run during this triage).

**Working conclusion carried into research/plan/validate below: this backlog item
(`4d856751-...`) is very likely a duplicate of `c7466f05-...`, filed separately
(same underlying user report, same repro) and left open after the other item's fix
shipped.** The remaining real work is verification + duplicate cleanup, not new
implementation — the research/plan/validate phases below are scoped accordingly:
confirm nothing regressed, confirm no residual gap versus the original ACs, and
recommend closing this item as a duplicate rather than re-implementing already-
shipped code.

## Acceptance criteria (carried forward, now framed as "confirm still true")

1. A `new_worktree` session created from a source repo whose ambient HEAD diverges
   from `origin/<default-branch>` branches from the resolved default branch's tip,
   not the ambient HEAD.
2. When ambient HEAD does diverge from the resolved base, the divergence is
   surfaced to the caller (not silent) — via `Instance.CreationWarning` /
   `SessionDetail.creation_warning`.
3. `run_command`/`read_session_output` against a freshly created, ready session
   return real terminal output (not silently empty) for a trivial command like
   `echo alive-check`.
4. A session that hasn't finished initializing yet is distinguishable from one that
   ran a command and produced no output, via the `SESSION_NOT_READY` signal.
5. (New, from this triage) This backlog item is confirmed to be a duplicate of
   `c7466f05-3d19-4d25-a822-9ea1ac7a6faa` (already fixed via #833/#837) and is
   closed/merged into that item rather than re-triggering implementation.

## Complexity

Complexity: 1 (quick task) — per the finding above, the described functionality is
already implemented and merged; the remaining work is verification and duplicate-
item cleanup, not feature design. Research is scoped to Agents 1 (confirm the
shipped fix's approach/dependencies are sound), 4 (residual risk / gap-check against
the original ACs), and 6 (sanity-check the shipped approach against alternatives),
per sdd:2-research's Complexity-1 calibration.

## Open Questions

None — the pre-existing-fix finding above resolves the item's original ambiguity.

## Out of scope

- Re-implementing base-branch resolution or scrollback wiring — both already exist
  and are covered by passing tests.
- Netflix-internal `compute-nop` repro specifics — not reproducible outside that
  environment; the underlying git-worktree-basing and terminal-capture logic was
  validated against this repo's own git/tmux/session code instead, per the item's
  own note that this is expected.
