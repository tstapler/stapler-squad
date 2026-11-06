# Validation: Backlog Compose — Background Creation + Image Fix

## Acceptance Criteria Checklist

### AC-1: Images survive AI parsing (legacy intent-review path)

**Test scenario**: Type "fix the login bug" + attach a PNG in
`BacklogComposeWidget`. Submit via the "backlog: " prefix path (which goes
through `BacklogItemIntentReview`). Open the created item.

**Expected**: The item's description contains the `![filename](/api/local/serve/…)`
markdown reference. The image renders in the description's markdown preview.

**Validates**: F-5 (image preservation), A-1 (prompt fix), A-2 (client fallback).

**Automated test**: `BacklogItemIntentReview.imagePreservation.test.tsx` — unit
test of `draftToInitialValues` with a stubbed LLM response that omits the image.

### AC-2: Compose widget creates item immediately and closes

**Test scenario**: Open the omnibar → "Create backlog item" → type text → submit.

**Expected**:
1. The compose dialog closes in ≤1 second (no "Parsing…" spinner shown).
2. A toast notification appears: "Backlog item created — triage starting".
3. The item appears in the backlog list.
4. The item's `triageStatus` shows `running`.

**Validates**: F-1, F-2.

**Automated test**: `Omnibar.backlogCompose.test.tsx` — mock
`createBacklogItemFromChat` to resolve immediately; verify dialog closes,
notification fires, no transition to intent review.

### AC-3: Triage-complete notification fires

**Test scenario**: Create a backlog item via compose. Wait for triage to
complete (test: mock `WatchBacklogItems` emitting an `item_updated` event with
`triageStatus = "completed"`).

**Expected**: A notification appears: "Triage complete — \<item title\> is
ready for review". Clicking "View" navigates to `/backlog?item=<id>`.

**Validates**: F-3.

**Automated test**: `useBacklogCreationNotifier.test.ts` — inject a mock items
array with `triageStatus = "completed"` after tracking the item; verify
`addNotification` is called with the expected message.

### AC-4: Guidance-needed notification fires

**Test scenario**: Create an item via compose. Mock `WatchBacklogItems` emitting
an update with `hasPendingGuidanceRequests = true`.

**Expected**: A notification appears: "AI needs your input — \<item title\> has
a question for you". Clicking "Answer" navigates to `/backlog?item=<id>`.

**Validates**: F-4.

**Automated test**: `useBacklogCreationNotifier.test.ts` — same mock pattern.

### AC-5: `GuidanceRequestPanel` unchanged; shows questions on item detail page

**Test scenario**: Navigate to the item detail page after receiving the guidance
notification.

**Expected**: `GuidanceRequestPanel` is present and shows the pending question(s).
No new component is introduced; the panel is reused as-is.

**Validates**: F-4 (no rebuild), NF-1.

**Automated test**: Existing `GuidanceRequestPanel.test.tsx` passes unchanged.

### AC-6: Compose-widget images are preserved in background-created items

**Test scenario**: Attach a PNG in the compose widget. Submit (new background
path). Open the created item.

**Expected**: The item's description contains the `![filename](/api/local/serve/…)`
reference. The image renders.

**Validates**: F-5, F-6. This is guaranteed by using raw text as description in
`CreateBacklogItemFromChat` — no LLM rewrite involved.

**Automated test**: `Omnibar.backlogCompose.test.tsx` — verify that
`createBacklogItemFromChat` is called with the full text including the image
markdown.

### AC-7: Items created by other means do not trigger compose notifications

**Test scenario**: Import a GitHub issue or create via the manual form. Simulate
its triage completing.

**Expected**: No compose-creation notification fires.

**Validates**: F-7.

**Automated test**: `useBacklogCreationNotifier.test.ts` — verify the hook only
fires for tracked item IDs, not for arbitrary items in the watch stream.

### AC-8: `has_pending_guidance_requests` field set correctly

**Test scenario**: Create a guidance request for an item in the database.
Call `GetBacklogItem` / observe `WatchBacklogItems` update.

**Expected**: `has_pending_guidance_requests = true`. Answer the request;
`has_pending_guidance_requests = false`.

**Validates**: NF-3 (pending guidance detection).

**Automated test**: Go unit test in `server/services/backlog_service_test.go`
or `server/services/guidance_service_test.go` — create item + guidance request,
fetch item, assert field.

## Risks Mitigated by This Plan

| Risk | Mitigation |
|------|-----------|
| LLM still drops images (prompt non-compliance) | Client-side fallback in A-2 guarantees images are appended even if the LLM ignores the new rule |
| `createBacklogItemFromChat` error leaves user stranded | Error branch fires an error notification (plan.md Concern 1) |
| Notification fires twice for same event | `notifiedComplete` / `notifiedGuidance` flags prevent repeat fires |
| Guidance re-fires after re-triage | Reset flag when `hasPendingGuidanceRequests` → `false` |

## Out-of-scope Validation

- E2E tests for the full creation → triage → notification flow are not in scope
  for this item (they require a live LLM; marked as a follow-up).
- Mobile-specific testing: `BacklogComposeWidget` is the same on mobile since it
  uses the Omnibar's responsive modal. No additional mobile-specific test needed.
