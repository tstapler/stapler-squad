# Adversarial Review: GitHub Work Continuity Implementation Plan

**Date**: 2026-06-24
**Reviewer**: Adversarial analysis agent
**Verdict**: BLOCKED

---

## Claim Verification Results

### Verified CORRECT

- `session/pr_status_poller.go` exists. `SetInstances`, `AddInstance`, `RemoveInstance`, `Start`, `Stop` all present (lines 90, 97, 104, 126, 141). The file compiles (it imports only stdlib + `github/` + `log`).
- Proto fields 33–39 on the `Instance` message in `proto/session/v1/types.proto` — CONFIRMED. Fields are `github_pr_state=33`, `github_pr_is_draft=34`, `github_pr_priority=35`, `github_approved_count=36`, `github_changes_req_count=37`, `github_check_conclusion=38`, `last_pr_status_check=39`.
- `GitHubBadge.tsx` and `GitHubBadge.css.ts` exist in `web-app/src/components/sessions/`.
- `SessionCard.tsx` passes GitHub fields to `GitHubBadge` at lines 457–474 — CONFIRMED.
- `UnfinishedWorkService` exists in `server/services/unfinished_work_service.go` and `scanResultToProto()` is present at line 507.
- `UnfinishedWorktree` proto last field is 20 (`session_ids`) — CONFIRMED. Fields 21+ are available.
- `BUG-020` and `BUG-021` are in `docs/bugs/open/` — CONFIRMED.

### Verified INCORRECT

- **`github.CachedAuthState` type does not exist.** Story 2.1 references `*github.CachedAuthState` as a reusable type. `PRStatusPoller` stores auth state as inline `authOK bool` + `authCheckedAt time.Time` fields — not an exported type. The implementer will have to invent this type or restructure the design.

- **Wrong Makefile target.** Plan says `make generate-proto` (lines 257, 387, 683). The actual target in `Makefile:374` is `make proto-gen`. If an implementer follows the plan literally they will get `make: *** No rule to make target 'generate-proto'`.

- **`EventBus WorktreeUpdated` event name is wrong.** Stories 2.2 and 6.4 reference a `WorktreeUpdated` event type. The actual event type is `unfinished.work_updated` (`EventUnfinishedWorkUpdated` const in `session/unfinished/events.go:11`). The EventBus subscription pattern uses the `pkgevents.EventType` string, not a "WorktreeUpdated" label.

- **`GetOwnerRepoFromRemote` does not exist.** Story 2.1 says "verify this function exists in `github/client.go`; if not, add it." It does not exist. There is `GetRemoteURL(repoPath string) (string, error)` which returns the raw URL string, and `ParseGitHubRef(input string)` in `github/url_parser.go` that parses it. The implementer must compose these two; neither alone provides the owner+repo pair. This is a known gap but it's called out in the plan as "verify and possibly add" — low severity on its own, but the SSH URL patterns are handled by `ParseGitHubRef` so the actual implementation is safe.

---

## Fatal Flaw 1: `WorktreePRPoller` Will Create a Circular Import

**Severity: BLOCKER**

The plan places `WorktreePRPoller` in `session/worktree_pr_poller.go` (package `session`) and its struct definition includes:

```go
scanFeed   <-chan []unfinished.ScanResult
```

This requires `package session` to import `github.com/tstapler/stapler-squad/session/unfinished`.

Current import graph:
- `session/unfinished` imports: `log`, `pkg/events`, stdlib only. Does NOT import `session`.
- `session` imports: `github/`, `log`, stdlib. Does NOT import `session/unfinished`.

Adding `session` → `session/unfinished` is not a cycle by itself. However, `server/dependencies.go` imports both `session` (line 18) and `session/unfinished` (line 25). The issue surfaces if `session/unfinished` ever needs to reference `session.Instance` for the "session exclusion" logic described in the plan. The plan's Story 2.1 states:

> "skip any `ScanResult.WorktreePath` that appears in `PRStatusPoller`'s instance list (to avoid double-polling)"

