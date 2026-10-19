# Requirements: Backlog Compose — Background Creation + Image Fix

_Derived from backlog item f201543d-ceca-49c5-a74b-bc187d7aecac_

## Problem Statement

Two related defects in the AI-driven backlog item creation flow (the omnibar
`BacklogComposeWidget` → `BacklogItemIntentReview` path):

1. **Bug — Image attachments are lost**: When a user attaches an image in the
   compose widget, the upload succeeds and a markdown reference
   `![filename](/api/local/serve/attachments/.../file.png)` is inserted into
   the textarea. After submission the text is passed to `parseBacklogItemIntent`,
   which asks the LLM to rewrite the description. The LLM's system prompt
   (`session/backlog_intent.go:28`) tells it to "expand on" the input but does
   not instruct it to preserve image markdown references verbatim. The LLM
   routinely reformulates or omits the `![…](…)` references, so the resulting
   backlog item has no image attached.

2. **UX — Synchronous parsing blocks the user**: After the compose widget
   submits, `BacklogItemIntentReview` shows a "Parsing your message…" spinner
   while waiting for `ParseBacklogItemIntent` (90-second budget). Only after
   parsing completes does the user see the review form; only after they fill and
   submit the form is the item actually created. The user cannot leave the page
   or do anything else until creation completes.

## Functional Requirements

### F-1 — Immediate creation
When the user submits the compose widget, the backlog item MUST be created
immediately without waiting for AI parsing. The item is created from the raw
text using the existing `CreateBacklogItemFromChat` RPC (`title = first line,
description = full text`). The compose dialog MUST close immediately after the
creation call returns.

### F-2 — Background triage
`CreateBacklogItemFromChat` already triggers `MaybeTriggerTriage` on the newly
created item; this MUST continue unchanged. The triage session runs in the
background; the user MUST be free to navigate away.

### F-3 — Triage-complete notification
When a compose-created item's `triageStatus` transitions from `running` to
`completed` or `failed`, a notification MUST appear in the notification bar
identifying the item by title and providing a direct link to its detail page.

### F-4 — Guidance-needed notification
When a compose-created item's triage session creates a pending guidance request
(currently surfaced via `GuidanceRequestPanel` in `BacklogItemDetail`), a
notification MUST appear in the notification bar stating that AI input is
needed. Clicking the notification MUST open the item's detail page where the
existing `GuidanceRequestPanel` shows the pending question(s). The panel MUST
NOT be rebuilt; it is reused as-is.

### F-5 — Image markdown preserved
Regardless of whether the item is created synchronously (compose path) or the
old intent-review path, image markdown references `![alt](url)` present in the
raw text MUST be preserved verbatim in the item's `description` field.

For the new compose path (F-1) this follows automatically because the raw text
is used as the description. No separate fix is needed here.

For the legacy intent-review path (`BacklogItemIntentReview`), the system
prompt in `session/backlog_intent.go` MUST be updated to instruct the LLM to
preserve image references verbatim, AND `draftToInitialValues` in
`BacklogItemIntentReview.tsx` MUST have a client-side fallback that detects
`![…](…)` patterns in `rawText` and appends any that are absent from the
parsed description.

### F-6 — No loss of compose-widget text
If the user has typed text AND attached an image before submitting, both MUST
appear in the created item's description.

### F-7 — Notification tracking scope
Notifications MUST be scoped to items created in the CURRENT browser session
via the compose widget. Items created by other means (import, manual form,
chat) do not trigger these notifications.

## Non-functional Requirements

### NF-1 — No existing component rebuild
The `GuidanceRequestPanel` and the notification infrastructure (`NotificationContext`,
`useNotifications`) MUST be reused unchanged. No new notification types are
required beyond what `addNotification` already supports.

### NF-2 — Compose widget changes are backward-compatible
The `BacklogComposeWidget` must continue to emit `onSubmit(text)` — the caller
already owned "close and background creation"; this just changes what the caller
does with the text. The compose widget itself needs no changes.

### NF-3 — Pending guidance detection
To support F-4, `BacklogItem` needs a way to signal "has pending guidance
requests." The simplest approach: add a `has_pending_guidance_requests` bool
field to the `BacklogItem` proto (and to `mapBacklogItem` in
`useBacklogService.ts`) that is set by the server when any guidance request for
this item is in status `pending`. This allows the existing `WatchBacklogItems`
event stream to carry the signal without a new RPC.

## Out of Scope

- Removing or deprecating `ParseBacklogItemIntent` / `BacklogItemIntentReview`
  (they are used by the "backlog: " prefix omnibar path, which is a different
  entry point; keep them, but also apply the image-preservation fix to them per
  F-5).
- Changes to triage pipeline logic, session spawning, or any other part of the
  lifecycle beyond creation and notification.
- Push notifications (Slack, browser push) — only the in-app notification bar.
