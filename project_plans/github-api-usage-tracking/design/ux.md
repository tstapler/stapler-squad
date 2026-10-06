# UX Design: github-api-usage-tracking

Phase 3 (design) output. Builds directly on `../requirements.md` and
`../research/ux.md` (house style, accessibility, mental model, and error-state
research — read that file first if anything below seems unmotivated) and
`../implementation/plan.md` (story-level acceptance criteria this design must
be consistent with; every string quoted verbatim below is copied from a plan
story so the design and the implementation contract cannot drift).

Sole user: Tyler, solo developer, reached for reactively after a rate-limit
WARN or a stalled poller (`../research/ux.md` §7). Design goal: answer "am I
about to hit a wall" in zero clicks, "what used it up" in one click (open the
source table — already visible, no interaction needed), "is this normal" via
the volume history.

---

## 1. Surfaces Inventory

| # | Surface | Type | Interactive? | Where designed |
|---|---|---|---|---|
| A | GitHub API Usage panel (quota tiles, volume history, source breakdown, reconciliation) | Web UI, `/analytics/github-api` | Yes — window selector, refresh, banners with actions | §2 |
| B | Warn-threshold editor (embedded in panel) | Web UI, same page | Yes — form input, validate, save | §3 |
| C | Poll-interval / retention / probe-interval config keys | `~/.stapler-squad/config.json` | No — hand-edited JSON, restart to apply | §4.1 |
| D | Structured log lines (WARN/INFO for thresholds, drops, reconciliation, token mismatch) | `journalctl --user -u stapler-squad` / log file | No — read-only CLI output | §4.2 |

Nav entry (`web-app/src/lib/nav-pages.ts`, "GitHub API Usage" under the
Insights group per plan Story 4.1.2) is not a separate surface — it is the
one-click discoverability path onto Surface A and is covered by an
acceptance criterion in §5, not its own wireframe.

---

## 2. Surface A — GitHub API Usage Panel

### 2.1 Wireframe

Follows `ApprovalAnalyticsPanel.tsx`'s shell exactly: `panel` → `titleRow` →
banners → `cards` grid → `tableSection` × 2 → reconciliation line → threshold
editor (Surface B, §3). No charting library; every bar is a `barTrack`/
`barFill` div pair with `aria-hidden="true"`, paired with sibling text.

