# Research: Similar Features, Prior Art, and Edge Cases — stale-session-detection

**Agent**: 2 (Features)
**Date**: 2026-08-06

## 1. Original issue / competitive context (source of truth)

`gh issue view 41 --repo TylerStaplerAtFanatics/stapler-squad` (closed, migrated to this
project). Full competitive-context text:

> From systematic evaluation of 122 agent orchestration tools:
> - **Agent-Kanban** — 2-hour stale agent detection threshold with automatic surfacing
> - **Gastown** — Convoy system with autonomous stall detection; stalled work triggers
>   escalation to Deacon

Proposed defaults from the issue: `stale_session_threshold_minutes: 30`, `stale_notify: true`,
separate thresholds for review-queue vs. actively-running sessions, `session_age_minutes` as
an auto-approval-rule condition. No further detail on Agent-Kanban/Gastown internals is in the
issue body — both are named only as threshold/behavior precedent (2h stale, autonomous
stall→escalation), not linked to external docs. Treat "2 hours" and "escalation to a human/
higher tier" as the two transferable ideas, not verified implementation detail (UNVERIFIED
beyond what's quoted above — no external source was fetched for this research pass).

## 2. Existing staleness detectors in this codebase — confirmed current state on `main`

Three independently-tuned, already-shipped thresholds exist today (not two, as
requirements.md's older description implied — **`review-gate-stale-session-rework` has
already shipped `maxReworkBlockStaleness`**, confirmed live in
`server/services/backlog_service_triage.go:1030`, not just planned):

| Threshold | Value | File:line | Consumer | Notes |
|---|---|---|---|---|
| `ReviewQueuePollerConfig.StalenessThreshold` | 5 min | `session/review_queue_poller.go:49` | Review Queue "Stale" badge (`ReasonStale`, `session/review_queue_determiner.go:262`) | Recalibrated from 2min per ADR-001; low-stakes informational signal |
| `maxReworkBlockStaleness` | 15 min | `server/services/backlog_service_triage.go:1030` | `notifyIfActiveWorkSessionStale` / `ResolveReworkBlockedStaleIfRecovered`, gates `AutoReopenAfterFailedReview`'s rework-block path | NEW in ADR-001, ships with a durable `StuckReasonReworkBlockedStale` mark + a **resolve-side counterpart** that clears the mark once the session is no longer stale |
| `maxWorkSessionStaleness` | 2 hours | `session/backlog_lifecycle.go:2098` | `reconcileStaleWorkSessions` → `StuckReasonStaleWork`, the durable stuck-item detector feeding `backlog-stuck` UI | Unchanged by ADR-001; "tuned for interactive/foreground triage sessions where a liveness signal isn't available" |

`project_plans/review-gate-stale-session-rework/decisions/ADR-001-staleness-threshold-recalibration.md`
is the authoritative rationale doc for why these three are separate and not unified — this
project's config surface should treat "informational card badge" as a **fourth**, narrower use
case, not silently reuse one of the three, per that ADR's own reasoning ("their purposes
differ enough to justify separate tuning going forward, even if they happen to start close
together").

**Sibling-project status** (per Constraints in requirements.md, re-confirmed here):
- `review-gate-stale-session-rework`: fully planned (requirements → plan → validation →
  pre-mortem → adversarial-review, `project_plans/review-gate-stale-session-rework/implementation/`)
  and **the threshold recalibration itself has shipped to `main`** (verified via the constants
  above and the code comments citing ADR-001 directly). No further overlap risk for those two
  specific thresholds.
- `review-queue-state-detection` and `review-queue-event-driven`: each has `plan.md` +
  `validation.md` only (no adversarial-review/pre-mortem files) — planning-complete, shipped
  status of the actual code not independently re-verified in this pass beyond what's already
  visible in `review_queue_poller.go` (event-driven `LastMeaningfulOutput` updates and
  working/idle/stuck classification are already live per the field's own doc comments; treat
  as INFERRED shipped, not re-audited task by task here — out of this project's scope per
  requirements.md Constraints).

## 3. `notifyIfActiveWorkSessionStale` — the closest analog to the new card indicator

`server/services/backlog_service_triage.go:1155-1240`, with shared helper
`workSessionStaleness` (line 1045) and resolve-side `ResolveReworkBlockedStaleIfRecovered`
(line 1258). This is the single most relevant precedent for the new feature and answers
several of the open questions directly:

- **Self-clearing, not sticky.** `workSessionStaleness` is recomputed live from
  `TimeSinceLastMeaningfulOutput` on every call — there is no persisted "is stale" boolean.
  `ResolveReworkBlockedStaleIfRecovered` clears the durable `StuckReasonReworkBlockedStale`
  mark as soon as a fresh recompute says `!stale`. **Direct answer to the open question "does
  the indicator clear when a session produces output again": yes, by design, and the new
  card indicator/grouping should follow the identical pattern** — compute live from
  `LastMeaningfulOutput`/`LastTerminalUpdate` on every render/poll, never latch a boolean.
