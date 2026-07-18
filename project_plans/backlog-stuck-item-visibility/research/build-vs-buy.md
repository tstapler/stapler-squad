# Research: Build vs. Buy — backlog-stuck-item-visibility

## Context recap

Self-hosted, single-user Go+ent+sqlite+ConnectRPC+React tool. No multi-tenant needs,
no budget/appetite for paid SaaS, existing infra should be reused. Four root causes:
(1) no PR-ready-to-merge notification path, (2) rework-cap silently parks items with one
ephemeral toast, (3) stuck-state bookkeeping is in-memory only, (4) items cycle
in_progress ↔ review without converging, no persistent record.

---

## 1. Existing OSS library or framework

### (a) Durable "stuck state" / dead-letter-style tracking in Go

**Findings**: No dead-letter-queue or job-tracking library is present in `go.mod`. The
repo already has the durable primitives this feature needs, built on `ent`:

- `session/ent/schema/backlog_item.go` — `BacklogItem` with `status`, `updated_at`,
  indexed by `(status, updated_at)` and `(status, priority)`.
- `session/ent/schema/backlog_status_event.go` — `BacklogStatusEvent`, an **append-only
  log of every status transition** (`from_status`, `to_status`, `triggered_by`, `note`,
  `created_at`), already indexed on `(item_id, created_at)`, cascade-deleted with the
  parent item.

This is exactly the shape a "stuck state" / dead-letter table needs: a timestamped
transition ledger keyed by item. Standing up a generic DLQ library (e.g. a
Postgres-oriented outbox/DLQ package) would duplicate `BacklogStatusEvent` and add a
dependency with no multi-tenant or cross-service use case to justify it. The gap isn't a
missing library — it's that this event log currently has no reader that classifies a
run of transitions as "stuck" and nothing surfaces that classification durably (see
`notifyReworkCapHit`, which is a fire-and-forget `eventBus.Publish`, not a DB row).

**Verdict: Not recommended (library). Recommended: extend the existing `ent` schema** —
add a small `stuck_reason`/`stuck_since`/`resolved_at` field set to `BacklogItem` (or a
thin new `BacklogStuckState` table keyed by `item_id`) and a query over
`BacklogStatusEvent` to detect non-converging cycles. This is "a few DB columns/rows,"
not a framework-worthy problem.

### (b) GitHub PR status polling

**Findings**: The repo does **not** depend on `github.com/google/go-github` or any other
GitHub SDK — grep of `go.mod`/`go.sum` and the whole tree found no such import. Instead
there is a hand-rolled, already-battle-tested `github` package
(`/home/tstapler/Programming/stapler-squad/github/`):

- `http_client.go` — builds authenticated `*http.Request`s against the GitHub REST API
  using the `gh` CLI's stored token (`getGHToken`).
- `etag_cache.go` — `ETagCache` + `GetPRInfoConditional`, giving free 304-based polling.
- `rate_limit.go` — `RateLimiter` with `IsLimited`/`WaitIfLimited`.
- `priority.go` — `DerivePRPriority`/`IsTerminal`, which **already encodes a
  "ready-to-merge" signal** (`PRPriorityReady` = approved + CI passing) and a terminal
  set (`PRPriorityComplete` = merged/closed).
- `session/pr_status_poller.go` (`PRStatusPoller`) — the single shared polling loop,
  workspace-wide ticker, `noPRPollAfter` backoff map, auth-check caching. Extended (not
  duplicated) per **ADR-022** to also poll worktree-only PRs via a merged
  `owner/repo/branch` index.
- `session/backlog_plugin_github_prs.go` — a second, simpler raw-`net/http` GitHub PR
  fetch path used by the backlog sync plugin (own `githubPR`/`githubCheckRun` structs).

**Verdict: Not recommended (new client).** Confirm and reuse what's there: root cause #1
("PR ready to merge, no notification path") should consume `PRStatusPoller`'s existing
`onUpdated` callback / `DerivePRPriority`==`PRPriorityReady` signal and turn it into a
durable, deduplicated notification, rather than adding a second GitHub client or
polling loop. ADR-022's own reasoning ("one polling loop, one ETag cache, one rate-limit
budget") applies directly to this feature too.

---

## 2. SaaS / managed API (Stale bot, Probot apps, hosted PR-reminder services)

**Findings**: GitHub's own Stale bot / Probot-based "stale PR" apps, and third-party
hosted reminder services, operate by posting comments/labels on GitHub or pinging Slack
— they have no concept of this tool's internal states (`in_progress`/`review`/rework
iteration count/headless triage sessions). They also require either a GitHub App
install with webhook delivery to a public endpoint, or a scheduled Action in the repo's
own CI. Given:

- This is a single-user, self-hosted internal tool — there is no team to notify via
  GitHub comments, and no appetite for exposing a webhook receiver or granting a
  third-party App installation access to a private repo for a purely internal
  reconciliation signal.
- Three of the four root causes (rework-cap parking, in-memory bookkeeping, cycling
  detection) are about *this tool's own* state machine, which no external stale-PR bot
  has any visibility into — a SaaS bot literally cannot solve them.
- Only root cause #1 (PR-ready-to-merge) overlaps with what a stale-bot-style service
  does, and even there the existing `PRStatusPoller` already fetches everything needed
  locally with zero external dependency.

**Verdict: Not recommended.** Adds an external dependency, a webhook surface, and/or a
GitHub App grant to solve at most 1 of 4 root causes, all of which are already covered
by in-repo polling. The requirements doc's own scope explicitly excludes touching
GitHub auto-merge/branch-protection settings, confirming the intent is to solve this
without GitHub-side automation.

