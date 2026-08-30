# Research Summary: backlog-status-visibility

**Date**: 2026-08-01

No independent Phase 2 research was run for this item. `project_plans/backlog-session-lifecycle-ux/research/` already contains a complete, current (2026-08-01) 5-file research set covering the identical problem statement:

| File | Covers |
|---|---|
| `stack.md` | Existing tech (ent, ConnectRPC, vanilla-extract, `Collapsible`/`useShowMore` primitives) — no new dependency needed. |
| `features.md` | What already-shipped precedents (`BacklogStatusEvent`, `BacklogProgressNote`, `WorkflowHistorySection`) to model the new respawn-event trail on. |
| `pitfalls.md` | Confirms `end_reason`/`pause_reason`/`remediation_attempts` are already wired to the proto/DTO layer (so this is UI-wiring, not new plumbing, for 2 of 3 asks); flags real operational volume data (15+ restarts/day, 6 items respawned in one sweep) that later informed the pre-mortem's "hundreds of rows, not tens" finding. |
| `ux.md` | Progressive-disclosure placement, accessibility triad (icon + visible text + `aria-label`), 11-point UX acceptance criteria. |
| `build-vs-buy.md` | Build (all pieces are thin wiring over existing infra); no third-party option considered or needed. |

Re-running research here would reproduce the same code-reading (same files, same commit history) against an unchanged codebase and is skipped as redundant — see `requirements.md`'s duplicate-work finding for the reasoning.

**One gap this pass adds**: neither research set investigated whether a *second* backlog item (this one, `0a366262…`) triaging to the same conclusion as an already-in-flight manual effort is itself a signal worth feeding back into the backlog automation (e.g. a similarity/duplicate-detection check before an item reaches `idea`→triage). Not pursued further here — out of scope for this item's own delivery, flagged as a suggestion in the final triage output instead.
