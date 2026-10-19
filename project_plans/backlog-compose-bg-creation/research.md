# Research: Backlog Compose — Background Creation + Image Fix

## 1. Current Flow (as-built)

### 1.1 Compose widget → intent review → create

```
BacklogComposeWidget.onSubmit(text)
  → Omnibar sets backlogReviewText = text
  → BacklogItemIntentReview renders with initialText = text
    → useParseBacklogItemIntent
        → parseBacklogItemIntent(text, knownRepoPaths)   ← blocks up to 90s
        → setPhase("review")
    → BacklogItemForm(initialValues = draftToInitialValues(draft, rawText))
    → handleSubmit → createBacklogItem(data)
```

Key files:
- `web-app/src/components/backlog/BacklogComposeWidget.tsx` — compose UI; calls `onSubmit(text)`
- `web-app/src/components/backlog/BacklogItemIntentReview.tsx` — wraps parsing + form
- `web-app/src/components/sessions/Omnibar.tsx:1720-1773` — renders the two modes
- `session/backlog_intent.go` — `BuildBacklogIntentSystemPrompt`, `ParseBacklogItemIntentDraft`
- `server/services/backlog_service_intent.go` — `ParseBacklogItemIntent` RPC handler
- `server/services/backlog_service_chat.go` — `CreateBacklogItemFromChat` (creates + triggers triage)

### 1.2 Image attachment mechanic

`useBacklogImageAttachment` (`web-app/src/lib/hooks/useBacklogImageAttachment.ts`):
- Uploads file to `POST /api/v1/upload-backlog-attachment`
- Server stores in `backlogAttachmentDir`, serves via `/api/local/serve/…`
- Inserts `![filename](/api/local/serve/attachments/.../file.png)` at cursor in textarea

The upload and serve endpoints are unconditional — the file exists on disk and
is served correctly. The problem is downstream: the LLM reformulates the
description and drops the markdown reference.

### 1.3 LLM system prompt (confirmed root cause)

`session/backlog_intent.go:28`:
```go
const backlogIntentSystemPrompt = `You are a product analyst turning a loosely-described task
into a well-formed backlog item. ...
2. description: a fleshed-out description of the task — expand on the input, but do not invent
   unrelated scope or fabricate specifics (file paths, function names, numbers) that are not
   implied by the input.
...`
```

There is NO instruction to preserve `![…](…)` image references verbatim. The
LLM's "expand on the input" instruction actively encourages rewriting, which
causes it to drop or reformulate image markdown.

### 1.4 `CreateBacklogItemFromChat` (the background creation path)

`server/services/backlog_service_chat.go:74`:
```go
createResp, err := s.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
    Title:       deriveChatItemTitle(message),   // first line, max 80 chars
    Description: message,                        // full raw text
}))
```

`CreateBacklogItem` calls `MaybeTriggerTriage` internally, so triage is
automatically started. The raw text (including image markdown) is preserved
verbatim as `Description`. This path is **already what we want** for the
background flow.

### 1.5 WatchBacklogItems event stream

`proto/session/v1/backlog.proto:1373`:
- `BacklogItemUpdatedEvent` carries the full `BacklogItem` in its `item` field
- `triageStatus` is already part of `BacklogItem`
- Currently NO `has_pending_guidance_requests` field exists on `BacklogItem`

`useWatchBacklogItems.ts` merges incoming events into a local item store.
The frontend can detect `triageStatus` changes by comparing old vs new state.

### 1.6 Notification infrastructure

`web-app/src/lib/contexts/NotificationContext.tsx`:
- `addNotification({ title, body, type, action?: { label, onClick } })` adds a
  toast to the notification bar
- Already used in `web-app/src/app/page.tsx` (session state changes),
  `web-app/src/components/sessions/SessionList.tsx`, and others

### 1.7 GuidanceRequestPanel

`web-app/src/components/guidance/GuidanceRequestPanel.tsx`:
- Fetches pending guidance requests for a `scope` ("backlog-item") + `scopeKey` (item id)
- Renders questions and answer form
- Already embedded in `BacklogItemDetail.tsx` and `TriageReviewPanel.tsx`
- No changes needed; notifications will link to the item detail page

### 1.8 Detecting guidance requests

`BacklogItem` (mapped in `useBacklogService.ts`) has no `hasPendingGuidanceRequests`
field. Options:
1. Add `has_pending_guidance_requests: bool` to `BacklogItem` proto; backend sets it
   whenever any `GuidanceRequest` for the item is in `PENDING` status. This allows
   the existing `WatchBacklogItems` stream to carry the signal.
2. On the frontend, poll `ListGuidanceRequests` for recently-created items while
   triage is running.

Option 1 is cleaner and reuses the existing event bus. Option 2 adds a polling
path and couples the notification to a polling interval.

**Decision**: Use Option 1. Add `has_pending_guidance_requests` to `BacklogItem`
proto and set it in the existing `backlogItemToProto` helper. The field is
already watched by the frontend via `BacklogItemUpdatedEvent`.

### 1.9 `VaguenessPromptModal`

`web-app/src/components/backlog/VaguenessPromptModal.tsx` is shown in `page.tsx`
after `createBacklogItem` to prompt users of vague items. It is NOT the
"AI question form" referred to in the description — that's `GuidanceRequestPanel`.
`VaguenessPromptModal` is not part of this change.

### 1.10 Omnibar flow change

`Omnibar.tsx:1720-1773`:

Current compose submit handler:
```tsx
onSubmit={(text) => {
    setBacklogComposeText(null);
    setBacklogReviewText(text);  // ← transitions to intent review
}}
```

New handler:
```tsx
onSubmit={async (text) => {
    setBacklogComposeText(null);
    onClose();
    const result = await createBacklogItemFromChat(text);
    if (result) {
        trackForNotification(result.item.id, result.item.title);
        addNotification({ title: "Backlog item created", body: `"${result.item.title}" — triage in progress` });
    }
}}
```

A `useBacklogCreationNotifier` hook watches `WatchBacklogItems` for items in the
tracking set and fires `addNotification` on `triageStatus` transition and
`has_pending_guidance_requests` becoming `true`.

## 2. Risk Areas

### R-1: `createBacklogItemFromChat` is called in `BacklogItemDetail` too
`BacklogItemDetail.tsx:1012` calls `createBacklogItemFromChat(message, item.id)`
for refinement turns (when `ExistingItemId` is set). This path is unchanged —
notifications are only scoped to NEW items created from the compose widget
(F-7 in requirements).

### R-2: Image references and triage
When the item is created with `CreateBacklogItemFromChat`, the raw text
(including image markdown) is the description. The triage agent runs with
access to this description. The triage agent cannot follow
`/api/local/serve/…` URLs (they require the local HTTP server). This is
acceptable — the image is for human context, not for the triage agent to read.
A future improvement could pass image data to the triage agent directly.

### R-3: Notification deduplication
If the user creates multiple items in quick succession, each gets its own
notification. The `addNotification` call identifies items by id via the
notification `id` field, so duplicates are naturally prevented.

### R-4: Proto change requires `make build`
Adding `has_pending_guidance_requests` to `BacklogItem` requires regenerating
the proto. This is a one-field change with no breaking effects on existing
callers (proto3 defaults: `false`).

### R-5: Legacy intent-review path still needs the image fix
`BacklogItemIntentReview` is still used by the "backlog: " prefix path
(typed directly into the omnibar). The image preservation fix (system prompt +
client fallback) applies to that path independently of the compose-widget
background change.
