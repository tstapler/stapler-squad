# Architecture Research: session-pr-creation

Agent 3 (Architecture). Scope: this is CRUD-shaped (one RPC pair + one modal) —
no Event-Command-Policy table, per the task brief.

## 1. Where the new RPC(s) live

**New methods directly on `SessionService`** (`server/services/session_service.go`),
following the `RunOneShot` / `GetSessionDiff` precedent, not a new top-level
service.

- Proto: add to `service SessionService` in `proto/session/v1/session.proto`
  (same file/service block as `rpc RunOneShot(...)` at line 281).
- Registration: **no separate wiring needed.** `server/server.go:348` registers
  the entire `SessionService` in one call —
  `sessionv1connect.NewSessionServiceHandler(deps.SessionService, ...)` — so
  any new method on `*SessionService` is exposed automatically once
  `make proto-gen` regenerates the connect bindings. This is simpler than the
  research question assumed; there is no per-RPC registration step.
- Run `make proto-gen` after the proto edit (regenerates
  `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`).

**Why not delegate to `GitHubService`** (`server/services/github_service.go`,
which already backs `GetPRInfo`/`MergePR`/`ClosePR` etc.): `GitHubService` is
constructed with only `storage` (`NewGitHubService(concStorage)`,
`session_service.go:361`) — it has no `headlessPool`, which
`headless.DraftPRDescription` requires. `SessionService` already has both
`s.headlessPool` (`session_service.go:183`) and the exact
`git.NewGitWorktreeFromStorage` reconstruction pattern `GetSessionDiff` uses
for non-live sessions (`session_service.go:2683`). Threading a `headlessPool`
dependency into `GitHubService` for this one feature would be new plumbing for
no benefit — putting the new methods directly on `SessionService` (mirroring
`RunOneShot`, which already does its own headless-pool call) needs zero new
constructor wiring.

## 2. Two-step RPC shape — required, not just a latency question

AC1/AC2 require the modal to open **pre-filled** with a title/body the user
can then edit before confirming. That is structurally two round trips
regardless of `DraftPRDescription`'s latency: the server must produce the
draft before the client can render it for editing, and only the user's edited
version is submitted back. So the shape is:

1. **`DraftPullRequest(session_id)`** — unary, synchronous. Server:
   - resolves the session's `GitWorktree` (see §3),
   - computes the diff (`session.GetGitDiff`, same helper `pushAndCreatePR`
     uses),
   - calls `headless.DraftPRDescription(ctx, pool, sessionTitle, "", diff, branchName)`
     for the body (same call, same prompt template — do not add a new one,
     per requirements),
   - resolves default base branch (see §3a),
   - returns `{title, body, base_branch, has_commits_ahead}`.
2. **`CreatePullRequest(session_id, title, body, base_branch)`** — unary,
   synchronous. Calls `GitWorktree.CreatePR(title, body)` directly (the
   mechanical path — see §4 on the base-branch gap), persists
   `GitHubPrUrl`/`GitHubPrNumber` + publishes the session-updated event exactly
   like `RunOneShot` does today (`session_service.go:3686-3709`), and returns
   the PR URL.

Both stay **unary/synchronous**, matching `RunOneShotRequest`/`Response`'s own
shape (`proto/session/v1/session.proto:1946`) — there is no streaming
precedent for LLM calls in this codebase, and `DraftPRDescription` is a single
short headless call (draft-body only), not a full agentic turn, so it fits
comfortably inside a normal ConnectRPC unary timeout. No new async/polling
mechanism is needed.