- **Notification is explicitly one-shot per stale period, not repeated.** The doc comment at
  line 1194 states it's "naturally rate-limited without extra dedup bookkeeping" because it
  only runs from inside `AutoReopenAfterFailedReview`, itself gated by
  `autoReopenWithBackoffGate`'s `RemediationDue` backoff (minimum 30 minutes between
  attempts). **This is a narrower guarantee than "fires once and never again" — it can refire
  after 30 min if still stale.** The new project's `stale_notify` needs its own explicit
  answer to "fires once when crossing the threshold vs. re-fires periodically while stale" —
  don't assume this precedent's 30-min backoff transfers; it exists because of the specific
  `AutoReopenAfterFailedReview` call site, not stale-detection generally.
- **Deliberately does not stop/kill the session.** Explicit policy note at line 1171-1176: a
  slow-but-alive agent is never force-stopped just because it's stale — ties to
  "the stop_session-deletes-branch incident." The new feature is observational/notification
  only, consistent with this.
- **Best-effort/silent-skip on missing dependencies** (`sessionStopper == nil`,
  `eventBus == nil`, no active session found) rather than erroring — same posture the new
  card indicator should take if `LastMeaningfulOutput`/`LastTerminalUpdate` are both zero
  (new session, never produced output yet — do not flag as stale).

## 4. Paused/archived sessions — confirmed real gap, needs an explicit guard

- `SessionStatus` enum (`proto/session/v1/types.proto:325-350`): `ACTIVE`, `PAUSED` ("worktree
  removed but branch preserved"), `CREATING`, `STOPPED` (terminal), `HIBERNATED` (checkpoint
  written, tmux killed), `RESTORING`. There is no separate `ARCHIVED` wire status — archival is
  tracked via `Instance.ArchivedAt` (`session/instance_actor_setters.go:202-266`,
  `ArchiveWithStop` stops the session as part of archiving).
- `SessionCard.tsx:23` already has the exact guard needed:
  `isPausedOrStopped = session.status === SessionStatus.PAUSED || session.status === SessionStatus.STOPPED`,
  currently used to gate other Tier-1 UI (per its usage at line 173, snapshot-fetch gating for
  `ACTIVE`-only). **A paused/stopped/hibernated/archived session has legitimately-old
  `LastMeaningfulOutput` by design (no process is running to produce output) — computing
  staleness the same way as an `ACTIVE` session would flag every single paused session as
  "stale" permanently, which is a false signal, not a real one.** The new stale indicator and
  "Stale" grouping strategy must scope the check to `SessionStatus.ACTIVE` (and possibly
  `NEEDS_APPROVAL`/whatever the live "waiting for user" sub-state is called today) and
  explicitly exclude `PAUSED`, `STOPPED`, `HIBERNATED`, `CREATING`, `RESTORING`, and
  `ArchivedAt != nil`. This is not a hypothetical edge case — it's the direct, mechanical
  consequence of reusing `LastMeaningfulOutput` as the sole signal without a status guard, and
  requirements.md's own "reuse the existing timestamp, don't add new tracking" constraint makes
  this guard mandatory rather than optional.

## 5. "Waiting on user" vs. genuinely stuck

The Review Queue poller already distinguishes these today via separate config knobs on the
same struct (`session/review_queue_poller.go:33-40`): `InputWaitDuration` (3s — "flag if
waiting for input") is a materially different, much shorter signal than `StalenessThreshold`
(5min — "flag if no meaningful output"). `LastPromptDetected`/`LastPromptSignature` in
`ReviewState` (`session/review_state.go:66-72`) already track when a prompt requiring input was
last seen, separate from `LastMeaningfulOutput`. **This means "waiting on a user prompt" is
already a distinguishable state from "silent/stuck" in the existing data model** — a session
sitting at a prompt for 45 minutes is not the same failure mode as a session producing zero
output for 45 minutes (crashed, infinite-looped, network-hung). The new stale indicator should
consider whether to visually distinguish "stale + waiting on you" (lower urgency, user already
knows) from "stale + not waiting on anything visible" (higher urgency, likely actually stuck) —
the underlying signals to do this already exist and don't require new tracking, consistent with
the "reuse existing timestamps" constraint. This was not called out in requirements.md's
Open Questions and is worth surfacing to planning as an additional differentiator the existing
plumbing supports for free.

## 6. Clock skew

No explicit clock-skew handling exists anywhere in the staleness code inspected — all three
existing thresholds use `time.Since(t)` against timestamps written by the same process on the
same host (`ReviewState.LastMeaningfulOutput` is set locally by the poller reading local tmux
output, not from a remote/distributed timestamp). Given the CLAUDE.md architecture (single Go
binary, `localhost:8543`, single-developer self-hosted instance, no distributed session state),
clock skew between "the clock that wrote the timestamp" and "the clock that reads it" is
**not a realistic failure mode for this feature** — both are `time.Now()` on the same machine.
The one adjacent precedent worth reusing conceptually (not literally) is the CI-status
staleness guard in `server/services/approval_handler.go:318-324`: a cached value older than
`2x` the poller interval is treated as unknown rather than trusted — this is a "don't trust
data that's older than N poll cycles" pattern, not a clock-skew guard, but it's the closest
existing idiom if planning wants a "don't flag stale off of a timestamp that hasn't been
refreshed recently enough to be meaningful" safeguard (e.g., if `LastMeaningfulOutput` itself
stops being updated due to an unrelated poller bug, the session would appear "stale" forever
rather than "unknown" — worth a planning note, not a blocker).

