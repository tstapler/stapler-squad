# Research: Pitfalls & Risks — backlog-agent-communication

**Date**: 2026-07-23

## Direct Evidence From Today's Bug Docs (BUG-040 – BUG-045)

These are not analogies — they are the concrete, already-reproduced failure shapes
any new communication/escalation surface must be designed against.

- **BUG-040** (`pr_pending` item loses its PR reference): two independent
  write-ordering defects — (1) a best-effort field persist whose failure was only
  logged, never gated on; (2) a destructive field-clear that ran *before*
  confirming the compensating action (reopen) actually succeeded. **Pitfall for
  this project**: any new "structured handoff" or "PR metadata" write introduced
  here must not repeat either shape — check writes that matter, and never clear a
  reference before confirming its replacement landed. The fix pattern used
  (`stayInReviewAndNotify` on persist failure; re-fetch-and-check before destructive
  clear) is the concrete template to reuse.
- **BUG-041** (backlog nudge retry never backs off on send failure) — a *different*
  retry path than `RemediationDue` that lacked the same backoff discipline.
  **Pitfall**: if this project introduces new retry/notification loops (e.g.
  repeated "still waiting for human on this escalation" nudges), they must reuse
  `RemediationDue`/the existing backoff schedule, not invent a parallel one that
  can independently drift into a hot loop.
- **BUG-042** (orphaned control-mode tmux clients overload the tmux server, open at
  time of this research) — a live example of exactly the class of "infrastructure
  itself is broken" problem dimension 2 is meant to surface. Notably: this bug was
  found and filed *manually* by an agent noticing the problem while doing unrelated
  work, not through any structured tool. **Pitfall**: don't assume dimension 2's
  gap is "agents can't report infra problems" — they clearly can and do (six bugs
  filed in one day proves the capability exists). The actual gap is more likely
  *distinctness of signal* (does this report get triaged with the same urgency and
  routing as an item-specific failure?) and *discoverability* (a human has to
  notice a new markdown file) — design dimension 2 against the real gap, not an
  imagined one.
- **BUG-043** (chronic abandoned-review respawn failures) and **BUG-044** (unbounded
  PR branch drift from main) — both examples of a stuck condition that kept
  recurring because the *cause* was structural, not transient — exactly the shape
  an "ask for help" escalation should catch *before* `MaxRemediationAttempts`
  exhausts and parks the item silently for up to 72h between attempts. **Pitfall**:
  if "ask for help" is purely agent-initiated and voluntary, an agent that doesn't
  recognize it's in a chronic-failure shape (because each individual attempt looks
  locally reasonable) will never call it — dimension 3 may need to compose with a
  *reconciler-side* trigger too (e.g. auto-suggest escalation once a
  `StuckReason` row nears its attempt cap), not purely agent self-report. Flag this
  explicitly in planning as a design question, not an assumption.
- **BUG-045** (review fallback reads the shared main checkout instead of the item's
  actual state, open at time of this research) — directly threatens any
  "fresh re-reviewer adjudicates a dispute" design for pain point B: a dispute
  re-review needs a *worktree that still exists*, or it inherits this exact bug and
  produces a second wrong verdict instead of resolving the first one. **Pitfall**:
  do not design the dispute-adjudication flow assuming the review environment is
  trustworthy — either require the re-review to run before worktree reap
  (timing constraint) or treat BUG-045 as a blocking prerequisite bug for whichever
  part of the plan depends on codebase-read re-review.

## General Multi-Agent Escalation/Communication Pitfalls

- **Escalation as an all-purpose crutch**: LLM agents facing a genuinely hard task
  can over-escalate rather than trying harder, if escalating is low-cost and
  research/self-recovery is comparatively effortful. Mitigation pattern already in
  this codebase for the *analogous* automated-retry problem: cap + backoff
  (`MaxRemediationAttempts`, `remediationBackoffSchedule`). An agent-initiated
  "ask for help" tool needs an equivalent discipline — e.g. require evidence of at
  least one self-directed remediation attempt in the tool's own arguments schema,
  not just a free pass to call it immediately.