```
┌────────────────────────────────────────────────────────────────────────────┐
│ GitHub API Usage              role="group" aria-label="Time window"        │
│                                 ( 1 )( 7*)( 14 )( 30 )( 90 )        (⟳)     │
│                                  aria-pressed per button      aria-label=   │
│                                                              "Refresh GitHub│
│                                                               API usage"    │
├────────────────────────────────────────────────────────────────────────────┤
│ [role="alert"] Usage tracking is unavailable — the analytics database      │
│  could not be opened. Counts below are not reliable.        <state 4 only> │
│                                                                              │
│ [role="alert"] `gh` CLI is authenticated as a different user — quota       │
│  figures below describe the native client's token only.  <mismatch only>  │
│                                                                              │
│ [note, neutral tone] 6 events were dropped (buffer full) — totals below    │
│  are a lower bound.                                        <if dropped>0  │
├────────────────────────────────────────────────────────────────────────────┤
│  cards grid — one QuotaTile per observed resource, independently scaled    │
│                                                                              │
│  ┌───────────────────┐  ┌───────────────────┐  ┌───────────────────┐      │
│  │ CORE               │  │ SEARCH             │  │ GRAPHQL            │    │
│  │ data-testid=        │  │ data-testid=       │  │ data-testid=       │    │
│  │  quota-tile-core     │  │  quota-tile-search  │  │  quota-tile-graphql│    │
│  │                     │  │                    │  │                    │    │
│  │ 4,200 / 5,000        │  │ 3 / 30              │  │        —           │    │
│  │ (84.0% remaining)    │  │ (10.0% remaining)   │  │  not yet observed   │    │
│  │ [███████░░░] ok      │  │ [█░░░░░░░░░] crit   │  │  [flat, no fill]    │    │
│  │  aria-hidden fill    │  │  aria-hidden fill   │  │                    │    │
│  │ warn below 500       │  │ warn below 3 of 30  │  │                    │    │
│  │ ≈12h to exhaustion ·  │  │ resets in 12m       │  │                    │    │
│  │  resets in 50m        │  │                    │  │                    │    │
│  │ as of 2m ago          │  │ as of 2m ago        │  │                    │    │
│  └───────────────────┘  └───────────────────┘  └───────────────────┘      │
├────────────────────────────────────────────────────────────────────────────┤
│  data-testid="exhaustion-events-stat"   data-testid="polling-paused-stat"  │
│  ┌────────────────────────────┐  ┌────────────────────────────────────┐  │
│  │ 0 rate-limit exhaustions     │  │ 0 polling pauses in the last 7 days │  │
│  │  in the last 7 days          │  │  (success tier)                     │  │
│  │  (success tier)              │  │                                      │  │
│  │  — vs. nonzero: "2 rate-limit│  │ — vs. nonzero: "3 polling pauses    │  │
│  │  exhaustions in the last 7   │  │  (≈16m total) in the last 7 days"   │  │
│  │  days" (critical tier)       │  │  (warning tier)                     │  │
│  └────────────────────────────┘  └────────────────────────────────────┘  │
│  Both stats always render, adjacent to each other, directly below the      │
│  quota tiles — never only discoverable via the source table or raw JSON.   │
├────────────────────────────────────────────────────────────────────────────┤
│  Request Volume — last 7 days                    per-resource toggle       │
│  ┌────────────────────────────────────────────────────────────────────┐   │
│  │ Date        Total    Bar (aria-hidden, local max = 340)             │   │
│  │ Aug 4        120     [████████████░░░░░░░░]                        │   │
│  │ Aug 5         96     [█████████░░░░░░░░░░░]                        │   │
│  │ Aug 6        340     [████████████████████]  ← this window's max    │   │
│  │ Aug 7         88     [████████░░░░░░░░░░░░]                        │   │
│  │ Aug 8         91     [████████░░░░░░░░░░░░]                        │   │
│  │ Aug 9         87     [████████░░░░░░░░░░░░]                        │   │
│  │ Aug 10        40     [████░░░░░░░░░░░░░░░░]                        │   │
│  └────────────────────────────────────────────────────────────────────┘   │
│  (windowDays=1 renders 24 hourly rows instead of 7 daily rows)             │
├────────────────────────────────────────────────────────────────────────────┤
│  Source Breakdown                                                           │
│  ┌────────────────────────────────────────────────────────────────────┐   │
│  │ Source                 Count    Share    Bar                        │   │
│  │ pr_status_poller         40     93.0%    [████████████████████]     │   │
│  │ gh_cli.merge_pr          ≈3      7.0%    [██░░░░░░░░░░░░░░░░░░]     │   │
│  │  title="one gh invocation may issue more than one API request"      │   │
│  │ worktree_pr_poller        0      0.0%    [░░░░░░░░░░░░░░░░░░░░]     │   │
│  │  ← zero row: poller exists, made 0 calls this window (not hidden)   │   │
│  └────────────────────────────────────────────────────────────────────┘   │
│  ≈ = approximate: gh CLI invocation count, not a physical-request count —  │
│  one invocation may issue more than one API request.        <persistent,  │
│  visible footnote text, only when ≥1 row uses "≈" — not tooltip-only>     │
├────────────────────────────────────────────────────────────────────────────┤
│  Reconciliation  (neutral tone unless residual > 20% of consumed quota)    │
│  40 requests this window were not attributed (gh CLI, another instance,    │
│  or another tool). See §2.4 for the framing rule.                          │
├────────────────────────────────────────────────────────────────────────────┤
│  Warn Threshold — see Surface B, §3                                        │
└────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Interaction Flow

The panel is a diagnostic tool reached reactively, not a dashboard checked on
a cadence (`../research/ux.md` §7) — the flow below optimizes for
fast time-to-answer from a cold open.

1. **Entry.** Tyler sees a rate-limit WARN in `journalctl` or notices a poller
   stalled. He opens the app, opens the drawer/More sheet, then clicks
   "GitHub API Usage" under Insights (2 clicks from anywhere in the app —
   `headerNav: false` per plan.md Story 4.3.1, not top-level nav).
2. **First paint, zero interaction required.** The panel defaults to
   `windowDays = 7` and fetches once on mount (`useGitHubApiUsage`, no
   client-side polling). The quota tiles (JTBD question 1: "am I about to hit
   a wall") render above the fold with no scrolling needed on a standard
   viewport, matching `ApprovalAnalyticsPanel`'s cards-before-tables order.
3. **Diagnose the "what used it up" question.** Tyler reads the Source
   Breakdown table directly below the tiles — no interaction needed, it's
   already rendered. He identifies the top consumer by count/share, aided by
   the `≈` marker on `gh` CLI rows warning him those are approximations. The
   explanation of what `≈` means is a persistent footnote below the table
   (see §2.1 wireframe), not something he has to hover a mouse to discover —
   this doc rejects hover-/tooltip-only affordances elsewhere (§3.2 step 3)
   for the same screen-reader/touch-accessibility reason, and the source
   table's own approximation marker must not contradict that.
4. **Diagnose the "is this normal" question (optional).** If the current
   window doesn't answer it, Tyler clicks a window button (`14`, `30`, `90`)
   in the `role="group"` selector. Exactly one new request fires
   (`useGitHubApiUsage`'s refetch-on-window-change contract); `aria-pressed`
   moves to the newly selected button; all three sections (tiles' burn-rate
   context, volume table, source table) re-render from the same response.
5. **Manual refresh.** Tyler clicks the icon-only refresh button
   (`aria-label="Refresh GitHub API usage"`) to force a re-fetch without
   changing the window — e.g. immediately after restarting the service with a
   new poll interval. The button disables itself while the request is in
   flight, matching `ApprovalAnalyticsPanel`'s existing refresh affordance.
6. **Exit.** No modal, no multi-step wizard — Tyler navigates away when done.
   There is nothing to "complete" on this page except optionally editing the
   warn threshold (Surface B).

### 2.3 Error / Edge-Case States

Five distinct, mutually-exclusive states apply to the quota tiles and/or the
panel as a whole — collapsing any two of these into one generic "no data"
message is the specific failure this design must avoid (`../research/ux.md`
§6, plan Story 4.3.1). A sixth, transient loading state precedes all five on
cold open. Two further rows below cover always-visible headline stats
(exhaustion-events, polling-paused) that are not states in this mutually-
exclusive sense — both render on every load, just with different tiers
depending on their value — but are included here because they are the
element this Story's states table exists to make traceable, and because they
share the "never collapse distinct signals into one message" discipline the
five states embody. Copy is quoted verbatim from the plan so implementation
cannot drift from this design.

| State | Trigger | What renders | Exit path |
|---|---|---|---|
| **Loading (cold open)** | Initial mount, before the first `useGitHubApiUsage` response arrives (`../research/ux.md`'s hook contract; matches `ApprovalAnalyticsPanel`'s existing loading treatment: `loading` CSS class, centered text, no skeleton/shimmer) | A loading placeholder in place of the tiles/tables — **"Loading GitHub API usage…"** — no partial/zero data rendered while the request is in flight, so Tyler never mistakes an unloaded panel for a genuine zero reading | None needed — self-resolves the instant the first response (success or error) arrives; transitions directly into whichever of the other states applies |
| **Never observed** | Fresh install, or a resource (e.g. `graphql`) this process has never queried and the probe hasn't reported yet | Tile shows **"—"** with sub-line **"not yet observed"** — never a fabricated "5,000 / 5,000 (100%)" | None needed — self-resolves once the probe's first tick or a real request populates the resource. Panel remains otherwise fully usable. |
| **No data in window** | `volumeBuckets` empty for the selected window (tracking is working, nothing happened) | Two-line empty block in the volume section: **"No GitHub API activity recorded in the last 7 days."** + hint **"Recorded automatically as pollers and RPCs make GitHub API calls — check back after your first poll cycle."** | Widen the window (click `30` or `90`) or come back later — both are one click away in the same view, no navigation required. |
| **Stale reading** | `observed_at` older than 3× the probe interval (~15 min default) | Tile keeps its last-known numbers but appends **"as of 47m ago"** and drops to a muted visual treatment (reduced-emphasis text/border, not the success/warning/error fill) | None needed — self-resolves on the next probe tick or request; the muted treatment plus timestamp is itself the warning, so no dead-end call to action is required. |
| **Tracking unavailable** | `tracking_available: false` (analytics DB failed to open) | `role="alert"` banner: **"Usage tracking is unavailable — the analytics database could not be opened. Counts below are not reliable."** Numeric sections render **disabled** (dimmed, non-interactive), not as zeros — zero would misleadingly imply "measured and confirmed empty." | The banner is informational only (no user-actionable fix from the panel — the analytics DB is a startup-time failure). The GitHub calls themselves are unaffected (backend degrades tracking, not functionality), so the exit path is simply "the rest of the app still works"; this is stated implicitly by every other page remaining reachable via nav — no trap. |
| **Fetch failed** | `useGitHubApiUsage`'s RPC call itself fails — timeout, 5xx, or a malformed response body (Story 4.1.1's `error` field is set). Distinct from "tracking unavailable," which is a **successful** response reporting a known backend limitation (`tracking_available: false`); a fetch failure means the panel has no trustworthy response at all. | `role="alert"` banner, matching `ApprovalAnalyticsPanel`'s existing error/retry convention exactly (no new pattern): **"Failed to load GitHub API usage: {error message}."** with an inline **Retry** button. If a prior successful load already populated the panel, that stale data remains visible beneath the banner rather than being cleared — mirrors Story 4.1.1's "errors surfaced without discarding the last good data" contract. | One click: **Retry** re-invokes the hook's `refresh()` — the same affordance as the manual refresh button in §2.2 step 5, so no new interaction pattern is introduced. |
| **Exhaustion-events stat (always visible, two tiers)** | `exhaustion_events` in the `GetGitHubAPIUsage` response for the selected window — this is the number the Success Metric is checked against (plan Story 4.3.1, adversarial-review.md Blocker 3) | `data-testid="exhaustion-events-stat"`, placed near the quota tiles (see §2.1 wireframe), never buried in the source table. At `exhaustion_events: 0`: **"0 rate-limit exhaustions in the last {N} days"** in the `success` tier. At `exhaustion_events: 2`: **"2 rate-limit exhaustions in the last {N} days"** in the `critical` tier (same critical color token used for the quota tiles' critical fill, per `.claude/rules/css-architecture.md` — no new ad hoc color) | None needed — always visible, no interaction required; the tier color plus the explicit count is itself the signal |
| **Polling-paused stat (always visible, two tiers)** | `pause_events` / `total_paused_seconds` in the same response — a separate signal from exhaustion count so a clean exhaustion number achieved by silent pausing is never mistaken for "problem solved" (plan Story 1.2.4, pre-mortem.md P1 #3) | `data-testid="polling-paused-stat"`, directly adjacent to the exhaustion-events stat (§2.1 wireframe). At `pause_events: 0`: **"0 polling pauses in the last {N} days"** in the `success` tier. At `pause_events: 3`, `total_paused_seconds: 990`: **"3 polling pauses (≈16m total) in the last {N} days"** in the `warning` tier — `warning`, not `critical`, because pausing is expected occasional behavior, distinguishing its visual weight from the exhaustion stat's critical tier at a nonzero reading | None needed — always visible; the tier color plus explicit count/duration is itself the signal |

Two additional banners/notes, not full states (they can co-occur with any of
the five mutually-exclusive states above):

- **Token identity mismatch** — `role="alert"` banner: **"`gh` CLI is
  authenticated as a different user — quota figures below describe the
  native client's token only."** Exit path: informational; Tyler's remedy
  (`gh auth login` / `gh auth switch`) is outside this panel's scope, so the
  banner states the limitation rather than pretending to fix it.
- **Dropped events** — neutral note (not alert-styled, per §2.4's principle
  of not over-alarming a known, bounded gap): **"6 events were dropped
  (buffer full) — totals below are a lower bound."** No action required; this
  is a qualifier on the numbers already shown, not a separate error to
  dismiss.

**No dead ends.** Every state above either self-resolves without user action
or leaves the rest of the panel/app fully navigable — there is no state that
traps Tyler on a page with no way forward (verified in §5's acceptance
criteria).

### 2.4 Presenting "Unaccounted Requests" — informative, not alarming

The reconciliation residual (`unaccounted_requests`) is a *known, expected*
consequence of this feature's own documented approximation gaps — the `gh`
CLI wrapper counts invocations, not physical requests; a second
`STAPLER_SQUAD_INSTANCE` has its own separate database; another tool may
share the token (plan §"Unresolved Questions", ADR-028). A residual of, say,
40 out of 400 consumed requests is the *system working as designed and
telling the truth about its own limits* — not a bug. The design must not
present it the way it presents an actual error.

Rules applied to the reconciliation line in the wireframe above:

1. **Default to neutral, not alert, styling.** The reconciliation line is
   plain body text in the panel's normal foreground color, not wrapped in the
   `error`/`warning` banner treatment used for the five states in §2.3. A
   number is not automatically bad.
2. **Escalate only past the backend's own tolerance line.** The plan's
   backend already defines the threshold that matters: a residual exceeding
   20% of consumed quota triggers `WARN "github usage: reconciliation
   residual exceeds tolerance"` (Story 5.1.1). The UI mirrors that exact
   threshold — below 20%, neutral text; at or above 20%, the line adopts the
   `warning` tier's color + text treatment (still paired with the numeric
   readout, never color-only). This keeps the UI's judgment of "is this a
   problem" identical to the backend's, rather than inventing a second,
   possibly inconsistent opinion.
