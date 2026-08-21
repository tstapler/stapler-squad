# Findings: Pitfalls

**Project:** squad-ux-polish
**Subtopic:** Pitfalls
**Date:** 2026-04-17
**Sources:** Codebase analysis + web searches

---

## Summary

Three features — batch session creation, prompt injection at creation time, and PR creation from the review queue — each carry distinct failure modes. The most severe are: (1) partial-batch failures leaving orphaned tmux sessions and git worktrees; (2) the shell-readiness race condition in prompt injection causing silently dropped prompts (confirmed by multiple open Claude Code issues); and (3) wrong-account PR creation and diverged-branch failures at PR creation time. A fourth cross-cutting concern — sensitive data accidentally persisted in prompt history — is low-likelihood but permanently damaging.

The codebase already handles several of these (ETag caching for GitHub rate limits, session existence polling with backoff, branch collision detection), which narrows the list of genuine gaps.

---

## Options Surveyed

### Risk Area 1: Batch Session Creation

**1a. tmux session naming collisions**

The current `toStaplerSquadTmuxNameWithPrefix` function (`session/tmux/tmux.go:138-143`) strips whitespace, dots, and colons, but does NOT append any uniqueness token. The `start()` function checks `DoesSessionExist()` before creating, but this check is not atomic with the create: two goroutines creating sessions with the same sanitized name can both pass the existence check before either has created its session.

In the existing single-session flow, this race is benign because the user creates sessions sequentially. In batch creation, N sessions created concurrently with auto-generated names (e.g., all derived from the same template title "Feature: Auth") could collide after sanitization.

Mitigation already present: `toStaplerSquadTmuxNameWithPrefix` does not add a timestamp. The worktree path already uses a nanosecond timestamp suffix (`worktreePath + "_" + fmt.Sprintf("%x", time.Now().UnixNano())`), but the tmux session name does not.

**1b. git worktree conflicts**

The worktree path uniqueness is addressed by the nanosecond timestamp suffix (`session/git/worktree.go:166`). However, two sessions created within the same nanosecond (unlikely but possible on high-clock-resolution systems) would collide. More importantly, if two sessions are given the same title, `sanitizeBranchName(sessionName)` produces identical branch names, and the second `git worktree add` will fail with "branch already checked out" unless the existing-worktree reuse path (`findExistingWorktreeForBranch`) fires.

The "already checked out" path in `setupFromExistingBranch` (`worktree_ops.go:69-91`) silently reuses the existing worktree, meaning both sessions share a worktree. For batch creation of sessions with distinct titles, this is non-trivial; for batch creation from a template with identical names, it is a silent data-sharing bug.

Git itself enforces that a branch can only be checked out in one worktree at a time. Concurrent `git worktree add` calls for different branches on the same repo are generally safe as of Git 2.22+, but concurrent calls for the same branch collide with a clear error that the code must propagate cleanly rather than swallow.

**1c. Partial failures in batch**

The current `start()` function (`instance.go`) cleans up on its own error via deferred `Kill()`. There is no cross-session rollback or batch-level failure reporting. If 7 of 10 batch sessions succeed and 3 fail, the 3 that failed silently leave no trace in the storage layer. The caller receives a slice of errors but has no standardized way to report a "partial success" state to the UI.

**1d. Resource exhaustion (tmux + PTY)**

On macOS, `kern.tty.ptmx_max` defaults to 127 (confirmed in system research). Each tmux session consumes at least one PTY for the `attach-session` command (`RestoreWithWorkDir` path, `tmux.go:688-714`). Creating 20 sessions simultaneously pushes close to the default PTY limit. tmux itself has known issues with hanging at 100% CPU when file descriptors are exhausted (`tmux/tmux#14`). The stapler-squad CLAUDE.md mentions a `keepalive` session is always kept alive, consuming one additional slot.

Each git worktree copies all tracked files. A project with 100MB of source files creates 100MB per worktree. For a Node.js project with node_modules, this can reach 2GB per worktree. 10 batch sessions = up to 20GB disk usage.

---

### Risk Area 2: Prompt Injection Timing

**2a. Shell-readiness race condition**

