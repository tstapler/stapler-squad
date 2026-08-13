# ADR-001: Completion Summary — Generation Scope and Storage Independence from the Session Row

**Status**: Superseded by [ADR-002](./ADR-002-generation-trigger-scope-both-events-via-precleanup-snapshot.md) (2026-08-03)
**Date**: 2026-08-02

> **2026-08-03 update**: A later SDD pass re-ran ideation against the actual
> backlog item (`59bbff11-ee8b-418c-8484-64307cb14244`) and produced
> `requirements.md`/AC-1/AC-7, which explicitly require both `EventExited`
> *and* `EventStopped` to produce a full, retrievable, LLM-narrated summary —
> directly reversing this ADR's Decision §1 and §3 (EventExited-only,
> deterministic-only-on-stop). ADR-002 records the updated decision and the
> concrete pre-cleanup snapshot mechanism (`Instance.UpdateDiffStats()` called
> before `CleanupWorktree()`/`fireLifecycleEvent`) that makes full-fidelity
> generation on the `Destroy()` path safe despite the storage-deletion race
> documented below, which is still accurate and still the reason a naive
> "just also handle EventStopped" change would be wrong. Read this ADR for
> that race's evidence; read ADR-002 for the resolution actually implemented.

## Context

`research/architecture.md` recommended attaching the completion summary to the
`Session` ent row as a new TEXT column (Pattern B, like `session_artifacts`),
and reacting to both `EventExited` and `EventStopped` uniformly (mirroring
`instanceBacklogListener.OnLifecycleEvent`'s `case EventExited, EventStopped:`
in `session/backlog_lifecycle.go:797`).

While grounding the plan against the actual `DeleteSession` RPC handler
(`server/services/session_service.go:1953-2033`, the only UI-driven path for
ending a session — `SessionDetailView.tsx`'s `handleDeleteClick` at line 416
is the sole termination action in the frontend) and the `stop_session` MCP
tool (`server/mcp/tools_lifecycle.go:298-352`), a stronger constraint emerged
that the research passes did not surface:

```go
// server/services/session_service.go:1996-2023 (DeleteSession)
s.removeFromAllPollers(sessionTitle)          // synchronous
if inst := s.FindLiveInstance(sessionTitle); inst != nil {
    go func() { _ = inst.Destroy() }()        // ASYNC — worktree cleanup happens later
} else { ... }
...
if cancelled := s.approvalStore.CancelSession(sessionUUID); ... { ... }
if err := s.storage.DeleteInstance(sessionTitle); err != nil { ... }  // SYNCHRONOUS, unconditional
s.eventBus.Publish(events.NewSessionDeletedEvent(sessionUUID))
```

`s.storage.DeleteInstance` deletes the `Session` ent row synchronously inside
the RPC handler, **before** (or racing) the async `inst.Destroy()` goroutine
that fires `EventStopped`. By the time any `LifecycleListener` reacting to
`EventStopped` runs, the `Session` row is already gone. A completion summary
stored as a column on that row (Pattern B) or as a child entity with a
required unique edge back to `Session` (Pattern A as used by `DiffStats`/
`ReviewVerdict`) would be unwritable at that point — an `UPDATE`/upsert
against a deleted row affects zero rows, exactly the `n == 0` failure mode
`EntRepository.UpdateSessionArtifacts` already guards against for a *live*
row, but which has no recovery path once the row is truly gone.

This also breaks requirements.md AC-3 ("retrievable from the session's
history/detail view") for that path on its own terms: once `DeleteSession`
completes, there is no `Session` object left for `SessionDetailView.tsx` to
route to — the summary would be unreachable through any session-scoped UI
regardless of where it's stored.

By contrast, the **natural-exit path** (`instanceOnExitCallback`,
`session/instance.go:788-811`, firing `EventExited` when the wrapped `claude`
process exits on its own) never calls `Destroy()` — the worktree and the
`Session` row both remain intact. This is the path that actually matches the
requirements' framing ("when an agent session ends" — the agent finished, not
"the user deleted the session").

## Decision

1. **Scope automatic, persisted generation strictly to `EventExited`
   transitioning `Status: Active → Stopped`** (`session/instance.go:805-806`,
   `811`). This is the only path where AC-3's "retrievable from history/detail
   view" is achievable, because it's the only path that doesn't delete the
   `Session` row. Do **not** react to `EventStopped` (fired by `Destroy()`) in
   the async listener — reacting there would (a) still race the
   `CleanupWorktree()`/`CancelSession`/`DeleteInstance` sequence already
   documented as a hazard in `research/pitfalls.md` §1–2, and (b) for the
   `DeleteSession` path specifically, attempt to persist against a row that
   may already be gone.
2. **Give `CompletionSummary` its own ent entity, keyed by a plain indexed
   `session_uuid` string field — deliberately with no `edge.From(...).Unique().Required()`
   back to `Session`.** Unlike `DiffStats`/`ReviewVerdict` (which assume their
   parent `Session`/`ItemSession` row outlives them), a `CompletionSummary`
   row must be able to survive its originating `Session` row being deleted —
   see point 3. Modeled instead on `ClassificationAnalytics.session_id`
   (`session/ent/schema/classificationanalytics.go`), which is already an
   `Optional()` plain field, not an edge, for the same reason: analytics rows
   already outlive the sessions that produced them in this codebase.
3. **For the explicit `DeleteSession`/`stop_session` teardown path, take a
   cheap, synchronous, deterministic-only (no LLM call) snapshot and persist
   it *before* the async `Destroy()` goroutine is spawned** — i.e. before the
   worktree is torn down and before `storage.DeleteInstance` runs. This is
   best-effort: it satisfies "the document exists at the moment of deletion"
   (useful for a copy-before-you-delete UX) but is explicitly **not** promised
   to remain reachable through `SessionDetailView` afterward, since there is
   no session left to view it from. This limitation is intentional and
   in-scope for v1 — a future "view completion summaries for deleted
   sessions" surface is out of scope, matching requirements.md's own
   deferral pattern for adjacent features (PR creation, issue closure).

## Consequences

- `CompletionSummary` rows are **not** cascade-deleted when a `Session` row is
  deleted — they become orphaned (session_uuid points at a row that no longer
  exists). No eviction policy exists for this today (mirrors the existing gap
  already noted for `session_artifacts` in `research/pitfalls.md` §9) — an
  explicit non-goal for this pass, not an oversight.
- The two generation paths (natural exit vs. explicit stop) are asymmetric by
  design: natural exit gets the full pipeline (deterministic snapshot + async
  LLM narrative + regenerate affordance); explicit stop gets a synchronous,
  deterministic-only, one-shot snapshot with no narrative and no regenerate
  affordance (the session is gone, there's nothing to regenerate against).
  This asymmetry is the direct, evidence-based consequence of §1's finding,
  not an inconsistency to "fix" later without revisiting this ADR.
- `Hibernated`/`Paused`/`Restoring` transitions never trigger generation
  (matches `research/pitfalls.md` §4's recommendation) — and because
  `Instance.Resume()` (`session/instance.go:1364`) requires
  `Snapshot().Status == Paused` (never `Stopped`), and no code path resumes a
  `Stopped` instance today, the "staleness after resume" concern from
  requirements.md's Open Question 2 does not apply under current code. If a
  future feature adds resume-from-`Stopped`, it must add explicit
  invalidation/regeneration for any existing `CompletionSummary` row at that
  time — flagged here for that future work, not solved now.