3. **Always explain the "why" inline, not just the "what."** The copy names
   the known causes in the same sentence: **"40 requests this window were
   not attributed (gh CLI, another instance, or another tool)."** A bare "40
   unaccounted" with no explanation is what reads as alarming — a number with
   an unexplained shortfall implies the tracker is broken, whereas a number
   with a named, bounded cause reads as expected system behavior.
4. **Frame it as a trust signal for the other numbers, not a defect on its
   own.** Placed directly below the Source Breakdown (which it qualifies),
   not as its own top-level card competing with the quota tiles — it answers
   "how much can I trust the breakdown above," a supporting question, not the
   primary "am I about to hit a wall" question the tiles already answer.
5. **Never let it imply the tracked total is wrong** — it qualifies
   *completeness*, not *accuracy*: every tracked request is still real. Copy
   avoids words like "missing," "lost," or "error"; "not attributed" is
   accurate without being alarming.

This mirrors how the "dropped events" note in §2.3 is handled (neutral,
qualifying, non-alarming) — both are instances of the same underlying rule:
a known, bounded, self-reported gap in an otherwise-trustworthy dataset
should read as the tool being honest about its edges, not as the tool
failing.

---

## 3. Surface B — Warn-Threshold Editor

Embedded at the bottom of the same panel (Surface A), but treated separately
here because it has its own distinct interaction contract: view → edit →
validate → save → restart-required confirmation. This is the feature's only
form input and its only write path from the UI (poll intervals, retention,
and probe interval are config-file-only — see §4.1).

