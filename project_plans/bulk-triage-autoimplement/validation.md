# Validation: Bulk Triage and Auto-Implementation

Item: 2ec26b1c-710b-4838-8658-e9f8968e8599

---

## Unit Tests

File: `web-app/src/lib/backlog/bulkActions.test.ts`

### isBulkTriageEligible

| Test | Input | Expected |
|---|---|---|
| idea status is eligible | `{ status: "idea", triageStatus: undefined }` | `true` |
| ready status is eligible | `{ status: "ready", triageStatus: undefined }` | `true` |
| idea + triageStatus running is ineligible | `{ status: "idea", triageStatus: "running" }` | `false` |
| ready + triageStatus running is ineligible | `{ status: "ready", triageStatus: "running" }` | `false` |
| in_progress is ineligible | `{ status: "in_progress", triageStatus: undefined }` | `false` |
| archived is ineligible | `{ status: "archived", triageStatus: undefined }` | `false` |
| done is ineligible | `{ status: "done", triageStatus: undefined }` | `false` |
| triageStatus=null (non-running) is eligible | `{ status: "ready", triageStatus: null }` | `true` |
| triageStatus=completed (non-running) is eligible | `{ status: "ready", triageStatus: "completed" }` | `true` |

### isBulkAutoImplementEligible

| Test | Input | Expected |
|---|---|---|
| ready + skipPlanning=true is eligible | `{ status: "ready", skipPlanning: true, planApproved: false }` | `true` |
| ready + planApproved=true is eligible | `{ status: "ready", skipPlanning: false, planApproved: true }` | `true` |
| ready + both true is eligible | `{ status: "ready", skipPlanning: true, planApproved: true }` | `true` |
| ready + neither approved is ineligible | `{ status: "ready", skipPlanning: false, planApproved: false }` | `false` |
| idea + skipPlanning is ineligible | `{ status: "idea", skipPlanning: true, planApproved: false }` | `false` |
| in_progress + planApproved is ineligible | `{ status: "in_progress", skipPlanning: false, planApproved: true }` | `false` |
| archived is ineligible | `{ status: "archived", skipPlanning: true, planApproved: true }` | `false` |

### runBulkChunked

| Test | Input | Expected |
|---|---|---|
| full success, single chunk | 3 ids, chunkSize=5, all resolve | `{ succeeded: [id1,id2,id3], failed: [] }` |
| partial failure within one chunk | 3 ids, chunkSize=5, id2 rejects | `{ succeeded: [id1,id3], failed: [id2] }` |
| all fail | 3 ids, chunkSize=5, all reject | `{ succeeded: [], failed: [id1,id2,id3] }` |
| chunk boundary split: 7 ids, chunkSize=5 | 7 ids, first 5 in chunk 1, last 2 in chunk 2, all resolve | 2 separate `Promise.allSettled` waves; `{ succeeded: [id1…id7], failed: [] }` |
| partial failure spanning chunks | 7 ids, chunkSize=5, id3 and id6 reject | `{ succeeded: 5 ids, failed: [id3,id6] }` |
| empty ids list | `[]`, chunkSize=5 | `{ succeeded: [], failed: [] }` |
| chunkSize=1 runs serially | 3 ids, chunkSize=1, all resolve | `{ succeeded: [id1,id2,id3], failed: [] }` (3 separate waves) |

### runBulkSpawn / SpawnBulkResult

| Test | Input | Expected |
|---|---|---|
| all started | all spawnFn resolve with `{ sessionUuid: "x" }` | `{ started: [id1,id2], queued: [], failed: [] }` |
| all queued | all spawnFn resolve with `{ queued: true }` | `{ started: [], queued: [id1,id2], failed: [] }` |
| all failed | all spawnFn reject | `{ started: [], queued: [], failed: [id1,id2] }` |
| mixed outcomes | id1→started, id2→queued, id3→failed | `{ started: [id1], queued: [id2], failed: [id3] }` |
| empty ids list | `[]` | `{ started: [], queued: [], failed: [] }` |
| queued=true alongside sessionUuid treated as queued | `{ queued: true, sessionUuid: "x" }` | id placed in `queued` bucket, not `started` |

### formatSpawnBulkFeedback

