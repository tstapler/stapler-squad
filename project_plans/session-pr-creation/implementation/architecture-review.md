# Architecture Review: session-pr-creation
**Date**: 2026-08-06
**Verdict**: CONCERNS

## Constitution Violations

N/A — `docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository.

## Blockers

- [ ] **Task 1.3.1c (`DraftPullRequest` body drafting) — nil-pointer panic on `s.headlessPool`.** The task calls `headless.DraftPRDescription(ctx, s.headlessPool, ...)` and only handles the case where the call returns `draftErr != nil`. `s.headlessPool` is a `*headless.Pool` that is documented and checked as nilable at three other call sites in the same file (`server/services/session_service.go:1625`, `:1805`, `:3653` — the last is `RunOneShot`'s own guard: `if s.headlessPool != nil { ... } else { /* subprocess fallback */ }`). `DraftPRDescription` immediately calls `pool.CallBlocking(...)` on the passed pool with no nil receiver guard (`session/headless/features.go:280-289`) — passing a nil pool will panic, not return an error, so the plan's fallback-on-`draftErr` path never triggers. Any deployment/test environment without a configured headless pool (the exact scenario `RunOneShot` already defends against) will crash `DraftPullRequest` on the modal's happy-path pre-fill.
  **Remediation**: add `if s.headlessPool != nil { body, draftErr = headless.DraftPRDescription(...) } else { body = fallbackBody }` mirroring `RunOneShot`'s existing nil-check at line 3653, before Epic 1.5's tests are written (a test exercising the nil-pool case should be added alongside `TestDraftPullRequest_*`).

## Concerns

- [ ] **Epic 1.4/1.3 placement — new mutating RPCs + in-flight-guard state added directly to `SessionService` rather than an extracted domain service.** `server/services/session_service.go` is already 4525 lines and the codebase has an established, repeatedly-used pattern for exactly this shape of feature — a cohesive unit of RPC handlers plus its own per-feature state (an in-flight guard, a cache) — extracted into a dedicated `*XService` struct that `SessionService` holds a field for and delegates to (`reviewQueueSvc`, `githubSvc`, `workspaceSvc`, `checkpointSvc`, etc., declared at `session_service.go:74-134`). Confirms the pattern: the codebase's *other* in-flight guards of this exact `sync.Map`-per-feature shape already live on extracted services, not on `SessionService` itself — `server/services/workspace_service.go:72` (`inFlightSwitches`), `server/services/backlog_service.go:164,186` (`spawnInFlight`/`triageInFlight`), `session/session_summary_service.go:71-74` (`inFlight`). The plan's own architecture.md explicitly considered and rejected `GitHubService` as the home for these RPCs (correct call — it lacks a `headlessPool`), but then defaulted to bolting onto `SessionService` rather than the alternative the codebase's own convention points to: a small new `PRCreationService` (storage, eventBus, headlessPool, backlogLifecycleListener) hosting `DraftPullRequest`/`CreatePullRequest`/`resolveSessionWorktree`/`prCreationInFlight`, wired into `SessionService` exactly like `githubSvc` is. `RunOneShot` already living directly on `SessionService` is a weaker precedent for this choice — it predates the service-extraction pattern the newer features (`workspace_service.go`, `backlog_service.go`) already follow, so mirroring it continues a growth pattern the codebase is otherwise moving away from, not the current convention.
  **Remediation**: extract `PRCreationService` (or fold into an existing extracted service if one already has the right dependencies) rather than adding `DraftPullRequest`/`CreatePullRequest`/`prCreationInFlight`/`resolveSessionWorktree` as new members of `SessionService` directly. This is a mechanical move (same handler bodies, same dependencies) — low risk, keeps the file from growing further, and matches the pattern reviewers of this codebase will expect for new self-contained mutating RPC pairs.

- [ ] **Task 1.4.1c — `already_existed` field's documented contract does not match what the implementation can actually produce.** The proto doc comment for `CreatePullRequestResponse.already_existed` (Task 1.2.1b) reads: "True when `CreatePR` returned a pre-existing PR instead of creating one." But Task 1.4.1c's described logic sets `already_existed = true` *only* on the handler's own fast path (when `inst.GitHubPrUrl` was already cached on the session), and unconditionally `false` whenever the call falls through to a real `wt.CreatePR(...)` call — even though `CreatePR` itself may have silently reused a pre-existing PR via its internal `findExistingPR` check (`session/git/worktree_git.go:339-342`, and again on the race-retry path at `:350-354`). The task text acknowledges this directly ("that call itself already transparently reuses an existing PR... but from this handler's perspective it made a 'create' attempt either way") but the remediation offered is a code comment, not a fix — the field will report "Created PR #123" to the user in cases where a PR genuinely already existed (e.g., created by a different flow, or a race with another caller) and was only reused. This is a representable-but-wrong state: the type's contract promises information the implementation cannot supply.
  **Remediation**: since Epic 1.1 is already changing `CreatePR`'s signature (to add `baseBranch`), extend it in the same pass to also return whether it reused an existing PR (e.g. `CreatePR(title, body, baseBranch string) (prURL string, prNumber int, alreadyExisted bool, err error)`), sourced from `findExistingPR`'s own success paths inside `CreatePR`. This makes `already_existed` correct by construction instead of approximated by the caller, and is a small, additive change to a function already being touched by this plan. Update `prCreator`'s interface and `pushAndCreatePR`'s call site accordingly (it can ignore the new return value, same as it already does for `baseBranch=""`).

- [ ] **`CreatePullRequestResponse`'s `persisted bool` + `persist_error string` fields permit an illegal state the implementation happens not to construct.** Per the task-brief's own note: Task 1.4.1d's logic (`persisted := true; persistError := ""`, flipped together only in the error branch) never actually produces `persisted=true` with a non-empty `persist_error`, but the *type* allows a future caller or a future edit to this handler to desynchronize the two fields — the wire contract does not enforce the invariant "persist_error is set iff persisted is false." This is the canonical "sum type would prevent a runtime error" case the type-driven-design lens flags. It's a narrower risk than the `already_existed` finding above (no known code path currently violates it), so this is a CONCERN, not a BLOCKER.
  **Remediation**: either (a) model the outcome as a proto `oneof persist_result { bool persisted = 4; string persist_error = 5; }` so the illegal combination is unrepresentable on the wire, or (b) if the `oneof`'s generated-code ergonomics on the TS side are judged not worth it for a single low-traffic response field, add an explicit invariant assertion/test (`persisted == (persistError == "")`) to Epic 1.5's test suite so a future edit that desyncs the two fields fails a test rather than shipping silently. Given this is a response field (not stored state) and the plan's own reasoning for choosing flat fields over a sum type elsewhere in this same message (`already_existed`) is sound, (b) is proportionate; only pick (a) if this pattern is likely to be copied elsewhere.

## Nitpicks

- The `resolveSessionWorktree(sessionID string) (*session.Instance, *git.GitWorktree, error)` helper (Task 1.4.1a) is appropriately scoped as described — it covers only the not-found/no-worktree/not-started checks shared by both handlers, and correctly excludes the existing-PR short-circuit (1.3.1b) and fast-path reuse check (1.4.1c), which differ between the two callers. If it moves into an extracted service per the concern above, keep it private to that service and resist adding parameters that only one of the two callers needs — that's the point at which it would start becoming a dumping ground.
- `DraftPullRequest`'s response type technically permits a state where `existing_pr_url` is set *and* `title`/`body` are simultaneously populated (nothing in the proto forbids it, even though Task 1.3.1b's early return means the implementation never produces it). Low severity — `title=""` isn't really an illegal domain state, just "not populated" — but worth a one-line doc comment on the proto message noting the two are mutually exclusive by convention, for the next person who edits this handler.
- Pattern Decisions' `baseBranch string` (no newtype) and the CQS two-RPC-vs-`dry_run`-flag choice are both well-reasoned and hold up under the type-driven-design and design-patterns lenses respectively — no changes recommended there.
- ADR-001 / build-vs-buy's "keep `gh` CLI shell-out" conclusion is followed correctly throughout the plan (Epic 1.1, 1.4) — verified consistent, no gaps found.

## Re-review (iteration 1, 2026-08-06)

**New verdict: CONCERNS.**

- Blocker (nil `s.headlessPool` panic) — **RESOLVED**. plan.md's Post-Review
  Revisions #1 adds the nil-guard mirroring `RunOneShot`'s existing pattern,
  plus a dedicated test. No remaining panic path.
- Concern (extraction into a dedicated service) — **RESOLVED**. Post-Review
  Revisions #3 + new Epic 1.0 extract `PRCreationService` matching the
  `reviewQueueSvc`/`workspaceSvc`/`backlogSvc` convention exactly.
- Concern (`already_existed` contract mismatch) — **not addressed**, carried
  forward as an open concern. Still recommend threading a real
  `alreadyExisted` return value through `CreatePR` itself (remediation
  unchanged from above) — proportionate to fix during Epic 1.1 since that
  function's signature is already being touched, but not blocking.
- Concern (`persisted`/`persist_error` illegal state) — **not addressed**,
  carried forward. Recommend option (b) (an invariant test) as originally
  proposed — low cost, not blocking.

No remaining BLOCKERs. Proceed to `sdd:4-validate`.