### 3.1 Wireframe

```
┌────────────────────────────────────────────────┐
│  Warn Threshold                                  │
│                                                    │
│  <label htmlFor="warn-threshold-input">           │
│    Warn when remaining drops below (% of limit)   │
│  </label>                                         │
│  [ 25 ]%   [Save]                                 │
│   id="warn-threshold-input" type=number            │
│                                                    │
│  ── success path ──                               │
│  "Saved. Restart the service to apply."            │
│                                                    │
│  ── validation-error path (value = 150, blurred) ──│
│  [ 150 ]%   [Save, disabled]                       │
│  [role="alert"] "Enter a value between 1 and 90"    │
└────────────────────────────────────────────────┘
```

### 3.2 Interaction Flow

1. Tyler clicks/tabs into the labeled numeric input (current value
   pre-filled from config, e.g. `10` for the default).
2. He types a new value (e.g. `25`) and either blurs the field or clicks
   Save.
3. **Client-side validation runs on blur/change**, not only on submit —
   clamped range is `[1, 90]`. Out-of-range input disables the Save button
   immediately and surfaces `role="alert"` text: **"Enter a value between 1
   and 90."** This is a text node, not a browser-native validation bubble
   (`../research/ux.md` §5 — native bubbles aren't reliably announced by
   screen readers across browsers).
