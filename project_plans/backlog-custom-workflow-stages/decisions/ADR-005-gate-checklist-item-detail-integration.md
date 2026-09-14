# ADR-005: Gate Checklist Renders Per-Candidate-Transition, Reusing the Existing Liveness Panel

**Status**: Accepted
**Date**: 2026-09-11
**Review**: Adversarial review (2026-09-11) confirmed every load-bearing factual claim below and
found two gaps, both folded into the Decision as written now; see "Revisions from review" at the
end.
**Deciders**: Tyler Stapler (via SDD Phase 3 follow-on, `backlog-custom-workflow-stages`)
**Related**: `docs/adr/013-workflow-engine-replaces-valid-transitions.md` (ADR-013), `ADR-002-configured-workflow-engine-and-gates.md` (defines `PendingGates`/`GateStatus`, which this surfaces)

---

## Context

`design/ux.md`'s Surface 4 ("item-detail 'what's blocking this transition'") specifies a two-part
panel: a liveness/staleness section (top billing) above a per-gate checklist, triggered by
`PendingGates(itemId, nextCandidateTransition)`. Epic 2.10 built the checklist component
(`GateChecklist.tsx`, tested in isolation) and this project separately added the `GetPendingGates` RPC
it needs, but nothing renders `GateChecklist` inside `BacklogItemDetail.tsx` yet. Two things ux.md's
prose doesn't resolve on its own:

1. **What is "nextCandidateTransition"?** It reads as a single value, but every built-in
   `BacklogStatus` has multiple legal `AllowedTransitions` (forward, backward/manual-override, and
   `archived`) — there is no unambiguous single "next" edge to compute from the transition graph
   alone, and `item.allowedTransitions` (already shipped to the frontend for the Manual Override
   dropdown, `ManualOverrideSection.tsx`) is exactly that unfiltered list.
2. **Is the liveness/staleness panel actually new work?** Investigating the current item-detail page
   found it is not: `LifecycleSummary.tsx` already renders `StuckItem`/`StuckItemDetail` — reason,
   detail text, threshold, and a "Retry now" button wired to the same `TriggerRemediationNow` RPC
   ux.md's mockup describes — sourced from `useStuckBacklogItems()`. This predates Epic 2.10 (it's
   part of the BUG-055/BUG-083 liveness work) and ux.md's Surface 4 section describes it accurately
   without knowing it already shipped elsewhere on the page.

## Decision

1. **No new liveness UI.** `LifecycleSummary`/`StuckItem`/`StuckItemDetail` already satisfies Surface
   4's liveness-panel requirement (top-billed, above where the gate checklist will render, since
   `LifecycleSummary` already renders at the top of the item-detail header). This ADR's scope is
   the gate checklist only.
2. **`nextCandidateTransition` is resolved as "every target in `item.allowedTransitions`, excluding
   `archived`, that has at least one configured gate," not a single value.** The item-detail page
   calls `GetPendingGates(itemId, to)` once per non-`archived` entry in `item.allowedTransitions`
   (already available on the item, no new fetch to get the candidate list itself), and renders one
   `GateChecklist` block per target whose response is non-empty, labeled
   `"What's blocking <CurrentStatus> → <ToStatus>?"` exactly matching ux.md's mockup framing,
   generalized from "one" to "one per gated edge." A target with zero gates renders nothing for that
   edge — this is what makes the common built-in case (most edges have no configured gates) collapse
   to ux.md's own "no pending transition at all → section does not render" edge case, without a
   special case in the code for it. `archived` is excluded unconditionally: every codebase precedent
   treats it as an unconditional abandon/escape-hatch target (`OverrideVerdict`'s own bypass of gate
   checks is the same posture, one level up), so a "What's blocking X → Archived?" row would
   contradict that and confuse rather than clarify — this is a deliberate scope line, not an
   oversight, should an operator ever configure a gate on an `X → archived` edge.
3. **The frozen-snapshot notice is derived client-side from a live, *enabled* stage, not a new RPC
   field, and not just presence in the fetched list.** Rather than adding an "am I on a snapshot
   fallback" flag to `GetPendingGatesResponse`, the item-detail page already has (or gains, trivially)
   the live stage list via `useBacklogStages()` (Epic 2.9). The check must be
   `!stages.some(s => s.slug === item.status && s.enabled)`, not merely "slug not found" —
   `useBacklogStages()`'s `listStages({})` call returns every stage including disabled ones (each
   still carrying its slug), but the backend's `stageConfigCache` (which is what actually decides
   whether `ConfiguredWorkflowEngine` falls back to a snapshot) treats a *disabled* stage identically
   to a deleted one (`ListEnabledStages`/`ListEnabledTransitions`). A presence-only check would hide
   the frozen-config notice for an item stuck on a merely-disabled stage — exactly backwards. When the
   check is true, the notice renders: `"This item is on a stage no longer in the current
   configuration. Transitions shown reflect the configuration when it entered this stage."` — above
   the checklist section(s), exactly ux.md's wording.
