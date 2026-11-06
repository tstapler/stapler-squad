# Requirements: up-next-tabs-pr-nudge

**Date**: 2026-10-05
**Type**: feature addition (UI revamp + one new RPC/action)
**Complexity**: 3 — multi-component feature (proto + GitHub GraphQL + session-link key fix + steering safety + two UI areas, two PRs). The earlier "2 / focused" understated it: the safety prerequisites (key schema, token resolver, readiness spike, pane-ownership check) are real work, not polish.

## Problem Statement
Two separate problems, each with its own falsifier.

**Job story (trigger):** When CI turns red or a reviewer leaves threads on one of my open PRs while I am working in another session, I want to see that PR and get the owning session to fix it without opening the session and typing the instruction, so the PR does not sit failing unnoticed.

**(a) Findability.** The Up Next page (`/unfinished`, `web-app/src/app/unfinished/UnfinishedTab.tsx`) is one long scroll: Stuck Backlog Items, then In Progress (GitHub PRs section + worktrees grouped by repo), then the Backlog queue. Open PRs sit below the large Stuck section and are hard to see next to the other data.
- *Falsifier:* if, over the first 2 weeks after PR A ships, the user leaves the default PRs tab on more than half of visits (tab-usage counters, Task 3.1.1d), the default/placement hypothesis is wrong. Time-to-first-PR-card (same task) is recorded as a secondary signal; the old layout has no equivalent number, so it is a trend, not a before/after.

**(b) Actionability.** A PR card shows only "Open Session"/"+ Session"; nothing tells a linked session its CI is failing or that review threads are unaddressed, so the user types the fix instruction by hand.
- *Falsifier:* the nudge outcome and follow-up logs (Success Metrics) show few nudges, mostly non-delivered nudges, or nudges not followed by CI going green / threads resolving.

## Baseline
- Scroll past Stuck Backlog Items to reach `GitHubPRsSection`; it can show "Connecting to GitHub…".
- `PRCard` (`GitHubPRsSection.tsx`) shows title, CI chip, review chip, branch, one `sessionIds[0]` link. `UserPR` proto (`proto/session/v1/types.proto`) carries `session_ids`, `check_conclusion`, `changes_req_count`, but no unresolved-comment count or failing-check detail.
- To get a session to fix CI/comments, the user opens the session and types the instruction. `pr-fix-steering` only automates this for backlog-item sessions. The current frequency of hand-typed PR-fix instructions is **unknown** (no log exists); the user tallies them by hand during the first week after PR A ships (a count, not an estimate), and that tally anchors the DELIVERED/week threshold below.
- No tab/section state is remembered across visits.
- **Measured baseline: none.** The only evidence is the user's own report (open PRs are hard to find; fix instructions are typed by hand each time). No usage log records how often the user types PR-fix instructions into sessions, how long a failing CI sits unnoticed, or how many PR-fix messages are sent per week. Today's baseline is therefore qualitative; the nudge outcome log (Task 2.3.1e) starts the quantitative series at ship, and the first two weeks of data are the baseline for later judgment (not a before/after comparison against a number we do not have).

## Users / Consumers
The single user running stapler-squad locally, managing many concurrent agent sessions across repos.

## Core Value Hypothesis (testable)
If open PRs sit on a default, persisted tab and a failing PR can be nudged in one click, the user will (a) stop opening sessions to type PR-fix instructions and (b) act on failing CI sooner.

**The thresholds below are guesses**, labelled as such with the reason each was picked; revisit them once the first-week tally exists:
- *3 `DELIVERED` nudges per week:* roughly "every other working day", the lowest rate at which a one-click flow plausibly pays for building and maintaining it for one user. Replace with ~50% of the hand-typed tally once measured.
- *More than 50% of nudges attempted on live (steer-ready) sessions ending `BUSY`/`PAUSED`/`NOTHING_TO_FIX`:* a coin-flip cutoff; above it the button fails more often than it works. Sessions that were not live or not steer-ready are excluded: the card sends those users to "Open session" instead of offering a nudge (decided 2026-10-07), so they are a funnel stage, not nudge failures.
- *Leaving the PRs tab on more than 50% of visits:* same coin-flip logic for the default-tab choice.
- *Windows:* revisit at two weeks, kill at four (two weeks is the shortest window with ~10 working days of PR activity).

**Revisit criterion:** two weeks after PR B ships, if the log shows fewer than 3 successes per week (`DELIVERED` nudges plus "Open session" clicks from PR cards), or more than half of `nudge_outcome` lines with `session_live=true` end `BUSY`/`PAUSED`/`NOTHING_TO_FIX`, treat the one-click flow as not paying for itself and revisit (widen the readiness gate, or cut the nudge and keep the tabs). Also revisit if fewer than a third of PRs needing attention with a linked session have a live one (`up_next_funnel`): the nudge then rarely applies. **Kill criterion:** zero `DELIVERED` nudges and fewer than 4 "Open session" clicks in 4 weeks after ship means remove the nudge UI and keep PR A.

