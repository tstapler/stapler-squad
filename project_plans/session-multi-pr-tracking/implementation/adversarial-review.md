# Adversarial Review (round 5): session-multi-pr-tracking

**Date**: 2026-11-03
**Verdict**: CONCERNS (0 blockers, 6 concerns, 5 minors)

Scope: confirm the eight round-4 concerns are closed by iteration 4, then try to break the iteration 4 changes. VERIFIED = I opened the code or ran the command in this review. INFERRED = reasoned from plan text. `plan.md` was not edited. No local DB sample was available (`~/.stapler-squad/sessions.db` has 0 rows with a PR number), so no claim below rests on real data.

## Status of the round-4 concerns

| Round-4 | Status | Evidence |
|---|---|---|
| C1 `stopLiveWorkAndReviewSessions` unlisted | CLOSED | Row 14 and T5.4.5 present. Headline narrowed to "AUTOMATED path". |
| C2 sweeper ignores item-direct links | CLOSED in text, two nits (M1) | T4.3.1 step 4 runs after steps 1-3, fails closed on lookup error, test first. VERIFIED the old `baseSafeToDelete` read only the number and flag (`session_retention_sweeper.go:124-139`). The existing sibling convergence (same file, `sessionSafeToDelete`) still covers a shared worktree path with a live current-round session. |
| C3 ACs cover only session-only links | CLOSED | Story 5.1 AC "item-direct first PR then rework" states the outcome. |
| C4 current round can hide a live PR | CLOSED for the stated window, new gap (C6 below) | "Latest work session that has a link" fixes attach-before-discovery and rework-before-discovery. |
| C5 monotonic writer | CLOSED for ordering, new gaps (C4, C5, M2 below) | T1.3.2 conditional UPDATE: CAS on `observed_at`, sticky terminal, two exceptions. |
| C6 backfill failure | PARTIAL (C1 below) | Abort-on-error is stated and VERIFIED as the existing migration behavior (`ent_repository.go:263-268`). The same plan contradicts itself and leaves deterministic bad rows unclassified. |
| C7 stale terminal flag | PARTIAL (C2 below) | Seed rule and exhaustion rule are sound; the clear of the live flag does not reach the DB for sessions without links. |
| C8 unverified-terminal cap | CLOSED with caveats (C4, C5) | Counted classes, span via `first_fail_at`, boot and nightly reset, gauge. |

## Blockers

None. Every data-loss path I found withholds a delete in the default case and needs a second condition (a stale flag, an unfetchable PR, or a reopen with no webhook) to lose a checkout. Commits survive on the pushed branch, and `Destroy` still refuses a dirty worktree (`backlog_service.go:1357-1380` removes via `g.Cleanup()`; dirty refusal at `instance.go:2270-2287`, as cited in row 14).

## Concerns

- [ ] **C1. Startup abort can become a permanent boot loop, and the plan contradicts itself about abort.**
  - VERIFIED: any migration error aborts `NewEntRepository` (`ent_repository.go:263-268`), and the unit uses `Restart=on-failure` (`scripts/install-service.sh:345`). A deterministic bad row therefore crash-loops the service, and backlog automation goes down with it. No operator escape exists short of editing the DB.
  - The Migration Plan lists the only non-error skips as "unrecoverable host, empty uuid, tombstoned". Unclassified deterministic failures are everything else a row can do to the new schema: the ent validators (lower-case, exactly one owner), the one-primary index, the 50-link cap, an invalid `PRRef`.
  - **Item PRRef derivation is never specified.** VERIFIED: `backlog_item` has `pr_url`, `pr_number` and `repo_path` but no host, owner or repo column (`session/ent/schema/backlog_item.go:37, 98, 100`). The item candidate rule needs a `PRRef` for every `pr_number > 0` item. The plan does not say whether it parses `pr_url`, shells out to git per `repo_path` at boot, or skips. An item with a number and an empty or unparseable URL is neither a classified skip nor a defined error.
  - T1.5.1 still says "best-effort per row ... returns nil if the table is missing" (plan line 397), while the Migration Plan says the pass returns every error. An implementer following the task text reproduces the swallowing template that C6 was meant to remove.
  - The rollback AC imports `#12` into a session that already has a primary `#11`. The role of that imported link is unstated; if it is `primary`, the one-primary index rejects it, and under abort-on-error that is a boot loop on exactly the supported rollback path.
  - Recommendation: split errors into transient (DB busy, I/O: abort) and per-row deterministic (count, log by title, treat as skipped, rely on `LegacyPRUnrepresented` and its item analog). State the item `PRRef` source and the imported role (`followup`, never `primary`, when a primary exists). Fix the T1.5.1 text. Add a test with a malformed row that boots.

