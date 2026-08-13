# ADR-001: Quota-Headroom Signal Source — Combined Percentage-Heuristic + Reactive-Override

**Date**: 2026-08-10
**Status**: Accepted
**Project**: quota-aware-backlog-gating

## Context

Requirement: automatically pause/resume backlog automation (`BacklogController.IsEnabled()`)
based on remaining Claude Code account-wide session quota headroom. No first-class quota API
exists for Pro/Max/Team plans (confirmed in `research/build-vs-buy.md` §2 —
`/v1/organizations/rate_limits` explicitly excludes these tiers; `/usage` is an interactive-only
REPL command; `research/stack.md` §3 cites the open upstream issue
[anthropics/claude-code#40395](https://github.com/anthropics/claude-code/issues/40395) confirming
no CLI/API surface exists).

Two grounded, in-repo-only signal sources were identified by research, and they point in
different directions:

1. **`research/stack.md` §3–4**: `session/tokens.TokenStore` (already constructed at
   `server/dependencies.go:1152-1154`, already parsing every session's JSONL transcript under
   `~/.claude/projects/`) can be re-aggregated into a 5-hour rolling-window "usage volume"
   estimate — the same computation the OSS tool `ccusage blocks` performs. This would satisfy the
   requirements doc's explicit ask for a "configurable percentage threshold... mirroring
   `CapacityConfig`'s `WarnPct`/`AutoPct` pattern" directly, and it is *proactive* — it can warn
   before any session actually gets rate-limited.
2. **`research/build-vs-buy.md` §3, `research/pitfalls.md` §1, `research/architecture.md` §3–4**:
   aggregating the existing per-session `session/detection/ratelimit` reactive detection events
   (fanned in today at `server/services/session_service.go:4099`'s `wireRateLimitCallbacks`) into
   an account-wide "any session recently rate-limited" binary signal. Zero new heuristic risk —
   it's ground truth, not inference — but *reactive*: it can only prevent a second session from
   also getting hit, never the first (stated explicitly as an accepted limitation in
   `pitfalls.md` §1).

The key gap common to both: **Anthropic does not publish the actual token/message cap per plan
tier.** `TokenStore` gives usage *volume*, never the *budget it's a fraction of*. Any
percentage-based "headroom remaining" is therefore necessarily a heuristic estimate against a
user-supplied assumption, not an authoritative fraction — this must never be presented to the
user as precise.

## Decision

**Combine both signals, with distinct roles, exactly as suggested by the planning brief's own
framing**: percentage headroom as a configurable, hysteresis-gated *warn/soft-pause* tier;
reactive rate-limit aggregation as an *unconditional, hysteresis-free hard override*.

- **Soft signal (primary, proactive)**: `computeHeadroom()` buckets `TokenStore.GetAll()`'s
  `ParseResult.TurnTimeline` into `[now-5h, now]`, sums tokens, and compares against
  `config.QuotaConfig.AssumedWindowTokenBudget` (user-tunable; **default `0`, a sentinel meaning
  "disabled/not yet calibrated"** — see Consequences). When enabled and headroom drops below
  `PauseBelowHeadroomPct` for `ConsecutiveTicksToPause` consecutive reconcile ticks, `QuotaGate`
  calls `BacklogController.Disable()`. Resume requires headroom to stay above
  `PauseBelowHeadroomPct + ResumeMarginPct` for `ConsecutiveTicksToResume` ticks (Schmitt-trigger
  hysteresis, precedent: `session/health.go:136-152`'s `recoveryDebounced`).
- **Hard signal (override, reactive, always-on)**: if any session's rate-limit detector fired
  within the last `RateLimitWindowMinutes` (default 30), `QuotaGate` calls `Disable()`
  immediately — no consecutive-tick requirement, since this is ground truth, not inference.

Rationale for combining rather than picking one:

- If the user's `AssumedWindowTokenBudget` assumption is wrong (too generous, or never
  configured), the soft signal alone would fail to protect — the hard signal still catches actual
  quota exhaustion once it's observed, so miscalibration degrades to "reactive-only," never to
  "no protection at all."
- If the reactive detector's regex misses a wording variant (a real risk per `pitfalls.md` §1),
  the soft signal still provides proactive coverage independent of that failure mode.
- Neither alone satisfies both the letter of the requirements doc (a configurable percentage
  threshold, scope item 2) and the risk-averse research consensus
  (`build-vs-buy.md`/`pitfalls.md`/`architecture.md`'s convergent recommendation to lead with the
  reactive signal as lowest-risk). Combining satisfies both without contradicting either.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Pure percentage heuristic only (Option A) | No ground truth to fall back on if the assumed budget is wrong; three independent research files (build-vs-buy, pitfalls, architecture) converge on this being the higher-risk choice standalone |
| Pure reactive aggregation only (Option B, requirements.md's own "Fallback Increment") | By construction cannot prevent the *first* rate-limit hit in a session; discards the genuinely-available (if imprecise) proactive signal `TokenStore` already provides for free |
| Shell out to `ccusage blocks --json` (npx) | Adds a Node.js runtime dependency to a single Go binary for logic already computable natively from data this repo already parses; explicitly rejected in `build-vs-buy.md` §1 and contradicts `.claude/rules/prefer-go-git-over-subshells.md`'s general subprocess-avoidance preference |
| Poll Anthropic's Rate Limits API | Confirmed non-viable — excludes Pro/Max/Team subscribers entirely (`build-vs-buy.md` §2) |

## Consequences

- **`AssumedWindowTokenBudget` defaults to `0` (disabled sentinel)**: on first deploy, the soft
  percentage signal is inert and the system behaves exactly like the "Fallback Increment" (hard
  reactive signal only) until the user explicitly configures a real budget number (e.g. by
  observing their own usage via `ccusage blocks` or trial and error). This is a deliberate,
  safe-by-default rollout choice — it avoids a wrong out-of-the-box guess causing false pauses on
  day one, at the cost of the proactive half of this feature requiring one manual config step
  before it does anything beyond the fallback increment.
- Every notification and the `status_detail` UI string must state headroom as an estimate (e.g.
  "~%.0f%% remaining, assumed budget") and must never promise a specific resume ETA — the
  underlying cap is inferred, not authoritative (per `requirements.md`'s Feasibility Risks and
  `research/ux.md` §3).
- Two independent evaluators (soft + hard) must be combined by a single writer (`QuotaGate.Reconcile`,
  called only from the existing 60s reconcile ticker) to avoid the multi-goroutine race class
  documented in `pitfalls.md` §2.
