# Findings: Features — Session Triage & Approval UX Patterns

## Summary

Multi-session triage faces a fundamental tension: status visibility requires constant polling (expensive, unreliable, causes UI thrashing) vs. non-interrupting approval requires a background notification layer (adds complexity but preserves focus). The industry has settled on three patterns: (1) event-driven status refresh with delta updates (GitHub Actions, CI dashboards), (2) badge/icon notification layers with sidebar approval panels (Slack workflows), and (3) stable queue management via client-side acknowledgment rather than server-side mutation (Buildkite, Datadog). Stapler Squad's existing review queue system already implements pattern 1; approval flows need patterns 2 + 3 to work non-disruptively.

**Dominant trade-off**: Discoverability vs. interruptiveness. Modal approvals maximize discoverability but destroy focus. Badge + sidebar maximizes focus preservation at the cost of requiring the user to notice the badge. The industry has converged on badge + sidebar for professional tools where focus is the primary constraint.

## Options Surveyed

- tmux/ttyd/wetty/gotty - terminal web UIs
- GitHub Actions - CI dashboard with parallel job status cards
- Buildkite - pipeline DAG with inline approvals
- CircleCI - parallel job status with terminal preview on hover
- Slack approval workflows - badge + thread/sidebar
- GitHub PR review queue - stable queue with acknowledgment
- Datadog infrastructure list - triage card patterns
- PagerDuty incident cards - urgency triage UX
- OpsGenie alert cards

## Trade-off Matrix