4. **The "Fix in Stages settings →" link deep-links to the specific stage**, not a bare link to the
   settings list. `web-app/src/app/settings/backlog-stages/page.tsx`'s `editingStage` state gains a
   `?editStage=<slug>` query-param initializer (read once via `useSearchParams` on mount, matching
   this app's existing search-param-driven state precedent elsewhere), so the link is
   `/settings/backlog-stages?editStage=<slug>` and lands directly on that stage's open edit form —
   satisfying ux.md's explicit "never a dead end" requirement for real, not just via a link to the
   right page.
5. **Placement**: a new `GateBlockingSection` container component (owns the per-target
   `GetPendingGates` fetching, the frozen-snapshot check, and rendering N `GateChecklist` blocks)
   renders in `BacklogItemDetail.tsx` immediately below `LifecycleSummary` and above the existing
   scroll-area content — matching ux.md's "top billing above the checklist" ordering with the
   liveness panel already occupying that top slot.

## Rationale

- **Filtering to gated-only targets sidesteps the single-"next"-transition ambiguity entirely**
  instead of guessing which one edge is "the" next one — a guess that would be wrong for any custom
  graph with more than one gated forward path, and unnecessary since `PendingGates` is cheap to call
  per candidate (an in-memory cache read plus at most one `GateSatisfactionRepository` lookup per
  gate, not a heavy query).
- **Reusing `LifecycleSummary` instead of building a second liveness display** avoids exactly the
  "two nearly-identical status panels on one page" problem `research/ux.md` warns against elsewhere in
  this project, and it means Surface 4's actual net-new surface area is one component, not two.
- **Client-side snapshot-notice derivation** keeps `GetPendingGatesResponse` minimal (per ADR-002's
  established "thin wrapper" bias) and reuses data the page needs anyway for board/column rendering
  parity (Epic 2.9's hook), rather than teaching the RPC to explain server-internal cache-miss
  behavior to a client that already has the information available another way.

## Alternatives Considered

**Alt A: Add an explicit `next_candidate_transition` field to `BacklogItem`/the item proto, computed
server-side.** Rejected — this reintroduces the exact ambiguity this ADR exists to resolve (which one
edge is "next" server-side is just as undecidable as client-side), and duplicates
`item.allowedTransitions`, which already exists for this purpose.

**Alt B: Only show the checklist for the single edge the Manual Override dropdown currently has
selected.** Rejected — Manual Override is described in its own doc comment as "an operator escape
hatch," and its backend path (`OverrideVerdict`) is a deliberate bypass of the normal gate-check flow,
not the transition Surface 4's mockup is about (the automated pipeline's own next step). Coupling the
checklist to that dropdown's selection would show "what's blocking" a transition path that's about to
ignore blockers anyway — actively misleading.

**Alt C: Add a boolean `used_snapshot_fallback` to `GetPendingGatesResponse` instead of deriving it
client-side.** Considered and rejected only narrowly — it's a fine alternative, but the client already
has `useBacklogStages()` for Epic 2.9's board rendering, so deriving the same fact from data already
in memory avoids widening the RPC contract for a single boolean.

## Consequences

**Positive**: closes Epic 2.10's actual UI gap (the checklist becomes visible and functional);
reuses three existing pieces of infrastructure (`LifecycleSummary`, `item.allowedTransitions`,
`useBacklogStages()`) instead of building parallel versions of any of them.

**Negative**: N `GetPendingGates` calls per item-detail page load (N = `item.allowedTransitions.length`,
typically 3-5 for a built-in status) instead of one — acceptable given each call is a cheap
cache-backed read, but worth a follow-up batch-RPC (`GetPendingGatesForTransitions(itemId, []to)`) if
this ever shows up in profiling.

**Neutral**: the settings page's new `?editStage=` param is additive and has no effect when absent —
existing bookmarks/links to `/settings/backlog-stages` are unaffected.

---

## Revisions from review

Adversarial review (2026-09-11) verified every load-bearing factual claim in this ADR against the
actual code (the liveness panel's existing wiring, `item.allowedTransitions`'s real presence,
`OverrideVerdict`'s actual bypass of gate checks, `useBacklogStages()`'s real shape) and found two
gaps in the original Decision, both folded in above rather than left as follow-ups: (1) the
frozen-snapshot check must distinguish disabled-but-present stages from truly-live ones, not just
check slug presence, since the backend treats disabled and deleted identically; (2) `archived` is
excluded from the per-edge gate-checklist loop, since it is an unconditional escape hatch everywhere
else in this codebase and showing it as a "blocked" edge would contradict that.
