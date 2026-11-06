# Tech Stack — backlog-smart-creation

## Backend (Go)

- **ConnectRPC** — all service calls; proto definitions in `proto/session/v1/backlog.proto`
- **`session/backlog_intent.go`** — `BacklogIntentDraft`, `BuildBacklogIntentSystemPrompt()`, `BuildBacklogIntentUserPrompt()`, `ParseBacklogItemIntentDraft()` — the LLM intent-parsing layer already exists
- **`server/services/backlog_service_chat.go`** — `CreateBacklogItemFromChat` RPC; calls `CreateBacklogItem` with no `RepoPath` today
- **`server/services/backlog_service_lifecycle.go`** — `CreateBacklogItem`; calls `resolveRepoPathInput` then `MaybeTriggerTriage`
- **`server/services/backlog_service_triage.go`** — `MaybeTriggerTriage`; explicitly returns `false` when `repoPath == ""`
- **`server/services/backlog_service_intent.go`** — `ParseBacklogItemIntent` RPC; 90s budget; validates `SuggestedRepoPath` against `knownRepoPaths` allowlist

## Frontend (React / TypeScript)

- **ConnectRPC transport** — `web-app/src/lib/api/transport.ts`
- **`web-app/src/lib/hooks/useBacklogService.ts`** — client wrapper; `createBacklogItemFromChat(message, existingItemId?)`, `parseBacklogItemIntent(message, knownRepoPaths?)`; note: `createBacklogItemFromChat` proto call **does not accept `repoPath`** today
- **`web-app/src/lib/hooks/useBacklogCreationNotifier.ts`** — polls every 4s; fires triage-complete / guidance notifications; `trackItem(id, title)` entry point
- **`web-app/src/lib/hooks/useSessionRepoPaths.ts`** — returns deduplicated `repoRoot` list from active sessions via Redux store; already available
- **`web-app/src/components/backlog/BacklogComposeWidget.tsx`** — props: `{ initialText, onSubmit(text), onCancel(draft) }`; plain textarea + image attachment + Cancel/Create — no repo path field
- **`web-app/src/components/sessions/Omnibar.tsx`** — hosts `BacklogComposeWidget`; `createBacklogItemInBackground(text)` callback ignores `triageTriggered`
- **`web-app/src/components/sessions/OmnibarBacklogButton.tsx`** — the "Create Backlog Item" button

## Proto surface (relevant fields)

```
message CreateBacklogItemFromChatRequest { string message = 1; string existingItemId = 2; }
message CreateBacklogItemFromChatResponse { BacklogItem item = 1; bool triage_triggered = 2; }

message ParseBacklogItemIntentRequest {
  string message = 1; string repo_path = 2; repeated string known_repo_paths = 3;
}
message ParseBacklogItemIntentResponse {
  BacklogItemDraft draft = 1; bool success = 2; string error_message = 3;
  bool partial_result = 4; string suggested_repo_path = 5;
}
```

**Gap**: `CreateBacklogItemFromChatRequest` has no `repo_path` field. Adding it requires a proto change + Go handler update + TS client update.

## State management

- Redux store (`web-app/src/lib/store/`) — sessions slice exposes `selectActiveSessionsSortedByUpdatedAt` with `s.repoRoot` on each session; `useSessionRepoPaths` reads this
- No persistent client-side state for the compose widget beyond in-memory draft preservation (already implemented)
