# Research: Similar Features, Edge Cases, Failure Modes (Agent 2 — Features)

## 1. `pushAndCreatePR` — full edge-case catalog (`session/backlog_lifecycle.go:3585-3728`)

The new mechanical RPC for non-backlog sessions should handle the same class of
conditions, adapted from "transition a backlog item" semantics to "return a
connect error / response field to the modal caller." No backlog item exists
for a plain session, so anywhere `pushAndCreatePR` transitions backlog status
or calls `notify()`, the new path should instead just return a clear error to
the RPC caller (the modal has no item-status state machine to drive).

| # | Condition in `pushAndCreatePR` | Current backlog behavior | Adaptation for non-backlog RPC |
|---|---|---|---|
| 1 | No worktree (`GetWorktreeDataBySessionUUID` fails or `WorktreePath == ""`) | `fallbackToDone("no worktree")` — silently marks item done | Return `CodeFailedPrecondition`: "session has no worktree" — see §3 below, this should also gate whether the button/modal is even offered |
| 2 | Dirty working tree | `g.CommitChanges(commitMsg)` — auto-commits with a synthetic `"[claudesquad] work complete for %q (pre-PR)"` message, **error only logged, not fatal** | Same auto-commit behavior likely wanted (session diff should be fully committed before push) but the commit message should reference the session, not a backlog item title. If commit fails, decide: fail loud vs. proceed and let push fail naturally — `pushAndCreatePR` proceeds |
| 3 | Push fails | `stayInReviewAndNotify(... "push failed" ...)` — item stays in review, user notified with the specific `pushErr` | Return `CodeUnavailable`/`CodeInternal` with the literal push error message (acceptance criterion 6 requires the *specific* error, not generic) |
| 4 | PR already exists for this item (`item.PrNumber > 0 && item.PrURL != ""`) | Reuses cached PR URL/number without calling `CreatePR` at all | No backlog item to cache on — must check the **session's own** `GitHubPRURL`/`GitHubPRNumber` fields (`Instance.GitHubPRURL`, set via `SetGitHubPR`) first, OR call `findExistingPR()` fresh (cheap `gh pr list` call) since `CreatePR` already re-checks internally. Acceptance criterion 4 wants "modal reflects existing PR instead of attempting to create a duplicate" — since `GitWorktree.CreatePR` already calls `findExistingPR()` first and returns the existing PR without creating a new one (`session/git/worktree_git.go:338-341`), the RPC gets this reuse "for free" as long as it calls `CreatePR`, not a raw `gh pr create`. The *pre-fill* step (opening the modal) may also want to proactively call `findExistingPR` so the modal can show "PR #N already exists — open it?" instead of a create form. |
| 5 | Zero commits ahead of base (`HasCommitsAheadOfMain`) | Pre-flight check *before* calling `CreatePR` (BUG-063 fix) — routes to `fallbackToDone` with a specific message, since `gh pr create` would otherwise fail with an unhelpful "No commits between X and Y" error that isn't a retryable push/PR failure | Same pre-flight check should gate **both** whether the "Create PR" button is enabled/shown (acceptance criterion 1: "at least one commit ahead of its base branch") and, defensively, again inside the RPC handler (state can change between page load and click) — return a clear "nothing to ship" error rather than surfacing gh's raw message |
| 6 | `HasCommitsAheadOfMain` itself errors | Treated as inconclusive — logs a warning and **proceeds** with PR creation anyway ("fail open", by design: "a broken check never blocks a real PR creation attempt") | Same fail-open semantics should carry over — don't let a flaky ahead/behind check block a legitimate PR |
| 7 | `CreatePR` fails | `stayInReviewAndNotify(... "PR creation failed" ...)` with the specific `prErr` | Return the specific `gh pr create` failure (already includes `out` — the CLI's stderr/stdout) verbatim per criterion 6 |
| 8 | Persisting PR URL/number to storage fails after successful `CreatePR` | **Treated as a failure, not best-effort** (BUG-040 regression) — `stayInReviewAndNotify` even though the PR was actually created on GitHub, because every downstream consumer (reconciler, auto-merge) requires the *stored* value, not just the local variable | Same discipline applies here: if `SaveInstances` fails after `SetGitHubPR`, the RPC must not report plain success — the PR now exists on GitHub but the session card won't show it. Return an error (or at least a response field) indicating "PR #N created at <url> but failed to save to session — refresh may show a stale state," so the user isn't left thinking nothing happened when a real PR was opened |
| 9 | `EnablePRAutoMerge` fails | Best-effort — logged + a WARNING-priority notification, item still transitions | **Explicitly out of scope** per requirements ("Auto-merge / Copilot review wiring... nice-to-have, not required") — the new RPC likely should not call these at all for the manual flow, or only if the existing `AutoCreatePR`-style opt-in is later wired in (out of scope, flagged as follow-up) |
| 10 | `RequestCopilotReview` fails | Best-effort — logged + LOW-priority notification | Same — out of scope for this feature per requirements |
| 11 | Final status transition fails (`resolveToPRPending`) | `handlePRPendingTransitionFailed` | No analog for a plain session (no backlog status machine) |

**Test-name catalog** confirming this is well-covered on the backlog side (for
parity reference, not to be duplicated but to model equivalent Go tests for
the new RPC): `TestPushAndCreatePR_PushFails_LeavesItemInReview_AndNotifies`,
`TestPushAndCreatePR_CreatePRFails_LeavesItemInReview_AndNotifies`,
`TestPushAndCreatePR_PRFieldsPersistFails_StaysInReview_AndNotifies`,
`TestPushAndCreatePR_ReusesExistingPR_WhenAlreadySet`,
`TestPushAndCreatePR_NoWorktree_FallsBackToDone`,
`TestPushAndCreatePR_ZeroDiffBranch_FallsBackToDone`,
`TestPushAndCreatePR_AheadOfMainCheckErrors_StillAttemptsPRCreation`
(`session/backlog_lifecycle_test.go:2346-2729`). The new Go test for the
mechanical RPC (acceptance criterion 8) should mirror this same failure-mode
matrix minus the backlog-status-transition assertions.

## 2. `GitWorktree` mechanical primitives — exact semantics (`session/git/worktree_git.go:300-437`)

- **`PushBranch()`** (`:315`) — hardcoded `git push -u origin <branchName>`,
  60s timeout. No force-push option, no base-branch parameter (branch name is
  fixed at `GitWorktree` construction time). Failure wraps combined output +
  error.
- **`CreatePR(title, body string)`** (`:329`) — **no base-branch parameter at
  all**. It runs `gh pr create --title ... --body ... --head <branchName>`
  with no `--base` flag, so it always relies on `gh`'s own default-branch
  resolution. **This is a gap against acceptance criterion 3** ("base-branch
  selection in the UI") — `CreatePR`'s signature needs a new `base string`
  parameter (appending `--base <base>` to `args` when non-empty) before the
  modal's base-branch field can actually take effect; today the field would
  be UI-only and silently ignored.
  - Calls `checkGHCLI()` first (`session/git/util.go:46`) — checks `gh` is on
    `PATH` and `gh auth status` succeeds (10s timeout), else returns a
    specific "not installed" / "not configured, run gh auth login" error —
    this satisfies acceptance criterion 6's "gh not authenticated" case
    verbatim, already returns exactly that message.
  - Calls `findExistingPR()` **first**, before attempting create — if found,
    returns the existing PR immediately without creating a duplicate (this is
    what satisfies acceptance criterion 4 for free, see above).
  - On `gh pr create` failure, re-checks `findExistingPR()` once more (handles
    a race where the PR was created concurrently between the first check and
    the `gh pr create` call) before surfacing the error.
  - PR number is parsed from the URL via regex first (`prNumberFromURLRe`),
    falling back to a second `gh pr view --head` call only if URL-parsing
    fails — this was itself a past bug fix (silently-swallowed error left
    `prNumber` at 0, which broke `EnablePRAutoMerge` downstream) — worth
    preserving as-is, don't regress it.
- **`findExistingPR()`** (`:396`) — `gh pr list --head <branchName> --json
  number,url --jq '.[0] | .number, .url'`. Returns an error (not just empty)
  if no PR is found — callers must treat "err != nil" as the normal/expected
  "no PR yet" case, not a hard failure. **Important:** this only looks up by
  `--head <branchName>`, not by base branch — if the modal lets a user pick a
  *different* base branch than a previous open PR's base, `findExistingPR`
  will still find and reuse that old PR (whose base won't match the newly
  selected one) rather than creating a second PR against the new base. This
  is an edge case the design should explicitly decide on (reuse regardless of
  base mismatch, vs. compare `existingURL`'s base too).
- **`HasCommitsAheadOfMain(mainBranch string)`** (`:428`) — wraps
  `BranchAheadBehind`. Fail-open (returns `true`, i.e. "assume there's
  something to ship") both on error and when the branch doesn't exist
  locally — see edge case #6 above.

## 3. PR-URL persistence path used by `RunOneShot` (`server/services/session_service.go:3616-3718`)

The **exact** persistence contract the new mechanical RPC must reuse so the
session card's existing `githubPrUrl` display, PRStatusPoller, and any other
downstream consumer keep working unchanged:

```go
inst.SetGitHubPR(prURL, prNumber)                    // session/instance_actor_setters.go:155
if err := s.storage.SaveInstances(s.allInstances()); err != nil { ... }
s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_url", "github_pr_number"}))
```

- `SetGitHubPR` is actor-routed (`sendSyncErr`) — sets `Instance.GitHubPRURL`
  / `Instance.GitHubPRNumber` under the instance's internal actor lock and
  rebuilds the snapshot atomically (`session/instance_actor_setters.go:144-160`).
- Persistence is a **separate, fallible step** (`s.storage.SaveInstances`) —
  `RunOneShot` only *logs a warning* on save failure today (`log.Warn(...)`,
  not a returned error) — this is weaker than `pushAndCreatePR`'s BUG-040 "a
  persist failure must not be silently swallowed" fix on the backlog side.
  The new RPC should decide explicitly whether to match `RunOneShot`'s
  best-effort logging or apply the stricter backlog-side discipline (return
  an error/response flag so the modal can tell the user "PR was created on
  GitHub but the session card may not reflect it yet").
- After persisting, `RunOneShot` also calls
  `s.backlogLifecycleListener.RecordPRCreatedOutOfBand(ctx, inst.UUID, prURL,
  prNumber)` — a no-op for non-backlog sessions, but **must still be called**
  by the new mechanical RPC too, in case it's ever invoked against a
  backlog-linked session's worktree via the same generic "any session" button
  (the requirements don't exclude backlog sessions from using the new manual
  modal — a backlog session sitting in `review` could plausibly also get a
  manual "Create PR" click from a human). Skipping this call would silently
  regress the exact bug `RecordPRCreatedOutOfBand` was added to fix.
- `RunOneShotResponse.PrUrl` / `.BranchDivergedFromBase` are the proto fields
  populated on success — the new RPC's response message should carry
  equivalent fields (`PrUrl`, `PrNumber`, and probably an `already_existed:
  bool` to let the modal distinguish "created" vs. "reused" for its
  confirmation UI, since `CreatePR` doesn't currently expose that distinction
  to callers — it returns the same `(url, number, nil)` shape either way).

## 4. Worktree availability — does every session have one?

`session/instance.go:434-448` defines the `SessionType` constants (aliased
from `config.SessionType`): `SessionTypeDirectory`, `SessionTypeNewWorktree`,
`SessionTypeExistingWorktree`, `SessionTypeNewProject`, `SessionTypeOneOff`.

- `Instance.HasGitWorktree()` (`session/instance_worktree.go:204-206`) wraps
  `i.gitManager.HasWorktree()` — **not all session types have one.**
  `SessionTypeDirectory` and `SessionTypeOneOff` sessions operate directly on
  a plain directory/temp dir with no git worktree at all (one-off sessions
  aren't even guaranteed to be a git repo). Only `SessionTypeNewWorktree` /
  `SessionTypeExistingWorktree` (and likely `SessionTypeNewProject`, which
  git-inits a new repo) reliably have a worktree.
- **This directly affects acceptance criterion 1** ("session card / diff
  viewer for a session with an active worktree") — the button/action must be
  gated on `inst.HasGitWorktree()` (or equivalently `GetEffectiveRootDir()`
  resolving to a worktree path, not the bare `Path`), not shown
  unconditionally on every session card. A `SessionTypeDirectory` session
  could still be *inside* a git repo (just not a worktree-isolated one) —
  worth clarifying whether "Create PR" should also work there via `git push`
  from `Path` directly, or whether the feature is worktree-only. The
  requirements text explicitly says "session with an active worktree," which
  reads as scoping it to worktree-backed session types only — directory-mode
  sessions on a plain git checkout are out of scope unless product wants to
  broaden it later.
- `GetGitWorktree()` (`session/instance_worktree.go:196-201`) additionally
  requires `i.started.Load()` — errors with `"cannot get git worktree for
  instance that has not been started"` if called before the session has been
  started at least once. A newly created but never-started session would
  fail here even if its `SessionType` is worktree-based — another gating
  condition for the button/RPC.

## 5. Unstated needs / design gaps worth flagging to the plan phase

1. **Base-branch plumbing is incomplete end-to-end.** `CreatePR` has no
   `--base` support today (see §2) — acceptance criterion 3 needs a
   `GitWorktree.CreatePR` signature change, not just UI work. Flag this as a
   required implementation task, not an assumed pass-through.
2. **No diff-preview step mentioned in acceptance criteria**, but
   `DraftPRDescription` (`session/headless/features.go:280-294`) **hard-fails
   on an empty diff**: `fmt.Errorf("DraftPRDescription: empty diff, nothing
   to describe")`. If a user opens the modal for a session with zero net
   diff (e.g. already merged, or a no-op session), body generation itself
   will error before `CreatePR` even runs — this should surface as "no
   changes to describe" in the modal rather than a raw draft-description
   error, and ties back to the same `HasCommitsAheadOfMain` pre-flight gate
   in §1 edge case #5 that should prevent the modal from opening at all in
   that state.
3. **`DraftPRDescription`'s signature is backlog-item-shaped**
   (`itemTitle, itemDescription, diff, branchName`) — for a non-backlog
   session there is no `item.Description`/"problem statement" equivalent.
   The plan needs to decide what to pass as `itemDescription` for a plain
   session (empty string? the session's own title/goal field, if any exists
   — check `Instance` for a task/goal field via `get_session_goal`-style
   data). Passing an empty description changes the LLM prompt shape slightly
   from what backlog sessions get, worth a conscious decision, not a silent
   default.
4. **No "cancel mid-flow" data risk** — unlike `pushAndCreatePR`, which
   commits any dirty state before pushing (edge case #2 above), the modal
   flow must decide whether opening the modal / pre-filling the diff should
   also auto-commit dirty changes eagerly (so the previewed diff matches what
   actually gets pushed), or defer any commit until the user actually
   confirms — auto-committing on modal *open* (before confirmation) would
   surprise a user who cancels.
5. **Re-push / no-op on retry**: if a PR already exists and the user reopens
   the modal after adding new commits, `CreatePR`'s existing-PR short-circuit
   (§2) means clicking "confirm" again will just return the same PR without
   re-pushing new commits, unless the RPC handler calls `PushBranch()`
   unconditionally before checking for an existing PR (as `pushAndCreatePR`
   does — push always happens first, *then* the existing-PR-reuse check is
   only for skipping a second `gh pr create` call, not for skipping the
   push). The new RPC must preserve this ordering (push first, then
   create-or-reuse) or new commits silently won't reach GitHub on a second
   click.
6. **Response needs an explicit "reused vs created" signal** (see §3) so the
   modal/toast can say "Updated existing PR #N" vs. "Created PR #N" —
   `CreatePR` doesn't currently return this distinction to its caller.

## Key file/line references

- `session/backlog_lifecycle.go:3585-3728` — `pushAndCreatePR` (full edge-case reference)
- `session/git/worktree_git.go:315-437` — `PushBranch`, `CreatePR`, `findExistingPR`, `HasCommitsAheadOfMain`
- `session/git/util.go:46-61` — `checkGHCLI`
- `server/services/session_service.go:3616-3739` — `RunOneShot`, `RunOneShotForSession`, PR-URL persistence
- `session/instance_actor_setters.go:144-180` — `SetGitHubPR`, `SetGitHubPRNumber`
- `session/instance_worktree.go:163-206` — `GetEffectiveRootDir`, `GetGitWorktree`, `HasGitWorktree`
- `session/instance.go:434-448` — `SessionType` constants
- `session/headless/features.go:280-294` — `DraftPRDescription`
- `session/backlog_lifecycle_test.go:2346-2729` — existing edge-case test catalog to mirror
- `web-app/src/components/sessions/SessionActionsOverflow.tsx:55-616` — current one-shot "Create PR" button + `oneShotResult` state machine (done/error) to replace per acceptance criterion 7