Since `PRStatusPoller` lives in `session`, and the `WorktreePRPoller` needs to query it for exclusions, this means `WorktreePRPoller` (in package `session`) will hold a reference to a `*PRStatusPoller` — that part is fine (same package). But the `scanFeed <-chan []unfinished.ScanResult` field means package `session` must import `session/unfinished`. This is a **new import direction that does not currently exist**.

Go allows `session` → `session/unfinished` as long as `session/unfinished` does NOT import `session`. Currently it doesn't. But the plan is fragile here: any future addition to `session/unfinished` that imports `session` (e.g., for scanner-side session lookup) would immediately break the build with a cycle.

**More concretely: the plan does not mention this import direction at all.** It uses `unfinished.ScanResult` in the struct definition without noting that `session/worktree_pr_poller.go` would be the FIRST file in `package session` to import the `session/unfinished` subpackage. This is a design gap that could surprise the implementer when `go build` flags it. It is not currently a cycle but it is a new coupling that needs to be intentional.

**Patch needed**: Add a note in Story 2.1 that `session/worktree_pr_poller.go` will introduce a new `session` → `session/unfinished` import direction. State explicitly that this is intentional and that `session/unfinished` must never import `session` (enforce via a comment or `vet-architecture` rule). Alternatively, move `WorktreePRPoller` to a new package (e.g., `session/worktree_poller`) to keep the dependency graph cleaner.

---

## Fatal Flaw 2: `scanFeed <-chan []unfinished.ScanResult` Channel Does Not Exist

**Severity: BLOCKER**

Story 2.1's `WorktreePRPoller` struct declares `scanFeed <-chan []unfinished.ScanResult`. No such channel is produced by `unfinished.Scanner`. The Scanner's output interfaces are:

1. `GetAllResults() []ScanResult` — synchronous snapshot
2. `ScanDone() <-chan time.Time` — emits a timestamp after each full scan
3. EventBus events (`EventUnfinishedWorkUpdated`, `EventUnfinishedWorkRemoved`)

There is no `chan []ScanResult` anywhere. The plan's struct definition assumes an API that does not exist and would need to be added to `Scanner`.

Story 2.2 says "Scanner's `GetAllResults()` is called on the poller's first tick to seed the initial worktree list" — this is correct and feasible. But the struct field `scanFeed <-chan []unfinished.ScanResult` implies ongoing streaming of scan results via channel, which requires adding a new method to `Scanner` (e.g., `Subscribe() <-chan []ScanResult`) or rewriting the polling loop to use `ScanDone()` + `GetAllResults()`.

The plan never specifies which approach to use — it just declares the channel type and leaves the implementer to figure out that the API gap exists. This will waste hours when the implementer tries to wire `NewWorktreePRPoller(...)` in `dependencies.go` and finds no channel to pass.

**Patch needed**: Remove `scanFeed <-chan []unfinished.ScanResult` from the struct definition. Replace with: poll loop calls `GetAllResults()` after each `ScanDone()` tick (the channel that does exist). Story 2.1 should specify: "subscribe to `scanner.ScanDone()` and call `scanner.GetAllResults()` on each tick to refresh the worktree list."

---

## Fatal Flaw 3: `github.CachedAuthState` Type Reference Is a Fabrication

**Severity: BLOCKER**

Story 2.1's struct definition includes:

```go
authState  *github.CachedAuthState  // reuse same type as PRStatusPoller
```

This type does not exist in the `github` package. `PRStatusPoller` implements its auth caching with two unexported fields: `authOK bool` and `authCheckedAt time.Time`, alongside the `isAuthOK()` method. There is no exported `CachedAuthState` type.

The comment "reuse same type as PRStatusPoller" is factually wrong — there is nothing to reuse. An implementer following this spec will search for `github.CachedAuthState`, not find it, and be confused about whether they should create it (breaking the "no new types" guideline) or write inline fields like `PRStatusPoller` does.