| Pattern | Interruptiveness | Discoverability | Approval Speed | Context Preservation | Implementation Complexity | Stability |
|---|---|---|---|---|---|---|
| Modal popup | 5/5 (blocks) | 5/5 (can't miss) | 1 click | 0/5 (loses focus) | Simple | N/A |
| Floating badge + sidebar | 1/5 (dismissible) | 3/5 (depends on placement) | 2-3 clicks | 4/5 (stays on current page) | Medium | High |
| Side drawer | 2/5 (slides in) | 4/5 (visible if open) | 2-3 clicks | 3/5 (page visible behind) | Medium | High |
| Inline card approval | 1/5 (contextual) | 2/5 (hover only) | 2-3 clicks | 5/5 (inline) | High | High if list stable |
| Dedicated queue page | 2/5 (dedicated page) | 4/5 (nav button) | 3-4 clicks | 3/5 (new page) | Medium | Highest |

## Risk and Failure Modes

**Modal interruption fatigue**
- Failure mode: Hook approvals pop as modals over current session view; user gets interrupted repeatedly while working
- Conditions: Any approval event triggers a modal
- Mitigation: Never use modal for approval. Use sidebar/drawer or stable queue exclusively

**Badge blindness**
- Failure mode: Small badge on the side gets missed; user doesn't realize approvals are pending
- Conditions: Badge is placed in a low-attention area or doesn't animate
- Mitigation: Prominent badge in header with count; 5s pulse animation on new arrival; opt-in browser notification

**Stale queue**
- Failure mode: User goes to dedicated queue page but updates are slow/stale; they don't trust it
- Conditions: Polling rather than push updates; reconnect failures leave queue frozen
- Mitigation: <100ms push updates (already in place); show "live" indicator; show timestamp of last update

**Card information overload**
- Failure mode: Session card shows too much state; becomes unreadable
- Conditions: Adding status, approval pending, tests failing, rate limit, PR review status all at once
- Mitigation: Priority layering — (1) session name + status, (2) urgency badge (Approval/Error/Stale), (3) terminal preview on hover, (4) secondary state expandable

**Race condition: double approval**
- Failure mode: User approves via sidebar drawer AND via terminal prompt simultaneously
- Conditions: Approval mechanism exists in both places
- Mitigation: Disable approval button after click (optimistic UI); show "Already resolved" if hook already resolved; add idempotency key to approval RPC

**Queue jump causing wrong approval**
- Failure mode: User is approving an item in the queue; queue re-renders and item moves; user clicks wrong item
- Conditions: Server-side re-ordering while user has item in focus
- Mitigation: Client-side acknowledgment — when user clicks an item, mark it "acknowledged" locally to prevent re-ordering while in focus

## Migration and Adoption Cost

**Tier 1 (2 dev-days)**: Add approval badge to header + sidebar drawer + countdown timer on each approval. No modals. Purely additive to existing infrastructure.

**Tier 2 (3 dev-days)**: Enhance SessionCard with terminal preview (last 3 lines, on hover); add status badge hierarchy (Approval > Error > Stale > Running > Ready).

**Tier 3 (optional, 2 dev-days)**: Review queue page more discoverable (nav badge with count); approval filtering; approval history.

**User adoption cost**: Near zero. Change is additive and non-breaking. Existing keyboard queue navigation preserved.

## Operational Concerns

**Approval expiry**: Hook approval requests expire server-side (typically 30-60s). Show countdown timer. If <10s remaining, add visual warning (red background, pulse animation). Wire to `seconds_remaining` field in protobuf schema.

**Auto-approval transparency**: When "auto_yes" is set on a session, auto-approved decisions should appear in notification history (not in real-time queue) to maintain audit trail.

**Attribution**: Stamp approval records with (user, timestamp, decision). Needed for audit trail. Already handled in `ApprovalService.ResolveApproval()`.

**Runaway loop detection**: Alert user if auto-approval rate exceeds threshold (e.g., >10 approvals/minute = likely runaway loop).

## Prior Art and Lessons Learned

**GitHub Actions** — Keeping approvals on the workflow page (not interrupting) improves focus. Status badges on job cards (red/yellow/green) reduce triage time vs. opening each job. Shows action buttons (Cancel, Re-run) on card without opening job detail.

**Buildkite** — Approvals as nodes in the workflow (not interrupts) makes them feel "part of the plan". Stable DAG means user can reason about workflow execution without re-renders.

**Slack** — Badge indicator (unread channel count) in sidebar is extremely discoverable without being intrusive. Approval modals (deprecated) were hated by users → moved to sidebar thread. Message threading keeps approvals organized by context.

**PagerDuty** — v1 interrupted with desktop notifications for every alert → users hated it. v2 introduced alert suppression + grouping. Status dot (primary signal) + name (secondary) + time-since-event is a proven hierarchy for triage.

**CircleCI** — Used to full-page refresh on job completion → now delta updates only. Hovering a failed job shows the last few lines of output (terminal preview).

**Datadog** — Status is visual (color + icon) not textual — user spots issues before reading text. Time information ("5 minutes ago") is always present for triage.

## Open Questions

- [ ] What's the exact information density on a session card triage view? Options: (A) last N lines terminal preview (high density), (B) status + time (low density), (C) status + recent git commit (medium). Start with C, add A as optional on hover.
- [ ] Should approvals appear in both review queue AND sidebar? Recommendation: both, but review queue is source of truth and sidebar is a convenience filter.
- [ ] How many approvals before UI becomes unwieldy? Sidebar scrolls if >5; add pagination/filtering for >10.
- [ ] Should terminal preview be live stream or snapshot? Recommendation: start with snapshot (simpler, lower risk), upgrade to live if demanded.

## Recommendation

**Recommended pattern**: Badge + sidebar drawer for approvals; enhanced session cards with status priority badges for triage.

**Reasoning**: The industry has unambiguously moved away from modal approvals in professional tools that require sustained focus (Slack, PagerDuty, Buildkite all deprecated modal approvals). The badge + sidebar pattern achieves the required discoverability without the context interruption. The dedicated review queue page already exists in Stapler Squad — it just needs a global entry point (badge in header with count) to be findable without navigating.

**For triage cards**: Adopt PagerDuty/Datadog priority hierarchy: status badge (visual, primary) >> session name >> urgency indicator >> time since last activity >> action button. Terminal preview as on-hover enhancement, not default card content.

**Accept these costs**: 2-3 clicks instead of 1 for approval (vs. modal); badge blindness risk mitigated by prominent placement and animation.

**Reject these alternatives**:
- Modal: rejected because it interrupts focus — the primary pain point this project addresses
- Inline card approval only: rejected because it requires user to scan entire session list to find pending approvals — poor triage speed for 20+ sessions

## Pending Web Searches

1. `"GitHub Actions approval modal vs inline UX 2024"` — confirm GitHub moved approvals off modals for UX reasons
2. `"Buildkite pipeline manual gate approval UX"` — validate DAG-based approvals are more stable than modal-based
3. `"Slack approval workflow sidebar history"` — confirm when Slack deprecated approval modals
4. `"PagerDuty alert notification interruption study"` — verify research on notification interruption costs
5. `"CI dashboard triage card information density"` — validate what metrics appear on CI job cards