This is confirmed as an active, known issue in Claude Code's own codebase. Multiple GitHub issues (claude-code #23513, #33987, #37217, #40168) document that `tmux send-keys` fires before the shell has finished initializing. Heavy `.zshrc` setups (oh-my-zsh, nvm, mise, pyenv) can take 1-2+ seconds. The symptom is that the prompt text appears in the pane but is never executed — the shell absorbs it as garbage.

The current stapler-squad implementation passes the prompt as a command-line argument (`program = fmt.Sprintf("%s %q", program, i.Prompt)` at `instance.go:1499`) rather than using `tmux send-keys`. This is the correct approach — the prompt is embedded in the tmux `new-session` command, not sent after session creation. This avoids the race entirely, **but only for the initial prompt**. Any follow-up injections (e.g., the restart-marker `SendKeys` at `instance.go:1526`) are still subject to the race.

The PTY attach path (`RestoreWithWorkDir` after `Start`) adds a second async step: `Start` creates the detached session; `RestoreWithWorkDir` attaches a PTY; `StartController` begins watching. If a secondary prompt injection is added between `Start` and controller readiness, it can arrive before the program has entered its input loop.

**2b. Prompt truncation at ~255 bytes**

This is documented in `tmux/tmux#254` and confirmed by Claude Code issue #42391: commands delivered programmatically to tmux via `send-keys` can be silently split at approximately 255 bytes. The split happens in the delivery layer, not in `send-keys` itself. The current implementation passes the prompt as a CLI argument (quoted with `%q`), which avoids the `send-keys` path entirely. However, if a future code path switches to `SendKeys` for longer prompts, this limit becomes active.

The `SendKeys` function (`tmux.go:756-758`) is a raw PTY write with no length check. A 300-byte prompt sent via `SendKeys` would silently truncate.

**2c. Special characters and shell escaping**

The current implementation uses `fmt.Sprintf("%s %q", program, i.Prompt)` to shell-quote the prompt. Go's `%q` produces a double-quoted, Go-style escaped string, which is valid POSIX shell quoting but may produce `\n`, `\t` etc. as literals rather than the shell interpreting them as escape sequences. If the prompt contains single quotes, `%q` correctly escapes them; if it contains shell metacharacters (`$`, `` ` ``, `!`), `%q` wraps in double quotes, which means `$VARIABLE` would be expanded by the shell. This is a security concern if prompts are user-supplied.

**2d. User types into session while prompt is being sent**

If the session is already attached (via web UI) and a secondary prompt is injected while the user is typing, the keystrokes interleave. tmux does not provide atomic multi-keystroke delivery. The existing `SendKeys` writes directly to the PTY without locking.

---

### Risk Area 3: PR Creation / Review Queue

**3a. GitHub auth failures**

The existing `checkGHCLI()` in `session/git/util.go:42-56` checks `gh auth status` at each `PushChanges` / `OpenBranchURL` call. This is the correct check but has two gaps: (1) it does not distinguish between "gh not installed", "gh installed but not authed", and "gh authed but token expired mid-session"; (2) token expiry between the auth check and the actual API call returns a raw error from the `gh` subprocess rather than a structured error the UI can present helpfully.

The `PRStatusPoller` already handles this better with a cached auth check and rate-limit pause state (`pr_status_poller.go:53-54`). The one-click PR creation path does not have this infrastructure.

Token scope gaps: `gh pr create` requires the `repo` scope (not just `read:org`). If the user authenticated with `--scopes read:org`, the PR creation will fail with an HTTP 403 error that surfaces as an unformatted subprocess error.

**3b. Branch protection rules**

GitHub branch protection rules can block PR creation if: required CI checks have not run yet, required reviewers are not yet assigned, the PR author is the only required reviewer (self-review blocked), or a signed-commit requirement is in place. The `PushChanges` function calls `gh repo sync` and `git push -u origin`, but does not call `gh pr create`. The actual PR creation step is not yet implemented in the codebase (the existing functionality tracks existing PRs; it does not create new ones from a session).

When `gh pr create` is eventually implemented, protection-rule failures return stderr output like `pull request create failed: GraphQL: Protected branch rules not satisfied`. These must be parsed to provide actionable UI messages.

**3c. Stale session / diverged branch**

Sessions created from main may have their worktree branch diverge if main advances. `git push -u origin <branch>` in `PushChanges` succeeds regardless of main's state. However, `gh pr create` will show the PR as having merge conflicts if the branch cannot be fast-forward merged. The current diff (`worktree_ops.go`) computes diff from `baseCommitSHA`, which is the merge-base at worktree creation time — not the current state of main.