**AC4 (existing PR) needs no extra plumbing in the preview step.**
`GitWorktree.CreatePR` (`session/git/worktree_git.go:329`) already checks
`findExistingPR()` first and returns the existing URL/number instead of
creating a duplicate — it's already idempotent. Separately, the frontend
already receives `session.githubPrUrl` on every session (proto
`types.proto:87`, populated today by `RunOneShot`'s persist step) — so the
"Create PR" affordance can simply be hidden/replaced with "View PR" whenever
`githubPrUrl` is already set, without any new preview-time check. `DraftPullRequest`
does not need to duplicate `findExistingPR` — it's private to the `git`
package and unnecessary here.

## 3. Constructing the `GitWorktree` from `SessionService`

**Already reachable, two paths depending on liveness — exact precedent in
`GetSessionDiff` (`session_service.go:2649`):**

- **Live instance:** `session.Instance` already exposes
  `func (i *Instance) GetGitWorktree() (*git.GitWorktree, error)`
  (`session/instance_worktree.go:196`), returning the concrete `*git.GitWorktree`
  directly — no interface needed. This is already used elsewhere (e.g.
  `session/review_queue_determiner.go:71`). `SessionService.findInstance(id)`
  (used throughout, e.g. `RunOneShot` at `session_service.go:3627`) gets the
  `*session.Instance`; call `.GetGitWorktree()` on it.
- **Non-live / completed session:** reconstruct via
  `git.NewGitWorktreeFromStorage(wt.RepoPath, wt.WorktreePath, wt.SessionName, wt.BranchName, wt.BaseCommitSHA)`
  from `session.InstanceData.Worktree`, exactly as `GetSessionDiff` does at
  `session_service.go:2683-2689` for the "session not live" branch. Whether PR
  creation should even be offered for a non-live/completed session is a
  product question (AC1 says "session with an active worktree" — likely scope
  this to live instances only, but the reconstruction path exists if needed).

Either way, the result is a concrete `*git.GitWorktree`, which already has
`CreatePR`, `HasCommitsAheadOfMain`, `PushBranch`, etc. — the same type
`session/backlog_lifecycle.go`'s `prCreator` interface wraps.

### 3a. Default base branch

`pushAndCreatePR` hardcodes `bounceMainBranch = "main"` (`session/stuck_decisions.go:44`)
for its own ahead/behind checks — not a generic "resolve the repo's actual
default branch" helper. A real one already exists and is used elsewhere:
`Scanner.ResolveDefaultBranch(repoPath string) string`
(`session/unfinished/scanner.go:694`, used by
`server/services/unfinished_work_service.go:464`). Reuse that for AC1's
"defaulting to the repo's main branch" rather than hardcoding `"main"` — the
existing hardcode is a narrower internal control (backlog bounce-back
detection), not the right precedent for a user-facing default.

## 4. Gap: `CreatePR` has no base-branch parameter

`GitWorktree.CreatePR(title, body string)` (`session/git/worktree_git.go:329`)
shells out to `gh pr create --title ... --body ... --head <branch>` — **no
`--base` flag**. Base branch resolution today is entirely `gh`'s own implicit
default-branch resolution. AC3 ("base-branch selection in the UI") therefore
needs a signature change — e.g.
`CreatePR(title, body, baseBranch string) (...)`, passing `--base baseBranch`
to `gh pr create` when non-empty. This is a small, additive change but touches
a method the `prCreator` interface also declares
(`session/backlog_lifecycle.go:249`) — updating the interface and its only
other caller (`pushAndCreatePR`, which can pass `""` to preserve today's
implicit-default behavior) is in scope for the plan phase, not something to
defer.

## 5. Interface-pollution check: no new interface needed

`session/backlog_lifecycle.go`'s `prCreator` interface (line 246) **is
justified** — it has two real implementations: `defaultPRCreatorFactory`
(production, wraps `git.NewGitWorktreeFromStorage`, line 258) and test fakes
installed via `SetPRCreatorFactory` (line 621, used across
`backlog_lifecycle_test.go`/`backlog_lifecycle_stuck_test.go`). That's the
consumer-side, 2+-implementation pattern the checklist calls for — it does not
need to change for this feature.

The new RPC handlers on `SessionService` should **not** introduce a parallel
interface. Call the concrete `*git.GitWorktree` returned by
`Instance.GetGitWorktree()` / `git.NewGitWorktreeFromStorage(...)` directly —
`SessionService` has exactly one implementation to call and no test-fake
requirement beyond what existing session-service tests already do (they
construct real worktrees against temp git repos, e.g.
`server/services/session_retention_sweeper_test.go:136`, rather than mocking
`GitWorktree`). Introducing a `prCreator`-style interface here would be a
speculative one-implementation abstraction — the anti-pattern
`.claude/rules/interface-pollution-checklist.md` calls out. If a Go unit test
for the new RPC needs to avoid shelling out to `gh`, follow
`backlog_lifecycle.go`'s own pattern: a package-level factory var overridable
in tests (same shape as `prCreatorFactory`), not a new interface on
`SessionService`.

## 6. Frontend placement

- **Existing one-shot button to replace** (AC7): `SessionActionsOverflow.tsx`
  lines 606-618 — the `onRunOneShot` menu item (`🚀 Create PR` /
  `✅ PR Created` / `❌ Retry?` states, driven by `oneShotResult`/
  `isRunningOneShot` local state). Wired from `SessionCard.tsx:113,144,876`
  up through `web-app/src/app/page.tsx:294` (`handleRunOneShot`, which just
  calls `runOneShot(sessionId, "Create a pull request...", 0)`) and
  `web-app/src/app/review-queue/page.tsx:343`. All of these call sites need
  the prop/handler swapped from "run one-shot" to "open PR modal."
- **New modal**: no dedicated PR-creation modal exists yet. Closest structural
  precedent is `ResumeSessionModal.tsx`
  (`web-app/src/components/sessions/ResumeSessionModal.tsx`) — a focused
  single-purpose modal taking `session`, `onConfirm`, `onCancel` props, using
  `useFocusTrap`, local `useState` for editable fields, and an
  `isSubmitting` guard. Follow that shape for the new
  `CreatePullRequestModal.tsx` (or similar name) with title/body/base-branch
  fields, colocated `.css.ts` per ADR-009 (`.claude/rules/css-architecture.md`).
- **Diff viewer**: `web-app/src/components/sessions/DiffViewer.tsx` is the
  component named in the task brief. AC1 says the action should live "on a
  session card / diff viewer" — the overflow menu (session-card-level, same
  location as today's button) is the natural primary entry point since it's
  already where users trigger PR creation; adding a second entry point inside
  `DiffViewer.tsx` is optional/nice-to-have, not required to satisfy AC1's
  wording (which offers "session card" as a sufficient location).

## 7. Data flow / refresh after success

No new refresh mechanism needed — reuse what `RunOneShot` already relies on:

- Backend: on success, call `inst.SetGitHubPR(prURL, prNumber)` +
  `s.storage.SaveInstances(...)` + `s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_url", "github_pr_number"}))`
  — identical to `session_service.go:3691-3696`.
- That event flows through the existing `WatchSessions` server-streamed RPC
  (`rpc WatchSessions(...) returns (stream SessionEvent)`,
  `proto/session/v1/session.proto:29`), which the frontend already subscribes
  to (`web-app/src/lib/hooks/useSessionService.ts`, exercised in
  `useSessionService.test.ts:107-297`). Session cards re-render from that
  stream today for every other async field update (status changes, PR URL
  from `RunOneShot`, etc.) — the new `CreatePullRequest` RPC gets this for
  free by publishing the same event type. No polling or new websocket wiring
  is required.
- Also worth reusing: `s.backlogLifecycleListener.RecordPRCreatedOutOfBand(...)`
  (`session_service.go:3706-3708`) — if the session behind this RPC happens to
  be backlog-linked, this call keeps the backlog item's `pr_pending`
  transition in sync exactly as `RunOneShot` already does. Skipping it for the
  new mechanical RPC would reintroduce the exact bug that comment documents
  (item stuck in `review` forever). Call it unconditionally in
  `CreatePullRequest` too (it's a documented no-op for non-backlog sessions).

## Open items for the plan phase (not decisions made here)

- `CreatePR`'s new `baseBranch` parameter (§4) requires updating the
  `prCreator` interface and its one production caller
  (`pushAndCreatePR`) — small but touches shared code; plan should call this
  out as its own task with its own test update.
- Whether `DraftPullRequest`/`CreatePullRequest` should gate on
  `HasCommitsAheadOfMain` before even opening the modal (AC1: "at least one
  commit ahead of its base branch") — `HasCommitsAheadOfMain(mainBranch string) (bool, error)`
  already exists on `GitWorktree` (used by `pushAndCreatePR`,
  `session/backlog_lifecycle.go:3638`) and can be reused directly in
  `DraftPullRequest`.
