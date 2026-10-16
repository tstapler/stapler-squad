# Architecture Review: github-call-optimization
**Date**: 2026-09-08
**Verdict**: CONCERNS

No `docs/adr/ADR-000-architecture-constitution.md` exists in this repository — no constitution
check applies.

`kibitzer` is on `PATH` and `.claude/inspect.json` exists, but this review is grounded primarily
in direct `Read`/`Grep` verification of the touched files against the plan's own line-number
claims (a stronger check than a batch architecture check for a not-yet-written-code review) —
every concrete finding below cites the file/line evidence used.

## Blockers

None remaining. Both blockers raised in iteration 1 were verified resolved in iteration 2
(re-review against the live code):

- **Task 1.1.1b / new Task 1.1.1a-prime** (`github/client.go`) — RESOLVED. Plan.md now contains
  Task 1.1.1a-prime, which adds `ctx context.Context` as the first parameter directly to
  `github.GetPRComments`, `GetPRDiff`, `PostPRComment`, `MergePR`, `ClosePR`, replacing each
  function's internal `context.WithTimeout(context.Background(), ...)` with
  `context.WithTimeout(ctx, ...)`. `github/client.go` is now listed in Story 1.1.1's Files. Verified
  against the live file: the five functions are at lines 589/634/657/680/712 and take no
  `context.Context` parameter today, and their internal `context.WithTimeout(context.Background(), ...)`
  calls are at exactly the lines the plan cites (598, 642, 665, 698, 720) — the plan's line
  references are accurate.

- **Story 1.3.3 / Task 1.3.3b** (`session/git/worktree_git.go`) — RESOLVED. Plan.md's Task 1.3.3b
  now lists all 11 `gh`-prefixed call sites (71, 84, 301, 356, 390, 404, 674, 834, 854, 871, 885),
  matching `grep -n 'commandRunner().Run(.*"gh"' session/git/worktree_git.go` run against the live
  file, which returns exactly those 11 lines (count verified: 11). The plan also explicitly notes
  the original 7-line list omitted lines 71, 84, 301, and 390, and widens the task's time estimate
  accordingly.

## Concerns

- [ ] **Story 3.2.2 / Task 3.2.2a + Story 3.3.1** (`github/http_client.go`,
  `session/pr_status_poller.go`) — `PRStatusPoller.handleFetchError` (`session/pr_status_poller.go:370-382`)
  classifies errors via `strings.Contains(msg, "rate limit")`/`"429"`/`"401"`/`"Unauthorized"` —
  stringly-typed error classification, not a sentinel error (`github.ErrGitHubAccessDenied`/
  `ErrGitHubRefNotFound` exist as sentinels in `github/repos.go:18-25`, but every rate-limit error
  in `github/http_client.go:222,226,235` and `github/etag_cache.go:92,96,98,102` is a bare
  `errors.New`/`fmt.Errorf` string with no wrapped sentinel). Story 3.3.1's acceptance criteria
  requires the new `AdmitOrigin` rejection error to land in the *same* `handleFetchError` branch a
  real rate-limit error does today, "confirmed by a test" — but this only works if whoever
  implements Task 3.2.2a happens to phrase the rejection error text to satisfy
  `handleFetchError`'s substring check (the plan's own example reason string, "background headroom
  reserved," contains neither "rate limit" nor "429"). If the literal reason string is returned
  verbatim as the error, `handleFetchError` returns `false`, so `fetchAndUpdatePRStatus` falls
  through to a generic `log.Warn("PR status poller: failed to fetch PR", ...)` instead of the
  rate-limit-specific branch — not a correctness bug (both paths just `return` without corrupting
  PR state — verified by reading `session/pr_status_poller.go:308-362`), but it does mean Task
  3.3.1a's test, as specified, is only accidentally satisfiable and will force a rework cycle once
  the mismatch is discovered. This is also the "Extend as-is" case the
  `code-architecture-best-practices` skill flags: the plan bolts a new rejection path onto an
  already-stringly-typed error-classification scheme without stating that choice.
  **Remediation**: either (a) require `AdmitOrigin`'s rejection error text to explicitly contain
  "rate limit" (e.g. `"github: background rate-limit headroom reserved"`) and say so in Task
  3.2.2a's acceptance criteria, or (b) the more durable fix — wrap the rejection in a new sentinel
  (`github.ErrAdmissionRejected`) and change `handleFetchError` to check `errors.Is` first, falling
  back to substring matching only for pre-existing untyped errors. (b) also closes the gap for any
  future rate-limit error text that doesn't happen to contain the magic substring.

