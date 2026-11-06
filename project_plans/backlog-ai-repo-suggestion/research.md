# Research: Backlog AI Repo Suggestion

**Date:** 2026-10-11

---

## 1. Current code paths — verified

### Omnibar trigger
- `web-app/src/lib/omnibar/detector.ts` — `ChatBacklogItemDetector` (priority 15): fires on `backlog: <text>` prefix. Returns `InputType.ChatBacklogItem` with `parsedValue = message`.
- `web-app/src/components/sessions/Omnibar.tsx:1426` — on submit with `ChatBacklogItem` detection: sets `backlogReviewText = detection.parsedValue` (message text only; no repo context). Control transitions to `BacklogItemIntentReview`.

### Intent parsing service
- `web-app/src/lib/hooks/useBacklogService.ts:982-1003` — `parseBacklogItemIntent(message: string)` sends `{ message }` only; line 983 comment explicitly states `repo_path is intentionally omitted`.
- Return type: `ParsedBacklogItemDraft | null` — interface at line 417: `{ title, description, acceptanceCriteria, confidence }`.

### Server handler
- `server/services/backlog_service_intent.go:28` — `ParseBacklogItemIntent` reads `req.Msg.RepoPath` from the request proto and threads it into `BuildBacklogIntentUserPrompt(message, repoPath)`.
- `session/backlog_intent.go:44-53` — `BuildBacklogIntentUserPrompt` appends `"\n\nRepo: <path>"` when `repoPath != ""`. The plumbing already exists server-side; the client just never sends the field.

### Proto definition
- `proto/session/v1/backlog.proto:521-545` — `ParseBacklogItemIntentRequest` has field `string repo_path = 2` (optional). `ParsedBacklogItemDraft` has: `title`, `description`, `acceptance_criteria`, `confidence`. **No `suggested_repo_path` field exists yet.**

### Form pre-population
- `web-app/src/components/backlog/BacklogItemIntentReview.tsx:32-44` — `draftToInitialValues` maps `ParsedBacklogItemDraft` fields → `Partial<BacklogItem>`. Includes `title`, `description`, `acCriteria`. Does NOT include `repoPath`.
- `web-app/src/components/backlog/BacklogItemForm.tsx:95` — `repoPath` state initialized from `initialValues?.repoPath ?? ""`.

### Known repos available on client
- `web-app/src/lib/hooks/useSessionRepoPaths.ts:8` — `useSessionRepoPaths()` returns deduplicated `repoRoot` strings from all sessions in the Redux store. Already used in `RepoPathInput.tsx:142`. This is the complete set of repos the system knows about.

---

## 2. Touchpoints for the change

| Layer | File | Change needed |
|---|---|---|
| Proto | `proto/session/v1/backlog.proto` | Add `repeated string known_repo_paths = 3` to request; add `string suggested_repo_path = 5` to draft |
| Go backend | `session/backlog_intent.go` | Update `BuildBacklogIntentUserPrompt` to include the known_repo_paths list + instruct LLM to select one |
| Go backend | `server/services/backlog_service_intent.go` | Pass `req.Msg.KnownRepoPaths` through; map returned selection into `Draft.SuggestedRepoPath` |
| TS client | `web-app/src/lib/hooks/useBacklogService.ts` | Add `knownRepoPaths?: string[]` param to `parseBacklogItemIntent`; add `suggestedRepoPath?: string` to `ParsedBacklogItemDraft` interface |
| React | `web-app/src/components/backlog/BacklogItemIntentReview.tsx` | Call `useSessionRepoPaths()`, pass to `parseBacklogItemIntent`; map `suggestedRepoPath` in `draftToInitialValues` |
| React | `web-app/src/components/backlog/BacklogItemIntentReview.tsx` | Show auto-suggest indicator chip on `repoPath` field (FR-6) |

---

## 3. LLM prompt design for repo selection

The current `backlogIntentSystemPrompt` is a stable, prefix-cached string. Adding repo-selection instruction requires either:

