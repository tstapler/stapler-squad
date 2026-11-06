# Architecture — backlog-smart-creation

## Current data flow (broken)

```
User types text in BacklogComposeWidget
  → onSubmit(text)
  → Omnibar.createBacklogItemInBackground(text)
  → useBacklogService.createBacklogItemFromChat(message)
    → ConnectRPC: CreateBacklogItemFromChat { message }
      → BacklogService.CreateBacklogItem { Title, Description }  // no RepoPath
        → MaybeTriggerTriage(repoPath="")  // returns false, triage skipped
  → addNotification("triage starting")  // unconditional, misleading
  → trackItem(id, title)                // polls for triage completion (never fires)
```

## Target data flow (after fix)

```
User opens BacklogComposeWidget
  → useSessionRepoPaths() populates suggested repoPath (top active session's repoRoot)
  → User sees pre-filled repo path field (editable/clearable)

User submits (text + repoPath)
  → Omnibar.createBacklogItemInBackground(text, repoPath)
  → useBacklogService.createBacklogItemFromChat(message, repoPath)
    → ConnectRPC: CreateBacklogItemFromChat { message, repo_path }  // new field
      → BacklogService.CreateBacklogItemFromChat: passes repoPath to CreateBacklogItem
        → MaybeTriggerTriage(repoPath)  // now fires when repoPath is non-empty
  → result.triageTriggered ?
      "triage underway" notification
    : "add a repo path to enable triage" notification
  → trackItem(id, title)  // polls for real triage completion

Background (concurrent with above):
  → parseBacklogItemIntent(text, knownRepoPaths) [fire-and-forget]
    → on success: UpdateBacklogItem or SetBacklogItemMetadata with enriched title/desc/ACs
    → addNotification("Backlog item enriched")
```

## Component boundaries

| Layer | File | Change needed |
|---|---|---|
| Proto | `proto/session/v1/backlog.proto` | Add `string repo_path = 3` to `CreateBacklogItemFromChatRequest` |
| Backend handler | `server/services/backlog_service_chat.go` | Forward `req.Msg.RepoPath` to `CreateBacklogItem` |
| TS client | `web-app/src/lib/hooks/useBacklogService.ts` | Add `repoPath?: string` to `createBacklogItemFromChat` signature; pass to proto |
| Compose widget | `web-app/src/components/backlog/BacklogComposeWidget.tsx` | Add `repoPath` to onSubmit; add repo path UI (combobox/input) |
| Omnibar | `web-app/src/components/sessions/Omnibar.tsx` | Pass `repoPath` through; fix conditional notification |
| Enrichment | `web-app/src/components/sessions/Omnibar.tsx` or new hook | Call `parseBacklogItemIntent` post-creation; apply enrichment |

## Enrichment write-back

`ParseBacklogItemIntent` returns a draft but does not persist it. To apply enrichment:
- Option A: Reuse `UpdateBacklogItem` (if it exists and accepts title/description/ACs)
- Option B: Add a new `EnrichBacklogItem` RPC that takes the item ID + draft and applies it server-side
- Option C: Persist enrichment client-side only until the user opens the item (poor UX for multi-tab)

**Recommendation**: Option A first — check if `UpdateBacklogItem` already supports these fields. Fall back to a targeted new RPC if needed.

## Brainstorm mode (AC-4)

The brainstorm panel is a lightweight chat below the textarea. It does not require a new session or tmux connection — it uses `ParseBacklogItemIntent` or a simpler prompt-based API for single turn Q&A. Each chat turn is appended to context for the next call. On submit, the final draft text (possibly rewritten by the user based on the chat) is used as the message.

This is architecturally independent of the triage fix (ACs 1, 2, 3, 5) — it can ship separately.

## Proto regeneration

After adding `repo_path` to `CreateBacklogItemFromChatRequest`, run:
```
make proto-gen
```
The generated TS client will automatically expose the new field.
