# Pitfalls & Risks: pr-event-webhooks

Scope: wiring `check_run`/`workflow_run` (CI completion) and `pull_request_review`/
`issue_comment` (review feedback) GitHub webhook events into the existing
`PRFixSpawner.AutoReopenForPRFix`/`AutoReopenAfterFailedReview` reopen-and-fix loop.

This is a sibling of `project_plans/webhook-triggers/research/pitfalls.md`, which already
covers HMAC verification mechanics, delivery-ID dedup, replay attacks, secret storage,
payload-size DoS, and prompt-injection/SSRF for the base `push`-triggered-session-creation
receiver — **read that doc first**; it is not repeated here except where this feature's shape
changes the analysis. This doc is scoped to what's specific to reacting to CI/review events
and driving them into an *existing* fix loop that itself produces new commits/pushes/comments.

## 1. Feedback loops — the fix loop's own actions can re-trigger it (MUST-avoid)

This is the single most repo-specific risk of this feature, and nothing in the current
codebase guards against it today.

**The mechanism.** `AutoReopenForPRFix` (`server/services/backlog_service_triage.go:2018`)
spawns a work session that runs `/backlog/ship` → `/github:pr-ship`
(`session/backlog_lifecycle_pr.go:60-72`'s `agentShipPrompt` doc comment), which pushes new
commits, may resolve merge conflicts with its own commits, and reacts to CI. Every one of
those actions **is itself a GitHub event of exactly the type this feature listens for**:
- A push from the fix session → CI re-runs → new `check_run`/`workflow_run` `completed`
  events fire, for a PR this instance is still tracking (`pr_pending` once the fix session
  re-ships, or still `in_progress` while it's working).
- `/github:pr-ship`'s own automated PR comments (status updates, CI-failure summaries it
  posts) are `issue_comment` events on the same PR.
- A stapler-squad-authored review-response commit or comment can itself look like "review
  feedback" if a reviewer bot or a second stapler-squad instance is also watching the repo.

**Confirmed gap**: `grep -rn "sender\.\|IsBot\|\\[bot\\]\|actor\." server/services/*.go
server/workflows/*.go` (excluding tests and unrelated "event sender"/"frame sender" hits)
returns **zero** matches — there is no existing actor/sender/bot-authorship filtering
anywhere in the webhook or trigger-firing code today. `GitHubWebhookHandler.Handle`
(`server/services/github_webhook_handler.go:37-130`) only ever inspects
`repository.full_name` and `ref` (`extractGitHubRepoAndBranch`); it has never needed to look
at `sender`/`pusher` because the existing `push`→session-creation feature has no loop-back
path (a new session doesn't push to the branch a `github_push` Workflow watches). This
feature is the first one where the automation's own output lands back on the same input
channel — the precedent doesn't exist to copy.

**Standard mitigation (industry pattern, not yet implemented here):**
- Check the event's actor field and skip/no-op when it matches the automation's own
  identity: `sender.login`/`sender.type == "Bot"` for `issue_comment`/`pull_request_review`,
  and for `check_run`/`workflow_run` there is no direct "who pushed the commit that
  triggered this run" field on the event itself — that requires cross-referencing
  `check_run.head_sha`/`workflow_run.head_sha` against the commit's author (a separate
  lookup, either from the payload's embedded `commit` object where present, or a follow-up
  API call).
- Simpler and cheaper for this repo's actual shape: **CI completion events don't need actor
  filtering at all** — `check_run`/`workflow_run` `completed` with a `success` conclusion
  should never call `AutoReopenForPRFix` regardless of who pushed (only `failure`/
  `timed_out`/etc. conclusions are actionable), and a `pr_pending` item transitioning away
  from `pr_pending` (into `in_progress` via the fix session) means subsequent CI events for
  the *same* delivery cascade land on an item that's no longer `pr_pending` — `AutoReopenForPRFix`
  already rejects with `fmt.Errorf("item %s is not pr_pending (got %s)", ...)` at
  `backlog_service_triage.go:2027-2029` for exactly this shape, so the item-status check is
  a partial, already-existing backstop (see §2 — it's a CAS-protected backstop, not a filter
  by intent).
- The gap that check doesn't close: **`issue_comment`/`pull_request_review` events generated
  by `/github:pr-ship`'s own status-update comments**, arriving *while the item is still
  `pr_pending`* (before the fix session has re-shipped) or *after* it's back in `pr_pending`
  with a fresh PR-fix comment thread the tool itself posted — these would need explicit
  actor filtering (bot/service-account login check) since the status-check backstop doesn't
  apply. This needs a decision at plan time: either filter on a known bot login (fragile —
  depends on what identity `gh`/`/github:pr-ship` authenticates as) or filter on comment
  *content* markers (the tool could prefix its own comments with a recognizable marker,
  more robust but requires touching `/github:pr-ship`).

## 2. Duplicate/out-of-order delivery and concurrent reopen attempts

**Multiple fires per PR are expected, not an edge case.** A single CI run for a PR with N
jobs produces N `check_run` deliveries (`queued`, `in_progress`, `completed` — up to 3× per
job) plus 1+ `workflow_run` deliveries per workflow. A PR with a 5-job CI matrix can
plausibly emit 15+ webhook deliveries for one push. If each `completed`+`failure` delivery
independently calls `AutoReopenForPRFix`, the naive design fires the reopen path once per
failing job, not once per CI run.

**Does the existing active-session guard close this? Mostly yes, verified two ways:**
- **In-memory check (partial, not the real guard).** `findActiveWorkSession(sessions)`
  (`backlog_service_triage.go:2048`) is computed from a `ListItemSessions` snapshot taken at
  the top of `AutoReopenForPRFix` — if two webhook-driven calls race before either has
  spawned a session, both could observe "no active session" (a real TOCTOU window in the
  in-memory check alone).
- **The actual guard is a DB-level CAS, and it's proven under concurrency.** The status
  transition immediately after the active-session check
  (`s.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusInProgress,
  precondition, ...)`, `backlog_service_triage.go:2072`) passes a
  `BacklogItemPrecondition{ExpectedStatus: pr_pending, ExpectedUpdatedAt: &updatedAt}`.
  `session/ent_repository_backlog_transition_test.go`'s
  `TestTransitionBacklogItemStatus_should_letExactlyOneWinnerThrough_When_TwoWritersRaceConcurrently`
  and `server/services/backlog_service_lifecycle_test.go`'s
  `TestTransitionBacklogItemStatus_should_FailCASForLoser_When_ConcurrentOverrideRaces` are
  existing, passing regression tests proving this precondition is enforced as an atomic
  compare-and-swap at the DB layer, not just an application-level read-then-write. Two
  concurrent `AutoReopenForPRFix` calls for the same item **will** race past the in-memory
  active-session check together, but only one will win the CAS; the loser's
  `TransitionBacklogItemStatus` call returns an error (currently just propagated as
  `fmt.Errorf("transition to in_progress: %w", err)` — the webhook handler must treat this
  as a benign "someone else already reopened it" outcome, not retry or alert).
- **Net assessment**: the TOCTOU gap in the *in-memory* check is real but not exploitable
  into a double-reopen, because the DB CAS is the actual safety net. What it does **not**
  prevent: N webhook deliveries each independently doing the *work* before the CAS point —
  `ListItemSessions`, `tombstoneOrphanWorkSessions`, the rework-cap `ListItemSessions` scan
  — that's wasted DB read work per duplicate delivery, not a correctness bug, but worth
  short-circuiting cheaply (see next point).
- **Recommended additional guard, cheap and matches this feature's own event burst shape**:
  before calling `AutoReopenForPRFix`/`AutoReopenAfterFailedReview` at all, check the target
  item's *current* status is still `pr_pending`/`review` (a plain read, not a CAS) — this
  turns the common case (14 of 15 CI-job deliveries arriving after the first one already
  flipped the item to `in_progress`) into a cheap no-op read instead of a full guard-check
  + CAS-loss cycle, and reuses the delivery-ID dedup (`claimTriggerFireEvent`,
  `webhook_trigger_common.go:99`) already built for the `push` path so a GitHub-retried
  delivery (non-2xx response) doesn't re-attempt at all.

## 3. Payload trust / spoofing — fails closed today, confirmed by reading the code

`readAndDecodeWebhookBody` (`server/services/webhook_trigger_common.go:64`) itself does not
perform signature verification — that happens per-candidate in
`GitHubWebhookHandler.Handle`'s loop (`github_webhook_handler.go:95-109`). Traced the empty-
secret path end to end:
- `decryptWorkflowSecret` (`webhook_trigger_common.go:26`) returns an error when
  `wf.WebhookSecretEncrypted == ""` ("has no webhook secret configured") — that candidate
  `continue`s and never reaches `VerifyGitHubSignature`.
- `verifyHMACSHA256Signature` (`server/services/webhook_signature.go:38-40`) independently
  also returns `false` for `secret == ""` even if it were reached.
- Net: an empty/misconfigured secret makes `signatureVerifiedAny` stay `false` for every
  candidate, and the handler rejects the whole request with `401` (`github_webhook_handler.go:111-117`)
  — **fails closed**, matching the same "empty secret ⇒ every request 401s, buttons/hooks
  silently don't work until noticed" pattern `.claude/docs/slack-phase2-public-reachability.md`
  already documents for Slack Phase 2's `verifySlackSignature`. This repo has now established
  the same fail-closed idiom twice independently (Slack, GitHub push) — the new event types
  reuse the identical `VerifyGitHubSignature`/`decryptWorkflowSecret` pair, so this property
  carries over with no new code needed, but should be called out explicitly in the plan's
  test matrix (an empty-secret test case already likely exists for `push`; confirm it's not
  accidentally scoped only to that event type once `check_run`/`workflow_run`/
  `pull_request_review`/`issue_comment` share the same per-candidate verify loop).

## 4. Public reachability — identical treatment needed, broader blast radius than Slack's case

`.claude/docs/slack-phase2-public-reachability.md`'s core warning: **do not tunnel the whole
port** (`ngrok http 8543`) to reach one signed endpoint — front it with a path-scoped reverse
proxy (nginx `location = /path { proxy_pass ... } location / { return 404; }`) so only the
one HMAC-verified path is internet-reachable, and register the tunnel's public URL only for
that path.

`/webhooks/github` needs the **identical** treatment, and the requirements doc
(`project_plans/pr-event-webhooks/requirements.md:59-64`) already states this explicitly.
Confirmed by reading `server/server.go`'s route registration: `/webhooks/github` is one more
`http.ServeMux` route alongside every other `/api/hooks/*` local-only receiver, the
ConnectRPC session API, and the dashboard, all bound to the same `:8543` mux. A naive
`ngrok http 8543` to reach this one path exposes, at minimum:
- Every other `/api/hooks/*` receiver designed only for localhost trust
  (`/api/hooks/permission-request`, `/api/hooks/stop`, `/api/hooks/pre-tool-use`, per the
  Slack doc's own enumeration) — none HMAC-verified, all currently protected only by the
  "only Claude Code on this machine calls them" assumption.
- The full ConnectRPC session-management API (create/read/update/delete sessions, read
  scrollback, run commands in a session's tmux pane) — no separate internet-facing auth
  layer exists for it; it relies on the same localhost-only trust boundary.
- The React dashboard itself, unauthenticated.

This is a **strictly worse** blast radius than Slack's case: Slack's endpoint handles button
clicks with a narrow, already-scoped side effect (resolve one pending approval), whereas the
ConnectRPC API exposed alongside `/webhooks/github` on the same naive tunnel grants a remote
attacker full session/tmux control if they reach any other path. The plan should reuse the
exact nginx pattern from the Slack doc (`location = /webhooks/github`) and add it as its own
checklist item — the two endpoints can share one reverse-proxy config with two `location`
blocks if the operator wants both tunneled from the same box, but each path must be listed
explicitly (no wildcard `/api/hooks/` or `/webhooks/` prefix match that would also let a
future hook route through unreviewed).

## 5. GitHub API quota — resolving payload → item is a local DB lookup, but building fix context is not

**The PR→item lookup itself needs no GitHub API call.** Confirmed: `BacklogItemData` already
carries `PrNumber`/`PrURL` fields (`session/storage.go:830`, `session/backlog_lifecycle_pr.go:1242`,
used throughout `backlog_service_triage.go`) — this directly answers the open question in
`requirements.md:108-112` ("does matching require a new repo/PR-number lookup... BacklogItem
doesn't currently index by PR number — needs confirming"): it already does. Resolving
`check_run.check_suite.pull_requests[].number` (or `workflow_run.pull_requests[].number`,
`issue_comment`'s parent issue number when it's a PR) to a tracked item is a
`ListBacklogItems`-shaped local DB query filtered on `status == pr_pending` (or `review`) and
matching `PrNumber` — no GitHub call, no rate-limit cost.

**Building the `fixContext` string the reopened session gets IS API-shaped today, and that's
the part that could add load.** Traced the existing caller chain:
`BacklogLifecycleListener`'s `ReconcilePRPending` builds `fixCtx` from `prStatus.FeedbackText`
(`session/backlog_lifecycle_pr.go:1517`), and `prStatus` there originates from
`PRStatusPoller.fetchAndUpdatePRStatus`'s `github.GetPRInfoConditional(ctx, owner, repo,
prNumber, p.etagCache)` call (`session/pr_status_poller.go:347`) — a real GitHub API call,
ETag-cached so an unchanged PR costs zero quota on repeat polls. A webhook handler that wants
an equivalently rich `fixContext` (which checks failed, what the reviewer actually said,
formatted the same way `FeedbackText` is) has two options with very different quota
implications:
- **Reimplement/call the same PR-status-fetching path from the webhook handler** — this adds
  a *fresh, uncached* API call per webhook delivery (the whole point of a webhook is
  low-latency reaction, so an ETag from the poller's last 60s-old fetch is likely stale
  anyway) — a burst of `check_run` deliveries (one per job) each independently fetching full
  PR status would multiply calls by job count, on top of whatever the poller is still doing
  in the background (the requirements doc, correctly, keeps the poller running as a
  fallback — Goal 3 — so its usage doesn't go away).
- **Build `fixContext` from the webhook payload's own fields where sufficient** (a
  `check_run`'s `name`/`conclusion`/`output.summary`, a `pull_request_review`'s `body`/
  `state`, an `issue_comment`'s `body`) and only fall back to an API call when the payload
  genuinely lacks what's needed (e.g. `workflow_run` doesn't include per-job failure detail,
  only the run's overall conclusion — a `check_run` per job is the payload shape that
  carries job-level detail directly). This keeps the common case at zero extra API cost.
- **Debounce, don't fetch-per-event.** Since N `check_run` deliveries for one CI run share
  the same underlying event ("this PR's CI just finished"), the plan should collapse them —
  e.g. only act on `check_run`/`workflow_run` when `action == "completed"` for the run/suite
  as a whole (not per-job `in_progress`/`queued`, which fire but carry no actionable
  conclusion yet), which is already naturally most of the volume reduction needed without
  any explicit rate-limiting logic.
- This repo has no engineered rate-limit-quota tracking for GitHub API calls today (confirmed:
  `github.GetPRInfoConditional`/`github.CheckGHAuth` are used as-is with no calling budget or
  backoff beyond the poller's own interval and ETag cache) — a burst amplification here would
  surface as GitHub API 403s (secondary rate limit) with no existing alerting distinguishing
  "webhook burst exhausted quota" from any other API failure mode. Worth a debug-level log
  tag distinguishing webhook-triggered API calls from poller-triggered ones if any
  supplementary fetch is added, so this is diagnosable if it happens.

## 6. Silent failures — this repo's audit-trail pattern exists and must extend to the new event types

The base `push` receiver already establishes the right pattern via `TriggerFireEvent` rows
(`persistTriggerFireEvent`, `webhook_trigger_common.go:47`) — every outcome (`rejected`,
`no_match`, `fired_success`, `fired_failed`) is persisted, not just successes. This pattern
must extend cleanly to the new event types and their different failure shape:

- **"No relevant `pr_pending`/`review` item found for this PR number" is a `no_match`-shaped
  outcome, same as today's repo/branch non-match** — should persist a `TriggerFireEvent` (or
  equivalent) row rather than silently 200-and-drop, per this repo's own
  `feedback_document_ai_decisions_in_edge_cases` convention (self-heal/auto-close/no-op
  decisions need a visible trail) and the sibling pitfalls doc's §6 finding that "ignored"
  must leave a record, not just a fire.
- **`AutoReopenForPRFix`/`AutoReopenAfterFailedReview` returning an error must be visible
  at the HTTP-handler layer, not just logged and dropped.** Both functions already return
  `error` and their existing callers (`ReconcilePRPending`) check it
  (`session/backlog_lifecycle_pr.go:1415`, `:1430` — `log.ErrorLog().Printf(...AutoReopenForPRFix...)`)
  — the webhook handler's new call site must do the same: log the error AND persist a
  `fired_failed`-shaped `TriggerFireEvent` outcome, mirroring `renderAndFireTrigger`'s
  existing `fired_failed` path (`webhook_trigger_common.go:128-131`) rather than inventing a
  third, undocumented failure-handling shape for this one call site.
- **The `AutoReopenForPRFix` "is not pr_pending" rejection (§2, `backlog_service_triage.go:2027-2029`)
  is an expected, benign outcome for late-arriving duplicate CI events, not a real error** —
  the plan should classify it distinctly (e.g. `outcome: "no_match"` or a dedicated
  `"already_in_progress"`, not `"fired_failed"`) so an operator scanning fire-event history
  for real failures doesn't get paged by ordinary CI-job-fanout noise (§2's expected N-per-PR
  delivery volume).
- **`notifyRespawnBlockedByActiveSession`/`notifyReworkCapHit`** (already-existing machinery
  `AutoReopenForPRFix` calls internally, `backlog_service_triage.go:2049`, `:2062`) already
  produce a `MarkStuck` row + event-bus notification for their own skip conditions — these
  fire automatically once the webhook handler calls the existing function, so no new work is
  needed there; just don't duplicate or shadow this signal with a second, webhook-specific
  "blocked" notification that says something different.

## Summary of concrete design implications for the plan phase

1. CI-completion events: act only on `action == "completed"` with a failing conclusion;
   treat `queued`/`in_progress` and `success` conclusions as `no_match`, not fires.
2. Before calling `AutoReopenForPRFix`/`AutoReopenAfterFailedReview`, do a cheap status read
   (item still `pr_pending`/`review`?) to short-circuit the common CI-job-fanout duplicate
   case; rely on the existing DB-level CAS (§2) as the real correctness guard, not the
   in-memory active-session check alone.
3. Reuse `claimTriggerFireEvent`'s delivery-ID dedup for the new event types too (GitHub
   retries non-2xx responses the same way regardless of event type).
4. Explicitly decide and document the actor-filtering story for `issue_comment`/
   `pull_request_review` (§1) — this is new territory this repo hasn't needed before, unlike
   the fail-closed signature verification and DB CAS, which are already proven.
5. Prefer payload-native fields for `fixContext` over a fresh API fetch per event; only add a
   supplementary GitHub call where the payload genuinely lacks the needed detail, and tag its
   logs distinctly from the poller's own calls.
6. Path-scope any tunnel/reverse-proxy to `/webhooks/github` exactly as
   `.claude/docs/slack-phase2-public-reachability.md` prescribes — never the whole `:8543`
   port, which would also expose the ConnectRPC session API and every localhost-only
   `/api/hooks/*` receiver.
7. Persist a `TriggerFireEvent`-shaped audit row for every outcome, including "no tracked
   item for this PR" and "AutoReopenForPRFix returned an error" — classify the benign
   "not pr_pending anymore" rejection separately from genuine failures so it doesn't create
   audit noise proportional to CI job count.