There is no pre-PR-creation check that computes divergence from current main and warns the user. A PR with unresolved merge conflicts still creates successfully on GitHub but requires manual rebase before merge.

**3d. Wrong GitHub account**

`gh` uses the account authenticated for the remote URL. If the user has multiple accounts (personal + work) and the worktree was cloned with SSH key for account A but `gh auth` defaults to account B, `gh pr create` silently creates the PR under the wrong account. The gh CLI v2.40+ added `gh auth switch` but does not auto-switch based on remote URL. A workaround exists via `GH_TOKEN` env variable, but stapler-squad does not currently set this.

**3e. GitHub API rate limits**

The `PRStatusPoller` already handles rate limits correctly with ETag caching, a 60s pause on rate-limit errors, and a semaphore of 5 concurrent fetches. The new PR creation path (calling `gh pr create` for each completed session) is a write operation and does not benefit from ETags. Write operations (POST/PATCH) count against the 5000 req/hr primary limit. For batch PR creation of 20 sessions simultaneously, this is unlikely to hit the limit but could encounter secondary rate limits (concurrent write burst).

---

### Risk Area 4: General Cross-Cutting Pitfalls

**4a. Template drift**

Session templates (planned feature) encoding repository paths, program flags, or branch name patterns will silently become stale as the repository moves, renames, or restructures. There is no validation that a stored template's `path` still exists or that its `program` is still installed.

**4b. Sensitive data in prompt history**

The `command_history.go` / `command_history_test.go` files indicate that a prompt/command history feature is planned or partially implemented. If users save prompts containing API keys, passwords, or OAuth tokens into the prompt history, those secrets persist in `~/.stapler-squad/` alongside session state JSON. The current `instance.go` stores `Prompt string` in the serialized `InstanceData` — if this is a raw user prompt, it persists across restarts.

---

## Trade-off Matrix

| Risk | Severity | Likelihood | Detection Difficulty | Mitigation Available |
|------|----------|------------|----------------------|----------------------|
| **B1a** tmux name collision on batch | User-visible failure (second session fails) | Medium — only with same-name batch | Easy (clear error from tmux) | Append index/UUID suffix to session names |
| **B1b** git branch collision (shared worktree) | Silent wrong behavior (two sessions share files) | Medium — same-title batch | Hard (no error shown to user) | Enforce unique branch names server-side |
| **B1c** Partial batch failure, no rollback | User-visible partial failure, orphaned resources | High — any batch > 1 session | Medium (errors logged, not surfaced) | Batch result type with per-session status |
| **B1d** PTY/disk exhaustion | User-visible failure (forkpty error) | Low — requires ~100+ sessions | Easy (OS error) | Warn at N>20 sessions; document PTY limits |
| **P2a** Shell-readiness race (prompt dropped) | Silent wrong behavior (no prompt sent) | Low — current impl uses CLI arg | Hard (no error, session starts prompt-less) | Use CLI arg path; verify with capture-pane |
| **P2b** Prompt truncation via SendKeys | Silent wrong behavior (truncated prompt) | Low — only if SendKeys path used | Hard (silent, no error) | Length check before SendKeys; use CLI arg |
| **P2c** Shell metachar expansion in prompt | Security: $VAR expansion or command injection | Low — requires specific characters | Hard (wrong behavior, no error) | Use `--` separator and single-quote escaping |
| **P2d** User input interleave during injection | User-visible mangled input | Low — timing window is small | Easy (user sees garbled terminal) | Don't inject after session attached to user |
| **PR3a** GitHub auth failure | User-visible failure at PR creation | Medium — first-time or token expiry | Medium (raw gh stderr) | Structure auth errors; detect scope gaps |
| **PR3b** Branch protection blocks PR | User-visible failure (PR blocked) | Medium — org repos commonly protected | Easy (gh returns error) | Parse protection errors; show action message |
| **PR3c** Diverged branch, merge conflicts | Silent wrong behavior (PR created but conflicted) | High — any long-running session | Medium (PR exists but stuck) | Warn if branch diverged before PR creation |
| **PR3d** Wrong GitHub account | Silent wrong behavior (PR under wrong user) | Low — single-account users unaffected | Hard (no error, just wrong author) | Show which account will be used before action |
| **PR3e** API rate limit on batch PR creation | User-visible throttle | Low — 20 PRs << 5000/hr limit | Easy (gh returns rate limit error) | Sequential PR creation with jitter |
| **G4a** Template drift | User-visible failure (template creates bad session) | Medium — over time | Easy (errors on startup) | Validate template paths on load |
| **G4b** Sensitive data in prompt history | Data loss / security (secrets persisted to disk) | Low — user must include secrets | Hard (no warning, silent persistence) | Warn on secret-like patterns; allow history clearing |

