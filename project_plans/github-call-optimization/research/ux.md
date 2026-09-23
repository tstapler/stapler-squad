# UX Research: github-call-optimization

Agent 5 (UX), Phase 2. Scope: the thin user-facing surface of an otherwise
backend/infra project (rate limiting, caching, observability for GitHub API
calls). Explicitly out of scope: any new UI panel.

## 1. Existing UI surfaces (as they are today)

### What's actually wired up in `web-app/src/` for the five interactive RPCs

`server/services/github_service.go` defines `GetPRInfo`, `GetPRComments`,
`PostPRComment`, `MergePR`, `ClosePR` (proto: `session.proto:99-103`,
`SessionService`). Grepping the frontend for each RPC's camelCase client
method name (`getPRInfo`, `getPRComments`, `postPRComment`, `mergePR`,
`closePR`) turns up:

- **`GetPRComments`** — the only one with a live call site:
  `web-app/src/components/shared/vcs-widget/VcsWidgetComments.tsx:53`,
  a lazy fetch on first expand of a "Comments" section in the full-mode
  `VcsWidget`. Its error handling (`VcsWidgetComments.tsx:59-63`) is a bare
  `catch` → `console.error` + `loadState = "error"` → renders the string
  `"Failed to load comments"` (`VcsWidgetComments.tsx:108`). No distinction
  between rate-limited, network, auth, or any other failure — everything
  collapses to that one generic line, and the real error only exists in the
  browser console.
- **`MergePR`, `ClosePR`, `PostPRComment`, `GetPRInfo`** — **no frontend call
  site exists at all.** `web-app/src/lib/features/features/pr.ts` registers
  them in the feature registry (rpcIds `pr:merge`, `pr:close`,
  `pr:post-comment`, `pr:get-info`) with `componentPaths: []`, i.e. the
  registry itself records that no component invokes them yet. There is no
  merge button, close button, or comment-post form anywhere in
  `web-app/src/components/`. PR status shown in the UI (`GitHubBadge`,
  `SessionCard`, `VcsWidgetGithubRow`) is all populated from **session
  fields set by the background pollers**
  (`session/pr_status_poller.go`/`worktree_pr_poller.go` writing
  `session.githubPrState`, `.githubMergeable`, `.lastPrStatusCheck`, etc. —
  see `web-app/src/lib/vcs/adapters.ts:69-96`), not from a client-triggered
  `GetPRInfo` call.

  **Implication for this project**: the requirement's "user-initiated
  merge/comment/refresh/view got a raw error in the moment" pain point, as
  literally described, can currently only happen for `GetPRComments` (view)
  and indirectly for whatever RPC backs the "View Diff"/session refresh
  flow (below) — `MergePR`/`ClosePR`/`PostPRComment` aren't reachable from
  the UI today, so this project's priority-aware admission control mostly
  protects *future* UI or other API consumers (CLI, MCP tools —
  `mcp__stapler-squad__report_pr_created` etc. — worth confirming with
  Tyler whether those go through the same admission path), plus the
  `GetPRComments` path and the git-status refresh path that does exist.

### The generic-error-dump pattern that *does* exist today

`web-app/src/lib/hooks/useSessionVcs.ts:87-88` takes whatever string the
backend's VCS-status RPC returns as `response.error` and does
`setError(new Error(response.error))` — no classification, no mapping.
`VcsPanel.tsx:43-54` renders that `error.message` **verbatim** next to a
generic "Retry" button:

```tsx
<span className={styles.errorIcon}>⚠️</span>
<span>{error.message}</span>
<button className={styles.retryButton} onClick={handleRetry}>Retry</button>
```

This is the concrete instance of "raw API error dumped on the user" the
requirements describe — confirmed by reading the code, not inferred. If the
backend's admission-control layer surfaces a raw `"rate limited until
2026-09-08T15:23:00Z"`-style string here, it appears character-for-character
in this box today.

### Freshness signals that already exist (relevant to scope item 4)

`VcsWidget.tsx` already has three separate "how fresh is this" indicators
in `mode === "full"`:

- `Local: {relative-time}` from `data.statusAsOf` (git-level status)
- `PR status confirmed {relative-time}` from `data.github.lastCheckedAt`
  (`VcsWidget.tsx:91-95`) — this is exactly `session.lastPrStatusCheck`,
  i.e. the poller's last successful tick.
- `VcsWidgetBlockingReasons.tsx:9-24` — a **staleness notice** literally
  built for this project's problem: `STALE_THRESHOLD_MS = 3 * 60_000` (3×
  the poller's 60s cadence), with a comment reading "tolerates a couple of
  missed ticks before flagging staleness, so ordinary poll jitter doesn't
  flicker the notice." When stale, it renders "These reasons may be out of
  date — PR status hasn't refreshed recently" as an extra `<li>`.