4. With a valid value, Tyler clicks Save. The value persists through the
   dedicated `UpdateGitHubUsageConfig` RPC (plan Story 3.2.3) — this feature's
   own write path, following the same bespoke-RPC-per-settings-surface shape
   as `DefaultsService.UpdateGlobalDefaults` and
   `UnfinishedWorkService.UpdateUnfinishedWorkConfig`. No generic config-update
   RPC exists in this codebase to reuse (confirmed by Story 3.2.3's own
   research: grepping `proto/session/v1/session.proto` for a generic
   `rpc Update...Config` finds none), so this field gets its own RPC rather
   than being routed through a shared one.
5. **The confirmation states the restart requirement explicitly**:
   **"Saved. Restart the service to apply."** — never "Applied" or
   "Updated," because no hot-reload exists in `config/` (`../research/ux.md`
   §6, plan Story 4.3.2). This is the single most important copy decision in
   this surface: saying "Applied" here would cause Tyler to believe he has
   throttled a poller he has not yet throttled, defeating the entire point of
   the feature during exactly the high-stress moment (an active rate-limit
   scare) when he's most likely to be editing this value.
6. Total steps to change the threshold: **focus → type → save = 3 steps**,
   matching the acceptance criterion in §5.

### 3.3 Error / Edge-Case States

| Case | What renders | Exit path |
|---|---|---|
| Out-of-range value (e.g. `0`, `91`, `-5`, non-numeric) | `role="alert"` text **"Enter a value between 1 and 90"** near the field; Save button disabled | Correct the value in the same field — no navigation, no dialog to dismiss |
| Save RPC fails (network/server error) | Inline error near the Save button (house `error`/`role="alert"` convention), original value retained in the field so nothing is lost | Retry by clicking Save again; the field still holds the value Tyler typed, so no re-entry is required |
| Save succeeds | **"Saved. Restart the service to apply."** confirmation, field keeps the new value | None needed — informational; the rest of the panel remains usable while a restart is pending |

---

## 4. Non-Interactive Surfaces (condensed)

### 4.1 Config file — poll intervals, warn threshold, retention, probe interval