- [ ] **C2. The C7 fix does not persist for sessions that have no link, so the stale `true` survives in the DB.**
  - VERIFIED: the full-save create and update paths can only set the flag (`ent_repository.go:411-413`, `:758-760`, the plan's own root cause in T2.3.3). T2.3.3 makes the save clearable only when `InstanceData.HasPRLinks` is true.
  - T2.2.1 step 1 clears the in-memory `GitHubPRStatusTerminal` when `scanAndLinkPRURL` changes the number. VERIFIED the current block writes in memory only (`session_driver.go:961-967`). For a session that gets no link (fork session, empty uuid, backfill-skipped class, where `AttachPR` attaches nothing and "keeps the legacy scalar path"), the clear never reaches the row. After a restart the session reloads number B with flag `true`, the legacy poller skips it, and `LegacyOnlyPRSafe` (sweeper step 2, gate clause iii) treats it as safe while B is open. That is the C7 scenario.
  - Also a wave tension: T2.2.1 (clear) lands before T2.3.3 (clearable save) in the numbering.
  - Recommendation: have the legacy clear call the uuid-keyed clearable writer directly (or make step 2 and clause iii distrust a flag that was true when the number changed, which needs a stored marker), and add a test through the real save and reload path for a no-link session.

- [ ] **C3. A `closed` PR that is reopened without a webhook is treated as terminal forever, and cleanup then runs under an open PR.**
  - T1.3.2 rule (b) makes merged and closed sticky; the only reopen path is the `pull_request.reopened` webhook, which is an optional, separately exposed endpoint (`docs/how-to/expose-github-webhook-endpoint.md` in the repo index). Terminal rows are not polled.
  - Closing and reopening a PR is routine (a reviewer or author closes, then reopens). A closed-unmerged PR is a terminal verdict (`AllClosedUnmerged`), which leads to archive and worktree cleanup, so the checkout can be removed while the PR is open again. This is a new fail-open edge in the mechanism whose purpose is "no worktree under a non-terminal PR".
  - The plan lists it under "Needs the owner" as parity with legacy, but legacy had no cleanup gate to advertise. Merged cannot reopen, so only `closed` is exposed.
  - Recommendation: poll `closed` (unmerged) rows at a low cadence for a bounded window (for example 24 hours with an ETag 304 each interval), or have the gate re-fetch a closed row once immediately before it removes. Keep merged sticky. Add the closed-to-merged transition to the T1.3.2 tests (allowed by the text but not tested).

- [ ] **C4. The resets of `poll_fail_count` and `first_fail_at`, and the exhaustion span, are under-specified in ways that interact with I1 and the CAS.**
  - T3.2.5 (refresher start) and T1.3.5 (nightly GC) write those columns with a bare UPDATE. Invariant I1 says `UpdatePullRequestState` is the only code that changes a state column after insert, and the `pr-state-write-guard` allow-list names only `ent_repository_session_pr.go` and `session_pr_gc.go`. The T3.2.5 reset in `pr_status_poller.go` would trip the guard or force an exemption; the plan states neither. The reset also carries no `observed_at`, so it is outside the CAS.
  - `VerifyExhausted()` is "5 counted failures, `first_fail_at` to now >= 30 minutes". Measuring to now means a row at count 5 with a short streak (backoff makes the next poll 16 ticks away) becomes "terminal" at minute 30 by the passage of time alone, with no failure observed in the preceding window. Measure `first_fail_at` to the last counted failure (`last_polled_at` of a failed poll).
  - The boot reset happens at refresher start, so a retention or cleanup pass that runs before it sees the previous run's exhausted rows as terminal. Run the reset inside the migration or before the sweeper and cleanup tickers start.
  - Whether a non-counted failure (5xx, timeout) breaks or preserves the streak is unstated.
  - Recommendation: route the reset through the repository file, state it as a documented I1 exception with a test, define the span as first-to-last counted failure, and order the reset before any consumer.

- [ ] **C5. The exhaustion classifier is the only thing standing between an unfetchable row and a delete, and it is not defined.**
  - Exhaustion turns an unverified-terminal row into a terminal one, which opens paths 1-7. The counted set is "404/401/403 that are not rate-limit responses". How "rate-limit" is detected is unspecified. GitHub secondary rate limits return 403 with `Retry-After` or a message body rather than `X-RateLimit-Remaining: 0`; the cold drain (20 requests per tick, many hosts) is the kind of burst that triggers them. A private repo the token cannot see returns 404; an expired token returns 401 for every row at once.
  - The stale-flag case C7 describes (flag `true`, PR B actually open) is the one row type for which exhaustion is wrong, and it is also the row type most likely to be in a repo or org the token cannot reach.
  - Recommendation: define the classifier (header and body based, secondary limit explicitly excluded), treat 401 as a token problem that pauses verification globally instead of counting per row, and test each.

- [ ] **C6. "Latest work session that has a link" lets an earlier round's PR re-enter the set when the latest session's links go away.**
  - The rule is evaluated at read time. If the user detaches the only link of the latest-with-links session `s2` (a wrongly discovered PR), or `s2`'s links are collected, the earlier session `s1` becomes "latest with links" and its PRs rejoin the set. A merged round-1 PR plus nothing open yields verdict `Done`, so a `pr_pending` item completes on a PR from a rejected earlier round. The new ACs test only the "new session has no link yet" direction (Story 5.1, plan line 757).
  - Recommendation: add the detach and delete variants to the ACs and decide the outcome (either accept and document, or require `s2` to be the latest work session by `created_at` for the set to move backwards, treating a link-less latest session as contributing nothing only until it has ever linked, which needs the item tombstone or a stored "has linked" bit).

## Minors

- M1. `ListItemSessionsBySessionUUIDs` returns `map[string]ItemSessionSummary` (VERIFIED `session/storage_backlog.go:450`), one item per session uuid. Sweeper step 4 therefore checks one item for a session that belongs to several. Use `ListItemSessionsForItems` shape or state the single-item assumption.
- M2. CAS compares `observed_at` with `last_polled_at` and drops the loser at debug level with no metric. If the wall clock steps back, or a row ever carries a future `last_polled_at`, every later observation is silently dropped and the row freezes (a merge is never recorded). Count the drops (`ssq_pr_state_observation_dropped_total{reason}`), warn above a threshold, and clamp a future `last_polled_at` to now.
- M3. Orphan GC wording conflicts: the Migration Plan says the nightly GC deletes states "with no links"; T1.3.5 says they are kept for the retention window. Without the window, a concurrent attach (`EnsureState` finds the row, GC deletes it, the link insert follows) can lose the state row. Make the delete one conditional `NOT EXISTS` statement and keep the window.
- M4. An exhausted unverified-terminal row that is a session-only link in a `pr_pending` item's current round reads as `AllClosedUnmerged` (terminal, not merged). Only seeds where the item is not itself a candidate are exposed (a terminal-flag session, item without `pr_number`), but the verdict then treats an unfetchable, possibly merged PR as abandoned. Make an exhausted row contribute `unknown` to the verdict, terminal only for the gate and sweeper.
- M5. Frequent restarts (every `make install-service`) reset verification each time, so unfetchable historical rows may never exhaust and keep their worktrees. This is the safe direction; note it in the runbook so a retained worktree is not misread as a gate bug.

## What I tried and could not break

- Single-writer plus CAS under overlap: a slow older fetch cannot land `open` over `merged` (rule a, then b). The reopen proof needs `source=webhook_reopened`, a strictly later `observed_at` and a successful `open`, so a stale tick response cannot reopen. A reopen fetch that lands after a later merge is rejected by rule (a).
- Item-direct links never superseded: the outcome is a held item, not a loss; the stuck report names the PR and `DetachItemPullRequest` is the exit (Story 5.1 AC, T5.2.2).
- Unverified terminal treated as non-terminal until exhausted: for a `pr_pending` item the seed merge rule (non-terminal wins) means the verdict never sees it; for a done or archived item it only delays cleanup by at most the cold drain. Gate clause (i) is restricted to `work`-role sessions, which matches what `cleanupItemWorktreesExcept` removes (VERIFIED `backlog_service.go:1357-1366`, the `Role != work` skip).
- Tombstone and backfill idempotence without a marker; the shared-worktree sibling check still protects a superseded round's checkout from the sweeper.
