# ADR-001: Staleness Threshold Recalibration

**Date**: 2026-07-24
**Status**: Accepted
**Project**: review-gate-stale-session-rework

## Context

Two consumers of "how long has this session been silent" logic currently share a single hardcoded value: `session.DefaultReviewQueuePollerConfig().StalenessThreshold`, set to 2 minutes (`session/review_queue_poller.go:49`).

1. **The general Review Queue "Stale" badge** (`session/review_queue_determiner.go:262`) — a low-priority, informational queue signal ("might be worth a look").
2. **`notifyIfActiveWorkSessionStale`** (`server/services/backlog_service_triage.go:934`) — gates whether `AutoReopenAfterFailedReview` treats a live-but-silent work session as safe to leave alone vs. worth flagging as blocking an automated rework attempt.

Live evidence: a user report showed 37 of 41 Review Queue items flagged "Stale," and a rework attempt blocked by `notifyIfActiveWorkSessionStale`'s toast, both traced to this 2-minute value being too short for both purposes — a single slow LLM turn or large tool call routinely exceeds 2 minutes without the session being stuck.

Investigating the value's origin: `git log -S "reduced from 5min"` found it was introduced in commit `fc2bfdde` ("feat(web): enhance terminal output and session card UI," 2025-10-22), a large multi-feature commit whose only relevant changelog line is "Enhanced detection thresholds for review queue" — no documented rationale for the reduction from 5 minutes to 2 minutes exists. This is evidence the 2-minute value was not a deliberately validated choice, strengthening the case for recalibrating it now with documented rationale rather than treating it as intentional prior art to preserve.

A structurally similar, already-shipped detector (`StuckReasonStaleWork`'s `reconcileStaleWorkSessions`, gating automated remediation of a genuinely-stuck `in_progress` work session) uses a 2-hour threshold (`maxWorkSessionStaleness`, `session/backlog_lifecycle.go:1874`), documented as "tuned for interactive/foreground triage sessions where a liveness signal isn't available."

## Decision

Three distinct, independently-named thresholds, replacing the single shared 2-minute value for two of these three purposes:

| Threshold | Value | Consumer | Rationale |
|---|---|---|---|
| `ReviewQueuePollerConfig.StalenessThreshold` | **5 minutes** (was 2 min) | General Review Queue "Stale" badge | Low-stakes, informational signal — false positives cost only visual noise, not a blocked automated action, so it doesn't need the same conservatism as the rework-block gate. 5 minutes is chosen as a previously-used, non-invented value (the pre-reduction default) rather than an unvalidated new number, pending live re-verification against the 37/41 false-positive baseline (validation.md's live-data check). |
| `maxReworkBlockStaleness` (NEW) | **15 minutes** | `notifyIfActiveWorkSessionStale` / `AutoReopenAfterFailedReview`'s rework-block gate | This threshold gates an automated decision (whether to treat a session as safely "still working" vs. worth durably flagging) — false positives directly reproduce the original bug (blocking rework, spamming notifications), so it needs more headroom than the badge. 15 minutes is chosen as a middle point: long enough to absorb a slow turn, a large tool call, or brief tool-output buffering (the false-positive causes identified in pitfalls.md #1) with comfortable margin, while remaining short enough to give the user a durable, actionable signal well before `maxWorkSessionStaleness`'s 2-hour window — this scenario is specifically "an automated action is currently blocked," which is more urgent than a generic in_progress item quietly running long. |
| `maxWorkSessionStaleness` (unchanged) | 2 hours | `StuckReasonStaleWork` / `reconcileStaleWorkSessions` | Unchanged — out of scope for this fix; cited here only as calibration precedent. Its own tuning story (interactive/foreground sessions) does not directly transfer to the rework-block gate's different scenario. |

The task-protocol's rule 8 (Phase 3 of the plan) is given an explicit ~2-3 minute polling cadence, chosen to keep at least 5x headroom below `maxReworkBlockStaleness` so a compliant, quietly-waiting agent cannot itself trip the new gate.

## Consequences

- Three named, documented constants replace one silently-shared, undocumented one — a future investigator hitting a similar "why is X stale" question has three specific values and rationales to check, rather than one number of unclear origin reused for an unrelated purpose.
- Both new/changed values (5 min, 15 min) are **best-effort estimates**, not empirically derived from the live instance's actual turn-duration distribution — this ADR explicitly flags that live post-deploy monitoring (validation.md) may show either needs further adjustment. That is an expected, planned-for outcome, not a plan failure — but any future adjustment should update this ADR's rationale table rather than silently drifting the constant again without documentation (repeating the exact failure mode — an undocumented threshold change — this ADR exists to correct).
- `maxReworkBlockStaleness` and `ReviewQueuePollerConfig.StalenessThreshold` are intentionally NOT unified into one value, despite both now being small numbers in the same order of magnitude — see the Decision table's rationale column for why their purposes differ enough to justify separate tuning going forward, even if they happen to start close together.

## Alternatives Considered

- **Reuse `maxWorkSessionStaleness` (2h) directly for the rework-block gate** (a build-vs-buy.md-flagged candidate): rejected as the primary choice because 2 hours delays an actionable signal for a scenario framed around "an automated action is currently blocked right now," which reads as more urgent than a quietly-running in_progress item. A shorter, purpose-specific value was judged more appropriate; kept 2h as unchanged, uninvolved precedent rather than a value to inherit.
- **Leave the general badge threshold unchanged (2 min) and only fix the rework-block gate**: rejected — the 37/41 false-positive evidence implicates the badge threshold directly, and requirements.md explicitly scoped assessing it as in-scope research rather than assuming it's fine because it "shares code today" with the gate that's clearly wrong.