**Patch needed**: Remove `authState *github.CachedAuthState` from the struct definition. Replace with the same inline pattern: `authOK bool; authCheckedAt time.Time; authMu sync.Mutex` and a private `isAuthOK()` method mirroring `PRStatusPoller`. If auth state is desired as a shared type, explicitly say "define `github.CachedAuthState` as a new exported struct with fields `...` and move `PRStatusPoller`'s auth logic into it" — but that's a refactor that's not called out as a task anywhere.

---

## Non-Fatal Issues (CONCERNS)

### Concern 1: Missing Task for `GitHubUserService` Service Registration

Story 3.4 says "Service is registered in the HTTP mux alongside existing services" and lists `server/server.go` or `server/routes.go` as the file to modify. But there is no `server/routes.go` — routes are in `server/server.go`. More importantly, the existing pattern in `server/server.go` (lines 332–354) registers each service with an `if deps.GitHubUserService != nil` guard. There is no task that explicitly adds `GitHubUserService` to the `ServiceDeps` and `RuntimeDeps` structs in `server/dependencies.go`, nor adds the registration block in `server/server.go`.

Story 3.4's file list includes `server/dependencies.go` but only says "add `UserPRCache` and `GitHubUserService` to deps" without specifying: (a) which struct fields to add, (b) which lines to add the `nil` guard registration. This is implementable but thinly specified given the pattern complexity.

**Recommendation**: Add a checklist item to Story 3.4: "Add `GitHubUserService *services.GitHubUserService` to both `ServiceDeps` and `RuntimeDeps` structs; add the mux registration block in `server/server.go` following the pattern at lines 332–354."

### Concern 2: Open Questions 1 and 3 from requirements.md Are Implicitly Resolved But Not Stated

Open Question 1: "how much of Stories 1–2 is already merged to main?" — the plan's "Current State Audit" table answers this fully. No gap.

Open Question 2: "all repos or only tracked repos?" — answered in requirements with "all repos" assumption. Plan's Story 3.1 uses GraphQL `is:pr author:$login` which returns all repos. Consistent.

