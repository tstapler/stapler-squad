# Requirements: session-multi-pr-tracking

**Date**: 2026-11-03
**Type**: feature addition (data-model change across session, backlog, server and web-app)
**Complexity**: 3 — system design (multiple epics; touches session persistence, PR polling, backlog completion gate, RPC/proto, UI)

## Problem Statement
A stapler-squad session can work on several PRs over time (a follow-up PR, a stacked PR, a rework PR, a PR opened after the first merged). The session model stores exactly one PR: `GitHubPRNumber` / `GitHubPRURL` on `Instance` (`session/instance.go:344-347`, mirrored in `InstanceSnapshot`, `session/instance_snapshot.go:42-43`). Three problems follow:

1. **Lost PR history** — earlier PRs a session opened or merged are no longer visible on the session card, detail view or search once the single field is overwritten.
2. **Wrong PR tracked** — status polling, comments, diff, merge, close and "Ask to fix" (`session/pr_tracking.go`, `session/pr_status_poller.go`) all act on the one stored PR, even after the session has moved on to a different PR.
3. **Backlog item to many PRs** — a backlog item or session that needs several PRs (stacked or follow-up) runs into code that assumes one PR (`session/backlog_lifecycle_pr.go`, completion gate, `pr_pending` state).

4. **Artifacts tab "sometimes works"** (epics E7-E10) — the Artifacts tab (`session/artifacts/`, `SessionArtifactsBlob`) shows "Extraction pending" for sessions whose scan found nothing (the empty scan is never stored, `session/artifacts/scan.go:63-67`), drops the newest entries at the caps, and there is no way to find a session from a PR URL, branch, commit or plan doc across sessions. Falsifiable claim to test in T7.1.1: the sessions with a scanned-but-empty tab stuck in "Extraction pending" are the main cause of the unreliability symptom.
5. **Stale GitHub issue after finished work** (Story 5.5) — an issue-backed backlog item that is already finished can leave its GitHub issue open (for example when the item was imported through a path that skips forward-sync, or the close failed), and nothing tells the user.

## Baseline
Today the user overwrites or loses the earlier PR link, finds it again via GitHub, or opens another session for the next PR. Existing partial workarounds: `session/github_pr_url_backfill.go` (recovers a missing URL), `project_plans/report-pr-created-branch-mismatch`, `project_plans/backlog-session-item-linking`. Phase 2 must confirm exactly where and when the field is overwritten, and what the backlog stores per item.

**Artifacts baseline (to be captured by T7.1.1 before any E7 code is written, and recorded in the T7.1.1 PR description and `project_plans/session-multi-pr-tracking/research/artifacts.md`)**: (a) number of sessions with empty `session_artifacts` but a non-empty `history_file_path`, as a share of all sessions with a history file (the "stuck at Extraction pending" share); (b) `[ArtifactExtractor]` warning counts by message (`seek failed`, `persist failed`, `scanner error`, `fingerprint failed`); (c) sessions whose newest PR or commit is missing from a blob because of a cap. Today finding the session behind a PR URL, branch or commit means opening sessions one by one or grepping transcripts by hand; there is no cross-session artifact index.

**Issue-close baseline**: the user must notice that a GitHub issue is still open for a finished item and close it by hand on GitHub. Forward-sync (`session/backlog_github_forward_sync.go`) closes the issue only on the transition to `done` (and, since commits 36ef043bc and 3ae9e6e78 of 2026-11-03, for MCP-imported sourceless items), so a missed, failed or skipped close (no token at that moment, GHE, an item finished before that code existed) is never retried. User value of the offer: in the normal case the done transition already closes the issue and the offer never appears; it matters only for the leftover cases, where re-importing the issue is the moment the user is looking at it, so one confirmed click replaces a trip to GitHub. Success is measured by the usage signal below, not by volume.

**Baseline (PR side and issue close; captured read-only by T1.5.4 on a `sqlite3 .backup` COPY of the production database, never the live one; to be filled before the first `reader:ui` flip)**: (a) sessions created in the 90 days before the capture whose `session_artifacts` PR URLs hold more than one distinct PR URL: __ of __ sessions (the "sessions that touched several PRs" share, the size of the multi-PR problem today); (b) backlog items with status `done` and a GitHub issue `external_url`: __, and of the 50 most recent, how many issues are still open on GitHub (read-only `gh issue view --json state`): __ (the size of the issue-close problem today). These numbers are the denominators for the keep or cut rule below.

## Users / Consumers
- The operator (Tyler) using the web UI and mobile app.
- Autonomous drivers and backlog automation (`session/autonomous_driver.go`, `session/session_driver.go`, `session/backlog_lifecycle*.go`).
- Agents calling MCP tools (`report_pr_created`-style tools) and the PR status poller.

