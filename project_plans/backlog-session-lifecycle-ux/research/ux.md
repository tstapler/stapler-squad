# UX Research: backlog-session-lifecycle-ux

## 1. Comparable UX patterns

The relevant precedent, GitHub Actions / CI retry badges / k8s pod status reasons, all converge on the same shape: **a compact glyph+word status at rest, full history only on demand**. None of them put a timeline inline by default — that's reserved for a detail view or hover popover. Concretely:

- GitHub Actions: job list shows a single icon (✓/✗/●) + short status word (`Success`, `Failed`, `In progress`) per run. Retry history isn't shown per-row at all — you click into the run to see "Re-run jobs" history as a flat list of attempts, each with its own icon+timestamp. No inline sparkline or counter on the list row.
- CI retry badges (e.g. CircleCI, Buildkite): a small "×N" superscript/badge appended to the status icon (`✗ ×3`) signals "failed and retried N times" without expanding anything — the count alone answers "how many times," full detail is a click away.
- k8s pod status reasons (`CrashLoopBackOff`, `ImagePullBackOff`, `OOMKilled`): the *reason string itself* is the primary signal, shown inline next to the phase (`Waiting: CrashLoopBackOff`), not hidden behind a click — because for an operator the reason IS the actionable fact, not an aside.

**Verdict for this repo**: the existing `StuckItem`/`BlockerChip` vocabulary already implements the CI-badge pattern correctly (icon + text label + optional duration, collapsed detail below). The gap identified in requirements.md is that this same *pattern* — not a new one — needs to be extended to `end_reason`, `pause_reason`, and remediation/respawn history, which currently only exist as tooltip (pause_reason) or nothing at all (end_reason, respawn events). The k8s lesson applies directly to end_reason: an error end_reason (`process_error`, `claude_not_found`, `timeout`) is exactly the "reason string IS the actionable fact" case and should be inline-visible on the card, not buried in an expand. A clean end_reason (`""`/`shutdown`) is the CI-green-check case — icon only, no extra text needed, don't manufacture a label for "nothing went wrong."

For respawn count specifically, the CircleCI `×N` badge is the right minimal-footprint primitive: a small count suffix on the existing session-status chip (e.g. `⛔ Errored ×3`) rather than a separate widget. Full respawn history (reason/timestamp/triggering-resulting session per event) is exactly what belongs in a `CollapsibleSection`/timeline in the item detail panel — matches GitHub Actions' "click into the run for full re-run list" behavior.

## 2. User mental model — fastest-scan format

Tyler's actual workflow (per project memory) is: `/unfinished` and the item detail panel, checked routinely to answer one question — **"is this item actively self-healing, or is it truly stuck and needs me?"** That's a binary triage decision, not a research task. The existing `stuckReason.ts` vocabulary already encodes exactly this distinction via icon+color:

- 🟢/🟡 = benign/in-progress (`PR_READY_UNMERGED`, `ABANDONED_REVIEW`, `PLAN_NOT_APPROVED`, `AUTONOMOUS_STUCK`, `ORPHANED_TRIAGE`) — system is mid-flow or waiting, not necessarily broken
- 🟠/🔴/🟥/⛔ = escalating severity (`STALE_WORK`, `REWORK_CAP`, `REWORK_BLOCKED_STALE`, `PUSH_FAILED`, `SPAWN_FAILED`, `PR_PENDING_NO_PR`) — increasingly "needs a human"
- 🔁 = actively cycling (`BOUNCING`)
- ⚪ = unknown/unspecified