| Test | Input | Expected |
|---|---|---|
| pure success, N=1 | `{ started: [id1], queued: [], failed: [] }` | `"Started 1 session"` |
| pure success, N=3 | `{ started: [id1,id2,id3], queued: [], failed: [] }` | `"Started 3 sessions"` |
| started + queued, no failure | `{ started: [id1], queued: [id2], failed: [] }` | `"Started 1, queued 1"` |
| started + queued plural | `{ started: [id1,id2], queued: [id3,id4], failed: [] }` | `"Started 2, queued 2"` |
| started + queued + failed | `{ started: [id1], queued: [id2], failed: [id3] }` | `"Started 1, queued 1 (1 failed)"` |
| multiple failures | `{ started: [id1], queued: [], failed: [id2,id3] }` | `"Started 1 (2 failed)"` |
| all queued, none started | `{ started: [], queued: [id1,id2], failed: [] }` | `"Queued 2 sessions"` (or equivalent; no "Started 0") |

---

## Component Tests

File: `web-app/src/components/backlog/BacklogBulkActions.test.tsx`

### BacklogBulkActions — Triage button (AC 1, AC 13)

| Test | Setup | Assertion |
|---|---|---|
| button visible when items selected | render with `selectedCount=1` | element with `data-testid="backlog-bulk-triage-selected"` is in the document |
| button hidden when no items selected | render with `selectedCount=0` | `data-testid="backlog-bulk-triage-selected"` is not rendered (or the entire bulk toolbar is hidden) |
| click fires onTriageSelected | render with `selectedCount=2`, `busy=false`; click button | `onTriageSelected` mock called once |
| button disabled when busy | render with `selectedCount=2`, `busy=true` | button has `disabled` attribute; click does not fire `onTriageSelected` |

### BacklogBulkActions — Triage feedback strings (AC 3)

These are page-level integration tests (see Integration Tests section); component tests only cover that the handler is called. Feedback rendering tests belong in a page.tsx integration test or via the handler callbacks passed as props.

### BacklogBulkActions — Auto-implement button (AC 4, AC 13)

| Test | Setup | Assertion |
|---|---|---|
| button visible when items selected | render with `selectedCount=1` | element with `data-testid="backlog-bulk-auto-implement-selected"` is in the document |
| button hidden when no items selected | render with `selectedCount=0` | `data-testid="backlog-bulk-auto-implement-selected"` not rendered |
| button disabled when busy | render with `selectedCount=3`, `busy=true` | button has `disabled` attribute |

### BacklogBulkActions — Auto-implement dialog open/close (AC 5)

| Test | Setup | Assertion |
|---|---|---|
| clicking button opens dialog | render with `selectedCount=2`, `eligibleAutoImplementCount=2`; click button | `data-testid="backlog-bulk-auto-implement-dialog"` appears in the document |
| dialog not present before click | render with `selectedCount=2` | `data-testid="backlog-bulk-auto-implement-dialog"` not present |
| Cancel button closes dialog | open dialog; click Cancel | dialog element removed from document; `onAutoImplementSelected` not called |
| Confirm button fires handler and closes dialog | open dialog; click Confirm (`data-testid="backlog-bulk-auto-implement-confirm"`) | `onAutoImplementSelected` called once; dialog element removed |
| clicking overlay closes dialog | open dialog; click overlay behind dialog | dialog element removed from document |
| dialog shows eligible count in heading | render with `eligibleAutoImplementCount=3`; open dialog | heading text contains "3" and references autonomous/sessions |
| dialog shows ineligible notice when count > 0 | render with `eligibleAutoImplementCount=2`, `ineligibleAutoImplementCount=1`; open dialog | notice text contains "1" and "ineligible" / "skipped" |
| dialog hides ineligible notice when count = 0 | render with `ineligibleAutoImplementCount=0`; open dialog | ineligible notice element not present |

### BacklogBulkActions — Dialog autonomous text (AC 7)

| Test | Setup | Assertion |
|---|---|---|
| dialog communicates autonomous execution | open auto-implement dialog | dialog text includes "autonomously" or "without manual steps" |

### BacklogBulkActions — Dialog keyboard accessibility (AC 12)

| Test | Setup | Assertion |
|---|---|---|
| dialog has correct ARIA attributes | open dialog | element with `data-testid="backlog-bulk-auto-implement-dialog"` has `role="dialog"` and `aria-modal="true"` |
| Escape key closes dialog | open dialog; fire `keyDown` event with `key="Escape"` on dialog element | dialog removed from document; `onAutoImplementSelected` not called |
| dialog has accessible label | open dialog | `aria-labelledby` attribute on dialog element resolves to a visible heading element |