## Why a manual nudge and not extending `pr-fix-steering`
`pr-fix-steering` is automatic and keyed by backlog item (`steerActiveSessionForPRFix`, item-keyed guard); most PRs the user watches have no backlog item, and an automatic send into a session the user is mid-conversation with is the interruption they want to control. A manual click keeps the user deciding when and which session, uses the same readiness gate and a shared per-session guard so the two paths cannot collide, and doubles as the experiment: if the click is rarely used, automation would not have helped either. Extending auto-steer to PR-only sessions stays out of scope until the revisit review says the click pays off.

## Product fallback if the GraphQL cost gate fails
If the widened poll costs more than 50 points per poll or more than 10% of the hourly limit (Task 1.2.1a), the behavior is the degraded mode (ADR-001): cards show failing checks and merge conflicts plus a "? threads" chip, the tab badge reads "3+" with the explanation, thread counts load lazily on card expand or at nudge time, and the nudge is offered for failing checks and conflicts until thread detail is known. The feature still ships; it does not wait for a cheaper query.

## Success Metrics
**Outcome metrics (measured from the nudge log, Task 2.3.1e; the user reads them with a log query documented in Task 6.1.1f):**
- Nudges sent per week (`DELIVERED` count), and "Open session" clicks per week from PR cards (`openSessionClicks` in `up-next-tab-stats`; a success signal because for not-idle sessions it is the primary action).
- **Funnel (per snapshot sent to a client):** `up_next_funnel` log line with `prs_needing_attention` -> `with_linked_session` -> `with_live_session` (a linked session the server reports `steer_ready`); then clicked (`nudge_request`) -> `DELIVERED` (`nudge_outcome`). Snapshots repeat per page load and stream open, so read ratios, not totals.
- Nudge outcome mix (`DELIVERED` / `BUSY` / `PAUSED` / `DUPLICATE` / `NOTHING_TO_FIX` / other) per week, over `nudge_outcome` lines with `session_live=true` (the delivery-rate denominator: nudges attempted on live sessions).
- Time from CI failure first seen by the poll to nudge (`attention_age_s` in the log line; in-memory first-seen map, so it resets on restart and is a lower bound).
- **Outcome (does the nudge work?):** after a `DELIVERED` nudge, did the PR stop having the nudged reason? The poll already carries the data, so the cache emits a `nudge_followup` log line (Task 2.3.1h) when a nudged PR's reasons clear (CI green, thread count 0, conflict gone) with `resolved_after_s`, and `expired` after 24 h without clearing. Read as: share of `DELIVERED` nudges followed by `resolved` within 24 h. In-memory (lost on restart), PRs closed or merged in the interim log as `closed`; this is correlation (a human may have fixed it), not proof the agent did.
- **Findability (PR A):** client-side tab-usage counters in localStorage (`up-next-tab-stats`: visits, share landing on / leaving PRs tab, ms to first PR card; Task 3.1.1d), read with the snippet in Task 6.1.1f. Single browser, single user, so it is a self-report aid, not telemetry.

**Existence checks (necessary, not sufficient):**
- On load, open PRs are visible with zero scrolling when the PRs tab was last selected (persisted).
- "See failing PR" to "session nudged" is one click on the PR card (vs. open session + type prompt today).
- Every open PR shows all linked local sessions, not just the first.
- Selected tab survives reload and navigation.

## Appetite
Medium (1–2 weeks) as requested. Plan estimate: PR A about 62 h (fits, top of range), PR B about 58 h, whole feature about 119 h (about 3 weeks, exceeds appetite; see plan "Estimates").

## Constraints
- Web-app conventions: pnpm, vanilla-extract CSS, `data-testid`/ARIA locators, e2e spec `// @feature` header, feature registry update.
- Reuse the existing steering primitive and PR pipeline (`useGitHubPRs`); additive proto fields via `make proto-gen`.
- GitHub requests via `NewConditionalRequest*` (`.claude/rules/norawghrequest.md`); no extra quota beyond existing polling where avoidable.
- Manual testing on a separate instance, never `make install-service`.
- Safety prerequisites: PR-to-session link keys must be host/owner/repo-aware (today they are owner-only and collide), and the nudge's fresh PR fetch must use the PR's owning-account token via a token resolver, never the host-default token.

## Non-functional Requirements
- **Performance SLO**: not specified
- **Scalability**: tens of PRs, hundreds of worktrees
- **Security classification**: internal. `NudgeSessionForPR` is a write-capable RPC (types text into a live agent PTY), so it is guarded like `ProbeProgram`: loopback Host/Origin check on the unauthenticated :8543 listener (DNS-rebinding and cross-origin defense) and the existing auth middleware on the remote-access :8444 listener (plan Task 2.3.1f). Nudge text carries link/path/author only, never comment bodies.
- **Data residency**: no special requirements