---

## 3. LLM-generated implementation vs. battle-tested library (cycle-detection logic)

**Findings**: `sony/gobreaker` (circuit breaker) and backoff libraries solve a
different problem shape: they gate *retry attempts against a flaky external
dependency* based on a rolling failure/success window, then trip open/half-open/closed.
Root cause #4 is not about retrying a flaky call — it's about classifying a **finite,
already-durable sequence of status transitions** (`BacklogStatusEvent` rows) as
"converging" vs. "stuck in a loop." That's a much simpler shape:

- Count `in_progress → review → in_progress` transitions for an item within a lookback
  window (or since last "real" state change), compare against `maxAutoReworkIterations`
  (already 3, already enforced durably — see §4).
- No decay/half-open/backoff-timer state machine is needed since the count is not being
  used to *gate calls*, only to *raise a durable flag* once a threshold is crossed.

The repo already has `github.com/cenkalti/backoff/v5` in `go.mod`, but only as an
**indirect** (transitive) dependency — nothing in the codebase directly imports it
(confirmed via repo-wide grep). It is not "already adopted" for this kind of use.
`sony/gobreaker` is not a dependency at all.

**Verdict: Viable to hand-write, not recommended to adopt a circuit-breaker library.**
The counting/threshold logic is a straightforward query over the existing
`BacklogStatusEvent` log (see §4) — safe to write by hand, small enough to unit-test
exhaustively, and not a rediscovery of exponential backoff or circuit-breaker semantics.
Pulling in `gobreaker` would mean forcing a call-gating abstraction onto a
classify-and-flag problem it wasn't designed for.

---

## 4. Fork or adapt existing in-repo patterns

**Findings, mapped directly to the four root causes:**

| Root cause | Existing pattern to extend | File |
|---|---|---|
| #1 PR-ready, no notification | `PRStatusPoller` + `github.DerivePRPriority`/`IsTerminal` — already computes `PRPriorityReady`; poller already has an `onUpdated(*Instance)` hook for the event bus | `session/pr_status_poller.go`, `github/priority.go` |
| #2 Rework-cap ephemeral toast | `notifyReworkCapHit` (backlog_service_triage.go:29) already exists and fires at the right moment — it just calls `eventBus.Publish(...)` with no persistence. The **counting itself is already durable**: `workCount` is derived from `s.storage.ListItemSessions(ctx, item.ID)`, a real DB query, not an in-memory counter | `server/services/backlog_service_triage.go` lines 26–41, 406–425, 467–486 |
| #3 In-memory stuck bookkeeping | `BacklogStatusEvent` append-only transition log already persists every status change with `triggered_by`/`note`/`created_at`, cascade-deleted with the item, indexed by `(item_id, created_at)` | `session/ent/schema/backlog_status_event.go` |
| #4 Non-converging cycles | Same `BacklogStatusEvent` log gives the raw transition sequence to detect cycling; `maxAutoReworkIterations` (=3) is the existing, already-durable iteration cap to reuse as the "stuck" threshold rather than inventing a new one | `session/ent/schema/backlog_status_event.go`, `server/services/backlog_service_triage.go:55` |

The one **new** piece of durable state genuinely missing is a persisted notification /
stuck-flag record — there is no `Notification` ent schema at all (confirmed: no file
matches `*notif*` under `session/ent/schema/`); `events.NewNotificationEvent` +
`EventBus.Publish` (`pkg/events/bus.go`) is purely in-process pub/sub with no storage
layer. That is the actual gap, not the detection logic or the GitHub polling.

**Verdict: Recommended — fork/extend, do not build parallel infrastructure.** Concretely:
1. Add a small `stuck_since` / `stuck_reason` (enum: `pr_ready_unmerged`,
   `rework_cap_hit`, `non_converging_cycle`, `no_signal`) set of columns to
   `BacklogItem` (or a 1:1 `BacklogStuckState` table) — reuses the existing item.
2. Reuse `PRStatusPoller`'s `onUpdated` hook to detect `PRPriorityReady` persisting
   across N polls and write the stuck flag, instead of a new poller.
3. Reuse `notifyReworkCapHit`'s call sites (already at the exact two moments the cap is
   hit) to also persist the flag, not just publish an ephemeral event.
4. Add a query over `BacklogStatusEvent` for cycle detection, gated by the existing
   `maxAutoReworkIterations` constant (or a similarly-scoped new constant) rather than a
   circuit-breaker library.
5. New ConnectRPC read endpoint + React view lists items with a non-null stuck flag —
   this is the only genuinely new surface, and it is a straightforward read over data
   that steps 1–4 now persist.

---

## Summary of verdicts

| # | Question | Verdict |
|---|---|---|
| 1a | Dead-letter/stuck-state library | Not recommended — extend existing `ent` schema (`BacklogItem`/`BacklogStatusEvent`) |
| 1b | New GitHub client library | Not recommended — reuse existing `github/` package + `PRStatusPoller` |
| 2 | SaaS stale-PR / reminder bot | Not recommended — can't see internal state; existing local polling already covers the PR-side signal |
| 3 | Circuit-breaker/backoff library for cycle detection | Not recommended — hand-write a threshold query over `BacklogStatusEvent`; `gobreaker`/`backoff` solve a different (call-gating) problem shape |
| 4 | Fork/extend existing patterns | Recommended — `BacklogStatusEvent`, `PRStatusPoller`, `notifyReworkCapHit`, and `maxAutoReworkIterations` cover detection; the only new piece needed is a durable notification/stuck-flag record, which doesn't exist yet |
