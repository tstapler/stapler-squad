# Requirements: inbox-pr-watch-consumer

**Date**: 2026-10-21 (revised 2026-10-22 to match `implementation/plan.md` and ADR-001; third repair pass after the round-3 triad)
**Type**: feature addition (new consumer of an external local service)
**Source**: design discussion in `tstapler/gh-signal` (PRs #4, #7, #9). No interview was run; derived from reading both codebases.

## Problem

A session that opens a PR gets no push notification when a reviewer comments or CI fails
unless the PR is tracked by a backlog item in `pr_pending`. Both existing mechanisms key
off backlog items:

- `PRStatusPoller` / `ReconcilePRPending` (~60s poll) and `AutoReopenForPRFix`, which steers
  the item's active session since #645. `PRStatusPoller` itself never steers: `session/pr_status_poller.go` contains no steer or nudge call (VERIFIED by grep, 2026-10-22), it only records PR fields on the session.
- The `/webhooks/github` receiver (#628), which needs the `webhook_triggers` and
  `pr_event_webhooks` flags plus a public hook with a per-repo secret.

On the operator's machine (checked 2026-10-21) neither webhook flag is set and no GitHub hook
points at Squad, so only the 60s backlog poll runs. A session started by hand, with no
backlog item, is never nudged by anything. `schedule_wake` `ci_event`/`pr_event` is poller-based, covers
only open PRs of connected accounts, and has no comment or review trigger.

`gh-signal` (`github.com/tstapler/gh-signal`) already receives GitHub webhooks on
`webhooks.staplerstation.com` and keeps a durable per-consumer inbox of PR events behind a
loopback HTTP API (protocol v1, `docs/agent-inbox.md` there). Nothing in Squad consumes it.

## Goal

When a Squad session has an open PR, a new review, review comment, PR comment or failed CI
result on that PR steers that session within a bounded delay of the inbox receiving it, with
no per-session Monitor and no second public webhook.

**Qualifiers on the bound.** It holds when the session is idle at a safe prompt, the steer request succeeds, and (for comment and review events only) the operator's GitHub login is resolved. An event that meets a busy session, a guard hold, an unresolved login or a coalescing window is held, not lost: a window hold is bounded by the min steer interval (default 5 min), every other hold only by the 24 hour held-age ceiling (plan D13). The observed delay is logged for every steer (`item_to_steer_seconds`, plan D16, success metric 15) so the soak can check the bound.

The bound itself depends on an owner decision that is still pending (ADR-001 decision 3, listed under "Decisions awaiting owner"):

- **With the ADR-001 ticker exception**: poll interval (default 5s) plus the per-request timeout (2s) = 7s.
- **Without it** (Squad reuses its existing 60s ticker): up to one 60s tick plus the 2s request timeout, about 62s (INFERRED from the tick period).

Bursts are coalesced per `(session, PR, kind class)` with two classes, human feedback
(comment, review comment, review) and CI. The first steer for a quiet PR is immediate; later
human-feedback events inside the per-session minimum steer interval (default 5 min) are **held unacked until the window closes**, then one steer covers all of them and all are acked together, so a second comment inside the window is never lost; a CI failure for a new head SHA always steers, whatever the window (plan D12).

## Decisions awaiting owner

Both are Tyler's. Neither blocks starting the build; each changes a stated bound or risk. Each has a working fallback, so the build never waits.

| # | Decision | What changes |
|---|---|---|
| 1 | **ADR-001 ticker sign-off** (decision 3: an explicit exception to ADR-006 rule 6 for a loopback-only 5s poll). ADR-001 stays Proposed until then. | Approved: Goal bound is 7s. Rejected: the 60s ticker is used and the bound is about 62s. |
| 2 | **Open human-feedback residual-risk sign-off** (ADR-001 decision 4). The new config key `human_feedback_mode` is `all`, `allowlist` or `off`. With the proposed default `all`, any account that can comment on a watched public PR can wake the agent once per window per session and PR. | **Accepted**: ship with `human_feedback_mode = all`. **Rejected**: the shipped default becomes `allowlist` with an empty `actor_allowlist`, which behaves as `off` (no comment or review steers, a load-time warning, shown in the status surface) until the operator lists **other** trusted logins (teammates, review bots). The operator's own login is never useful in the list, because self events are always dropped. A solo operator with no other logins stays on **CI-only steering**: CI failure events do not depend on the actor, so they keep working; human-feedback steers stay off until an allowlist is set. |

## Alternatives considered

| # | Option | Why not (or why) |
|---|---|---|
| 1 | Turn on `webhook_triggers` + `pr_event_webhooks` and forward events from the gh-signal bridge to Squad's `/webhooks/github` | Cannot reach non-backlog sessions. VERIFIED: the handler's only router call is `TriggerPRFixForEvent` (`server/services/github_webhook_pr_fix.go:684`), which calls `findPRPendingItemForEvent` and returns `matched=false` when no `pr_pending` item exists (`session/backlog_lifecycle_pr.go:1843-1851`); the unmatched path only logs provenance (`server/services/github_webhook_provenance.go:89-122`). It also needs two flags, a per-repo HMAC secret and a second public hook, which this goal excludes. |
| 2 | `gh-signal inbox watch` + a Monitor in every session | Per-session setup the agent must remember, one process per session, and the work is lost when a session is hibernated. This is the manual workaround today. |
| 3 | **This plan**: Squad runs one host-level consumer of the gh-signal inbox and routes events to the sessions that own the PR | Chosen. Reuses gh-signal's existing webhook path and durable inbox; one events request per interval; works for every session whose PR fields are populated and whose item is not in `pr_pending`, without a second public webhook. Cost: depends on gh-signal P0 (HTTP `watch`) and on a hook existing in each repo. |
| 4 | **Extend `PRStatusPoller` to steer ad-hoc sessions** | Strong competitor. The poller already polls ad-hoc sessions and already holds `CheckConclusion` and `ChangesRequestedCount` on the session snapshot (`session/instance_snapshot.go:58-59`), so it needs no gh-signal, no webhook hook and no P0, and has no external dependency at all. It shares the 1-6 minute lag (60s tick plus 5 min no-PR backoff, `session/pr_status_poller.go:63,67`) and has no comment trigger today (INFERRED: the snapshot has no comment field; `github.PRInfo` carries review counts and `Checks` but no comment count, `github/client.go:66-95`). **Rough cost class (INFERRED, not scoped):** about one M task plus one S task, namely transition detection and a steer callback in `session/pr_status_poller.go` (the `SetOnUpdated` hook at `:221` and `applyPRUpdate` at `:488` are the touch points), one `server/` wiring file, and, only if comments are wanted, a comment/review count on `PRInfo`. That is order 150-250k raw tokens against 2,620k raw for this plan. What it cannot produce: comment and review-text events, and CI steers faster than its 60s tick. Not chosen because comments and sub-minute latency are the stated goal; kill criterion 6 measures how much of the delivered value only this plan could have produced. |
| 5 | **Cheaper Monitor variant**: Squad auto-starts `gh-signal inbox watch` / `wait` once per session | Removes the "agent must remember" cost of alternative 2, but keeps one process and one consumer ID per session (the orphan and cleanup problem in pre-mortem failure 5), is lost on hibernation, still needs P0 for coverage reporting, and still needs Squad to read each process's output and steer. It buys nothing over the host consumer except a simpler routing table. |

**Why this roadmap now (stated as a bet).** The only first-hand evidence is the operator's own
observation recorded in "Problem" (checked 2026-10-21): hand-started sessions are never nudged.
**The bet:** the operator's unmet need is text events (comments, review comments) and sub-minute CI latency on hand-opened PR sessions, which is exactly what alternative 4 cannot give. Nothing yet proves it. **Evidence that would refute it:** (a) the task 0.3 baseline finds few ad-hoc PR sessions or only CI-shaped feedback; (b) kill criterion 6 fires (fewer than 30% of delivered steers are ones a poller could not produce); (c) the 20-entry sample finds most comment steers unwanted (criterion 3).
Evidence that is **missing** today: how many ad-hoc PR sessions exist, how often feedback lands while
such a session is idle, and whether CI-failure plus changes-requested alone would capture most of the value. Plan task 0.3 measures what the session store and logs can answer
before any build task starts; what it cannot answer is recorded there as a gap. **Task 0.3 calibrates thresholds (kill criterion 1's expected-steer figure E, the expected hook coverage); it is not a go/no-go gate.** The owner pursues the full scope; the plan proceeds regardless and the baseline sets the context in which the kill criteria are read.

## Non-goals

- Backlog-tracked PRs whose session's newest linked item is in `pr_pending`. The existing reconcile path (#645) owns exactly those; this consumer must not
  double-steer. Sessions linked to an item in any other status are in scope (see out of scope below for why).
- Registering GitHub webhooks **from Squad's own code**. Squad never calls the GitHub hooks
  API. The one-time per-repo registration is done by gh-signal; Squad asks for it by calling
  gh-signal's HTTP `watch` (plan prerequisite P0), which wraps gh-signal's existing
  `ensure_coverage`. Where coverage is unavailable, Squad only notifies the operator.
- Putting any PR text (comment or review bodies) into a pane.

## Out of scope (accepted limits, documented in the operator doc)

- Fork sessions (`GitHubIsFork`): the poller skips them (`session/pr_status_poller.go:323-326`) and gh-signal sends no CI events for fork PRs (`docs/agent-inbox.md:80-83`).
- GitHub Enterprise hosts: the inbox keys on `owner/name` with no host, so only `github.com` is watched.
- Events that happened before subscription, except the one-time catch-up read on first subscribe (and on resume of a suspended session). The inbox never replays. The catch-up covers a failing CI conclusion and (in `human_feedback_mode = all` only) changes requested; plain comments that landed before subscription are not recovered.
- The 1-6 minute lag before Squad learns a hand-opened PR's number (`PRStatusPoller` 60s tick plus 5 min no-PR backoff, `session/pr_status_poller.go:63,67`). Catch-up inherits this lag. This lag is not measured by this feature (success metric 11).
- **Backlog scope, decided from code.** Only a session whose newest linked item is in `pr_pending` is excluded, because #645 steers `pr_pending` items only (`session/backlog_lifecycle_pr.go:1789-1797`). A session linked to an item in `in_progress`, `review` or any other non-`pr_pending` status is **not** steered by #645, so this consumer covers it; a session whose newest link is `done` or `archived` is covered too. Residual: a brief window in which PR fields exist before the item reaches `pr_pending` (BUG-058, `docs/bugs/open/BUG-058-pushAndCreatePR-non-atomic-pr-field-write-and-transition.md`), and an item that #645 reopens out of `pr_pending`; the shared guard signature (60s duplicate window) and a catch-up suppression after a `pr_pending` exclusion bound the double-steer risk (plan D4).
- Gone sessions (stopped, permanently failed, removed): events for them are dropped with a counter. Suspended sessions (paused, hibernated, crashed) are different: see the ack rule below.
- **Per-session snooze.** Not built: it needs either a persisted per-session field plus a control surface (a UI, proto or MCP change, all excluded below) or a memory-only flag that silently resets on restart. The supported opt-outs are the feature flag, the `ignore_repos` config list and `human_feedback_mode` (plan Config table).
- Status history (the 200-entry event ring buffer, counters, the stranger-activity window) is in memory and resets on restart; the structured log lines are the durable record.
- Any UI change (no proto or web-app work).

## Users

Tyler, running Squad with sessions that open or adopt PRs outside the backlog flow.

## Success metrics

Latency terms: *poll interval* defaults to 5s (`poll_interval_seconds`); *min steer interval* defaults to 5 min (`min_steer_interval_seconds`; `0` disables coalescing for debugging). Metrics 10 and 11 and kill criterion 2 require gh-signal **P0** (HTTP `watch` reports coverage); under the 404 fallback coverage is "unknown" and those measures are **not computable**.

1. **Latency.** Under the Goal qualifiers, a review comment on a PR owned by an idle ad-hoc session produces one steer within one poll interval plus the 2s per-request timeout of the inbox receiving it (default: 7s with the ADR-001 exception; about 62s on the 60s ticker), and the inbox item is acked. Verified two ways: on a fake clock in the integration test, and against a real `httptest` server with a real slow response on the real clock (plan task 5.1), so the bound is not an artefact of the fake clock. The first steer for a quiet PR is never delayed by the min steer interval.
2. **Dedup.** The same event delivered twice (at-least-once) produces one steer.
3. **Busy session.** A busy session is not written to; the item stays pending and is steered once the session is ready. Busy, guard-busy and guard-cooling-down are holds, not failures: they raise no *immediate* operator notification. A hold that lasts 60 minutes raises one `HeldLong` notice and at the 24 hour ceiling one `HeldDropped` notice (plan D13, D15); only a failed write raises `SteerFailed`.
4. **Backlog exclusion.** A PR whose session's newest linked backlog item is in `pr_pending` produces no steer from this consumer; a session linked to an item in any other status, or to none, is watched.
5. **Degradation.** Inbox down, token missing or token rejected degrades to a logged, rate-limited warning; Squad startup and every other feature are unaffected.
6. **Event policy.** The following produce no steer and are acked with a counted reason: approved reviews, `pull_request` events, edited or deleted comments, cancelled or otherwise non-failure CI, per-job `check_run` events, the operator's own events (self), bot actors, human-feedback events when `human_feedback_mode` is `off` (or `allowlist` with an empty list) or when the actor is outside the allowlist in `allowlist` mode, unknown event kinds, events whose repository does not match the subscription, hidden sessions, gone sessions, and events past the held-age ceiling (full list: plan D7).
7. **One steer per failed run.** A recorded fixture of one failed CI run with three jobs (events for `check_run` x3, `check_suite`, `workflow_run`) produces exactly one steer. A CI failure after a comment steer, inside the window, still steers (plan D12).
8. **Catch-up.** A PR whose CI is already failing, or (in `human_feedback_mode = all`) that already has changes requested, when it is first subscribed (or when its suspended session resumes) gets one steer from the catch-up read.
9. **No self-echo.** A comment authored by the operator's own login (the session's `gh pr comment`, or a comment typed in the GitHub web UI) produces no steer, and with the login unresolved, comment and review events are held rather than steered.
10. **Coverage gap** (needs P0). When gh-signal reports no webhook coverage for a repo, exactly one operator notification is raised per repo until coverage appears. Squad re-checks uncovered repos every 10 minutes (`coverage_recheck_minutes`, plan D9) so the notification clears after the operator fixes the hook; without the re-check the state would never clear.
11. **Outcome** (coverage half needs P0). Over the soak window, the fraction of ad-hoc-PR sessions (non-backlog, github.com, non-fork) whose repo has coverage is at least 80% (repos in `ignore_repos` leave the denominator). For lag, prwatch logs `discovered_to_subscribed_seconds`, the time from the first tick that sees the PR number in the session snapshot to successful subscription. For PRs opened through Squad, `pr_created_to_subscribed_seconds` is also logged. For a hand-opened PR the creation time is unknown, so **the poller's own 1-6 minute discovery lag is unmeasured** and stays an accepted limit.
12. **Operator is told about runtime failures.** Each of these states raises exactly one plain-text notification with a next step, deduped on state and cleared on recovery: inbox unreachable, token rejected (401), token missing, token permissions wrong, non-loopback URL, and leader lock not held.
13. **Operator can ask why.** The status surface (plan D14) answers "why wasn't my session nudged?" for uncovered repos, skipped sessions and dropped events, using per-repo coverage state, a bounded list of recent event dispositions (no actor, no text) and the next time each session is eligible to be steered. It also answers when the feature is off (a text answer, not a 404). Every notification points at it.
14. **Steers get acted on** (observable proxy). After each delivered steer prwatch logs whether the session left idle within 2 minutes (`agent_turn_observed`). This measures a turn, not usefulness (plan D17).
15. **Latency is observable.** Every delivered steer logs `item_to_steer_seconds`, the time from the inbox item's `created_at` to the steer (plan D16), so the Goal bound and its qualifiers can be checked in production. The inbox `created_at` is the inbox's ingest time, not GitHub's event time (INFERRED).
16. **Stranger activity is visible without logging identities.** Every human-feedback event is classified `actor_class` in {self, bot, allowlisted, other} (no login is ever logged), counted, shown in the status surface, and more than 3 `other`-class steers on one repo in a rolling 7 days raises a `StrangerActivity` operator notification suggesting `human_feedback_mode = allowlist` (plan D11, D15).

## Assumptions and kill criteria

Assumptions (each one is a way this feature can be useless), with how each is measured:

- A1. The operator wants unattended agents to be woken by comments from accounts that are neither the operator, a bot nor on an allowlist ("other" actors) on public repos. Measured by the `actor_class = other` steer counter per repo (log field and status counter, no logins). **Pause threshold:** more than 3 `other`-class steers on one repo in a rolling 7 days raises the `StrangerActivity` notice asking the operator to set `human_feedback_mode = allowlist` (or `off`). **Kill threshold:** criterion 5. Owner decision 2 above sets the shipped default.
- A2. A gh-signal hook exists, or can be created by `gh-signal inbox watch`, in the repos where ad-hoc PRs are opened. Today only `tstapler/gh-signal` and `tstapler/stapler-squad` have hooks (pre-mortem, `~/.config/gh-signal/config.d/95-webhooks.toml`). Measured by: metric 11 coverage fraction (needs P0).
- A3. Ad-hoc sessions populate `GitHubOwner/GitHubRepo/GitHubPRNumber` soon enough (1-6 min) for subscription to follow quickly. The earlier proxy (share of first subscriptions that fire a catch-up steer) is **dropped as a kill signal** because it is confounded by the discovery lag and by CI duration, and the snapshot carries no check completion time (`github.CheckItem` has no timestamp, `github/client.go:155-161`, VERIFIED) so "failure predates subscription" cannot be computed. It is still reported (informational, no threshold). The measured part is `discovered_to_subscribed_seconds`. **Kill threshold (criterion 4):** p95 above 600 seconds, which is far beyond the design (one reconcile interval of 60s plus a tick) and means the subscription path is broken. The pre-discovery lag stays unmeasured and accepted.
- A4. The agent re-reading the PR with `gh` after a generic notice is enough to act on it. Measured by: metric 14 (`agent_turn_observed`) and the Epic 6.5 manual check of the agent's actual response. Same kill threshold as criterion 3 below.

Kill criteria, evaluated after a 4-week window of enabled use (owner: Tyler; data from the structured steer log and the status endpoint). **The window starts when the flag is on and P0 is live. Deadline: P0 must be live within 7 days of the flag being turned on; if it is not, the window restarts on the day P0 goes live (a start date only, no scope change). If P0 never lands, the review runs at 4 weeks with criterion 2 reported as not evaluable.**

1. Steers delivered in the window is below `max(5, ceil(0.25 x E))`, where **E** is the expected steer count per 4 weeks from the task 0.3 baseline: (ad-hoc PR sessions in hooked repos per 4 weeks) x (mean actionable events per PR in the 0.3 sample). If 0.3 left E `UNMEASURED`, the floor 5 applies alone.
2. Coverage fraction (metric 11) is below 50% (**requires P0**; with the 404 fallback this criterion cannot be evaluated and the review says so instead of passing it). **Decision rule for 50% to 80%:** not a kill. The review extends the window by 2 weeks during which every uncovered repo is either hooked (the CoverageGap next step) or added to `ignore_repos` (ignored repos leave the denominator); if coverage is still below 80% at the end of the extension, treat it as a kill.
3. More than 30% of delivered steers have `agent_turn_observed=false`, **or** in Tyler's sample of up to 20 steer-log entries read against their PRs (all of them if fewer than 20; not evaluable below 5) fewer than 70% (14 of 20) were *wanted*. An entry is *wanted* if the announced activity needed action from the PR's author. Operator dismissal is not observable (there is no UI), so it is not used.
4. Assumption A3's threshold above is exceeded (p95 `discovered_to_subscribed_seconds` above 600).
5. Assumption A1's kill threshold: more than 10 `other`-class steers in a rolling 7 days on any single repo while `human_feedback_mode = all` and the operator has not changed the mode.
6. **Alternative-4 counterfactual.** Each delivered steer is logged with `alt4_class` (plan D16): `text_event` (a comment, inline comment or non-changes-requested review, which the poller's snapshot cannot see), `ci_subminute` (a CI steer delivered within 30 seconds of the inbox item's `created_at`, which a 60-second poller tick would on average miss; 30s is half the tick, INFERRED) or `ci_other` (also producible by an extended poller). If `text_event` plus `ci_subminute` is below 30% of delivered steers, an extended `PRStatusPoller` would have produced at least 70% of the value at the cost class in alternative 4: disable this flag and build alternative 4 instead.

If any holds, disable the flag, remove the wiring, and keep the inbox client package only if another consumer needs it.

## Constraints

- **Untrusted text.** Inbox events carry `"trust":"untrusted"`; bodies are written by
  anyone who can comment. The steer text is a fixed template built only from the enum `kind`
  (`review`, `comment`, `inline comment`, `CI failure`, or a fixed enum list when several kinds coalesce, for example "review, CI failure"), the
  PR number and a validated `owner/name`, preceded by the provenance prefix
  `[stapler-squad automated notice]`. It never contains the actor, the action, or any
  `summary`/`details` text. The session re-reads the PR itself (`gh pr view <n> --repo <owner>/<repo> --comments` for review
  and comment activity, `gh api repos/<owner>/<repo>/pulls/<n>/comments` for inline review comments because `--comments` may omit them (INFERRED, verified in plan task 6.5), `gh pr checks` for CI). The text also tells the agent that genuine Squad
  notices appear only in the terminal and never inside PR text, so a PR comment that imitates the
  prefix is not a notice. The intended action: read the feedback and, if it is within the scope of
  the task the session was given, address it; otherwise tell the operator. This applies equally to
  an idle or finished ad-hoc session. Actors are used only for policy decisions (self, bot, allowlist, `human_feedback_mode`). **Neither actor logins nor text are ever logged or shown**: logs, counters, the status surface and the ring buffer carry only an `actor_class` in {self, bot, allowlisted, other}.
- **Loopback only.** The inbox URL must be `127.0.0.1`, `::1` or `localhost`; refuse
  anything else. The bearer token comes from a file path (default
  `~/.local/state/gh-signal/inbox.token`), is never logged, never put in a URL or process
  args, and the file is not written by Squad.
- **Reuse, don't reinvent.** Steer through the existing `SessionSteerer`
  (`server/services/backlog_service.go:100`): `IsHiddenSession` is checked first, then
  `SteerSessionGuarded`, which itself applies readiness (`IsSafeSteerStatus`), pane ownership and the
  `sessionNudgeGuard` dedup used by #645. prwatch does not pre-check `IsReadyForSteer` separately.
  Follow the consumer-defined-interface DI pattern.
- **Feature flag**, default off, in the same family as `pr_event_webhooks`.
- **Delivery is at-least-once** on the inbox side. **Ack rule:** ack when an event is
  delivered, is a duplicate of a delivered steer (except a CI event with a new head SHA, which is held), is covered by the same CI head SHA (`covered_same_head_sha`), is folded into a window-closing steer that was delivered (all folded items are acked together with it), or is a deliberate policy drop (self, bot, `human_feedback_mode` off or actor not in the allowlist, approved
  review, `pull_request` event, edited or deleted comment, non-failure CI, per-job `check_run`, unknown kind, repository mismatch, hidden session, gone session, held-age ceiling).
  **Hold** (leave unacked, retry every tick, never ack) on: session busy, guard busy, guard cooling-down, `no_status_source`, `unspecified`, `not_tracked` while the session is still listed live, a CI duplicate with a new head SHA, comment and review events while the login is unresolved, human-feedback events waiting for the coalescing window to close, and events for a suspended session that the on-resume catch-up does not re-read. A failed write is a hold that also raises `SteerFailed`. Holds are bounded by the 24 hour ceiling (a window hold by the min steer interval).
  An event for a **suspended** (paused, hibernated, crashed) session is acked as dropped (`session_not_live`) **only** for kinds the
  on-resume catch-up re-reads (a CI failure, and changes requested in `human_feedback_mode = all`); plain comments are held until the session resumes or the
  held-age ceiling.
- Non-Claude programs get plain-English text only (no slash commands), as in #645; every
  program gets the same text.
- **Operator-facing text** (notifications, status output, the operator doc) is plain text. Raw outcome enum words (`guard_busy`, `cooling_down`, ...) do not appear in it. One operator vocabulary is used everywhere (plan Domain Glossary): a **steer** is text written into a session, **waiting** is an event held for later (internal: held), **not steered** is a dropped event, and **covered** means only that a repository has a gh-signal webhook that reaches the inbox.

## Non-functional requirements

- **Scalability**: a handful of concurrent sessions; **one events request per tick** (plus further pages only when a page is full) regardless of session count, **a subscription listing about every 60 s** (`reconcile_interval_seconds`, expressed in time so it holds under the 60s fallback ticker too), **a coverage re-check every 10 min for uncovered repos only**, and **watch, subscribe and unsubscribe calls only when the watch set changes** (plan D1, D9). No events request at all while no session has a watched PR.
- **Security classification**: internal; adds one loopback client and one file read.
- **Accessibility / UI**: none in the first slice (no web-app change). Operator-facing text
  (notifications, status output, operator doc) is plain text with no icon- or color-only meaning and no terminal escape codes.
- **Operability**: a status endpoint answers "why wasn't my session nudged?" (plan D14, tasks 5.3a/5.3b), including when the feature is off and with the effective configuration and any corrected or ignored settings.

## Appetite

Large band by the estimation reference (`~/.claude/skills/sdd/skills/ESTIMATION.md`):
about 3.85M cost units of implementation work, 5.8-7.7M with the verification multiplier,
across 8 agent waves (W0 to W7, six of them build waves; see plan "Effort"). The calendar is set by the wall-clock blockers
listed in the plan (the two owner decisions above, gh-signal P0 merge and restart, a hooked repo for the
live e2e, the 4-week soak). INFERRED; recalibrate after the first implementer run. No scope reduction is proposed.

## Questions: what is resolved and what is not

1. **Decided in the plan, implementation pending.** gh-signal needs an HTTP `watch` (subscribe plus ensure-webhook); it is prerequisite P0 in the plan, with a fallback to `/v1/subscriptions` and an "unknown coverage" state when the endpoint is absent. Whether the crate boundary allows it is UNVERIFIED (spike in plan task 0.2).
2. **Resolved.** One host consumer `squad:<hostname>` with Squad routing; plan Decision D2 and ADR-001 decision 1.
3. **Resolved.** A hand-opened PR gets `GitHubPRNumber` from `PRStatusPoller` after 1-6 minutes and never gets `GitHubPRURL`; owner/repo come from worktree detection and are resolved from the git remote when empty. See `implementation/pre-mortem.md` section (a) and plan D8.
4. **Open (owner), each with a working fallback.** The two decisions under "Decisions awaiting owner".
5. **Open (evidence).** The baseline in plan task 0.3 (how many ad-hoc PR sessions, how often feedback lands while idle). What the session store cannot answer is recorded there.