---

## Integration Tests

File: `web-app/src/app/backlog/page.tsx` (tested via Jest + React Testing Library, mocking RPC hooks)

### handleTriageSelected — eligibility filter (AC 2, AC 3, AC 9)

| Test | Setup | Assertion |
|---|---|---|
| calls triggerTriage only for eligible items | selectedIds = two items: one `idea`-status eligible, one `in_progress`-status ineligible; `triggerTriage` mock returns resolved | `triggerTriage` called exactly once with the eligible id; not called with the ineligible id |
| reports skip count in feedback | 1 eligible, 1 ineligible selected; triggerTriage succeeds | feedback string contains "1" and "skipped" or "ineligible" |
| nothing-to-triage feedback when all ineligible | only in_progress item selected | feedback string matches "Nothing to triage" (with ineligible count) |
| clears selection on full success | 2 eligible selected, both triggerTriage calls succeed | selection cleared after handler resolves |
| leaves failed IDs selected on partial failure | 2 eligible selected; id2 triggerTriage rejects | selection updated to contain only id2; id1 deselected |
| feedback on partial failure | 1 succeed, 1 fail | feedback contains both success count and failure count |

### handleAutoImplementSelected — eligibility pre-filter (AC 6, AC 8, AC 9)

| Test | Setup | Assertion |
|---|---|---|
| only ready+approved items dispatched | selectedIds = three items: `ready`+`planApproved`, `ready`+neither approved, `in_progress`; spawnSessionFromItem mock | `spawnSessionFromItem` called once with the ready+approved id only |
| feedback "Started N" on full success | 2 eligible, all spawn return `{ sessionUuid }` | feedback contains "Started 2" |
| feedback includes queued count | 1 started, 1 queued (`{ queued: true }`) | feedback contains "Started 1, queued 1" |
| feedback includes failure count on partial fail | 1 started, 1 failed | feedback contains "Started 1" and "(1 failed)" |
| clears selection on full success (no failures) | 2 eligible, both started or queued | selection cleared |
| leaves failed IDs selected on partial failure | 2 eligible; second item's spawn rejects | selection updated to contain only the failed item's id |
| nothing-eligible feedback when no eligible items selected | only in_progress items selected | feedback "Nothing eligible for auto-implement"; selection cleared |

---

## Traceability Matrix

| AC # | Description (abbreviated) | Test(s) |
|---|---|---|
| 1 | "Triage selected" button visible when ≥1 selected | Component: "button visible when items selected" |
| 2 | Triage fans out only to eligible items; ineligible skipped | Integration: "calls triggerTriage only for eligible items" |
| 3 | Triage feedback string variants | Integration: "nothing-to-triage feedback", "reports skip count", "feedback on partial failure" |
| 4 | "Auto-implement selected" button visible when ≥1 selected | Component: "button visible when items selected" (auto-implement section) |
| 5 | Dialog opens; Cancel closes; Confirm fans out | Component: "clicking button opens dialog", "Cancel button closes dialog", "Confirm button fires handler" |
| 6 | Ineligible items excluded from auto-implement | Integration: "only ready+approved items dispatched" |
| 7 | Dialog communicates autonomous execution | Component: "dialog communicates autonomous execution" |
| 8 | Auto-implement feedback string variants | Integration: "feedback 'Started N'", "feedback includes queued count", "feedback includes failure count" |
| 9 | Selection cleared on full success; failed IDs remain on partial failure | Integration (both triage and auto-implement): "clears selection on full success", "leaves failed IDs selected" |
| 10 | proto/session/v1/backlog.proto unchanged | Static: no proto file appears in the diff; enforced by the existing proto CI check |
| 11 | Jest tests cover eligibility, dialog cancel/confirm, feedback, partial-failure | All unit and component tests above |
| 12 | Dialog keyboard-accessible (focus trap, Escape to cancel) | Component: "Escape key closes dialog", "dialog has correct ARIA attributes" |
| 13 | Both buttons disabled when busy=true | Component: "button disabled when busy" (triage section), "button disabled when busy" (auto-implement section) |