Five new optional keys in `~/.stapler-squad/config.json`, all `omitempty`,
all defaulting to today's hardcoded values (plan Story 3.3.1):

```json
{
  "github_usage_retention_days": 30,
  "github_rate_limit_warn_percent": 10,
  "pr_status_poll_interval_seconds": 300,
  "worktree_pr_poll_interval_seconds": 60,
  "github_rate_limit_probe_interval_seconds": 300
}
```

Acceptance criteria:

- An untouched (or absent) `config.json` produces byte-identical poller/
  threshold behavior to today — no key is required.
- Any value outside its clamp range (`intervals [10s, 1h]`, `threshold
  [1, 90]`, `retention [0, 365]`, `probe [60s, 6h]`) is silently clamped, and
  the clamp is logged once at WARN with the effective value used — the file
  is never rejected outright for one bad key.
- A changed value takes effect only after a full service restart (`make
  install-service` or `systemctl --user restart stapler-squad`) — there is no
  file-watcher or hot-reload path, matching the UI's own "restart required"
  copy in §3.2 so the two surfaces never contradict each other.
- `github_usage_retention_days: 0` explicitly disables the category-scoped
  prune (falls back to the global age/count retention only) — `0` means
  "disabled," not "prune immediately."
- Editing this file requires no schema migration and is fully reversible —
  removing a key reverts to the default on next restart.

### 4.2 Structured log output

Representative lines (all extend existing log lines per the plan's
Observability Plan — no new log format introduced):

```
WARN  github API: rate limit running low   remaining=450 limit=5000 call_site=pr_status_poller
WARN  github usage: reconciliation residual exceeds tolerance   probe_used=4600 tracked=4560 unaccounted=40 resource=core
WARN  github usage recorder buffer full — dropping event   dropped=7
WARN  github usage: gh CLI token identity differs from native client   gh_login=tstapler native_login=TylerStaplerAtFanatics
INFO  github usage recorder started   buffer_size=2048
```

Acceptance criteria:

- The three pre-existing WARN lines (`rate limit running low`, `secondary
  rate limit hit`, `primary rate limit exhausted`) keep their exact message
  text and existing fields — only a new `call_site` field is added — so any
  existing log-scraping/alerting a user may have built continues to match.
- Every new WARN/INFO line names the specific quantity it's reporting
  (`dropped`, `unaccounted`, `probe_used`/`tracked`) rather than a bare
  "something went wrong" — consistent with this repo's evidence-and-claims
  discipline, a log line alone should let Tyler diagnose without opening the
  UI.
- No log line requires cross-referencing another line to be meaningful in
  isolation (each carries `call_site`/`resource` context inline).
- `journalctl --user -u stapler-squad -f` (per `.claude/rules/
  systemd-user-service.md`) is sufficient to watch these in real time with no
  additional flags or filters required.
- These lines are a *secondary* channel — the panel (Surface A) is the
  primary, structured way to answer the same questions; the log is for
  the moment before Tyler thinks to open a browser tab.

---

## 5. UX Acceptance Criteria

Testable by a human, organized by concern.

**Task completion**
1. From any page in the app, Tyler can reach the quota tiles in **≤ 2
   clicks**: open nav → click "GitHub API Usage."
2. The "am I about to hit a wall" question (current quota per resource) is
   answered in **0 additional clicks** after the panel loads — tiles render
   above the fold with the default 7-day window, no interaction required.
3. Tyler can change the observation window in **1 click** (window selector
   button) and see all three sections (tiles' burn-rate sub-line, volume
   table, source table) update from a single re-fetch.
4. Tyler can change the warn threshold in **3 steps**: focus field → type
   value → click Save (§3.2).
5. Tyler can force a refresh without changing the window in **1 click**
   (refresh button).

**Error handling / no dead ends**
6. Each of the five states in §2.3 (never-observed, no-data-in-window,
   stale, tracking-unavailable, fetch-failed) renders **visibly distinct
   copy** — a manual tester switching between them (e.g. via mocked RPC
   responses in the Playwright spec, plan Story 5.2.2) must be able to tell
   which state is active from the text alone, with no two states sharing a
   message.
7. The tracking-unavailable banner reads exactly **"Usage tracking is
   unavailable — the analytics database could not be opened. Counts below
   are not reliable."** and numeric sections are visibly disabled (not
   rendered as `0`).
8. The threshold editor's validation error reads exactly **"Enter a value
   between 1 and 90"** and appears within the same view as the input — no
   navigation away from the field is required to see or fix it.
