# Adversarial Review: backlog-self-resolve

**Date**: 2026-08-02
**Verdict**: BLOCKED

## Blockers

- [ ] **FR10's "eventually surfaces through the existing stuck-item notification path(s)" claim is
  false for the scenario the requirement is actually about — code-verified, not inferred.** The plan's
  Observability Plan and pitfalls.md §6 both assert FR10 is satisfied entirely by the pre-existing
  `pr_pending_no_pr` detector (`reconcilePRPendingWithoutPRItems`,
  `session/backlog_lifecycle.go:2543-2590`). Reading that function directly: its loop is gated
  `if item.PrNumber != 0 { continue }` (`session/backlog_lifecycle.go:2553-2555`) — it only flags
  `pr_pending` items with **no** PR reference at all (`pr_number == 0`), by explicit design (see its
  own doc comment, lines 2526-2542: "flags any item stuck in pr_pending status with no PR reference at
  all"). FR10's actual scenario is the opposite: an item that reached `pr_pending` via a successful
  `report_pr_created` — so `PrNumber` is already nonzero — and then got stuck because a *subsequent*
  `report_duplicate` verification call kept failing. This is not a hypothetical edge case; it is the
  literal worked example requirements.md opens with (item `da58b867-...`, its own PR #281 already
  open, `PrNumber != 0` by construction). That item is structurally invisible to `pr_pending_no_pr`
  forever — it will `continue`-skip it on every tick. The other `pr_pending`-scoped reconciler,
  `ReconcilePRPending` (`backlog_lifecycle.go:3850+`), requires `PrNumber != 0` (the opposite gate,
  line 3857) and only reacts to the state of the item's *own* PR on GitHub (merged/closed) — it has no
  concept of "repeated report_duplicate verification failures" and produces no stuck notification for
  that condition; an item in this shape just looks like a normal, still-open `pr_pending` item to it.
  As written, FR10's second half has no working implementation path for the scenario the requirement
  and its own worked example describe.
  **Recommendation**: this needs a decision before implementation starts, not a Phase 5 surprise —
  either (a) extend `reconcilePRPendingWithoutPRItems`'s scope (or add a narrowly-targeted new stuck
  reason keyed off the verification-failure log line rather than `PrNumber`) so a `PrNumber != 0` item
  stuck by repeated `report_duplicate` failures is also caught, or (b) get explicit owner sign-off
  narrowing FR10's acceptance criterion for v1 (the same way UQ-1/UQ-2 were explicitly resolved rather
  than left ambiguous), and update the Observability Plan to stop citing a detector that doesn't cover
  this case.

## Concerns