**Option A**: Extend the system prompt with a static placeholder instruction about repo selection (when a list is present in the user prompt).
- Pro: system prompt stays stable (cache-friendly), dynamic list in user prompt only.
- Con: the LLM must be instructed to return a new field in the JSON schema.

**Option B**: Add a second JSON field `suggested_repo_path` to the schema in the system prompt.
- This changes the system prompt, breaking prefix caching.
- But only for callers that send `known_repo_paths` — if we keep the old prompt for calls without the list, we preserve caching for the majority.

**Chosen approach (Option A, revised):**
- Keep the system prompt stable by extending it to mention the optional `suggested_repo_path` field: it is set to the best-matching repo path from a list provided in the user prompt, or an empty string.
- The user prompt conditionally appends a `Known repos: [...]` block only when the list is non-empty.
- Since the system prompt itself changes, prefix caching breaks for all calls — but this is a one-time change that will re-warm quickly.

**Updated JSON schema in system prompt:**
```json
{"title":"...","description":"...","acceptance_criteria":["..."],"confidence":0.9,"suggested_repo_path":""}
```
`suggested_repo_path` = one of the provided paths (exact string from the list), or `""` if none matches. The LLM must never invent a path not in the list.

---

## 4. Proto field numbering

Existing `ParseBacklogItemIntentRequest` fields: `message=1`, `repo_path=2`. Add `known_repo_paths=3`.  
Existing `ParsedBacklogItemDraft` fields: `title=1`, `description=2`, `acceptance_criteria=3`, `confidence=4`. Add `suggested_repo_path=5`.

Both changes are additive (no field renumbering or removal) — backward-compatible per proto3 rules.

---

## 5. Client repo list sizing

`useSessionRepoPaths()` returns all unique `repoRoot` strings from all sessions. In practice this is typically 1–5 repos, but could grow to dozens on a well-used installation. We should:
- Cap at 20 paths (beyond that, token cost grows but LLM accuracy doesn't improve).
- Send only the path, not metadata (the LLM will match basename/component against text).

---

## 6. UI indicator (FR-6)

When `suggestedRepoPath` is set, `BacklogItemIntentReview` should render a small dismissible chip near the `repoPath` field (below or inline) saying "Suggested from your sessions". Clicking ✕ on it sets `repoPath` back to `""` so the user can clear the auto-suggestion without deleting it by hand.

`BacklogItemIntentReview` currently renders `BacklogItemForm` as a black box. To show the indicator, we need to either:
- (a) Track `wasAutoSuggested: boolean` state in `BacklogItemIntentReview` and pass it as a prop to `BacklogItemForm`, or
- (b) Pass `suggestedRepoPath` to a new prop on `BacklogItemForm` which renders its own chip.

Option (b) keeps the indicator co-located with the `repoPath` field and is cleaner.

---

## 7. Tests to update / add

- `server/services/backlog_service_intent_test.go` — add test for `known_repo_paths` populated → correct path in `SuggestedRepoPath`; add test for empty list → no suggestion.
- `session/backlog_intent_test.go` — add tests for `BuildBacklogIntentUserPrompt` with `knownRepoPaths` non-empty.
- `web-app/src/components/backlog/BacklogItemIntentReview.test.tsx` — add test that when `parseBacklogItemIntent` returns a `suggestedRepoPath`, the form's `repoPath` field is pre-populated.
- `web-app/src/components/sessions/__tests__/Omnibar.backlogIntentReview.test.tsx` — add test for the suggest chip rendering.

---

## 8. Edge cases

1. **Repo path contains a component that matches text ambiguously** (e.g. two repos named `api`). The LLM resolves this by selecting the best match; if it returns an empty string, the form degrades to today's blank behavior. No regression.
2. **`known_repo_paths` not present** (old client talking to new server, or test setup). Proto3 default is empty slice — server passes empty list to prompt, no `Known repos:` block injected, `suggested_repo_path` returned empty. No regression.
3. **New server field not yet in generated code** (stale `make build`). The gen/ directory is gitignored and must be regenerated. The implementation task must call `make build` (which re-generates protos via `buf generate`).