9. **No dead ends**: every error/banner/empty state above either
   self-resolves without user action (never-observed, stale, dropped-events,
   reconciliation) or leaves the rest of the panel and the app's nav fully
   reachable (tracking-unavailable, token-mismatch, fetch-failed — the Retry
   button and the app's nav both stay reachable while the error banner is
   shown) — a manual tester must be able to navigate away from every state
   without being blocked by a modal or a non-dismissable overlay (none
   exists in this design; confirm none is introduced during implementation).
10. A failed threshold-save retains the typed value in the field — Tyler
    never has to retype it to retry.

**Accessibility**
11. **Keyboard-only navigation**: every interactive element (window buttons,
    refresh button, threshold input, Save button) is reachable via Tab in a
    logical order and operable via Enter/Space — no custom widget requiring
    arrow-key handling is introduced (the window selector is native
    `<button>` elements in a `role="group"`, per house convention).
12. **Screen-reader labels present**: the window selector group has
    `aria-label="Time window"`; each window button has `aria-pressed`
    reflecting selection state; the refresh button has
    `aria-label="Refresh GitHub API usage"`; the threshold input has a
    visible `<label>` associated via `htmlFor`/`id` (not placeholder-only);
    every `barFill` decorative div has `aria-hidden="true"` with the
    equivalent value present as sibling text. The `exhaustion-events-stat`
    and `polling-paused-stat` elements are plain text nodes (not icon-only or
    color-swatch-only), so they need no separate `aria-label` — the full
    sentence ("0 rate-limit exhaustions in the last 7 days", "3 polling
    pauses (≈16m total) in the last 7 days") is already the accessible name a
    screen reader announces, identical to what a sighted user reads.
13. **Color contrast ≥ 4.5:1**: verified by construction — this design
    introduces no new color pairing; every state (success/warning/error/
    critical tiers) reuses `vars.color.success|warning|error|critical` +
    their `*Text` variants, which `web-app/src/styles/theme.css.ts` already
    documents as WCAG-AA-checked (`../research/ux.md` §2). A manual spot
    check with a contrast checker against the rendered tiles in each of the
    6 supported themes is still required before ship, since a token pairing
    correct in one theme is not automatically correct in all six.
14. **Color is never the sole signal**: every gauge, tile, and banner pairs
    its color with visible text conveying the same information (e.g. "3 / 30
    remaining (10.0%)" alongside the red fill) — verified per-component in
    plan Story 4.2.1's own acceptance criteria, restated here as a
    cross-cutting design invariant. This applies to the exhaustion-events and
    polling-paused stats too: the tier color (success/warning/critical) is
    never the only way to tell 0 from nonzero — the explicit count (and, for
    pauses, the humanized duration) is always present as text in the same
    element, so the distinction survives grayscale/high-contrast rendering
    and screen-reader use.
15. Changing the time-window selection is announced to assistive tech via
    the focused button's `aria-pressed` state change (already-established
    pattern); no additional `aria-live` region is required for this feature
    unless a future accessibility review finds it insufficient in practice
    (matching `../research/ux.md` §5's explicit "note as an open item rather
    than over-engineering ahead of an actual finding").

**Resource-separation invariant** (the one finding `../research/ux.md`
flagged as a hard requirement, not a preference)
16. **No element anywhere on the panel displays a blended quota number
    across resources** — core (5,000/hr) and search (30/hr) are always two
    independently-labeled, independently-scaled tiles/rows; a manual tester
    must not be able to find any percentage, count, or gauge that sums or
    averages across `GitHubResource` values. This applies to the quota
    tiles, the volume-over-time bars (scaled to each resource's own local
    max, never a shared scale), and the source breakdown (attributed by
    call site, not by resource, so it does not blend resources either).
17. A low-limit resource (search, 30/hr) always shows its warn threshold and
    current state in **absolute counts alongside the percentage** (e.g.
    "warn below 3 of 30," "3 / 30 remaining (10.0%)") — a percentage-only
    reading is never the only figure shown for a resource whose 10% is a
    single-digit number of requests.

---

## 5a. Mobile / Responsive Considerations

This project's standing practice is to consider both mobile and desktop form
factors for every frontend feature (touch targets, responsive layout, mobile
keyboard behavior). This panel is reached reactively (§2.2) — including from
a phone if Tyler is away from his desk when a rate-limit WARN fires — so it
is in scope even though the primary use case is desktop.

- **Touch targets.** The time-window selector buttons (`1`/`7`/`14`/`30`/`90`)
  and the warn-threshold editor's Save/Cancel-equivalent actions (Save; the
  implicit "cancel" is simply navigating away, there is no explicit Cancel
  button per §3) must each have a minimum hit area of 44×44px (WCAG 2.5.5 /
  iOS HIG), even though their visual label is a short number or word — pad
  the button box, not just the text, consistent with existing house
  convention for icon-only buttons like the refresh control.
- **Layout reflow.** `ApprovalAnalyticsPanel.css.ts` (the shell this panel's
  wireframe follows exactly, per §2.1) already establishes the two patterns
  to reuse rather than reinvent:
  - The quota-tiles grid (`cards`) uses
    `gridTemplateColumns: "repeat(auto-fill, minmax(130px, 1fr))"` — this
    reflows to fewer columns automatically as the viewport narrows, and to a
    single column on a phone-width screen, with no `@media` query needed.
    The exhaustion-events/polling-paused stat pair (§2.1, §2.3) should follow
    the same auto-fill/wrap pattern so they stack vertically rather than
    overflowing or shrinking to unreadable text on a narrow viewport.
  - The volume-history and source-breakdown tables use a `tableWrapper` with
    `overflowX: "auto"` — on a narrow viewport the table keeps its columns
    and becomes horizontally scrollable rather than reflowing into a
    stacked/card layout. This panel should reuse that same `tableWrapper`
    pattern for its own volume/source tables rather than introducing a new
    mobile-specific table layout, so behavior stays consistent with the rest
    of the app's analytics surfaces.
- **Mobile keyboard.** The warn-threshold input is `type="number"` (§3.1),
  which already triggers the numeric keypad on mobile browsers with no
  additional attribute needed; no other field in this feature accepts
  freeform text input.

---

## 6. Traceability to Plan Stories

| Design section | Plan story/task |
|---|---|
| §2.1–2.2 panel shell, tiles, flow | Story 4.2.1 |
| §2.1 volume + source sections | Story 4.2.2 |
| §2.2 window selector + refresh | Story 4.2.3 |
| §2.3 five states (incl. fetch-failed) | Story 4.3.1 |
| §2.1 wireframe + §2.3 exhaustion-events/polling-paused stats | Story 4.3.1, Tasks 4.3.1a/4.3.1b |
| §2.3 token mismatch banner | Story 5.1.2 |
| §2.4 reconciliation framing | Story 5.1.1 |
| §3 threshold editor | Story 4.3.2 |
| §4.1 config keys | Story 3.3.1 |
| §4.2 log lines | Observability Plan (plan.md, top-level) |

This document is the UX contract Phase 4 implementation and Phase 6
verification should be checked against; a divergence between the shipped
panel's copy/behavior and the strings quoted here should be treated as a
regression against this design, not as an acceptable implementation detail.

---

## 7. Engineering Risk Acknowledgment (Accepted, Not Fixed Here)

The Phase-4 Product Triad Review's Engineering leg passed "ready" but flagged
that three pre-existing open bugs live in files this feature edits or
depends on:

- `docs/bugs/open/BUG-021-check-gh-auth-mutex-contention.md` — `github`'s
  auth-check path holds a mutex during an external auth check. Story 5.1.2
  (token-identity parity check) adds a second `gh api user`-adjacent
  auth-check call site in the same area of the package.
- `docs/bugs/open/BUG-022-etag-cache-rwmutex-map.md` — `github/etag_cache.go`'s
  `ETagCache` uses `sync.RWMutex` over a plain map. Story 1.2.1's
  `usageTransport` wraps every request through `ghHTTPClient`, including the
  conditional-request path `ETagCache` participates in.
- `docs/bugs/open/BUG-023-pr-status-poller-mutex-churn.md` — `session/pr_status_poller.go`'s
  single `sync.Mutex` serializes instance-list, auth-state, and poll-result
  access. Task 3.3.1b changes how `PRStatusPoller` is constructed
  (`NewPRStatusPollerWithConfig`), and Story 1.1.2's context-carried
  attribution touches the same poller's request path.

**This is accepted as tracked risk, not fixed as part of this feature.** All
three are pre-existing performance/lock-contention issues (not correctness
bugs) in files this feature only touches incidentally — none blocks this
feature's own acceptance criteria, and re-architecting their concurrency
primitives is out of this feature's appetite. Per this repo's
`.claude/rules/fix-flaky-tests-dont-defer.md`-style discipline of not
silently re-excusing a known issue in code you're about to touch: before
starting implementation on Story 1.2.1 (`github/http_client.go` /
`usage_transport.go`), Task 3.3.1b (`session/pr_status_poller.go`
construction), or Story 5.1.2 (the auth-check path), check whether active
fix work on BUG-021/022/023 is already in flight (e.g.
`git log --all --oneline --grep='BUG-02[123]'` or an open PR touching the
same files) to avoid a merge collision — a concurrent rewrite of the exact
mutex/map this feature edits is a foreseeable conflict, not a hypothetical
one.