## Success Metrics
- A session that has opened N PRs shows all N, each with its own state (open, merged, closed), in the session detail view and in session-list data; today only 1 is visible.
- PR actions (status refresh, comments, merge, close, ask-to-fix) target an explicitly identified PR; the default is the session's current active PR, never a stale one. Verified by tests where PR A is merged and PR B is open and actions hit B.
- A backlog item with 2+ PRs reaches done when at least one of its tracked PRs is merged and none are open (closed-unmerged allowed if another merged; owner decision 2026-11-03, see Decisions), and is not stuck in `pr_pending` because of an old PR.
- Existing single-PR sessions load unchanged, with no data loss, via a migration of the legacy fields.
- Re-importing a GitHub issue whose backlog item is already finished (status `done`, and for items with PRs at least one merged and none open; archived and duplicate items never count) while the issue is still open on GitHub says so in the response, and the user can close the issue with one click on a confirm prompt; the issue is never closed without that confirmation (owner request 2026-11-03).
- Artifacts (E7-E10) success, each measured by a named test or query: (a) zero sessions with a completed scan that found nothing are left in "Extraction pending" (state `none_found`, Story 7.2 ACs and the T7.1.1 baseline query re-run after the fix returns 0); (b) searching a known PR URL, branch, commit SHA prefix and plan-doc path string returns the owning session in each of N=4 Go tests (T9.1.1/T9.2.1, one per string kind), and `FindSessionsByPullRequest` is case-insensitive on owner/repo; (c) the `/artifacts` browse view lists every scanned session's records and reports the count of unscanned sessions (`unscanned_session_count` equals the number of sessions with no stored blob, tested in T10.1.1); (d) the read paths stay bounded (`ssq_artifact_search_seconds` p95 below 100 ms on the test corpus is a signal, not a gate).
- Adoption signal (not a gate; read at each soak review): the new capabilities are actually used, tracked by the counters in Observability Requirements (active-PR pin uses, artifact search queries by kind and hit or miss, issue-close offers offered, closed and declined). Keep or cut rule at the soak review (owner-confirmable in plan.md Owner confirmations pending #12), read over 30 days of use after `reader:ui` is on and compared with the baseline above. The thresholds are DEFAULTS until the T1.5.4 / T7.1.1 baseline is recorded; once it is, the owner sets each as a fraction of baseline usage (for example pin plus undo at a quarter of the baseline's multi-PR sessions per 30 days, search and browse at a fraction of the baseline's cross-session lookups); the defaults are pin plus undo below 5 uses, artifact search below 10 queries or under half hits, artifact browse below 10 views, or zero issue-close offers. Each shortfall makes the owner decide keep, demote (move behind an overflow or drop the nav entry) or cut that capability; nothing is removed automatically. Small multi-PR share rule for the `reader:ui` keep decision: keep `reader:ui` if the baseline finds ANY session with more than one PR in the 90-day window; if it finds none, the multi-PR UI has no demonstrated user and the keep, demote or cut call is the owner's (the flag-off code path is unaffected either way).

## Size
Large (3–15M) — INFERRED, see ESTIMATION.md. Full scope is the default; this is informational, not a gate.

**Wall-clock blockers**: owner decisions on the data-model choice (new table or ent entity vs JSON on the instance) and on the backlog completion rule for multi-PR items; a soak window if the change ships behind a rollout flag; CI runs.

## Constraints
- Persistence is ent/SQLite (`session/ent`); schema change must not commit generated ent output (see CLAUDE.md "Modifying the ent ORM Schema").
- Instance fields must be read via `Snapshot()` (`.claude/rules/instance-lock-free-reads.md`).
- Any new RPC needs proto change + `make proto-gen` + registry update; new pages need a `route-meta.ts` row.
- GitHub requests go through `NewConditionalRequest` (`.claude/rules/norawghrequest.md`); polling must not raise rate-limit use per PR beyond today's ETag behavior.
- Automated lifecycle code must not revive archived sessions (`.claude/rules/noarchivedrevival.md`).
- Rollout flags are live-settable via the feature-flag RPC, never env vars.
- Mobile and desktop UX both required.
- The legacy single-PR fields stay readable during migration (older clients, stored JSON).

## Non-functional Requirements
- **Performance SLO**: session list RPC latency does not regress measurably; PR list is bounded per session and loaded without an N+1 query.
- **Scalability**: tens of PRs per session at most; polling cost grows with open PRs only (verified merged and closed PRs are not re-polled; backfill-seeded unverified-terminal rows are polled until verified, see ADR-002).
- **Security classification**: internal; no new credentials.
- **Data residency**: no special requirements.

## Scope
### In Scope
- Audit of every read and write of the single-PR fields across session, server, backlog, MCP and web-app.
- A session-to-PR relation (multiple PRs per session, with per-PR state, role and timestamps) plus migration from the legacy fields.
- Defining "current/active PR" and making PR actions and polling PR-aware.
- Backlog item multi-PR handling: completion gate, `pr_pending`, session-item linking.
- Proto/RPC changes, web-app display of PR history (session card and detail), mobile layout.
- Relax the create-PR early return (`CreatePullRequest` and the review-queue auto-create) so a session whose PRs are all terminal can open another: a minimal, additive change to `session-pr-creation`, needed for the "PR B after merged PR A" case of metric 2 (plan T4.2.4).
- Tighten the terminal-scan PR detection to the session's own repo and host and to push / `gh pr create` output lines (plan T2.2.1): a known behavior change that stops unrelated PR URLs attaching.
- Issue close on re-import (owner request 2026-11-03): when `ImportGitHubIssue` finds an existing item that is finished while its GitHub issue is open, offer a one-click close that reuses the forward-sync close action and comment. github.com issues with a token only (GHE is reported, not closed); behind the `session_multi_pr` flag family (scope `reader:backlog`); never a silent close. Plan Story 5.5.
- Mobile end-to-end coverage: the 390px assertions of the new UI run under the real `mobile-chromium` (gating) and `mobile-webkit` (informational) Playwright projects, not in a resized desktop window (owner request 2026-11-03; plan T6.3.3).
- Analyze and deepen the session artifacts integration (`session/artifacts/`, `SessionArtifactsBlob`, `PrUrls` in `server/adapters/instance_adapter.go`): how it feeds PR history, what else it should capture per session, and how it surfaces in the UI. Added at the owner's request on 2026-11-03. Confirmed by the owner: this means the existing Artifacts tab in the web UI (not the Claude Artifact tool). It "sometimes works" (reliability to be diagnosed), should be richer, and should support searching across all known artifacts to find sessions.
- Tests (unit, integration, e2e per repo conventions), registry updates, docs.

### Out of Scope
- Changing how PRs are created (`session-pr-creation`) beyond recording the new PR in the relation and the single early-return relaxation listed in scope.
- Cross-repo PR graphs and stacked-PR tooling (`git-stacked-prs` skill).
- GitHub Enterprise-specific behavior changes beyond keeping the existing host-aware paths working.
- Jules multi-output handling beyond recording its PRs through the same relation. Resolved in planning (plan.md "Open Questions from requirements.md: resolutions"): every PR in a Jules session's `Outputs[]` is attached as an item-direct link (`attach_source=jules`, consulting the item tombstone first; `jules_session_poller.go:505-516`, VERIFIED); Jules sessions are not session-keyed, and nothing else changes.
- Artifacts: semantic or embedding search (BM25 plus exact lookup only); writing artifact-derived PRs into `SessionPR` (kept separate, ADR-004); editing, deleting or annotating artifact records by hand; a persistent search index across restarts (memory-only, rebuilt at boot); scanning non-Claude program transcripts.
- Issue close: closing a GitHub Enterprise issue (reported, never closed); an agent-callable or MCP close path (an agent-callable close would break "never a silent close"; the MCP `import_github_issue` is unchanged); bulk close; reopening an issue; closing issues of unfinished, archived or duplicate items; changing the forward-sync done-transition close.

## Rabbit Holes
- Defining "current PR" when two PRs are open at once (stacked) and picking the right default for actions.
- Backlog completion semantics for multi-PR items; the existing completion gate is AC-based (`docs/reference/backlog-completion-gate-and-cleanup.md`).
- Migrating persisted sessions without losing legacy URL backfill behavior.
- PR-number reuse across repos and GHE hosts: the key must be (host, owner, repo, number), not number alone.
- Many consumers of `GitHubPRNumber` in snapshots, proto, MCP and TS; missing one reintroduces the stale-PR bug.

## Alternatives Considered
- Keep one field and add a `PreviousPRs []string` list (cheap, but leaves per-PR state unmodeled).
- JSON array on the instance row (simple migration, hard to query across sessions).
- Dedicated ent entity `SessionPR` with edges to session and backlog item (queryable, per-PR state; heavier migration).
- Derive PR history from git/GitHub on demand with no persistence (no schema change; costs rate limit and loses PRs when branches are deleted).

## Feasibility Risks
- Ent schema and migration complexity in `session/ent_repository_migrations.go`.
- Concurrency between the PR status poller and instance actor setters.
- The proto change may need coordination with the mobile app client.

## Observability Requirements
Log lines when a PR is attached to a session, when the active PR changes, and when migration backfills legacy fields. Metric: count of PRs per session (histogram) and poll count by PR state, to confirm merged and closed PRs stop being polled. Adoption and usage signal for the new capabilities (plan Observability Plan, emitting tasks named there): `ssq_pr_pin_total{action=pin|unpin|undo}` (active-PR pin use, T4.2.3), `ssq_artifact_search_queries_total{kind=text|pr|sha|branch,result=hit|miss}` (T9.2.1) and `ssq_issue_close_offer_total{outcome=offered|blocked|closed|failed}` (T5.5.1 and T5.5.2; "declined" is derived as offered minus closed minus blocked, because Not now sends no request). Alerts: two warning-level rules only, no paging: the PR request budget (K10) and a non-zero `ssq_pr_unverified_terminal_rows` for more than 24 hours, shipped as a dashboard panel plus alert rule in T3.3.3.

## Risk Control
Behind the live-settable rollout flag `session_multi_pr` (decided in planning) with four reader scopes (poller, backlog, actions, ui) and a `refresher_off` kill-switch scope. The flag gates READERS only: links and per-PR state rows are written and refreshed from the first deploy, the legacy `github_pr_*` fields are always the projection of the active PR (which differs from last-writer-wins only when two PRs are open), the retention sweeper, the cleanup gate and the boot-time maintenance read link rows regardless of the flag and fail closed, and the artifact scanner no longer seeds a PR. Rollback: switch the reader scopes off (readers return to the legacy columns, which were written in parallel for the whole rollout, so no data restore is needed); setting the `refresher_off` scope stops new polling live; reverting the PR is the only way to undo the always-on parts and it removes the delete protections. Full list: plan.md Risk Control.

## Decisions (owner, 2026-11-03)
- **Active PR**: derived, with an optional user pin. Order: pin, else open PR on the session's current branch, else newest non-terminal, else newest (refined in ADR-002: an unreachable `unknown` PR ranks below every open PR). Every PR action also accepts an explicit PR reference.
- **Backlog done rule**: done when at least one PR is merged and none are open (closed-unmerged allowed if another merged). Cleanup and the retention sweeper require all PRs terminal.
- **Artifact scanner vs SessionPR**: kept separate. `SessionPR` holds agent-claimed and API-created PRs only; `session_artifacts.PRURLs` stays display-only and does not seed the relation. Older sessions' history is therefore not recovered from the scanner.
- **Artifacts scope (all four)**: (1) fix tab reliability (store empty scans, re-enqueue when a history file is linked, include subagent transcripts, fix the caps that drop the newest entries); (2) richer structured capture (PRs, branches, commits, issues, plan docs, CI links, commands paired to results); (3) cross-session artifact search that returns sessions (BM25 index in `session/search/`, plus exact PR lookup); (4) a browsable artifacts view across sessions.

- **Accepted as written (owner, 2026-11-03, after plan review round 5)**:
  - Backfill-seeded "unverified terminal" PRs count as non-terminal for the cleanup gate, sweeper and done verdict until a poll verifies them (5 counted failures over at least 30 minutes), so old done/archived items briefly keep worktrees after boot.
  - Item-direct PRs (agent-reported, Jules, manual) are never superseded; release comes when the PR goes terminal or the user detaches it.
  - Force-reset and backward-to-ready stay ungated by open PRs, with a visible warning.
  - A PR reopened on GitHub is noticed only via the `pull_request.reopened` webhook; otherwise a merged/closed row stays terminal (documented accepted risk).

## Open Questions
Planning resolved the design questions (flag `session_multi_pr`, role chips in the detail list only, `Session.pull_requests = 100`, many-to-many through `PullRequestState` + `SessionPR`, Jules writes item-direct links; plan.md "Open Questions from requirements.md: resolutions"). Genuinely open:

- Every owner item is in ONE block, plan.md "Owner confirmations pending" (13 rows, each with a recommended default and what changes if overruled): ADR-003 cleanup-gate scope incl. ungated explicit removals; `liveWorkIdleWindow` N=10 minutes, the maximum hold and the live-session rule; the `reader:backlog` scope for issue-close; session archival ungated by open PRs; attach-to-done; the unverified-terminal wording and N=5; the two iteration-3 behavior changes; the boot-rehearsal database copy and +5 ms budget; the mobile-app check of the additive `Session` fields (blocks the `reader:ui` flip, not the merge); soak sign-off and discovery cadence; the adoption keep or cut thresholds; the copy checkpoint.
- The cause of the Artifacts "sometimes works" symptom is a hypothesis until T7.1.1 records the baseline counts.
