# Research: Build vs. Buy — quota-aware-backlog-gating

Date: 2026-08-10

Question: should account-wide Claude Code quota-headroom detection be built from
scratch, sourced from an existing OSS tool, sourced from a hosted Anthropic API, or
adapted from existing in-repo mechanisms?

## 1. Existing OSS library/tool (ccusage and family)

Searched for "ccusage", "claude-usage-monitor", "claude code usage tracker" (WebSearch,
2026-08-10). Found an active ecosystem of local usage analyzers, all parsing the same
source data Claude Code already writes to disk (`~/.claude/projects/**/*.jsonl`
transcript files):

- [`ccusage`](https://github.com/ccusage/ccusage) (npm, TypeScript/Bun) — the most
  mature/starred; `ccusage blocks` reports group usage into Claude's 5-hour billing
  windows with rate/projected-exhaustion columns. Its `--live` real-time monitor flag
  was **removed in v18.0.0** per its own docs — the maintainers pulled back from
  live/predictive monitoring.
- [`Claude-Code-Usage-Monitor`](https://github.com/Maciek-roboblog/Claude-Code-Usage-Monitor)
  (Python) — live terminal dashboard with ML-based burn-rate predictions for the
  5-hour window.
- Several smaller forks/variants (`claude-code-usage-tracker`, `aiusage`,
  `claude-usage-tracker`, etc.) — all do the same thing: parse local JSONL, apply
  token-count heuristics, estimate remaining budget against an *assumed* plan limit
  (Pro/Max 5x/Max 20x) since Anthropic does not publish the actual numeric caps.

**Pros**: mature parsing of the JSONL transcript format; some prior art on 5-hour
window bucketing logic worth reading for edge cases (UTC handling, gap detection).

**Cons**:
- All of them are inference/heuristic over the same local JSONL files this repo
  already has access to — none call an authoritative quota API (because none exists
  for Pro/Max/Team, confirmed in section 2). Adopting one doesn't buy a better signal,
  only someone else's heuristic.
- Node/Bun (ccusage) or Python (Usage-Monitor) runtime — stapler-squad ships as a
  single Go binary (see `.claude/docs/bundling-tmux.md` for the lengths already taken
  to avoid extra runtime deps). Shelling out to `npx ccusage` would add a Node
  dependency and a subprocess-plus-text-parsing boundary for a value stapler-squad can
  compute natively (see section 3/4 — it already parses this exact JSONL format in
  `session/tokens/parser.go`, consumed by `capacity_monitor.go`).
- License/maintenance churn risk for a single-user internal tool: not worth taking on
  an external dependency (and its update cadence) to reproduce logic already
  achievable in ~100 lines of Go reusing existing parsing.

**Verdict: Not recommended.** Useful as a reference for 5-hour-window bucketing
semantics, not as a dependency or subprocess to shell out to.

## 2. SaaS/managed API (hosted Anthropic endpoint)

Searched "Anthropic Claude Code usage API rate limit quota endpoint 2026" and
"claude code 5-hour session limit weekly quota check API" (WebSearch, 2026-08-10).

Findings:
- Anthropic's [Rate Limits API](https://platform.claude.com/docs/en/manage-claude/rate-limits-api)
  (`GET /v1/organizations/rate_limits`), launched April 25, 2026, returns
  organization-level Messages-API limits (RPM/TPM/TPD) — **not** the Pro/Max/Team
  session-based quota this feature needs. It's part of the Admin API and requires an
  Admin API key (`sk-ant-admin...`).
- Explicitly documented: **"Pro, Max, and Team subscribers can't read their limits
  from this API"** — the endpoint doesn't cover the consumer/subscription tier
  stapler-squad actually runs under.
- The only user-facing surface for the 5-hour/weekly session quota is the interactive
  `/usage` slash command inside the Claude Code CLI itself — a TUI output, not a
  machine-readable endpoint. Anthropic no longer publishes fixed prompts-per-window or
  hours-per-week figures at all (relative multipliers only: Max 5x/20x vs Pro).

**Pros**: none applicable — the API that exists doesn't cover this account's plan
tier or this quota type.

**Cons**: doesn't answer the actual question (session-wide Pro/Max quota headroom);
would require an Admin API key this single-user instance has no reason to provision;
wrong quota dimension entirely (org Messages-API limits vs. Claude Code session
quota).

**Verdict: Not recommended / not viable.** Confirms the requirements doc's own risk
assessment (`Feasibility Risks` in requirements.md) — no first-class quota API exists
for this use case as of 2026-08. The fallback increment described there (reactive
detection promoted to account-wide scope) is the realistic path, not a percentage-based
API-driven threshold.

## 3. Extend `session/detection/ratelimit/manager.go`

Read the file (347 lines). Pattern: per-session `Manager` wraps a `Detector` (regex/
pattern-matching over PTY output, not read in full here but referenced by
`ProcessOutput`), a `Scheduler` (schedules recovery input at the detected reset time),
and a `RecoveryHandler`, wired together with an internal pub/sub `EventBus`
(`eventDetected` / `eventRecoveryStart` / `eventRecoveryDone` / `eventRecoveryFail`).
External callbacks (`onDetectionCallback`, `onRecoveryCallback`) are the seam already
built for exactly this kind of "notify something outside this session" use case —
`session/detection/ratelimit/integration.go` presumably wires these to the server
event bus per-session today.

This is a solid foundation to extend rather than reimplement:
- The detection regex/pattern-matching (`Detector`) is already proven against real
  Claude Code rate-limit message text in production, per the requirements doc's own
  framing ("detects rate-limit messages after they appear... reactively").
- The event bus + external-callback seam is exactly the shape needed to fan
  per-session detection events *up* into an account-wide aggregator: subscribe an
  account-wide listener to every session's `Manager.SetDetectionCallback`/
  `SetRecoveryCallback`, maintain a rolling "any session hit a rate limit recently"
  state, and drive `BacklogController` off that aggregate rather than any single
  session's state.
- This is precisely the requirements doc's stated fallback increment: "any active
  session recently hit a rate limit" pauses backlog, promoted from per-session to
  account-wide scope — no new detection logic required, only a new aggregation layer
  above the existing per-session managers.

**Pros**: proven regex/detection logic reused as-is; existing callback seam avoids
new wiring into the PTY output pipeline; matches the requirements doc's own described
fallback scope; lowest implementation risk given no authoritative quota API exists
(confirmed in section 2).

**Cons**: still reactive — a rate-limit message has to actually appear in some
session's output before the aggregate state can react; no percentage-based/predictive
headroom signal (matches the requirements doc's accepted risk trade-off, not a new
gap introduced by this choice).

**Verdict: Recommended.** This is the primary signal source: aggregate existing
per-session `ratelimit.Manager` detection/recovery events into an account-wide rolling
state, subscribed via the existing callback seam.

## 4. Fork/adapt `capacity_monitor.go`'s polling+threshold+alert pattern

Read the file (390 lines). Pattern already present and directly reusable:
- `Start(ctx)` — ticker-driven poll loop (`config.CapacityConfig.PollIntervalSeconds`),
  matching the "piggyback on the existing reconcile ticker cadence" non-functional
  requirement in requirements.md.
- `checkThresholds()` — clean threshold-crossing check against configurable
  `*WarnPct`/`*AutoPct`-style config fields (`ContextWindowWarnPct`,
  `RateLimitWarnRemaining`, `CostBudgetUSD`), which is exactly the "configurable
  threshold(s) mirroring `CapacityConfig`'s pattern" scope item in requirements.md.
- `handleTransitionTrigger()` — de-duplicates repeat alerts (5-minute cooldown per
  session title) and publishes via `m.eventBus.Publish(events.NewNotificationEvent(...))`
  — this is the exact "visible notification, never silent" precedent the requirements
  doc cites as a constraint, ready to copy for the pause/resume transitions.
- `InstancePoller`/`SessionSwitcher` interfaces show the established pattern for
  giving a new monitor type read access to live session state without a circular
  import — a `BacklogQuotaMonitor` (or similar) would take an analogous
  `InstancePoller`-like interface plus the account-wide state from section 3, and gate
  `BacklogController.Enable()`/`Disable()` instead of switching a session's CLI.

This is not a literal fork (capacity_monitor.go's subject is per-session token/context
budget, explicitly out of scope per requirements.md) but its *shape* — ticker poll,
threshold check, cooldown-guarded notification, config-driven percentages — is the
right skeleton to copy for the new account-wide gate.

Also relevant to requirement 2 (foreground-session throttling): `session/backlog.go`
already defines `SessionRoleWork`/`SessionRoleReview`/`SessionRoleTriage` and
`IsTmuxBackedSessionRole()`, which distinguishes backlog-driven tmux sessions from
everything else. A foreground/human-driven session is implicitly "any active session
that is *not* backlog-owned" — this existing role distinction is the precise
definition the requirements doc's Rabbit Holes section asks to find rather than invent.

**Pros**: ticker/threshold/cooldown/notification skeleton is proven in this exact
codebase and satisfies the config-shape and notification constraints directly;
`SessionRoleWork`/`IsTmuxBackedSessionRole` already gives a foreground-vs-backlog
session distinction without inventing new state.

**Cons**: none significant — this is boilerplate reuse, not a functional dependency;
the new monitor is a sibling to `CapacityMonitor`, not a modification of it (keeps the
explicit out-of-scope boundary from requirements.md intact).

**Verdict: Recommended.** Copy the poll/threshold/cooldown/notify skeleton for the new
account-wide quota gate; reuse `SessionRoleWork`/`IsTmuxBackedSessionRole` for the
foreground-session distinction needed by requirement 2.

## Summary / Recommendation

Build natively on existing in-repo patterns — do not adopt an external OSS tool or
wait on a hosted API:

1. **Signal source**: aggregate `session/detection/ratelimit/manager.go`'s existing
   per-session detection/recovery events (via its callback seam) into a new
   account-wide rolling state. (Recommended — Option 3)
2. **Gate mechanism**: a new monitor modeled on `capacity_monitor.go`'s
   ticker+threshold+cooldown+notification skeleton, driving
   `BacklogController.Enable()`/`Disable()` instead of a session's CLI. (Recommended —
   Option 4)
3. **Foreground/background distinction**: reuse `session/backlog.go`'s
   `SessionRoleWork`/`IsTmuxBackedSessionRole()` rather than inventing new state.
4. **OSS tool (ccusage etc.)**: not recommended — same underlying JSONL data
   stapler-squad already parses natively, adds a Node/Python runtime dependency this
   Go single-binary project deliberately avoids, and their own maintainers (ccusage
   dropping `--live`) have pulled back from exactly the live-prediction use case this
   feature needs.
5. **Hosted Anthropic API**: not viable — the only quota API that exists
   (`/v1/organizations/rate_limits`) explicitly excludes Pro/Max/Team subscribers and
   measures a different quota dimension (org Messages-API limits, not Claude Code
   session quota). Confirms requirements.md's own risk assessment; the fallback
   increment (reactive signal promoted to account-wide scope) is the realistic
   Phase-3 design, not a percentage-threshold design built on a nonexistent API.

This is consistent with the requirements doc's own framing: no confirmed first-class
quota API exists, so the "fallback increment" (reactive detection promoted to
account-wide scope, hard pause/resume, no percentage threshold) is not a fallback —
it is the only feasible design given what's actually observable in 2026, and it maps
cleanly onto code that already exists and is already proven in production in this
repo.