Open Question 3: "Is `ScanResult.SessionIDs` currently populated or a stub?" — the field exists in `scanner.go:60` (struct definition) but is populated by `unfinished_work_service.go:82` (service layer, via `pathIndex`). The plan implicitly relies on this being correct (Story 2.3's `instancePRIndex` is modeled on `sessionPathIndex`). But the plan never states it confirmed this. An implementer reading only the plan won't know whether to trust `r.SessionIDs` from a raw `ScanResult` (not populated at scan time) vs from the enriched proto (populated by service layer). This matters for Story 2.1's `WorktreePRPoller`: if it receives raw `ScanResult` values from `GetAllResults()`, the `SessionIDs` field will be empty. The poller must not rely on it.

**Recommendation**: Add a note to Story 2.1: "Raw `ScanResult` from `scanner.GetAllResults()` has empty `SessionIDs` — this field is only populated by `UnfinishedWorkService` at the API layer. Do not rely on it in `WorktreePRPoller` to determine if a session exists."

### Concern 3: `make generate-proto` → Actual Target Is `make proto-gen`

Every reference to `make generate-proto` in the plan (lines 257, 387, 683) is wrong. The Makefile target is `make proto-gen` (`Makefile:374`). An implementer following the plan will get a make error on their first proto change.

**Recommendation**: Replace all occurrences of `make generate-proto` with `make proto-gen`.

### Concern 4: EventBus Event Name Mismatch

Stories 2.2 and 6.4 reference "EventBus `WorktreeUpdated` events." The actual constant is `EventUnfinishedWorkUpdated = "unfinished.work_updated"` in `session/unfinished/events.go:11`. The subscription in `unfinished_work_service.go` uses this event type. The plan's naming is informal and could lead an implementer to grep for "WorktreeUpdated" and not find the subscription point.

**Recommendation**: Replace "EventBus `WorktreeUpdated` events" with the actual constant: `EventUnfinishedWorkUpdated` from `session/unfinished/events.go`.

### Concern 5: Scope is 5 Weeks / Multiple PRs — Plan Should State PR Boundaries

The plan correctly says each Epic is "independently releasable" but does not specify the PR boundaries. With 5 weeks of work, this is 4-5 PRs at minimum. The current plan is structured as one document, which risks an implementer treating it as one PR. Story 1.3 (rate limit monitoring) adds unrelated complexity to what should be a clean mutex fix PR. Consider explicitly: "Epic 1 = PR 1, Epic 2 = PR 2, etc."

### Concern 6: `ListUnfinishedWorkResponse.last_scan` Field Conflict for `completed_worktrees`

Story 4.2 adds `repeated UnfinishedWorktree completed_worktrees = 2` to `ListUnfinishedWorkResponse`. But `ListUnfinishedWorkResponse` already has `last_scan = 2` (field number 2 at `unfinished.proto:52`). The plan's field number `= 2` for `completed_worktrees` is a direct conflict with `last_scan = 2`.

**Patch needed**: Change `completed_worktrees` to field number 3 (or the next available number after `last_scan = 2`).

---

## Summary

| # | Severity | Issue |
|---|----------|-------|
| 1 | BLOCKER | `session` → `session/unfinished` import is new and unacknowledged; structural risk |
| 2 | BLOCKER | `scanFeed <-chan []unfinished.ScanResult` channel does not exist in Scanner API |
| 3 | BLOCKER | `github.CachedAuthState` type referenced in Story 2.1 struct does not exist |
| 4 | BLOCKER | `completed_worktrees = 2` field number conflicts with existing `last_scan = 2` in `ListUnfinishedWorkResponse` |
| 5 | CONCERN | `GitHubUserService` mux registration not fully specified |
| 6 | CONCERN | Raw `ScanResult.SessionIDs` is empty; plan doesn't warn implementer |
| 7 | CONCERN | `make generate-proto` should be `make proto-gen` |
| 8 | CONCERN | EventBus event name "WorktreeUpdated" should be `EventUnfinishedWorkUpdated` |
| 9 | CONCERN | PR boundaries for 5-week plan not specified |

---

## Required Patches Before Implementation

### Patch 1 — Story 2.1: Remove `github.CachedAuthState` Reference

Remove this line from the struct definition:
```go
authState  *github.CachedAuthState  // reuse same type as PRStatusPoller
```
Replace with inline pattern mirroring `PRStatusPoller`:
```go
authOK        bool
authCheckedAt time.Time
authMu        sync.Mutex
```

### Patch 2 — Story 2.1: Remove `scanFeed` Channel Field

Remove `scanFeed <-chan []unfinished.ScanResult` from the struct. Replace the polling design with: "on each `pollTicker` tick, call `scanner.ScanDone()` to detect new scan completions, then call `scanner.GetAllResults()` to get the current worktree list." The `WorktreePRPoller` takes a `*unfinished.Scanner` as a constructor argument, not a channel.

### Patch 3 — Story 2.1: Acknowledge New Import Direction

Add an implementation note: "`session/worktree_pr_poller.go` introduces the first import of `session/unfinished` from within the `session` package. This is intentional. `session/unfinished` must not import `session` — add an architecture vet rule if the project has one."

### Patch 4 — Story 4.2: Fix Proto Field Number Conflict

Change `completed_worktrees` from field 2 to field 3 in `ListUnfinishedWorkResponse`. Field 2 is already `last_scan`.

### Patch 5 — All stories: Replace `make generate-proto` with `make proto-gen`

Three occurrences: lines 257, 387, 683 of `plan.md`.

### Patch 6 — Stories 2.2 and 6.4: Correct EventBus Event Name

Replace "EventBus `WorktreeUpdated` events" with "`EventUnfinishedWorkUpdated` events (const in `session/unfinished/events.go`)".
