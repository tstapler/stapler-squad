# Requirements: shipped-snapshot-display

**Date**: 2026-08-29
**Item ID**: 9832b7e3-edf8-469f-af79-e128604904f6
**Type**: verification / stale-item close-out (not a new feature)
**Complexity**: 1 — investigation confirmed the ask is already implemented

## Problem Statement (as filed)

The backlog item claims: durable "shipped snapshot" fields
(`shipped_check_conclusion`, `shipped_approved_count`, `shipped_changes_req_count`,
`shipped_file_stats`, `shipped_snapshot_capture_failed` — defined at
`session/ent/schema/backlog_item.go:97-123`) are captured by the backend but never
read or displayed by the frontend, because `web-app/src/lib/hooks/useBacklogService.ts`'s
`BacklogItem` conversion doesn't map them.

## Investigation Finding — the premise is stale

The literal claim about `useBacklogService.ts` is true (that file's `mapBacklogItem`
does not touch these fields), but that is not the path these fields actually take to
reach the UI, and that path is already fully built and shipped:

1. The five ent fields are exposed on a **separate** proto message,
   `BacklogItemShipStatus` (`session.v1`, fields 13-16 +
   `shipped_snapshot_capture_failed` → `snapshotCaptureFailed`), returned by the
   **`GetBacklogItemShipStatus`** RPC (`server/services/backlog_service_ship_status.go`),
   not by `BacklogItem`/`GetBacklogItem`.
2. The frontend already calls this RPC via
   `web-app/src/lib/hooks/useBacklogItemShipStatus.ts`, used from
   `web-app/src/components/backlog/BacklogItemDetail.tsx:181-182` as the fallback
   once the live per-session VCS data (`useVcsStatus`) is unavailable — i.e. exactly
   the "PR closed, live data gone" scenario the backlog item describes.
3. `web-app/src/lib/vcs/adapters.ts`'s `fromShipStatus`/`fromShipStatusGithub`
   already map all five fields into `VcsWidgetData`/`GithubSummary`
   (`checkConclusion`, `approvedCount`, `changesReqCount`, `fileChanges`,
   `snapshotCaptureFailed`, `snapshotAt`).
4. That data is already rendered:
   - `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.tsx` renders the
     CI conclusion badge, approved/changes-requested review counts, and a
     "Couldn't capture PR status at ship time" message when the snapshot capture
     failed.
   - `web-app/src/components/shared/VcsWidget.tsx` renders the file-stat list
     (`VcsWidgetFileList`) and the "As of `<relative time>`" snapshot timestamp,
     plus a neutral "shipped before detailed tracking was added" message when no
     snapshot exists at all.
   - Both are wired into `web-app/src/components/backlog/detail/VersionControlSection.tsx`,
     which is rendered from `BacklogItemDetail.tsx` for every item status except
     when `PullRequestSection` alone owns the PR-link line (`pr_pending`).
5. This was shipped in commit `aedd80648` ("feat(vcs-widget): render durable
   snapshot data + capture-failure copy (Phase 4)", 2026-07-18), confirmed on
   `main` (`git merge-base --is-ancestor aedd80648 origin/main` → true), with
   direct unit coverage of all five fields in
   `web-app/src/lib/vcs/adapters.test.ts:130-180`.

**Conclusion:** the two-part "ask" in the backlog item —(1) map the fields into the
frontend, (2) render them as a fallback once the live PR is closed — is done, via
`BacklogItemShipStatus` → `useBacklogItemShipStatus` → `fromShipStatus` →
`VcsWidgetGithubRow`/`VcsWidget` → `VersionControlSection`, not via
`useBacklogService.ts`'s `BacklogItem`. The item was filed against the wrong
plumbing path (`BacklogItem`/`useBacklogService.ts`) without checking whether the
same data reached the UI through the ship-status RPC instead.

## Residual Gaps (real, but narrower than the original ask)

None of these were named in the original ask; they are genuinely small polish items
surfaced while confirming the display path, not blockers to closing the item:

1. **List/board views show nothing.** `BacklogItemCard.tsx` never surfaces
   CI conclusion or review counts (live or shipped) — but this is equally true for
   *live* PRs, so it's a pre-existing, unrelated scope decision, not a shipped-
   snapshot-specific gap.
2. **No visible regression guard beyond the adapter/unit level** — there's no
   Playwright e2e assertion that a `pr_pending`→shipped item's detail view
   actually shows the snapshot CI/review badges once the live worktree is gone.
   `adapters.test.ts` covers the pure mapping function; nothing exercises the
   rendered DOM end-to-end.

## Scope

### In Scope
- Confirm (this document + research/plan) that the originally requested read/display
  path already exists and works as specified.
- Decide and record the disposition: close as already-implemented vs. add the one
  plausible follow-up (an e2e/component regression test pinning the rendered
  behavior), scoped small.

### Out of Scope
- Any change to backend capture logic (explicitly out of scope in the original ask).
- Adding CI/review badges to list/board card views (unrelated pre-existing gap, not
  requested).
- Any UI redesign of `VcsWidgetGithubRow`/`VcsWidget`.

## Acceptance Criteria (superseding the original ask)

1. The five ent-schema shipped-snapshot fields are confirmed to reach the UI via
   `BacklogItemShipStatus` → `useBacklogItemShipStatus` → `fromShipStatus` →
   `VcsWidgetGithubRow`/`VcsWidget`, with a cited commit/test showing this shipped.
2. `VersionControlSection` is confirmed to render as the fallback once the live PR
   is closed and `useVcsStatus` no longer has data (i.e., `vcsStatus` is falsy and
   `shipStatus` is used).
3. A short regression test exists (component or e2e) asserting that a shipped item
   with a captured snapshot displays CI conclusion + review counts, and that a
   snapshot-capture-failure renders the "couldn't capture" copy — closing the one
   real coverage gap found (Residual Gap #2), scoped to whichever of
   component/e2e is cheaper given existing test scaffolding.
4. Backlog item 9832b7e3-edf8-469f-af79-e128604904f6 is annotated/closed as
   already-implemented rather than worked as a net-new feature.

## Non-Functional Requirements
- No backend changes; no proto changes; no new dependencies.
- Any new test must run under the existing `cd web-app && npx jest --no-coverage`
  or `tests/e2e` harness — no new tooling.

## Rabbit Holes
- Do not re-plumb `useBacklogService.ts`'s `BacklogItem` to also carry these
  fields "for consistency" — that would be a second, redundant data path for data
  already available via `useBacklogItemShipStatus`, and no consumer needs it there.
