# Existing Features — backlog-smart-creation

## What already works (do not re-implement)

### ParseBacklogItemIntent
Full AI intent parsing RPC already exists. Given free text + `knownRepoPaths`, it returns a structured draft with `title`, `description`, `acceptance_criteria`, `confidence`, and `suggested_repo_path` (validated server-side against `knownRepoPaths`). The 90s budget is generous; the LLM system prompt is stable (prefix-cacheable). The TS wrapper `useBacklogService.parseBacklogItemIntent(message, knownRepoPaths)` already works. Covers AC-3 (background enrichment) and partially AC-1 (repo suggestion) with no new backend work.

### useSessionRepoPaths
`web-app/src/lib/hooks/useSessionRepoPaths.ts` already returns a deduplicated list of `repoRoot` paths from active sessions. This is the right source for `knownRepoPaths` passed to `ParseBacklogItemIntent` and for pre-filling the repo path field in the compose widget. Zero additional backend work needed.

### useBacklogCreationNotifier
The polling + notification infrastructure already exists. `trackItem(id, title)` registers an item; the hook fires "Triage complete" / "AI needs your input" notifications automatically. For the enrichment notification (AC-3), we just need to call `trackItem` and add an enrichment-complete signal — or fire a one-off notification from the `parseBacklogItemIntent` callback directly.

### Draft preservation
`BacklogComposeWidget.onCancel(draft)` already preserves text in Omnibar state (`backlogComposeText`). The `initialText` prop re-hydrates it on reopen. Image attachments are already preserved.

### BacklogItemIntentReview component
Still used by the `backlog:` prefix flow in Omnibar. Must not be removed (AC-6). No changes needed here.

## What is missing (new work)

1. **`repo_path` field on `CreateBacklogItemFromChatRequest`** — proto field + Go handler + TS client wrapper
2. **Repo path UI in BacklogComposeWidget** — a pre-filled (editable/clearable) input showing the top suggestion from `useSessionRepoPaths`
3. **Conditional notification message** — Omnibar's `createBacklogItemInBackground` must branch on `result.triageTriggered` (AC-2, AC-7)
4. **Background enrichment call** — After creation, call `parseBacklogItemIntent` in background with item text + `knownRepoPaths`; on success, update item server-side and fire a notification (AC-3). Needs either a new RPC or reuse of `UpdateBacklogItem`/`SetBacklogItemMetadata` to write enriched fields back
5. **"Explore with AI" brainstorm panel** — lightweight chat below the compose textarea (AC-4); the largest piece of new UI

## Prior plan relationships

- `backlog-compose-bg-creation` (implemented): established the background creation pattern. This plan builds on it.
- `ai-backlog-creation-prompt` (superseded): proposed routing through `BacklogItemIntentReview` before creation — explicitly rejected by `backlog-compose-bg-creation`. Do not revisit.
- `ai-backlog-input-ux` (mostly implemented): UX improvements (text preservation, multi-line, image attachment) are already in the codebase. AC-4 (brainstorm mode) is the one remaining gap.