## 7. `session_age_minutes` approval-rule condition — exact extension point identified

Two-part precedent, both from the **most recently added** condition on `ApprovalRuleProto`
(`require_ci_passing`, field 29 — the field-number sequence in
`proto/session/v1/types.proto:1076-1108` runs 1-29, so the new condition is field 30):

1. **Proto** (`ApprovalRuleProto`): add a typed field analogous to `bool require_ci_passing = 29`
   — e.g. `int32 min_session_age_minutes = 30` (0/unset = condition not active, consistent with
   how the other optional criteria fields default-empty).
2. **Matching context**: `pkg/classifier/classifier.go`'s `ClassificationContext` struct
   (line 61-76) is the consumer-side interface `RequireCIPassing` matches against
   (`ctx.CIStatus`, populated by the caller). The match itself is a single `if` in the rule's
   `Matches`-equivalent function (line 735: `if rule.RequireCIPassing && ctx.CIStatus !=
   ciConclusionSuccess { return false }`) — **all conditions on a rule AND together**; there is
   no OR/threshold-operator support in this matcher today, confirming requirements.md's framing
   ("deny approvals from sessions stale > 60 minutes") is a single AND'd condition, not a new
   sub-language.
3. **Population**: `server/services/approval_handler.go:306-327` is where
   `ClassificationContext` gets built and enriched per-request (`classCtx.CIStatus = ...`) right
   before `h.classifier.Classify(payload, classCtx)` is called — a `SessionAgeMinutes`
   (or `MinutesSinceLastOutput`) field would be populated here the same way, from
   `h.liveFinder.FindLiveInstance(sessionID).Snapshot()`'s `LastMeaningfulOutput`, mirroring how
   `ghInfo := inst.Snapshot().GitHub` is read.
4. **Open question resolution ("created" vs. "last output")**: requirements.md leans toward
   last-output time "for consistency with the rest of the feature." This research did not find
   a technical blocker to either choice — `Instance` has both a creation timestamp and
   `LastMeaningfulOutput` available at the same call site — so the decision is purely semantic/
   product, not constrained by what's wired. Recommend keeping requirements.md's default
   (last-output) since every other consumer of "staleness" in this codebase (all three existing
   thresholds) uses last-output, not creation time, and a `session_age_minutes` name that
   actually measures idle-since-output would be consistent in behavior even if arguably
   confusing in name — flag the naming mismatch for planning to resolve explicitly (e.g. call
   the field `min_stale_minutes` or `min_idle_minutes` instead of `session_age_minutes` to avoid
   the ambiguity, since the issue's own proposed name and its intended semantics don't quite
   match).

## 8. Unstated needs beyond the literal ask

- **A session that goes stale then produces output again must clear the indicator
  automatically** (confirmed as the existing precedent's behavior in §3 — not explicitly
  asked for in the issue, but users will expect it given every other stale/stuck indicator in
  this codebase already works this way; failing to do so would be a regression relative to
  existing UX, not just a missed nice-to-have).
- **Distinguishing "stale + waiting on a prompt" from "stale + no visible reason"** (§5) — the
  issue doesn't ask for this, but the data to do it already exists for free, and Tyler's stated
  problem ("miss one that silently stopped producing output... stuck agent, waiting prompt, or
  crashed process") explicitly lists "waiting prompt" as one of three distinct causes he wants
  to catch — a single undifferentiated "Stale" badge conflates exactly the three causes he
  named as needing to be told apart. Worth flagging to planning even though it wasn't in
  requirements.md's explicit scope.
- **Paused/archived sessions must be excluded from staleness computation** (§4) — not called
  out as an edge case anywhere in requirements.md's Open Questions, but it is the most likely
  source of a loud, immediate false-positive bug if missed (every paused/archived session in
  the workspace would light up "Stale" simultaneously on first deploy).
- **The rework-block precedent's notification cadence (§3) should not be assumed to transfer** —
  requirements.md's "fires once vs. repeatedly" question doesn't have a ready-made answer in
  the existing code; the existing 30-min backoff is an artifact of its own caller's gating, not
  a general "how often should stale-notify fire" policy. Planning needs its own explicit answer
  here (e.g., fire once per continuous stale period, re-arm only after the session produces
  output and goes stale again) rather than copying the rework-block cadence verbatim.
- **Naming clarity for `session_age_minutes`** (§7.4) — the issue's proposed name measures
  creation-to-now, but the intended semantics (per requirements.md's own resolution) are
  idle-since-last-output. Shipping a field literally named `session_age_minutes` that actually
  means "minutes since last output" invites future confusion; worth a one-line naming
  correction during planning even though it's not something the user explicitly flagged.
