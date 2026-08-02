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

---

## Iteration 2 Re-Verification (appended)

**Date**: 2026-08-02
**Scope**: re-check of the 11 fixes iteration 1 required, against the current `plan.md`. Not a fresh
full review — see the coordinator's brief for the exact 11-item checklist. Source claims were verified
by reading `session/backlog_lifecycle.go` and the ADR files directly, not by trusting plan prose.

### 1. FR10 Observability Plan citation — FIXED-CORRECTLY

Plan's Observability Plan (lines 81-84) now cites `ReconcilePRPending` (`session/backlog_lifecycle.go:3850`)
instead of `pr_pending_no_pr`. Verified directly against source:
- `ReconcilePRPending` is gated `if item.PrNumber == 0 || item.PrURL == "" { continue }`
  ([`session/backlog_lifecycle.go:3857`](session/backlog_lifecycle.go#L3857)) — the complementary
  condition to `reconcilePRPendingWithoutPRItems`'s `if item.PrNumber != 0 { continue }`
  ([`:2553`](session/backlog_lifecycle.go#L2553)), confirming it does cover items with a real PR
  reference, which is FR10's actual scenario.
- The healthy-but-stale-open-PR branch calls `l.markPRReadyUnmerged(...)`
  ([`:4069`](session/backlog_lifecycle.go#L4069)), which itself calls `MarkStuck(..., StuckReasonPRReadyUnmerged, ...)`
  ([`:4208`](session/backlog_lifecycle.go#L4208)) — confirmed.
- The CI-failing/blocked/conflicting branch reaches `MarkStuck(..., StuckReasonPRNeedsFix, ...)`
  ([`:3805`](session/backlog_lifecycle.go#L3805)) — confirmed.
- Both `StuckReasonPRReadyUnmerged` and `StuckReasonPRNeedsFix` strings appear in
  `web-app/src/components/backlog-stuck/StuckItemsSection.tsx` and `stuckReason.ts` — confirmed present
  in the UI surface the plan cites.

The blocker's root claim is now accurate. No further action needed on this item.

### 2. Task 2.2.1b — fail closed on `ListItemSessions` error — PARTIALLY-FIXED

The code fix is correct: the guard now returns `ErrInternalError` ("could not verify active-reviewer
state for this item — retry: %v") on a storage error instead of falling through. Verified in plan.md
line 252.

New gap: the task prose says "Task 4.1.4a-equivalent test coverage: add
`TestRequestReview_FailsClosed_WhenListItemSessionsErrors`" but no such task actually exists under
Epic 4.1 (Stories 4.1.1–4.1.5, Tasks 4.1.1a–4.1.5a enumerated at lines 390-438) — the test name is
mentioned only in a parenthetical inside Task 2.2.1b's own description, not tracked as a Phase 4
deliverable the way every other new-behavior test is (contrast Task 4.1.4a/4.1.4b, which do exist for
the sibling "active reviewer" guard behavior in the same story). Iteration 1's recommendation explicitly
asked for "a test that injects a storage error and asserts refusal" as a checkable deliverable, not just
a prose mention — a reviewer scanning Phase 4's task list for full FR2 coverage would miss this one.

### 3. Task 3.3.3a — default to conservative message on error — PARTIALLY-FIXED

The code fix is correct: `activeReview := lsErr != nil || services.HasActiveReviewSession(itemSessions)`
(line 362) correctly treats a storage error as "might be active," defaulting to the conservative "next
review pass" wording rather than the optimistic "Reviewer notified" text.

New gap, worse than item 2's: Task 3.3.3a's prose doesn't even mention a test name for this branch, and
Epic 4.2.5 has exactly one task (4.2.5a, `TestReportDuplicate_MessageSaysNextReviewPass_WhenReviewSessionActive`),
which covers the "active session present" case, not the "`ListItemSessions` errored" case. Iteration 1
explicitly asked for a test here too ("add a test"); it is missing entirely, not just untracked.

### 4. Shared `validateSelfResolveSource` chokepoint + call-site/ordering check — FIXED-CORRECTLY

`validateSelfResolveSource(item, toolName) (session.BacklogStatus, error)` is defined once (Task 2.1.1a,
line 214) and all 3 downstream sites use its return value, not `item.Status`, verified by reading each:
- Task 2.1.1b (line 218): `precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(validStatus), ...}` — uses the chokepoint's return.
- Task 3.1.2b (line 291): idempotency check runs first using **raw** `item.Status == string(session.BacklogStatusReview)` — this is intentional and correct, not a violation, because the idempotency no-op path is for an item that has *already left* the whitelist (status `review`), which the chokepoint would otherwise reject outright. Only in the "otherwise" branch (no exact-retry match) does it call `validateSelfResolveSource(item, "report_duplicate")` and carry `validStatus` forward.
- Task 3.3.1a (line 337): `ExpectedStatus: string(validStatus)` — explicitly "the value Task 3.1.2b's `validateSelfResolveSource` call returned (never `item.Status` directly...)".

Ordering is correct and does not contradict the chokepoint's existence — idempotency check (raw status)
necessarily precedes the whitelist chokepoint call, exactly as iteration 1's checklist required.

### 5. Epic 3.1 Goal — refusal tests must stub `verifyGitHubRef` to fail if called — FIXED-BUT-NEW-ISSUE

The ordering-safeguard text was added (line 260) and does require Epic 4.2.2's refusal tests to set
`verifyGitHubRef` to a `t.Fatal`-on-call stub. However, the range cited — **"every Epic 4.2.2 refusal
test (Tasks 4.2.2a-f)"** — is now wrong. Task 4.2.2f (`TestReportDuplicate_AllowsCrossRepoDuplicateRef`,
added per fix #8, line 479) is explicitly **not** a refusal test — its own acceptance text requires
`verifyGitHubRefExists`/`verifyGitHubRef` to **be called** (asserting it's invoked with the cross-repo
`owner`/`repo`) and to return success, with the item transitioning to `review`. Constructing 4.2.2f per
Epic 3.1's Goal text (`verifyGitHubRef` that calls `t.Fatal` if invoked) would make 4.2.2f fail by
construction — a direct contradiction between two sections both fixed in this iteration. The range
should read "Tasks 4.2.2a-e" (4.2.2e, the length-cap refusal test, correctly belongs in the "no GitHub
call" set; 4.2.2f does not). This is a new internal contradiction introduced by combining fix #5 and
fix #8 without reconciling the cross-reference.

### 6. Task 3.3.2a — accepted-tradeoff note on the `VerificationNotes` race — FIXED-CORRECTLY

Line 351 now ends with an explicit "**Accepted tradeoff (adversarial review)**" paragraph stating the
read-early/write-late race exists, is not CAS-protected, is judged low-probability given the
single-work-session-per-item-at-a-time model, and is deliberately not mitigated further (no re-read
immediately before final write) for v1. This directly satisfies iteration 1's recommendation ("state
explicitly... that this is an accepted low-probability tradeoff").

### 7. Task 4.2.2e — length-cap rejection test — FIXED-CORRECTLY

Exists at line 475-477 (`TestReportDuplicate_RejectsWhenDuplicateRefOrReasonTooLong`), table-driven over
both the 500-char `duplicate_ref` cap and 1000-char `reason` cap, explicitly asserts zero mutation and no
GitHub call.

### 8. Task 4.2.2f — cross-repo `duplicate_ref` allowed, explicit decision — FIXED-CORRECTLY

Exists at line 479-481 (`TestReportDuplicate_AllowsCrossRepoDuplicateRef`), with an explicit "Explicit
decision" paragraph reasoning that FR3's text does not restrict `duplicate_ref` to the item's own repo
and that cross-repo duplicates are a legitimate real-world case (cites this very repo's `origin`/
`personal` fork pair). This resolves iteration 1's concern about an undecided, untested behavior — see
item 5 above, however, for the new contradiction this addition created elsewhere in the plan.

### 9. ADR-003 / ADR-004 status — FIXED-CORRECTLY

Read directly:
- `decisions/ADR-003-triggeredby-agent-scope.md:4`: "Accepted (2026-08-02, ratified by item owner). Per
  the review-flagged 'grep check before merging'... the check was run during architecture review and
  confirmed low-risk... Phase 5's implementer should still re-run it..." — Accepted, with the residual
  action item iteration 1 asked about explicitly carried forward as a Phase 5 note rather than dropped.
- `decisions/ADR-004-report-duplicate-idempotency.md:4`: "Accepted (2026-08-02, ratified by item owner)".

Both now match ADR-001/002/005's "Accepted" status.

### 10. Task 4.2.6a — real two-tool-call test — FIXED-CORRECTLY

Task 4.2.6a (line 513-515) and Story 4.2.6's acceptance text (line 510) were both rewritten together.
The test now calls `reportPRCreated` (real handler call, `in_progress → pr_pending`, one status-event
row), then `reportDuplicate` on the same now-`pr_pending` item (real handler call, succeeds per the
whitelist, `pr_pending → review`, second status-event row) — a genuine two-handler-call sequence, not a
direct repository-layer CAS probe. The "loser" scenario is now a **third** `reportDuplicate` call on the
now-`review` item, asserted to hit the whitelist-rejection branch with no third status-event row. The
story's acceptance criteria text was also corrected to stop claiming "exactly one status-event row,"
which the original (pre-fix) framing had gotten wrong given the two calls target different, sequentially
legitimate transitions.

### 11. Task 4.1.5a — commit to one buildable construction — FIXED-CORRECTLY (writing quality note)

Task 4.1.5a (line 437) removed the unbuildable "unit-test in isolation" fallback (correctly noted as
impossible since Task 2.1.2a inlines the `errors.Is` branch with no extracted helper). It now commits to
a single buildable construction: two goroutines calling `requestReview` concurrently via
`sync.WaitGroup` against an item seeded at `in_progress`, relying on the DB-level atomic
`UPDATE...WHERE` (pitfalls.md §1) to guarantee exactly one winner/one loser, asserting the loser's error
text contains "state changed." This is genuinely buildable and matches the CAS-race scenario Story 4.1.5
describes.

Minor writing-quality note (not a correctness issue): the task's prose still walks through a rejected
"out-of-band transition" construction first (explaining why it only exercises the whitelist path, not
the CAS-race path) before landing on the real goroutine-race construction. This reads as leftover
reasoning-trace rather than a clean task description — worth tightening before Phase 5, but it does not
leave two competing buildable options the way the original "unit-test in isolation" fallback did, so it
does not reopen iteration 1's finding.

### Other newly-noticed issues

- **Epic 3.1 Goal's "Tasks 4.2.2a-f" range contradicts Task 4.2.2f's own success-path semantics** — see
  item 5 above. This is the one new problem introduced by iteration 2's edits considered independently
  of the 11 checklist items; it is a same-file internal contradiction, not a blocker (Phase 5's
  implementer would almost certainly notice 4.2.2f can't compile/pass with a `t.Fatal`-on-call stub and
  correct it in the moment), but it should be fixed before `sdd:4-validate` rather than left for
  implementation-time discovery.
- No other stale cross-references or renamed/removed symbols were found in a scan of the sections
  touched by these 11 fixes (Domain Glossary, Pattern Decisions, Epics 1.1–4.3, Observability Plan, ADR
  files) — the `validateSelfResolveSource` name is used consistently everywhere it's referenced (Domain
  Glossary line 40, Pattern Decisions line 65, Tasks 2.1.1a/2.1.1b/3.1.2b/3.3.1a), and `ReconcilePRPending`/
  `StuckReasonPRReadyUnmerged`/`StuckReasonPRNeedsFix` are used consistently between the Observability
  Plan and the underlying source.

### Overall Verdict: CONCERNS

No blocker-severity issue remains — the FR10 blocker (item 1) is genuinely fixed and verified against
source. 8 of 11 fixes are fully correct (items 1, 4, 6, 7, 8, 9, 10, 11); 2 are code-correct but missing
their promised test coverage as a trackable Phase 4 deliverable (items 2, 3); 1 fix introduced a new,
non-blocking internal contradiction (item 5's task range vs. item 8's new test). None of these three
gaps rises to blocker severity — the underlying code behavior for items 2/3 is correct, and item 5's
contradiction is a one-line range fix, not a design problem — but none should be waved through to
`sdd:4-validate` silently either.

**Recommended before `sdd:4-validate`**:
1. Fix Epic 3.1's Goal text: "Tasks 4.2.2a-e" (not "a-f") — 4.2.2f is a success-path test.
2. Add `TestRequestReview_FailsClosed_WhenListItemSessionsErrors` as an actual numbered task in Epic 4.1 (e.g. Task 4.1.4c), not just prose inside Task 2.2.1b.
3. Add a task for the FR5 `ListItemSessions`-error message-fallback path (e.g. Task 4.2.5b,
   `TestReportDuplicate_MessageSaysNextReviewPass_WhenListItemSessionsErrors`) under Story 4.2.5.

**Count: 8/11 fixed correctly, 2 partially fixed (code correct, test task untracked/missing), 1 fixed-but-introduced-a-new-contradiction.**