- [ ] **Story 5.3.1 / Task 5.3.1c** (`server/services/github_webhook_pr_fix.go`) —
  `GitHubPollerInvalidator.InvalidateForEvent(ctx, repoFullName string, prNumbers []int)` is
  designed to take the *whole* batch of changed PR numbers in one call (per Story 5.3.1's own
  acceptance-criteria example: `InvalidateForEvent(ctx, "tstapler/stapler-squad", []int{704})`),
  but Task 5.3.1c instructs wiring it "inside the existing per-PR-number loop" — and
  `handlePRFixEvent`'s existing loop (`github_webhook_pr_fix.go:558-568`, verified by direct read)
  is `for _, prNumber := range prNumbers { h.prFixRouter.TriggerPRFixForEvent(ctx, fullName,
  prNumber) }`, a per-single-int loop, because `TriggerPRFixForEvent` itself takes one `int`, not a
  slice. Calling a slice-typed `InvalidateForEvent` from inside that loop means either wrapping
  each single `prNumber` in a throwaway `[]int{prNumber}` on every iteration (defeating the
  batch-shaped interface — GoF/PoEAA Lens #10, an API contract whose actual call site never uses
  the shape it was designed for) or the task description is simply wrong about where the call
  goes. Neither is stated as a deliberate choice.
  **Remediation**: pick one and say so — either (a) change `InvalidateForEvent`'s signature to
  take a single `prNumber int` (mirroring `TriggerPRFixForEvent`'s existing per-PR shape it sits
  right next to), or (b) move the call outside the loop, invoked once with the full `prNumbers`
  slice collected from the loop (or directly from `extractPRFixEvent`'s output, before the loop
  starts).

- [ ] **`github/rate_limit.go`, Epic 3.1** — `RateLimiter` will hold two different concurrency
  primitives for two different fields with no documented relationship between them:
  `rateLimitedUntil time.Time` under `mu sync.RWMutex` (existing), and the new `snapshot
  atomic.Pointer[RateLimiterSnapshot]` (Task 3.1.1a). `Update()` writes both — conditionally (only
  on primary/secondary exhaustion) for `rateLimitedUntil` via `setLimitedUntil`, unconditionally
  for `snapshot` — meaning a caller reading both `IsLimited()` and `Snapshot()` in sequence (as
  Story 3.2.3 explicitly requires, "not a separate parallel state machine") can observe them from
  two different `Update()` calls in a race window, since nothing ties the two writes together
  atomically. The plan's Story 3.2.3 argues this is safe *by construction* because `IsLimited()` is
  checked first and is more conservative — that argument holds for the specific ordering Task
  3.2.2a uses, but it's an implicit invariant ("check `IsLimited()` before `AdmitOrigin`, always")
  resting on two independently-synchronized fields, not something the type system or a single lock
  enforces. A future call site that reads `Snapshot()` alone (e.g. the OTel gauge callback, Task
  1.2.1d) without also consulting `IsLimited()` would see a `Remaining` value that doesn't reflect
  an active secondary-rate-limit cooldown.
  **Remediation**: not a blocker since Task 3.2.3a adds a regression test asserting the check
  order, but the invariant deserves a doc comment on `RateLimiterSnapshot` itself (not just in the
  plan) stating "this reflects `remaining`/`limit` only; always check `IsLimited()` for the
  `rateLimitedUntil` cooldown separately, never infer cooldown state from `Remaining == 0`" so a
  future reader of `rate_limit.go` doesn't need to reconstruct this from the plan doc.

## Nitpicks

- **Domain Glossary / `GitHubPollerInvalidator`** — the interface method name `InvalidateForEvent`
  doesn't match either poller's own vocabulary (`InvalidateAndRefresh`, `InvalidateCache`); a name
  like `InvalidatePR`/`InvalidateForPRs` would read more consistently next to the two concrete
  methods it fans out to, though this is cosmetic.
- **Epic 1.3's two adapters** (`ghCLIExecAdapter`, `worktreeGHCommandAdapter`) — the Domain
  Glossary names both, but only `worktreeGHCommandAdapter`/`runGHCommand` appears as a concrete
  task (1.3.3a); `ghCLIExecAdapter` as a distinct named type never gets its own implementation
  task in Epic 1.3.2 — Task 1.3.2a-c just call `runGHCLICommand` directly at each `github/*.go`
  call site with no adapter struct in between. Harmless (the glossary entry may just mean "the
  pattern of wrapping `safeexec.CommandContext` sites," not a literal type), but worth a one-line
  clarification so a reader doesn't go looking for a `ghCLIExecAdapter` struct that was never
  meant to exist.
- **Task 3.1.1d / Epic 3.1's mandatory `-race` gate** — good practice per `build-vs-buy.md` §6's
  explicit call for it, but the plan should make this a named CI gate (e.g. add it to
  `make ready`'s scope or call out that `go test -race ./github/...` is a merge-blocking check in
  the PR description), not just a task acceptance criterion that could be satisfied once locally
  and never re-run.
