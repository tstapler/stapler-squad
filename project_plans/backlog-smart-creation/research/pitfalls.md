# Pitfalls — backlog-smart-creation

## 1. `createBacklogItemFromChat` signature has no `repoPath` today

The TS hook signature is `createBacklogItemFromChat(message, existingItemId?)`. Adding `repoPath` requires updating the proto, regenerating TS types (`make proto-gen`), and updating the hook. The comment at line 1019 of `useBacklogService.ts` says "repo_path is intentionally omitted: the omnibar has no 'current repo' context available before parsing" — this plan supersedes that comment by adding the repo path UI to the compose widget.

## 2. `MaybeTriggerTriage` returns false silently

There is no log line or response field that tells the caller *why* triage was skipped. The existing `triage_triggered` bool in the response is correct to check, but the backend never surfaces `repoPath == ""` as the reason. The fix is frontend-only (conditional notification message) — do not add noise to the backend log for this.

## 3. Enrichment write-back needs a path

`ParseBacklogItemIntent` only returns a draft in the response — it does not update the item. If `UpdateBacklogItem` does not support writing title/description/ACs, a new RPC is needed. Verify this before coding the enrichment loop. If a new RPC is needed, it must be added to the proto and the registry (`make registry-generate`).

## 4. `knownRepoPaths` length cap

`useBacklogService.parseBacklogItemIntent` already caps the list at `MAX_KNOWN_REPO_PATHS` (line ~1026). Do not send an unbounded list from the compose widget; call `useSessionRepoPaths()` and pass the result directly — the hook already deduplicates.

## 5. Race between enrichment and triage

Both enrichment (`parseBacklogItemIntent` background call) and triage (headless session) may update the item concurrently. If triage runs quickly, it may overwrite enrichment's title/description. Mitigation: enrichment should use a conditional update (only update fields that are still at their raw/default value), or enrichment should be skipped if `triageTriggered === true` (since triage will enrich anyway).

## 6. `BacklogComposeWidget` prop interface change

Adding `onSubmit(text, repoPath?)` changes the existing prop interface. Any other caller of `BacklogComposeWidget` must be updated to match, or the new argument must be optional with a default of `""`. Audit all usages before changing.

## 7. "Explore with AI" panel — brainstorm chat LLM budget

A brainstorm chat panel that calls the LLM on each user turn could be expensive. Use `ParseBacklogItemIntent`'s existing 90s budget or a faster/cheaper endpoint. Each turn should debounce (don't fire on every keystroke). The panel must not block the submit button regardless of pending LLM calls.

## 8. Notification duplication risk

If the user creates an item and triage completes quickly, they may receive both the creation notification and the triage-complete notification in rapid succession. This is fine UX (two distinct events) but test that `useBacklogCreationNotifier` does not fire the enrichment notification AND the triage-complete notification on the same item double-notifying the user about the same state.

## 9. ent schema / registry

Adding `repo_path` to `CreateBacklogItemFromChatRequest` only affects the RPC transport, not the ent schema (the item already has `repo_path` in the DB). But if a new `EnrichBacklogItem` RPC is added, run `make registry-generate` and ensure the JSON has `tested:true` or CI will fail.
