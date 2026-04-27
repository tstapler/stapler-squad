# ADR-003: PR Creation via One-Shot Agent vs Server-Side gh Shell-Out

**Status**: Accepted
**Date**: 2026-04-17
**Deciders**: Tyler Stapler

---

## Context

Story 3 adds a "Create PR" button to the `ReviewQueuePanel`. When clicked, the server must create a pull request for the session's branch. Two architectural approaches exist: (1) the server shells out `gh pr create` directly, or (2) the server spawns a one-shot `claude -p "<prompt>"` process in the worktree directory that runs non-interactively and creates the PR itself.

The existing `github/client.go` already uses `gh` CLI for all PR operations (GetPRInfo, MergePR, ClosePR, PostPRComment). A `CreatePR` function would be a natural addition there. However, the synthesis research recommends the one-shot agent approach, and the reasoning is sound.

Additionally, Story 1 introduces `OneShot: true` sessions that use `claude -p`. The `RunOneShot` RPC generalizes this into a reusable primitive beyond just PR creation.

---

## Decision

Add `RunOneShot(session_id, prompt) → RunOneShotResponse{output, error, exit_code}` to `SessionService`. The server spawns `claude -p "<prompt>"` as a subprocess in the session's worktree directory, captures stdout/stderr, and returns on completion. The "Create PR" button in `ReviewQueuePanel` uses `RunOneShot` with a default PR creation prompt (user-editable in a confirmation modal).

The server additionally adds `CreatePR` to `github/client.go` as a fallback path for non-Claude sessions (Aider, etc.) where `claude -p` is not available.

---

## Options Considered

### Option A: Server-side gh pr create shell-out (partially accepted as fallback)

Server runs `git push -u origin <branch>` then `gh pr create --title ... --body ... --head <branch>`.

**Partially accepted as fallback for non-Claude sessions.**

**Not primary because**:
- Server cannot write a meaningful PR description without session context (it doesn't know what changed or why)
- Server must separately handle: push, branch protection errors, auth failures, draft vs ready state, base branch selection
- The agent already has all of this context and can handle these cases autonomously
- `gh pr create --json` output format for parsing the PR URL is not supported (confirmed: github.com/cli/cli issue #6366); requires parsing stdout

### Option B: One-shot agent (claude -p) — accepted as primary

```go
// server/services/session_service.go (new handler)
func (s *SessionService) RunOneShot(ctx, req) (*connect.Response[RunOneShotResponse], error)
```

Server spawns:
```
claude -p "<prompt>" --cwd <worktree_path>
```

Agent runs non-interactively, creates the PR, outputs the PR URL on the last line, and exits.

**Accepted as primary because**:
- Agent has full session context to write a meaningful PR title and description
- Agent handles push, auth, branch protection, and conflicts autonomously
- Agent outputs the PR URL in its stdout; server parses the last line
- Same mechanism generalizes to any one-shot action (run tests, deploy, etc.)
- `claude -p` is confirmed to work in a worktree directory and picks up the worktree's CLAUDE.md

**Accepted costs**:
- One-shot PR creation takes 10–30s (agent running); UI shows spinner
- If user is running Aider or another program, `claude -p` may not be available; fallback to server-side `gh` shell-out

---

## Default PR Prompt

```
Create a pull request for your current branch.
Push the branch if it is not already pushed to origin.
Write a clear title and description summarizing what you changed, based on your work in this session.
Use 'gh pr create' or your preferred method.
Output the PR URL on the last line of your response.
```

The prompt is pre-filled in a confirmation modal; the user can edit it before running.

---

## Divergence Warning

Before showing the "Create PR" button as active (vs. showing a warning badge), the server should check:

```bash
git merge-base --is-ancestor HEAD origin/main
```

If the branch is diverged from the base branch, show a warning badge: "Branch diverged from main — review before creating PR." The button is still active; the warning is informational.

---

## Proto Design

```protobuf
rpc RunOneShot(RunOneShotRequest) returns (RunOneShotResponse) {}

message RunOneShotRequest {
  string session_id = 1;
  string prompt     = 2;
  // Optional: timeout in seconds (default 120, max 300)
  int32  timeout_seconds = 3;
}

message RunOneShotResponse {
  string output     = 1;  // Full stdout
  string error      = 2;  // Populated on non-zero exit or timeout
  int32  exit_code  = 3;
  string pr_url     = 4;  // Parsed from last line if it looks like a GitHub URL
}
```

---

## Consequences

**Positive**:
- General-purpose one-shot primitive reusable for future actions
- Context-aware PR descriptions without server-side knowledge of changes
- Minimal new server logic; agent does the work

**Negative / Accepted**:
- 10–30s latency vs <1s for direct `gh` call; spinner required
- Requires `claude` binary on PATH in the server's environment
- Fallback needed for non-Claude sessions

**Implementation notes**:
- Use `exec.CommandContext(ctx, "claude", "-p", prompt)` with `cmd.Dir = worktreeDir`
- Parse last line of stdout with `strings.HasPrefix(lastLine, "https://github.com")` check
- On success with PR URL: call `storage.UpdateInstance` to set `GitHubPRURL` — existing `PRStatusPoller` picks it up
- On timeout: kill the subprocess; return error in response
- Divergence check: `git merge-base --is-ancestor HEAD origin/<base>` in worktree dir; expose result as `GetVCSStatus` field or as a new field on the `ReviewQueueItem` proto
