# Adversarial Review: cross-host-claim-dedup

**Date**: 2026-09-23 (re-review, iteration 2 of 3 — scoped to the 3 previously-BLOCKED items)
**Verdict**: CLEAN

## Blockers

None. All three prior blockers are resolved in the current `plan.md`/ADR-002.

### Resolved: `ImportGitHubIssue` hard block with no override path

Story 2.2.1 was rewritten with an explicit "Revision note (resolves adversarial-review Blocker
1)" (`plan.md` lines 663–675). The behavior is no longer a hard RPC-level block:

- A `ClaimHeldByOther` verdict now returns a structured `already_claimed_elsewhere` outcome
  (not a bare `connect.CodeAlreadyExists` collapsed into a generic-failure bucket), naming
  `claiming_host_id`/`item_deep_link`, and still creates no item **by default**.
- A new `override: true` field on the import request (Task 2.2.1a; `proto/session/v1/session.proto`
  change listed under Story 2.2.1's Files) lets the operator resubmit and create the item anyway,
  logged as `import_github_issue.claim_override` for auditability — an in-product recovery path
  that doesn't require disabling the feature flag.
- Task 2.2.1c adds a test for the override path specifically (row IS created + log line present).

This directly answers the original blocker's recommendation. One loose end (see Minors): the
UX design doc (`design/ux.md` Surface 3) still contains its old "No Override is offered at this
surface... deliberate design constraint" text; `plan.md`'s revision note explicitly calls this
"now-stale" and supersedes it, but `design/ux.md` itself was not edited to match, so a reader of
that file alone would still see the superseded design. Not a blocker — plan.md is unambiguous
about which document wins — but worth a follow-up edit.

### Resolved: `RecordClaim` never called against production data in Phases 0-4

`plan.md` now has **Epic 1.3 / Story 1.3.1** ("Wire `RecordClaim` into `Storage.CreateBacklogItem`
via `session.ClaimRecorder`", lines 474–539), sequenced in Phase 1 — before Phase 2's checker
integration — with no #475 dependency:

- AC: any of the three existing creation paths, through `Storage.CreateBacklogItem`
  (`session/storage.go:906`), results in a `ClaimRecord` appearing in the local `ClaimIndex`
  keyed off the local `session.HostIdentity.ID`.
- AC: a `RecordClaim` failure never fails item creation (best-effort, matches ADR-002).
- AC: a nil recorder is a no-op (no behavior change pre-wiring).
- Tasks 1.3.1a–d cover the port definition, the call site, wiring from
  `server/dependencies.go`, and tests.

The Dependency Visualization and Risk Control sections were also corrected to state this
explicitly (lines 190–200): "Story 1.3.1 is what makes this true in practice, not just in
principle: without it, `RecordClaim` is defined but never called against production data, and
Phases 0-4 alone would be functionally inert." The "Phases 0-4 fully shippable" claim is no
longer misleading — the write side is now a real, sequenced task, not deferred into
#475-blocked Phase 5.

### Resolved: import-cycle at the ADR-002 choke point

Both `plan.md` and `ADR-002-record-claim-at-single-choke-point.md` (which now has an explicit
"Correction (post-review)" section) converge on the same concrete fix: a new port,
`session.ClaimRecorder` (`RecordClaim(ctx, record) error`), **defined in package `session`**,
wired via `Storage.SetClaimRecorder(...)` — mirroring the existing
`Storage.SetCallbackDispatcher`/`Storage.SetItemChangePublisher` precedent
(`session/storage.go:284-292`) exactly. `crossHostClaimChecker` (`server/services/claim_checker.go`)
is narrowed to a read-only `CheckClaim` method only — Task 2.1.1a/b/c explicitly note it "does
not have a `RecordClaim` method" and that nothing in the plan calls
`crossHostClaimChecker.RecordClaim`. This is option (a) from the original blocker's
recommendation, cleanly resolving the cycle: `session` never imports `server/services`, and the
concrete `ClaimRecorder` implementation (composing `*ClaimIndex`/`*ClaimGossiper`, both already
`session`-package types) lives entirely in `session` too.

## Concerns

None outstanding. The fix pass also addressed all three Concerns from the prior review:

- **Peer-fanout-in-triage-loop** — Story 2.2.2 was rewritten ("Revision note (resolves
  adversarial-review concern)", lines 735–744) to consult **only the local `ClaimIndex`**
  (gossip-propagated, on-disk) inside `DequeueNextQueuedItems`'s locked loop, never issuing a
  live peer query. The synchronous fan-out (Story 2.1.2) stays reserved for the
  interactively-triggered paths (`ImportGitHubIssue`, `check_cross_host_claim`). An explicit AC
  now states "the check never issues a live peer query," with the false-negative tradeoff named
  as accepted, not a bug.
- **Build-vs-buy GitHub-label pre-check disposition** — now has an explicit Pattern Decisions row
  ("Rejected for v1, not silently dropped") citing the source and reasoning.
- **Prune()-to-fanout AC linkage** — Story 2.1.2 gained an explicit AC: "A peer excluded by
  `HostRegistry.Prune()` is excluded from the fan-out set," tying Story 0.1.1's fix to the
  fan-out set directly rather than relying on sequencing alone.

As a byproduct of the Story 2.1.2 rewrite, the plan also added a new AC not in the original
review's scope but worth noting: the peer fan-out must be parallel with one shared deadline
(not sequential per-peer), with a test asserting elapsed time stays near one timeout rather than
N timeouts — closes a latency risk of the same shape as the concerns above.

## Minors

- ADR-001's rebuttal of `research/build-vs-buy.md`'s "Bottom line" (which literally recommends
  "extend `AdvertisementRecord`/`HostRegistry`") as being "about not adopting an external gossip
  library, not about the wire-format question" is a generous reading — build-vs-buy.md's own
  words do address wire format, not just library choice. ADR-001's independent reasoning
  (payload growth, signing scope, lifecycle mismatch) stands on its own regardless, so this
  doesn't change the decision, but the citation could be tightened. (Unchanged from prior review
  — ADR-001 was not touched by this fix pass.)
- This item's own two decision docs still live under
  `project_plans/cross-host-claim-dedup/decisions/`, the same "not in `docs/adr/`" location
  Story 0.1.2 is built to remediate for the *other* ADR-002. Mitigated in practice because
  Task 1.1.1a's doc comments use full relative paths rather than a bare "ADR-001"/"ADR-002"
  string. (Unchanged from prior review.)
- Task 0.1.1a still ticks the new prune loop on `session.DefaultHostAdvertisementInterval`
  (5 min) rather than a cadence derived from `DefaultHostRegistryTTL` (15 min); reasonable but
  still not explicitly justified in the task text. (Unchanged from prior review.)
- **New**: `design/ux.md` Surface 3 ("Already claimed" state during GitHub issue import) still
  contains the old "No Override is offered at this surface... This is a deliberate design
  constraint, not an oversight" text (around its "Interaction flow" step 4), which `plan.md`'s
  Story 2.2.1 revision note explicitly calls out as superseded by the new override support.
  `plan.md` is unambiguous about which document is authoritative, so this doesn't block
  implementation, but `design/ux.md` should get a follow-up edit (the wireframe/interaction flow
  need an Override affordance added to Surface 3) so a UI implementer reading only that file
  isn't misled.

## Note on review conditions

`plan.md` was under active, continuous edit by a concurrent fix pass while this re-review began
(observed growing from 803 → 966 → 1003 → 1028 → 1051 → 1065 → 1067 lines across several reads
within ~15 minutes). This review was deliberately held until the file (and the two ADRs) were
confirmed stable — no size/mtime change for 2+ consecutive minutes — before the findings above
were finalized, to avoid reviewing a moving target.