This is the vocabulary to **extend, not replace**, for the new surfaces:
- **"Actively self-healing"** = a session ended with a retryable end_reason (`timeout`, `process_error`) AND a respawn event followed within the backoff window AND `remediation_attempts < MAX_REMEDIATION_ATTEMPTS` (5, per `StuckItem.tsx`'s existing constant) → reuse 🟡/🟠 (in-progress/escalating-but-not-yet-parked) plus the existing `next_remediation_at` countdown already rendered on `/unfinished`.
- **"Truly stuck"** = `remediation_attempts >= MAX_REMEDIATION_ATTEMPTS` (the existing `isParked` computation in `StuckItem.tsx`) → reuse 🔴/🟥 "parked" treatment, and this is precisely the state `StuckItem.tsx` already disables the "Retry now" button for and labels "remediation attempts exhausted."
- A clean end (`end_reason: ""` or `shutdown`, no stuck-state row exists at all) needs **no badge** — absence of a warning chip is itself the signal, consistent with how `BlockerChip` is only rendered when a `StuckBacklogItem` exists for that item id.

Fastest-scan format recommendation: on the board card and sessions list, a single small chip using the *same* color/icon system, positioned next to the existing status chip, that shows the worst-of `{end_reason severity, pause_reason severity, remediation escalating}` for that item/session — clicking it (or expanding the card) reveals the full breakdown (which session ended why, respawn count, next retry time) via `CollapsibleSection`. Do not add a second, parallel icon vocabulary for end_reason/pause_reason — map their severities onto the same visual scale so a user scanning the board doesn't have to learn two color codes.

## 3. Accessibility

Every existing chip/badge in this codebase already follows one convention, confirmed in `StuckItem.tsx`, `BlockerChip.tsx`, and `SessionCard.tsx`:

- The color-bearing element carries `aria-label={label}` (the full text label, not just a category name) — e.g. `<span className={chipClass} aria-label={label} data-testid="blocker-chip">`.
- The decorative glyph inside is always `<span aria-hidden="true">{icon}</span>` — the icon is never the only content exposed to assistive tech.
- The visible text label is *also* rendered as a sibling text node (not just in `aria-label`) — sighted users get the word, not just the color, per `BlockerChip.tsx`'s explicit doc comment: *"Never color-only — the icon and text label always accompany the chip's color, in both variants."*
- Tooltip-only info (current `pause_reason` on `SessionCard.tsx`) uses a `Tooltip` component wrapping a `role="img" aria-label="Session status: ..."` span — but per requirements.md this is exactly the pattern being upgraded away from "tooltip-only" to "always-visible badge," since a hover tooltip is not discoverable on touch devices (mobile is an explicit constraint) and isn't scanned at a glance.

**Convention to follow for new end_reason/respawn-count chips**: same triad — visible icon (`aria-hidden`) + visible text label + `aria-label` on the container carrying the full sentence-level meaning (reuse `formatPauseReason`-style full-sentence formatting, not just the raw enum value, for the `aria-label`/tooltip text). For the respawn-count suffix badge (`×N`), the accessible name must spell out the count in words (`aria-label="Respawned 3 times"`), not rely on the visual `×3` glyph alone.

## 4. Error/edge-case UX

- **Respawn event whose target session was later deleted**: render the event row with the reason/timestamp/triggering-session as normal, and render the resulting-session reference as inert text (session short-id, no link) rather than a broken link or a silently-dropped row. This mirrors how `StuckItem.tsx` already handles `isPrStatusUnknown` — it doesn't hide the row when data goes stale, it adds an explicit "(check failing)" qualifier. Equivalent here: append "(session no longer exists)" rather than omitting the reference. Never silently drop a historical event because its target no longer resolves — the audit trail's value is specifically in surviving past the referenced entity's lifetime.
- **`end_reason` empty string vs. unset/unknown**: per `session/storage_backlog.go:289` the empty string is the *documented success case* ("or "" for a successful end"), not a missing-data case — proto3 can't distinguish "explicitly set to empty" from "never set" without a wrapper/oneof, so treat `end_reason == ""` as "ended cleanly, no badge" uniformly. This matches the recommendation in §2 (no badge for clean ends). If a genuinely-unset/still-running state needs distinguishing from a clean end, that's carried by session status (`isPaused`/running/etc.), not by `end_reason` — don't invent a separate "unknown" visual state for empty-string end_reason.
- **No respawn event exists at all for an item** (e.g. it only ever ran once and finished): the respawn-history `CollapsibleSection` should not render as an empty accordion row — either omit the section entirely for that item, or match `StuckItem`'s pattern of only rendering `BlockerChip`/badges when the underlying data exists (`StuckBacklogItem` row present). Avoid an "empty state" UI (e.g. "No respawns yet") for a case that will be the *majority* case (most items never need remediation) — that's visual noise for the common path, not signal.

## 5. Job-to-be-done

Functional job: let Tyler answer "should I intervene on this item right now, or is the system already handling it?" without leaving the board/detail view he already checks daily. Emotional job: reduce the low-grade anxiety of not knowing whether silence means "fine" or "silently broken" — visible respawn/end/pause reasoning turns an opaque retry loop into a legible, trustable one, which matters more for a solo-maintained automation pipeline than for any indicator's polish.

## Summary of concrete recommendations for planning phase

1. Reuse `stuckReason.ts`'s existing icon/color severity scale for end_reason and pause_reason badges — do not invent a second vocabulary. Map: clean end → no badge; retryable error end_reason while actively respawning → 🟡/🟠; `remediation_attempts >= MAX_REMEDIATION_ATTEMPTS` (parked) → 🔴/🟥.
2. Board card / session list: single compact chip (icon+label, `BlockerChip`-style, `variant="compact"` analog) showing worst-of status; full breakdown behind existing `CollapsibleSection`/`CollapsibleGroup` in the item detail, consistent with `backlog-item-detail-ux`'s shipped progressive-disclosure pattern.
3. Respawn count as a small `×N` suffix on the existing chip (CI-retry-badge pattern), full respawn timeline (reason, timestamp, triggering/resulting session) as a new `CollapsibleSection` in the item detail — each row keeps triggering/resulting session references even if the target session was later deleted (render as inert text + "(session no longer exists)" qualifier, never silently dropped).
4. Accessibility: every new badge follows the established triad exactly — `aria-hidden` icon span + visible text label sibling + `aria-label`/tooltip carrying the full-sentence meaning (formatPauseReason-style), never color-only. Respawn-count badge needs a spelled-out `aria-label` ("Respawned N times"), not just the `×N` glyph.
5. `end_reason === ""` is success, not unknown — no badge, no separate "unknown" state invented for it. Don't render an empty respawn-history section for items that were never remediated (the common case) — omit the section rather than showing an empty state.