- [ ] **FR2's active-reviewer guard fails open when `ListItemSessions` errors (Task 2.2.1b).** The
  guard is `itemSessions, lsErr := h.storage.ListItemSessions(ctx, itemID); if lsErr == nil &&
  services.HasActiveReviewSession(itemSessions) { refuse }` — on a storage error the guard is skipped
  entirely and the call falls through to attempt the transition as if no reviewer were active. FR2's
  text is an unconditional "must refuse" with no error-path carve-out. Consequence is bounded (the
  underlying CAS still prevents double-apply, and `TriggerReviewForSession`'s own idempotency — which
  Task 3.3.3b's own reasoning already leans on — is what makes a second trigger harmless), which is why
  this is a concern rather than a second blocker, but it's still a silent, untested violation of an
  explicit MUST on precisely the failure path this review was asked to check.
  **Recommendation**: fail closed on a `ListItemSessions` error (`ErrInternalError`, "could not verify
  active-reviewer state — retry") instead of falling through, and add a test that injects a storage
  error and asserts refusal.

- [ ] **FR5's message-accuracy guarantee has the identical swallowed-error shape (Task 3.3.3a).**
  `itemSessions, _ := h.storage.ListItemSessions(ctx, itemID)` discards the error; on failure
  `itemSessions` is `nil`, `services.HasActiveReviewSession(nil)` returns `false` (confirmed:
  `server/services/backlog_service_triage.go:1104`'s predicate just ranges over the slice), and the
  handler emits the *optimistic* "Reviewer notified" message — the literal thing FR5 says must never
  happen ("never claim the currently-running reviewer will see it") — precisely when the code has no
  actual evidence either way. No test covers this branch.
  **Recommendation**: on a `ListItemSessions` error, default to the conservative "next review pass"
  message rather than silently taking the "no active session" branch; add a test.

- [ ] **The CAS-trap mitigation (pitfalls.md §0, the plan's own "highest-severity finding") is enforced
  by getting 3 independent call sites right, not by one structural chokepoint.** The whitelist is a
  shared `map[session.BacklogStatus]bool` — good — but the *validate-before-pin* control flow itself
  ("reject first, only then use the validated status to build the precondition") is hand-written
  separately in Task 2.1.1b (`request_review`), Task 3.1.2b (`report_duplicate`'s whitelist check), and
  Task 3.3.1a (`report_duplicate`'s precondition build). Nothing stops a future edit at any of these 3
  sites from reading `item.Status` directly instead of a validated value — exactly the bug pitfalls.md
  §0 describes. architecture-review.md's nitpick here (an optional `isAllowedSelfResolveSource`
  predicate) doesn't close this: a predicate can be called and its result still ignored at the use site.
  **Recommendation**: one shared `func validateSelfResolveSource(status session.BacklogStatus)
  (session.BacklogStatus, error)` called exactly once per handler, with all downstream precondition/
  idempotency logic required to use its return value instead of `item.Status` directly — collapsing 3
  independently-reviewed call sites into 1.

- [ ] **No structural or test safeguard proves `report_duplicate`'s refusal-check ORDER
  (`SkipReviewGate` → role/link → whitelist → GitHub verify), only prose/task sequencing.** Pitfalls.md
  §5b explicitly flags that a reordering would burn a GitHub API call on a request that was always
  going to be refused. None of the Epic 4.2.2 refusal tests are specified to assert the
  GitHub-verification func is never invoked — they only check the returned error, which passes
  regardless of whether verification ran first.
  **Recommendation**: collect the pre-network refusal checks into one ordered gate function, and/or
  have the refusal tests set `h.verifyGitHubRef` to a stub that fails the test if called — turning
  "refused before any network call" into an enforced property, not a documented one.

- [ ] **Read-then-append race on `ItemSession.VerificationNotes` (Task 3.3.2a).** The fix for "don't
  silently discard prior notes on overwrite" reads `itemSession.VerificationNotes` early in the handler
  and writes `oldNotes + "\n\n---\n\n" + newEntry` late, after the GitHub network round-trip.
  `UpdateItemSessionVerificationNotes` is a plain overwrite with no CAS (confirmed by the plan itself,
  `session/storage_backlog.go:397`), so two genuinely-concurrent calls touching the same `ItemSession`'s
  notes (overlapping `report_duplicate`/`request_review` calls from the same work session, or a client
  retry racing the original in-flight call) can still lose an update — the append fix addresses
  sequential history, not concurrent writers. Low probability, but currently undocumented as an
  accepted tradeoff anywhere in the plan or ADRs.
  **Recommendation**: either state explicitly (Observability Plan or ADR-004) that this is an accepted
  low-probability tradeoff given the single-threaded-per-session tool-call model, or re-read
  `VerificationNotes` immediately before the final write to shrink the window.

- [ ] **No test for `duplicate_ref`/`reason` exceeding their length caps.** Task 3.1.1a states the caps
  (500 chars / 1000 chars) but this never rises to a Story-level acceptance criterion, and no task in
  Epic 4.2 exercises it — contrast with `request_review`'s existing
  `TestRequestReview_RejectsVerificationNotesOver4000Chars`, the same shape of test for a sibling tool.
  **Recommendation**: add `TestReportDuplicate_RejectsWhenDuplicateRefOrReasonTooLong` (table-driven
  over both fields) to Epic 4.2.

- [ ] **Cross-repo `duplicate_ref` is implicitly allowed but never decided or tested.** FR3's text
  doesn't constrain `duplicate_ref` to the item's own repo, and nothing downstream enforces it —
  `ParsedGitHubRef.Owner`/`.Repo` are parsed straight out of whatever URL is passed
  (`github/url_parser.go`), and `verifyGitHubRefExists` dispatches on those parsed values with no
  same-repo check against the item. No ADR discusses this, and no Epic 4 test exercises a
  `duplicate_ref` pointing at a different `owner/repo` than the item's own working repo — plausible in
  a multi-repo workspace (this very repo has `origin`/`personal` remotes).
  **Recommendation**: make an explicit call — confirm cross-repo is intentionally allowed and add a
  test proving it works, or add a same-repo restriction if that was the unstated intent.

- [ ] **ADR-003 and ADR-004 are still `Status: Proposed`, not `Accepted`**, unlike ADR-001/ADR-002/
  ADR-005 (all "Accepted," two explicitly "ratified by item owner"). The plan's own header treats all
  five ADRs' decisions as settled ("Status: Ready for implementation"), but two of five never went
  through the same explicit ratification step the plan uses as its bar for the other three — and
  ADR-003 documents a real downstream-consumer risk of its own ("any dashboard/query filtering
  `TriggeredBy == 'system'`... grep check before merging") that seems to warrant the same sign-off
  ADR-002/ADR-005 got.
  **Recommendation**: close this formal gap (ratify or explicitly defer) before coding starts.

- [ ] **Task 4.2.6a's "concurrency regression test" doesn't test what Story 4.2.6 promises.** Story
  4.2.6's acceptance criterion: "`report_duplicate` racing `report_pr_created` on the same item results
  in exactly one status-event row and a non-retryable message **for the loser**." Task 4.2.6a's actual
  construction, as written, never calls `reportPRCreated` as a racing party at all — it calls
  `reportDuplicate` once (which succeeds), then directly calls `storage.TransitionBacklogItemStatus` a
  second time with the *original* precondition to force a raw `ErrPreconditionFailed` at the repository
  layer, and asserts on `BacklogStatusEvent` row count. This never exercises `reportPRCreated`'s (or a
  second `reportDuplicate` call's) own handler-level error-message code, so it can't demonstrate "a
  non-retryable message for the loser" — no losing *handler call* happens in the test as described.
  **Recommendation**: rewrite this task before implementation — it needs two actual tool calls
  (`reportPRCreated` then `reportDuplicate`, or vice versa, on the same item) with an assertion on the
  second call's returned error text, not a direct repository-layer CAS probe standing in for it.

- [ ] **Task 4.1.5a's stated fallback ("directly unit-testing the branch logic... if not") has nothing
  to call.** Task 2.1.2a inlines the `errors.Is(transErr, session.ErrPreconditionFailed)` branch
  directly at the `TransitionBacklogItemStatus` call site inside `requestReview` — no task anywhere
  extracts this into a separately-callable function. The fallback path Task 4.1.5a offers ("unit-test
  the branch logic in isolation if triggering this from `requestReview` is awkward") is therefore not
  buildable as written.
  **Recommendation**: either commit to the "seed at in_progress, force a stale precondition" primary
  construction as the actual plan (it is buildable and is the one that should be used), or add a task
  extracting the classification logic into a testable helper (e.g. `classifyTransitionError(err)`) if
  isolated unit-testing is genuinely wanted as a fallback.

## Minors

- Story 3.1.2's source-status whitelist check is itself a 4th refusal condition not literally
  enumerated in FR6's three-item list, in mild tension with ADR-005's argument that "FR6 enumerates
  exactly three... no fourth condition appears" (used there to justify *not* adding an active-reviewer
  refusal). The whitelist check is almost certainly still correct and necessary — skipping it reopens
  the CAS-trap bug for `report_duplicate` — so this isn't asking to remove it, only to tighten ADR-005's
  wording so it doesn't read as "FR6's list is exhaustive" when the plan's own next story adds a 4th
  item to it for a different, valid reason (FR1/FR3's CAS-safety logic, not FR6).
- `allowedSelfResolveSourceStatuses` is a mutable package-level `var` (Go maps can't be `const`),
  shared verbatim by `request_review` and `report_duplicate`. Nothing in the type system prevents
  accidental mutation from a future test; low risk today (no `t.Parallel()` in `tools_backlog_test.go`
  currently), but worth a doc-comment note that it must never be written to outside its declaration.
- `GetPR`/sentinel-retrofit-on-`GetIssue` scope was spot-checked against FR3/FR4's literal text and
  found proportionate, not creep — it's a mechanical mirror of `GetIssue`'s existing shape, justified
  by a real, researched auth-mechanism-inconsistency risk (pitfalls.md §4) and ratified in ADR-002
  (with a documented, considered rejection of the narrower "reuse the `gh` CLI path" alternative). No
  behavior change for existing `GetIssue` callers (confirmed: none exist outside tests).
- Technology bets spot-check: no new `go.mod` dependency anywhere in this plan; every new function
  routes through existing `net/http`/`ghHTTPClient`/`getGHToken`/`github.ParseGitHubRef` primitives
  already used elsewhere in the `github` package. The plan's "100% existing primitives" claim holds up.
- `github/rate_limit.go`'s `DefaultRateLimiter` is effectively write-only dead infrastructure today:
  `ghHTTPClient` (`github/http_client.go:13`) is a bare `&http.Client{Timeout: 30*time.Second}` with no
  custom `Transport`, and `RateLimiter.Update` is never called from any production code path (confirmed
  by grep) despite `session/pr_status_poller.go`/`worktree_pr_poller.go` reading `IsLimited()` as if it
  were live. Pre-existing gap this plan doesn't introduce, but `GetPR`/`GetCommit` become two more
  callers that — like `GetIssue` today — will silently never feed it. Worth a one-line Observability
  Plan note, not a blocker.
- `Note` field population on `request_review`'s existing precondition-building code (Task 2.1.1b,
  previously always empty) is motivated by ux.md's opinion, not any literal FR — FR7 only mandates
  `VerificationNotes` persistence. Confirmed low-risk (no existing test asserts on `Note` content, so
  FR9 isn't jeopardized), but it's a behavior change to an already-shipped code path riding along on
  FR1's diff — worth a one-line callout for reviewer awareness.
- Malformed/truncated JSON bodies and unrecognized GitHub status codes fall into the generic retryable
  `ErrInternalError` bucket for `GetIssue`/`GetPR`/`GetCommit` alike — reasonable default (matches
  `report_pr_created`'s template) but untested in Story 4.2.3's four tasks.