This means scope item 4 (webhook-driven invalidation) doesn't need new UI —
it needs the *existing* `lastCheckedAt`/staleness-notice machinery to start
updating on webhook events instead of only on the 60s poll tick. The
"faster-feeling freshness" outcome is: the "PR status confirmed Xs ago"
timestamp gets smaller/fresher more often, and the stale notice (which
currently can be showing "may be out of date" for real, since a rate-limited
poller tick silently skips per the requirements' baseline) appears less
often. Nobody needs to be told this is now webhook-driven — it's the same
UI, doing its job better.

### A separate, unrelated "Rate Limited" indicator (don't confuse the two)

`SessionCard.tsx:421-450` and `SubStatusChip.tsx:125-135` render a "⏱ Rate
Limited" chip — but this is the **coding agent's own LLM-provider rate
limit** (`SubStatus.RATE_LIMITED`, `RateLimitState` enum, detected from PTY
output patterns), unrelated to GitHub API rate limiting. It's a useful
*pattern* precedent (small colored chip + `title` tooltip, not a full error
dump) but not a code path to reuse directly — it's a different subsystem
with a different enum.

### A precedent for exactly the classification this project needs

`web-app/src/lib/contexts/NotificationContext.tsx:35-48`,
`getFailureReasonToastMessage(failureReason: string)`, switches on a
`FailureReason` string union (`"GitHubResolutionError"`, `"StartupError"`,
`"Stale"`, `"Cancelled"`) set server-side and returns a friendly one-liner
for a toast, explicitly documented as "deliberately different wording" from
the persistent card copy (`lib/utils/sessionFailure.ts`'s
`getFailureMessage`) — transient toast vs. durable record are allowed to
diverge. **This is the template to copy** for GitHub rate-limit errors: add
a reason code (e.g. distinguishing "background-caused, will likely succeed
on retry" vs. "account-wide exhaustion") set by the backend admission-control
layer, and a parallel small switch function mapping it to plain language —
no new UI surface, no toast infrastructure to build, just a new `case`.

## 2. Comparable UX patterns (external tools)

- **`gh` CLI**: on rate limit, prints `API rate limit exceeded for
  <user>.` and, since a few versions back, the reset time in local
  human-readable form, not a raw epoch/UTC timestamp — the fix here isn't
  "add words," it's "give a number a human can act on" (a duration or local
  clock time, not a raw `Date.toISOString()`).
- **GitHub Desktop**: distinguishes "can't reach GitHub" (network/auth) from
  a soft "still working on it" spinner state; it doesn't expose rate-limit
  internals to the user at all in normal operation — GitHub's client apps
  generally treat rate limiting as something to absorb silently via
  backoff/retry, only surfacing it when a user-initiated action truly can't
  proceed.
- **VS Code GitHub Pull Requests extension**: shows a status-bar item with
  relative "PR list updated Xm ago"-style freshness and a manual refresh
  action; on failure it shows a dismissible notification with a **short**
  message + a "Retry" or "Show Details" action, never the raw HTTP body.
- **Common thread**: none of these tools distinguish *why* a call didn't go
  through beyond "transient, try again" vs. "you need to fix something"
  (auth) vs. "we already retried, still stuck." That maps directly onto this
  project's two-bucket ask in scope item 4/success-metric 1: (a) delayed
  by an internal, self-inflicted cause (background poller used the budget)
  vs. (b) genuinely account-wide exhausted.

## 3. User mental model — right level of transparency

Tyler is simultaneously the developer, operator, and sole user of this tool,
and per the requirements he **already knows** he shares one GitHub token
across machines/sessions/pollers. That changes the calculus versus a
multi-tenant product:

- Explaining "this failed because a background poller used your quota" is
  not over-explaining a hidden implementation detail to him the way it would
  be to an end user — he built the poller. But it's still not information
  he needs *in the moment he's trying to click merge*. The debugging-level
  detail belongs in logs (`~/.stapler-squad/logs/staplersquad.log`, which he
  already knows to check per `docs/how-to/debug-with-logs.md`), not inline
  UI copy.
- The minimum viable improvement over today's raw error is: **replace the
  literal `"rate limited until 2026-09-08T15:23:00Z"` string with a plain
  sentence that (a) states the actionable fact — when this will likely work
  — and (b) doesn't need him to parse a timestamp.** e.g. "GitHub is
  temporarily rate-limited — this should clear up in about 40s, retrying
  automatically" vs. "GitHub rate limit reached — try again later" for the
  account-wide case. No mention of "poller" or "background" needed in the
  UI copy itself; that's implementation detail the log line can carry.
- If Success Metric 1's *first* half is achieved (interactive actions
  auto-succeed via priority admission even under background load), the
  ideal UX is **no visible change at all** for the common case — the action
  just works, silently, which is a strictly better experience than any
  message. UI work is only needed for the residual case: genuinely
  account-wide exhaustion where the interactive action still can't proceed.

## 4. Error states and graceful degradation — should (a) and (b) look different?

Yes, and the distinction is exactly the one already modeled by
`getFailureReasonToastMessage`'s pattern — two buckets, not a spectrum:

- **(a) Delayed, self-inflicted (background exhaustion)** — with
  priority-aware admission control in place, this bucket should mostly
  disappear as a *user-visible* state (the interactive request preempts or
  waits a bounded, short amount behind background calls rather than being
  flatly rejected). If it's still visible at all (e.g. a merge click takes
  1-2s longer than instant), no error state is warranted — a normal loading
  spinner covers it. This is not a case that needs its own copy or badge.
- **(b) Genuinely account-wide exhausted** — this is the one case that
  still needs a real error surface, since success metric 1 explicitly
  allows "fail with a clearer, more actionable message" as the fallback.
  Concretely: reuse the existing error-rendering slot (`VcsPanel.tsx`'s
  `error` box, or wherever `MergePR`/`ClosePR` eventually get wired to a
  button) but swap the message source from "raw backend string" to "friendly
  copy keyed by a reason code," per §1's precedent. Include the reset time
  in relative terms ("try again in ~4 minutes") rather than an absolute
  timestamp — `formatRelativeTime` already exists and is used throughout
  this codebase (`VcsWidget.tsx`, `VcsWidgetComments.tsx`) for exactly this
  kind of formatting, so this is a call-site change, not new utility code.
- Concretely, do **not** build a third "ambiguous" state — the backend
  either knows it queued/deprioritized a background call to let this one
  through (success, no message) or it knows the whole token is spent
  (failure, clear message). Anything in between is over-engineering for a
  single-user tool.

## 5. Accessibility

Minimal, since no new UI panel is proposed:

- Any new/changed error copy in `VcsPanel.tsx`'s error box is already
  plain text inside a `div`, not inside an `aria-live` region — worth adding
  `role="status" aria-live="polite"` when this copy changes, matching the
  pattern `VcsWidget.tsx` already uses for its mergeability pill/blocking
  reasons (`styles.liveRegion`, `role="status" aria-live="polite"`,
  `VcsWidget.tsx:70-79`) so a screen reader announces the friendlier message
  without the user needing to refocus.
- If a future merge/close button is added and can be disabled or delayed by
  admission control, its disabled state needs an accessible name that
  explains why (`aria-disabled` + `title`/`aria-label`, mirroring
  `GitHubBadge.tsx`'s existing `aria-label` pattern at line 127) rather than
  just an inert-looking button — but this is scoped to *if* that button gets
  built, which is outside this project per §1.
- No color-only signaling: the existing rate-limit/priority chips already
  pair color with text/icon (`⏱ Rate Limited`, `✖ Error`), and any new
  reason-coded message should keep that pairing rather than relying on a
  red vs. yellow box alone.

## 6. Job-to-be-done

For Tyler specifically, the job is **"let me not have to think about
GitHub's rate limits at all, and when I occasionally do have to, tell me
something I can act on in one glance."** Evidence for "don't make me think
about it" over "let me understand the internals":

- He's asking for *admission control* (success metric 1's primary path is
  "interactive action should ideally succeed"), not a rate-limit dashboard
  — the requirements explicitly rule out an analytics-DB attribution
  dashboard.
- He already has the debugging-level tooling he'd reach for if he *did* want
  to understand the internals (structured JSON logs, `docs/how-to/debug-with-logs.md`'s
  pattern-clustering tool) — building a second, shallower version of that
  understanding into the UI would be redundant, not helpful.
- The emotional job is trust: a merge/comment action that "just works" most
  of the time, and on the rare account-wide-exhaustion failure gives a
  specific "try again in ~N minutes" instead of a wall-clock timestamp he
  has to do mental math on, is what keeps him from distrusting the tool
  during exactly the moment (shipping a PR) where trust matters most.

## Summary of UI-visible work implied by this research

This project's *own* UI-visible surface is small and mostly about **error
copy and freshness plumbing that already has scaffolding**, not new
components:

1. Add a reason-code (mirroring `FailureReason`/`getFailureReasonToastMessage`)
   for GitHub-call failures so `useSessionVcs.ts`'s `setError(new
   Error(response.error))` (and any future `MergePR`/`ClosePR`/`PostPRComment`
   error handling) can render friendly copy instead of the raw backend
   string, distinguishing at minimum "transient, retrying" vs. "account-wide,
   try again in ~N minutes."
2. No new components, badges, or panels — reuse `VcsPanel.tsx`'s existing
   error box, `VcsWidgetBlockingReasons.tsx`'s existing staleness notice, and
   `VcsWidget.tsx`'s existing `lastCheckedAt`/`statusAsOf` timestamps for
   the freshness half of the story.
3. `VcsWidgetComments.tsx`'s `"Failed to load comments"` should get the same
   reason-code treatment since it's the one RPC in this list with a real
   call site today.
4. Confirm with Tyler whether `MergePR`/`ClosePR`/`PostPRComment` having no
   frontend call site at all is expected/out-of-scope-for-now, since it
   changes how much of the "interactive action failed live" baseline pain
   point is currently reachable through the web UI versus only through
   other consumers (MCP tools, CLI).
