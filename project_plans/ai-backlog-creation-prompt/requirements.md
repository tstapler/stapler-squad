# Requirements: AI Backlog Creation Prompt

## Problem Statement

When a user clicks "Create backlog item," the UI shows a "Describe the work" compose widget.
After the user submits, the item is created directly from raw text — no AI parsing occurs.
Previously, submitting text would invoke `parseBacklogItemIntent` (an LLM call), which structured
the title, fleshed out the description, generated 2–5 acceptance criteria, and let the user review
before committing the item.

A regression introduced in commit `4a85c12c5` ("dedicated compose widget for AI backlog items",
Oct 11) re-routed `BacklogComposeWidget.onSubmit` to call `createBacklogItemFromChat` directly
instead of transitioning to the `BacklogItemIntentReview` AI-powered review step.

## User Story

> As a product owner using the backlog,
> when I click "Create backlog item" and describe my feature in plain language,
> I want the AI to parse my intent, flesh out the description, generate acceptance criteria,
> and let me review before the item is created,
> so that items have consistent, actionable structure from the start.

## Affected Code

| File | Role |
|---|---|
| `web-app/src/components/sessions/Omnibar.tsx` (lines ~1763–1767) | `BacklogComposeWidget.onSubmit` — calls `createBacklogInBackground` (no AI) instead of `setBacklogReviewText` (AI path) |
| `web-app/src/components/backlog/BacklogComposeWidget.tsx` | "Describe the work" compose modal introduced by regression commit |
| `web-app/src/components/backlog/BacklogItemIntentReview.tsx` | AI review step — no longer reached via the "Create backlog item" button |
| `web-app/src/lib/hooks/useBacklogBackgroundCreate.ts` | `createBacklogInBackground` — skips AI |
| `web-app/src/lib/hooks/useBacklogService.ts` | `parseBacklogItemIntent` (line ~1009) and `createBacklogItemFromChat` (line ~988) |
| `server/services/backlog_service_intent.go` | `ParseBacklogItemIntent` RPC — LLM call that produces structured draft |
| `session/backlog_intent.go` | `backlogIntentSystemPrompt` — system prompt for AI intent parsing |

## Secondary Issue

`CreateBacklogItemFromChat` passes an empty `repoPath`, causing `MaybeTriggerTriage` in
`backlog_service_triage.go` (line ~2826) to always return `false`. Items created through the
compose widget skip triage entirely.

## Acceptance Criteria

1. Clicking "Create backlog item" opens a text input for the user to describe the work (preserve
   the compose widget UX — don't regress back to omnibar-prefix approach).
2. After the user submits text in the compose widget, the AI intent-review step fires:
   `parseBacklogItemIntent` (LLM) is called with the submitted text.
3. The user sees the AI-structured title, description, and acceptance criteria in a review form
   before the item is persisted.
4. If the AI call fails or times out, the system falls back gracefully (user can still create the
   item from raw text without data loss).
5. The existing `backlog: <message>` omnibar-prefix flow continues to work unchanged.
6. Items created via the compose widget trigger triage when a repo path is available (secondary fix).
