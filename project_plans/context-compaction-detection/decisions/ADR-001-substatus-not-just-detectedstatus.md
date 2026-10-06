# ADR-001: Populate `SubStatus.COMPACTING` in Addition to `DetectedStatus.COMPACTING`

**Status**: Accepted
**Date**: 2026-08-06

## Context

`requirements.md`'s acceptance criteria (AC2, AC3, AC5) literally scope this feature to the internal `DetectedStatus` Go enum and its mapping into the proto `DetectedStatus` enum, plus a `deriveWorkingState.ts` mapping branch. Nothing in the requirements' "Existing System" section or its acceptance criteria mentions the separate `SubStatus` proto enum.

However, `architecture.md` and `ux.md` (Phase 2 research) traced the actual rendering path for the session-card badge and found a precedence rule that the requirements' framing missed:

- `SessionCard.tsx:533-548` renders `SubStatusChip` (driven by the proto `SubStatus` field) as the *primary* badge whenever `session.subStatus` is not `UNSPECIFIED`/`IDLE`.
- `StatusBadge` (driven by the proto `DetectedStatus` field, which is what AC2/AC3/AC5 literally scope to) is only shown as a *fallback*, gated on `session.subStatus === UNSPECIFIED || session.subStatus === IDLE`.
- During compaction, `SubStatus` will typically already be non-idle (most often `PROCESSING`, via `toProtoSubStatusFromInfo`'s existing `StatusProcessing, StatusExecuting → SUB_STATUS_PROCESSING` case at `server/adapters/instance_adapter.go:251`), because the session was already Processing/Executing immediately before compaction kicked in.

If this plan implemented AC2/AC3/AC5 exactly as literally written — `DetectedStatus` only — the new signal would be correctly computed on the backend, correctly serialized over the wire, correctly consumed by `deriveWorkingState.ts`'s fallback switch... and then never rendered, because `SubStatusChip` (driven by the still-`PROCESSING` `SubStatus` field) would keep winning the precedence check and showing the generic "Thinking…" chip instead. AC6 ("session card UI shows a visually distinct badge... `⟳ Compacting context`") would silently fail despite every other acceptance criterion passing.

## Decision

Add `SUB_STATUS_COMPACTING = 11` to the `SubStatus` proto enum (`proto/session/v1/types.proto:412-434`) alongside `DETECTED_STATUS_COMPACTING = 12` on `DetectedStatus`, and populate it in **both** places that actually feed the RPC response:

- `server/adapters/instance_adapter.go`'s `toProtoSubStatusFromInfo` (the function wired to the real session-list/watch RPC path, per `instance_adapter.go:167`)
- `server/adapters/review_queue_adapter.go`'s `subStatusFromItem` (the review-queue equivalent)

The two nominally-authoritative package-level functions in `session/detection/proto_mapping.go` (`DetectedStatusToProto`, `DetectedStatusToSubStatus`) are also updated for consistency and because tests exist against them directly, even though `DetectedStatusToSubStatus` currently has zero call sites on the actual RPC path (confirmed via `features.md`'s research — the two adapter functions above duplicate its logic instead of calling it, despite `DetectedStatusToSubStatus`'s own doc comment forbidding that duplication; fixing that duplication is out of scope for this plan).

## Consequences

- **Positive**: AC6 actually works — the badge renders during compaction, because `SubStatusChip` receives `SUB_STATUS_COMPACTING` instead of falling back to a stale `PROCESSING` value.
- **Positive**: `deriveWorkingState.ts`'s *primary* switch (on `SubStatus`) also gets a correct, direct mapping rather than relying solely on the fallback switch, which only fires when `SubStatus` is `UNSPECIFIED` — a condition that won't hold for real compacting sessions in practice.
- **Cost**: two more proto enum values instead of one, and two more manual (non-lint-enforced) Go switch sites (`instance_adapter.go`, `review_queue_adapter.go`) beyond what AC2/AC3 literally named. This is the single largest deviation from requirements.md's literal wording in this plan, which is why it's recorded as an ADR rather than a routine task addition.
- **Neutral**: no proto backward-compatibility risk — both new values are strictly appended, matching the existing append-only discipline for these enums (see plan.md's Pattern Decisions table on proto enum numbering).

## Alternatives Considered

1. **`DetectedStatus` only, exactly as AC2/AC3/AC5 specify.** Rejected: as described above, this compiles, passes unit tests written against `DetectedStatus`/`deriveWorkingState.ts` in isolation, and still fails AC6 in production because `SubStatusChip`'s precedence rule suppresses the fallback `StatusBadge` path whenever `SubStatus` is already non-idle — which it will be during real compaction.
2. **Change `SessionCard.tsx`'s precedence rule instead** (e.g., make `StatusBadge` win over `SubStatusChip` when `DetectedStatus === COMPACTING`, regardless of `SubStatus`). Rejected: this special-cases the rendering logic for one status instead of extending the enum, inverts an established precedence convention that every other status already follows, and would require its own new conditional in `SessionCard.tsx` that the "additive only, no side effects on other states" requirement (AC7's spirit) is specifically trying to avoid touching.