- **Structured-data schema drift**: any new JSON blob field (e.g. richer
  `verification_notes`, structured findings on a verdict) that isn't validated at
  the boundary can silently become an untyped grab-bag over time — this repo's own
  `AcCriteriaJSON`/`ParseAcCriteria` and `ReviewVerdictData.PerCriterion` patterns
  show the house style: a named JSON-string type + explicit `Parse`/`Serialize`
  functions, not `map[string]interface{}`. New structured fields in this project
  should follow that exact pattern.
- **Silent-drop on best-effort writes**: several existing comments in
  `tools_backlog.go` explicitly call out "best-effort... a failure here must not
  fail the call that already succeeded above" (e.g. `AppendProgressNote` after
  `UpdateAcCriterionStatus`). This is a deliberate, reasonable pattern for
  *secondary* enrichment data — but BUG-040 root cause #1 shows the same
  "best-effort, only logged" pattern is dangerous when applied to *primary* data
  (the PR reference itself). Any new tool in this project must classify each field
  it writes as primary (block/error on failure) vs. secondary
  enrichment (log-and-continue is fine) and be explicit about which is which.
- **New `StuckReason` proliferation without a corresponding UI update**: adding a
  `StuckReason` constant requires touching 5+ files (domain enum, `IsValid`,
  proto enum + regen, backend proto mapping, frontend exhaustive
  `Record<StuckReason,T>` maps) — BUG-040's fix shows the full checklist. A design
  that adds 2-3 new `StuckReason` values (plausible for dimensions 2/3) must budget
  for this multi-file touchpoint cost explicitly in the plan phase's task breakdown,
  not treat it as a single-file change.
- **Human notification fatigue from a solo operator's perspective**: this project
  explicitly serves *one* human. Every new `Notifier.Notify` call site competes for
  the same attention budget as existing ones (stuck-parked, review-without-verdict,
  etc.) — an escalation/dispute/infra-report system that fires notifications too
  eagerly degrades the value of *all* notifications, including the ones that
  already work well today. Planning should default new signals to the durable
  `/unfinished`-style surface (checked in batches) over ephemeral push notification,
  reserving push for genuinely time-sensitive cases.
- **Reviewer/implementer model bias in self-adjudicated disputes**: if a dispute is
  adjudicated by re-running the *same* review criteria with the *same* underlying
  model (even a "fresh" session), there is a real risk of correlated
  errors — the fresh reviewer may reach the same wrong conclusion for the same
  underlying reason (e.g. BUG-045's environment bug) rather than an independent
  check. This is a concrete argument, grounded in this repo's own live bug, for
  defaulting pain point B's adjudication to a human rather than purely automated
  re-review, at least until BUG-045-class environment risks are closed.

## Composability Risks (duplicating vs. reusing existing machinery)

- **Risk**: designing "ask for help" as an entirely new subsystem (new tool, new
  table, new UI page) when 80% of the plumbing (`StuckReason`, `RemediationDue`,
  `Notifier`, `/unfinished`) already exists and is battle-tested (11 stuck reasons,
  multiple fixed bugs proving the reconciliation model works). The requirements.md
  constraint to compose rather than duplicate is not just an aesthetic
  preference — every new parallel subsystem is another surface with its own BUG-04x
  potential (see BUG-041's independent-retry-loop shape as a cautionary tale of
  what happens when a *similar but separate* mechanism drifts from the
  well-tested one).
- **Risk**: forgetting that `PipelineEngine`/`PipelineModeRepository` is a *live,
  actively-developed* orthogonal system — any new MCP tool must not assume a fixed
  set of session roles (`work`/`triage`/`review`) is permanent; if pipeline modes
  become user-configurable with more/different stage roles, tools scoped to "the
  work session" or "the review session" by role string should be written against
  the `SessionRoleWork`/`SessionRoleReview` constants (not raw strings) so future
  role additions don't require re-auditing every call site.
