# Requirements: Backlog Smart Creation

**Project:** `backlog-smart-creation`
**Item ID:** df2d6ee8-13de-4647-9890-b5d17080341b
**Date:** 2026-11-05

## Problem Statement

When a user clicks "Create Backlog Item" in the omnibar, they get a plain `<textarea>` compose
widget. After submitting, `createBacklogItemFromChat` is called with no `repoPath` and no AI
enrichment happens at creation time. The notification says "triage starting" but triage may never
actually start because there's no repo context. Items arrive in the backlog as raw, unstructured
text with no title parsing, no acceptance criteria, and no path set.

The user wants two improvements:
1. **AI enrichment at creation**: The item should be enriched (structured title, description, ACs,
   suggested repo path) before or immediately after submission — ideally without blocking the user.
2. **Brainstorming experience**: A dedicated mode for exploring and refining an idea before
   committing it, distinct from the triage pass that runs after creation.

### Related Existing Plans

Three prior plans partially address this space and inform this design:

- **`ai-backlog-creation-prompt`** — identified the regression where `BacklogComposeWidget.onSubmit`
  calls `createBacklogInBackground` instead of `BacklogItemIntentReview`. Proposed fix: route
  through `setBacklogReviewText` so AI intent parsing fires before creation. Status: *superseded*
  by `backlog-compose-bg-creation`'s deliberate pivot to immediate creation + background triage.

- **`ai-backlog-input-ux`** — comprehensive UX improvements (text preservation, multi-line
  textarea, background processing, image attachment, accessibility). Most ACs are already
  implemented in the current codebase.

- **`backlog-compose-bg-creation`** — intentionally replaced the blocking AI-review step with
  immediate background creation + triage notifications. The design intent was correct (don't block
  the user) but the execution gap remains: triage requires a `repoPath` and the compose widget
  provides none.

## Root Cause Analysis

1. **Triage gap**: `createBacklogItemFromChat` omits `repoPath`. The backend `CreateBacklogItemFromChat`
   RPC calls `MaybeTriggerTriage`, which requires a non-empty `repoPath` to spawn a headless triage
   session. With an empty path, triage is skipped silently. The notification "triage starting" is
   shown unconditionally regardless of whether `triageTriggered` is `true`.

2. **No AI enrichment at creation**: The compose widget creates from raw text. `parseBacklogItemIntent`
   (the AI intent RPC) is never called — not at creation time, not in background. The only AI touch
   the item gets is from triage (which doesn't run; see above).

3. **No brainstorming mode**: The compose widget is a single textarea with no AI assistance. There's
   no way to explore an idea interactively before committing it.

## User Story

> As a developer using Stapler Squad,
> when I click "Create Backlog Item" in the omnibar and describe an idea,
> I want the system to (a) help me brainstorm/refine the idea interactively if I want to,
> and (b) automatically set up the item with an enriched title, description, ACs, and
> likely repo path before or immediately after I submit — without blocking me from other work —
> so that items land in the backlog in a useful, triageable state from the start.

## Acceptance Criteria

### AC-1 — Repo path auto-detection
When the user is in the compose widget, the system proposes a repo path derived from context
(active sessions, recently-used repos, or omnibar input). The proposed path is shown as a
pre-filled (but editable) field. If the user has not explicitly cleared it, this path is
passed to `createBacklogItemFromChat` on submit.

### AC-2 — `triageTriggered` truthful notification
The "triage starting" notification is only shown when `triageTriggered === true` in the
`createBacklogItemFromChat` response. When `triageTriggered === false`, the notification
message indicates triage could not start and shows the item link with a prompt to add a
repo path.

### AC-3 — Background AI enrichment (title, description, ACs)
After submission, `parseBacklogItemIntent` is called in the background (not blocking the user).
When parsing completes, the item is updated server-side with the structured title, fleshed-out
description, and generated acceptance criteria. A notification appears: "Backlog item enriched —
review it now" with a deep link. On parse failure, no notification is shown (silent no-op — triage
will re-enrich anyway).

### AC-4 — Optional brainstorm mode in compose widget
The compose widget offers an "Explore with AI" toggle/button. When activated, a lightweight
chat panel appears below the textarea where the user can ask the AI questions about their idea
(e.g. "what edge cases am I missing?", "suggest acceptance criteria"). The chat does not block
submission; the user can toggle it off and submit at any time. Chat history is passed to
`parseBacklogItemIntent` as additional context.

### AC-5 — Repo path persists to triage
When an item is created with a non-empty `repoPath`, the headless triage session correctly
receives that path and runs to completion. Verified: `triageTriggered === true` in the
server response AND the item's `repoPath` field is populated after creation.

### AC-6 — No regression on existing paths
- The `backlog: <message>` omnibar-prefix flow is unchanged.
- Items created via the backlog form or external import are unaffected.
- The existing `BacklogItemIntentReview` component is not removed (still used by the
  `backlog:` prefix path).

### AC-7 — Notification accuracy
Notifications from compose-widget creation accurately reflect the actual state:
- If triage started: "Backlog item created — triage underway" (with link).
- If triage did NOT start: "Backlog item created — add a repo path to enable triage" (with link).
- If background enrichment completed: "Backlog item enriched" (with link).
- If creation itself failed: error notification with the raw text preserved for copy-paste.

## Out of Scope

- Full rebuild of the BacklogItemIntentReview component.
- Changes to the headless triage prompt or triage pipeline logic.
- Push notifications (Slack, browser push) — in-app notification bar only.
- The backlog board, item detail page, or kanban view.
- Mobile-specific layout changes (beyond what already works in the compose widget).