---

## Risk and Failure Modes

### Highest priority: Partial batch failure with no status reporting

**Failure mode:** User requests 10 sessions. Sessions 1-7 succeed; session 8 fails (git worktree collision); sessions 9-10 are never attempted because the batch aborts. The 7 successful sessions exist in tmux and on disk. Sessions 8-10 have no tmux sessions but may have partial git worktrees (if `Setup()` succeeded but `Start()` failed).

**Current code behavior:** Each `Instance.start()` defers `Kill()` on its own error, cleaning up its tmux session and calling `gitManager.Cleanup()`. However, there is no batch-level coordination: if the batch loop returns early on first error, sessions 9-10 are never created and never cleaned up (they don't exist). Sessions 1-7 have already been committed to storage. The user sees an error but does not know which sessions were created.

**Detection difficulty:** High. The current API returns a single error from the failing session. The caller has no structured list of `(session, error)` pairs.

### Second priority: Diverged branch at PR creation time

**Failure mode:** Session created 3 days ago. Main has advanced 47 commits. Claude has done work on the feature branch. User clicks "Create PR". The push succeeds. GitHub creates the PR but marks it as having merge conflicts. User is surprised — they expected a clean PR.

**Current code behavior:** `baseCommitSHA` is set at worktree creation time to the merge-base of HEAD with main/master. The diff display uses this correctly. But there is no "divergence check" before PR creation to alert the user that main has advanced past their branch's merge-base.

### Third priority: Shell metacharacter expansion in prompt

**Failure mode:** User enters a prompt like "Set up the project with `API_KEY=$MY_SECRET`". Go's `%q` produces `"Set up the project with \x60API_KEY=$MY_SECRET\x60"`. The tmux `new-session` command passes this to the shell, which expands `$MY_SECRET` in the session's environment. If the variable is not set, it expands to empty. If it is set, its value is embedded in the Claude invocation and may appear in logs.

**Current code:** `fmt.Sprintf("%s %q", program, i.Prompt)` at `instance.go:1499`. Go's `%q` double-quotes and escapes with backslash, but double-quoted strings still undergo `$VAR` expansion in bash/zsh.

---

## Migration and Adoption Cost

| Mitigation | Effort | Blocks Feature? |
|------------|--------|-----------------|
| Batch result type (`BatchCreateResult` with per-session status) | Medium — new type + API change | Yes — needed for reliable batch UX |
| Unique tmux name enforcement (append index or short UUID) | Low — change name generation | No — backward compatible |
| Pre-PR divergence check (`git merge-base --is-ancestor`) | Low — one git command | No — warning only |
| Auth error structured parsing | Medium — parse gh stderr patterns | No — graceful degradation |
| Prompt sanitization (use `--` and single-quote escaping) | Low — change fmt.Sprintf | No — fix before enabling prompt feature |
| PTY limit warning (check `kern.tty.ptmx_max` before batch) | Low | No |
| Prompt history secret detection | Medium — pattern matching | No — polish item |

---

## Operational Concerns

**Orphaned resources:** Failed batch creation can leave git worktrees on disk at `~/.stapler-squad/worktrees/` without corresponding session records. The cleanup path (`gitManager.Cleanup()`) is called on `start()` error, but if the process is killed mid-cleanup (SIGKILL, OOM), worktrees can persist indefinitely. A startup reconciliation pass (compare stored sessions vs. `git worktree list --porcelain` for all known repos) would detect and offer to prune stale worktrees.

**PTY exhaustion on macOS:** macOS default `kern.tty.ptmx_max` is 127. Each `RestoreWithWorkDir` path (`tmux.go:688`) opens one PTY. With 20 active sessions + stapler-squad's own keepalive + control mode processes, the system approaches the limit. The error (`forkpty: resource temporarily unavailable`) is not currently caught and formatted for the UI.

**Disk growth:** 10 sessions on a large repo (1GB source tree) consumes 10GB. The `~/.stapler-squad/worktrees/` directory has no size monitoring or quota enforcement.

**gh CLI subprocess timeout:** `PushChanges` and `checkGHCLI` run `exec.Command("gh", ...)` with no timeout. A hung network connection blocks the goroutine indefinitely. The `PRStatusPoller` uses `exec.CommandContext` with a 10s timeout; the `git/worktree_git.go` functions do not.

---

## Prior Art and Lessons Learned

**Claude Code agent team issues (anthropics/claude-code #23513, #33987, #37217, #40168):** The same product (Claude Code) encountered and documented the send-keys race condition when spawning agent teams. The recommended fix from the community is to either (a) embed the command in the pane invocation (`tmux new-window -- <cmd>`) rather than using send-keys after creation, or (b) poll for shell readiness via a sentinel echo. Stapler-squad's current approach of embedding the prompt as a CLI argument is already the correct fix.

**git worktree add race (Git 2.22 fix):** Prior to Git 2.22, `git worktree add` used a stat-loop to find available names, creating a TOCTOU race. Post-2.22, it uses `mkdir` with `EEXIST` detection. The relevant risk for stapler-squad is not this internal git race but the application-level check in `worktree.go` that calls `findExistingWorktreeForBranch` before `Setup()` — these two calls are not atomic.

**GitHub ETag polling (already addressed):** The PR status poller already implements ETag conditional polling. The pattern is documented in ADR-002 (`project_plans/github-pr-status/decisions/ADR-002-etag-conditional-polling.md`). The new PR creation path does not need ETags but should reuse the concurrency semaphore to avoid rate limit bursts.

**tmux command length limit (255 bytes via send-keys):** Confirmed by `tmux/tmux#254` and Claude Code issue #42391. This is a real limit for programmatic send-keys delivery. The current CLI-argument approach bypasses this, but any future use of `SendKeys` for injecting prompts longer than 255 bytes will silently truncate.

---

## Open Questions

1. **Batch failure semantics:** Should a batch creation fail-fast (abort on first error) or fail-slow (attempt all, report per-session results)? Fail-slow produces cleaner UX (user sees which sessions succeeded) but leaves more cleanup to do.

2. **Branch naming for batch sessions:** If the user creates 5 sessions with the same template, what differentiates their branch names? Auto-append index (feature/task-1, feature/task-2)? UUID? User-supplied suffix? The sanitization currently produces identical names for identical titles.

3. **PR creation: new feature or reuse PushChanges?** `PushChanges` (in `worktree_git.go`) does a commit+push but does not call `gh pr create`. Is the intent to add `gh pr create` as a step in `PushChanges`, or as a separate `CreatePR` method? The review queue feature implies a dedicated CreatePR flow with title/body from the session metadata.

4. **Prompt history persistence policy:** Should `Instance.Prompt` (persisted to JSON) be treated as potentially sensitive and omitted from logs/exports? Is there a planned prompt history UI that would require explicit clearing?

5. **Multiple GitHub accounts:** Does stapler-squad need to support per-session `GH_TOKEN` overrides to handle workspaces that mix personal and work GitHub accounts?

---

## Recommendation

The top 5 mitigations to build in from day 1, in priority order:

**1. Batch result type with per-session status (Critical)**
Define a `BatchCreateResult` type containing `[]SessionCreateResult{Session *Instance, Err error}`. The batch creation endpoint returns this instead of a single error. The UI can display a "7/10 sessions created — 3 failed" summary with per-session error details. Without this, batch failures are unrecoverable puzzles.

**2. Unique branch/session names for batch (Critical)**
Before creating N sessions from a template, generate distinct names server-side: append a zero-padded index or 6-char random hex to the title before passing to `sanitizeBranchName` and `toStaplerSquadTmuxNameWithPrefix`. This prevents the silent shared-worktree failure mode.

**3. Pre-PR divergence warning (High)**
Before `PushChanges` / `CreatePR`, run `git merge-base --is-ancestor HEAD origin/main` (or the configured default branch). If the branch has diverged (main has commits not in the feature branch), surface a warning: "Main has advanced 47 commits since this session started. Rebase before creating PR?" This prevents the silent merge-conflict PR.

**4. Prompt sanitization via single-quote wrapping (High)**
Change `fmt.Sprintf("%s %q", program, i.Prompt)` to use a shell single-quoted argument with embedded single-quote escaping (`'string'` with `'\''` for any single quotes in the string). Single-quoted strings do not undergo `$VAR` expansion in bash/zsh, eliminating the metacharacter expansion risk.

**5. Structured auth error handling for gh CLI (Medium)**
Add a `ParseGHError(stderr string) error` function that classifies `gh` subprocess errors into structured types: `ErrGHNotInstalled`, `ErrGHUnauthorized`, `ErrGHTokenScope`, `ErrGHRateLimit`, `ErrGHBranchProtection`. Use `exec.CommandContext` with a 15s timeout for all `gh` subprocess calls in `worktree_git.go`. This prevents hung goroutines and gives the UI actionable error messages instead of raw subprocess stderr.

---

## Pending Web Searches

The following claims should be verified before implementation:

1. **PTY limit on macOS:** Verify that `kern.tty.ptmx_max` defaults to 127 on current macOS versions (Ventura/Sonoma/Sequoia). The research cites older sources.
   - Suggested query: `kern.tty.ptmx_max default macOS Sonoma Sequoia 2024 2025`

2. **gh CLI token scope for pr create:** Verify that `gh pr create` requires the `repo` scope specifically (not `public_repo` for public repos, not `workflow` for repos with CI rules).
   - Suggested query: `gh cli pr create token scopes required repo public_repo 2025`

3. **tmux send-keys 255-byte limit root cause:** Clarify whether the ~255-byte split is a PTY line discipline limit (POSIX `MAX_INPUT` = 255), a tmux-internal buffer, or a Claude Code application bug.
   - Suggested query: `tmux send-keys 255 byte POSIX MAX_INPUT line discipline limit root cause`

4. **git worktree concurrent safety (Git 2.22+):** Confirm that concurrent `git worktree add` calls for different branches on the same repository are data-safe on current Git versions.
   - Suggested query: `git worktree add concurrent safe different branches same repo Git 2.40 2025`

---

## Sources

- Codebase: `session/tmux/tmux.go` (session creation, SendKeys, PTY lifecycle)
- Codebase: `session/git/worktree.go`, `worktree_ops.go`, `worktree_git.go` (worktree creation, branch collision, PushChanges)
- Codebase: `session/instance.go` (prompt embedding, start() sequence, batch lifecycle)
- Codebase: `session/pr_status_poller.go` (rate limit handling, auth caching — prior art)
- Codebase: `project_plans/github-pr-status/research/pitfalls.md` (GitHub auth gaps, ETag, fork detection — prior art)
- [anthropics/claude-code #23513](https://github.com/anthropics/claude-code/issues/23513) — send-keys race with shell init
- [anthropics/claude-code #33987](https://github.com/anthropics/claude-code/issues/33987) — configurable delay for send-keys
- [anthropics/claude-code #37217](https://github.com/anthropics/claude-code/issues/37217) — command sent before shell ready
- [anthropics/claude-code #42391](https://github.com/anthropics/claude-code/issues/42391) — agent spawn fails silently at ~255 bytes
- [tmux/tmux #254](https://github.com/tmux/tmux/issues/254) — command length not documented
- [tmux/tmux #14](https://github.com/tmux/tmux/issues/14) — hangs on file descriptor exhaustion
- [kaeawc/auto-worktree #176](https://github.com/kaeawc/auto-worktree/issues/176) — git single-process limitation with concurrent worktree ops
- [cli/cli #326](https://github.com/cli/cli/issues/326), [#1094](https://github.com/cli/cli/issues/1094) — gh multi-account pitfalls
- [git-scm.com/docs/git-worktree](https://git-scm.com/docs/git-worktree) — branch-exclusive checkout enforcement
- [gitcheatsheet.dev worktree disk space](https://gitcheatsheet.dev/docs/advanced/worktrees/disk-space-management/) — inode and disk usage patterns
- [wilsonmar.github.io maximum-limits](https://wilsonmar.github.io/maximum-limits/) — macOS PTY and fd limits
