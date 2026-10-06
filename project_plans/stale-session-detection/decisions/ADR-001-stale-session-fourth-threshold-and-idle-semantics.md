# ADR-001: Fourth Staleness Threshold + Approval-Rule Idle Semantics

**Date**: 2026-08-06
**Status**: Accepted
**Project**: stale-session-detection

## Context

Three independently-tuned "how long has this session been silent" thresholds already
ship on `main`, all originating from `review-gate-stale-session-rework`'s
[ADR-001](../../review-gate-stale-session-rework/decisions/ADR-001-staleness-threshold-recalibration.md):

| Threshold | Value | File:line | Consumer |
|---|---|---|---|
| `ReviewQueuePollerConfig.StalenessThreshold` | 5 min | `session/review_queue_poller.go:49` | Review Queue "Stale" badge |
| `maxReworkBlockStaleness` | 15 min | `server/services/backlog_service_triage.go:1030` | `notifyIfActiveWorkSessionStale` / rework-block gate |
| `maxWorkSessionStaleness` | 2 hr | `session/backlog_lifecycle.go:2098` | `StuckReasonStaleWork` durable stuck-item detector |

`pitfalls.md` §1 (this project's Phase 2 research) explicitly names the risk of this
project silently becoming a "fourth uncoordinated number." This ADR is the required
rationale table extending ADR-001's pattern, per that research's recommendation.

A second, unrelated decision bundled into this ADR: the semantics of the new
approval-rule condition (originally proposed in the source issue as
`session_age_minutes`).

## Decision 1: A fourth, independently-named threshold for the SessionCard badge/notifier

`StaleSessionConfig.ThresholdMinutes` (`config/types.go`), default **30 minutes**,
config-driven (not a compile-time constant like the other three).

| Threshold | Value | Consumer | Rationale |
|---|---|---|---|
| `ReviewQueuePollerConfig.StalenessThreshold` | 5 min | Review Queue badge | Unchanged — out of scope, cited for calibration context only. |
| `maxReworkBlockStaleness` | 15 min | Rework-block gate | Unchanged — out of scope. |
| `maxWorkSessionStaleness` | 2 hr | Stuck-item detector | Unchanged — out of scope. |
| `StaleSessionConfig.ThresholdMinutes` (**NEW**) | **30 min**, config-driven | Main-session-list card badge, "Stale" grouping strategy, `StaleSessionNotifier` | Requirements.md's Open-Questions resolution (line 145) explicitly settles this at 30 min, matching the original issue's proposed default and Agent-Kanban's cited 2-hour precedent scaled down for a more proactive, glanceable signal. UX research (`research/ux.md` §1) independently suggested "10–15 min" as an anchor *before* requirements.md's resolution landed; we follow requirements.md's later, explicit resolution (30 min) as authoritative, not the earlier UX anchor — the discrepancy is a research-sequencing artifact, not a live disagreement. Unlike the other three, this value is user-configurable at runtime (not a recompile), so a too-aggressive default can be corrected without a code change. |

This is a genuinely new, fourth signal — not a reuse of any existing one — because its
consumers (a глance-level badge covering every card at once, and a client-configurable
grouping filter) have a different false-positive tolerance than any of the three
existing consumers: too short (5 min) makes every card flicker "stale" during a normal
LLM turn; too long (2 hr) defeats the "catch it while scanning the grid" job-to-be-done
from `requirements.md`'s problem statement.

## Decision 2: Approval-rule condition measures idle-since-last-output, not session age

The proto field is named `min_session_idle_minutes` (not the issue's originally proposed
`session_age_minutes`), and is computed from
`Instance.GetTimeSinceLastMeaningfulOutput()` (`session/instance_approval.go:114`) — the
same call the Review Queue badge already uses — not `Instance.CreatedAt`.

**Rationale**: `research/features.md` §7.4 and `research/architecture.md` §3 both
independently flag that the issue's proposed name ("age") and its own intended semantics
("time since last output," per `requirements.md`'s explicit resolution) don't match. A
long-lived-but-actively-working session would wrongly trip a creation-time-based "age"
condition despite being clearly alive — a more surprising, harder-to-debug false positive
than the idle-time interpretation. Renaming the field to `min_session_idle_minutes`
removes the ambiguity at the type level rather than documenting around it. This also
keeps the approval-rule condition, the SessionCard badge, and the "Stale" grouping
strategy all reading the *same* underlying signal
(`LastMeaningfulOutput`/`GetTimeSinceLastMeaningfulOutput()`), so a rule author writing
`min_session_idle_minutes: 60` means the same thing a user sees flagged stale on a card.

**Fail-closed requirement**: when the classifying request has no resolvable live
`Instance` (`h.liveFinder.FindLiveInstance(sessionID)` returns nil), `ClassificationContext.SessionIdleMinutes`
is left at its Go zero value (`0`) rather than populated. Because `matchesRule`'s check is
`rule.MinSessionIdleMinutes > 0 && ctx.SessionIdleMinutes < int(rule.MinSessionIdleMinutes)`,
an unset/zero `ctx.SessionIdleMinutes` can never satisfy a `MinSessionIdleMinutes > 0`
condition — the condition doesn't match, the rule doesn't fire, and evaluation falls
through to the next-priority rule (ultimately the escalate catch-all). This mirrors
`ApprovalHandler`'s existing `CIStatus = ""` staleness guard
(`server/services/approval_handler.go:321-324`) exactly: unknown data fails closed to
escalate, never silently auto-approves or auto-denies.

## Decision 3: Config field is `stale_session.notify_enabled`, not the issue's literal `stale_notify`

The original issue's proposed config shape used a flat `stale_notify` key.
`StaleSessionConfig` instead nests it as `NotifyEnabled` (JSON `notify_enabled`) inside
a new `stale_session` block on `Config`, matching `SessionRetentionConfig`'s existing
nested-struct convention exactly (`config/types.go`) rather than adding another
top-level flat key. Same reasoning as Decision 2: consistency with the codebase's
existing pattern for a config knob, not the source issue's literal proposed shape.

## Consequences

- Four named, documented thresholds now exist in the codebase (was three). A future
  investigator has one rationale table (this ADR) plus the prior one (ADR-001) to check,
  rather than guessing at a silently-reused or newly-invented number.
- `StaleSessionConfig.ThresholdMinutes` is a **best-effort estimate** (30 min), not
  empirically derived from live turn-duration data — consistent with ADR-001's own
  honesty about its 5/15-minute values. Future recalibration should update this ADR's
  table, not silently drift the default.
- The approval-rule condition and the card badge/grouping now share one Go call
  (`GetTimeSinceLastMeaningfulOutput()`) and one conceptual signal, even though they use
  independently-configured threshold *values* (a global `StaleSessionConfig.ThresholdMinutes`
  for the badge vs. a per-rule `MinSessionIdleMinutes` for approval rules) — this is
  intentional per `build-vs-buy.md`'s recommendation to share the *comparison*, not the
  *value*.

## Alternatives Considered

- **Reuse `maxReworkBlockStaleness` (15 min) for the card badge**: rejected — that
  threshold is tuned to gate an automated rework-block decision, not a glance-level
  visual scan; reusing it would recreate the exact "threshold tuned for one purpose,
  reused for another" failure mode ADR-001 itself was written to fix (the 37/41
  false-positive incident).
- **Keep the approval-rule field named `session_age_minutes` for continuity with the
  source issue**: rejected — the semantic mismatch (age vs. idle) is a real correctness
  risk for rule authors, not just a cosmetic one. This ADR's Status is `Accepted` and
  `min_session_idle_minutes` is final; plan.md's Unresolved Questions section reflects
  this (no open naming question blocks Phase 5). If a future reviewer wants to revisit
  the name, that is a new decision superseding this ADR, not a reopening of this one.
- **Unify all four thresholds behind one shared config value**: rejected — see this
  project's `plan.md` Pattern Decisions table ("Overall Architecture" row) for the full
  reasoning; in short, ADR-001 already proved the four consumers have different
  false-positive tolerances, and a shared value would recouple them.