## Scope
### In Scope
- Tab-driven layout: **PRs / Stuck / Worktrees / Queue**; PRs is the default on first visit.
- Persist last-selected tab (localStorage) with a `?tab=` URL param; the existing `?item=` deep link selects the Stuck tab.
- Tab badges with counts (PRs needing attention, stuck count).
- PRs tab: all open PRs with CI status (failing checks), unresolved review comments, and all linked sessions.
- Nudge on PR card: one-click send of a generated prefilled prompt (failing checks / unresolved comments with links) to a chosen linked session (default most recently active); with no session, "+ Session" starts one seeded with the fix prompt.
- Move existing sections into tabs without losing behavior (filters, dismiss/snooze, deep links).
- Merge conflicts are a third nudge reason (alongside failing checks and unresolved threads).
- Nudge only when the PR is nudgeable (failing checks, unresolved threads or merge conflict); a changes-requested-only PR counts toward the badge but gets no nudge.
- The nudge appends no auto `/github:pr-ship` (or any slash command); the user stays in the loop.
- The nudge prompt carries no third-party comment bodies: link, path and author only.
- Per-account poll status: a failed or expired account (401) is shown as its own state, distinct from "not connected", while other accounts' PRs still render (Task 1.2.2d).
- Degraded "3+" badge when thread counts are not in the poll (`detailsLoaded=false`), with an accessible name ("3 or more need attention").

### Out of Scope
- Automatic nudging (covered by `pr-fix-steering`).
- Editing the prompt before sending; nudging all sessions at once.
- Merging PRs or resolving review threads from the UI.
- Redesigning Stuck/Worktrees/Queue internals.

## Rabbit Holes
- Unresolved comment count and failing-check names aren't in `UserPR`; may need GraphQL `reviewThreads`/check-run data under the ETag/rate-limit rules.
- PR→session mapping is by branch checkout; forks, renamed branches, paused worktrees may be missed.
- `UnfinishedTab` uses `useSearchParams`; localStorage vs URL tab state must not fight the `?item=` deep link.
- Steering a session that is mid-turn or paused (reuse `diagnose_nudge_session` safeguards).
- jscpd duplication threshold (0.14% per `web-app/.jscpd.json`, verified; CLAUDE.md still says 0.12%, fixed in Task 6.1.1c) with heavily mocked new tests.
- Merge conflicts are a third nudge reason and ARE in scope (Scope list); the remaining rabbit hole is `mergeable: UNKNOWN`, which GitHub computes lazily, so the conflict state can be unknown on first poll and is shown as unknown, never false.

## Alternatives Considered
- Sticky anchor nav on the scroll page (rejected: user wants tabs + remembered state).
- Merge Stuck + Queue into "Needs attention" (rejected: user chose four tabs).
- Editable prompt before send (rejected: user chose one-click).

## Feasibility Risks
- Comment/check detail needs extra GitHub data per PR (quota, GHE host support).
- Steering a session not in a state to accept input.

## Observability Requirements
Standard request logging plus one structured `nudge_outcome` log line per `NudgeSessionForPR` call (outcome, reasons, session, PR key, latency, `attention_age_s`; never prompt or comment text) so the outcome metrics above can be computed (Task 2.3.1e).

## Risk Control
Not needed for rollout — additive RPC/proto fields. The RPC is write-capable; its access guard is specified in plan Task 2.3.1f rather than assumed.

## Open Questions
Resolved by the plan (`implementation/plan.md`, ADR-001..003):
- Source of truth for "unresolved comments" and failing check names: RESOLVED. GraphQL `reviewThreads` (count of non-resolved, non-outdated threads) and `statusCheckRollup.contexts` ride the viewer poll, with a cost gate and degraded mode (ADR-001, Story 1.2.1).
- Reuse `pr-fix-steering`'s `fixContext` builder server-side? RESOLVED: no. A new pure `BuildPRNudgePrompt` (link/path/author only, no comment bodies, no auto-ship command) because `buildSteerMessage` is item-coupled (Task 2.1.1c).
- Keep `UnfinishedNavBadge` semantics unchanged? RESOLVED: yes, unchanged (Story 3.3.1 AC; its existing tests pass without edits).

Still open (owner = plan task, not a requirements blocker):
- "Most recently active" signal for `LinkedSession.last_active_at` - Task 1.3.1b.
- Seed path for "+ Session" - Task 1.4.2a (timeboxed).
- Stuck-tab count source - Task 3.3.1a.
- Viewer-poll cost > 50 points - Task 1.2.1a gate.

## Delivery
Two PRs (plan "Delivery split"): PR A (tabs, persistence, PR card data) and PR B (nudge RPC + UI). Honest sizing (plan "Estimates"): PR A is about 62 focused hours and fits the 1-2 week appetite on its own (top of the range); PR B adds about 58 hours, so the whole feature is roughly 119 hours and **exceeds the 1-2 week appetite (about 3 weeks)**. The split exists for exactly that reason: PR A is independently shippable; PR A alone delivers the "find my PRs" half of the problem, PR B the "fix them in one click" half and the outcome metrics.
