# Plan: Backlog AI Repo Suggestion

**Date:** 2026-10-11  
**Project:** backlog-ai-repo-suggestion

---

## Approach

Extend the `ParseBacklogItemIntent` RPC to accept a list of known repo paths from the client and return a suggested repo path in the draft response. The client collects known repos from the Redux session store, sends them in the request, and pre-populates `BacklogItemForm.repoPath` from the response.

All changes are additive (no existing API, behavior, or test broken). The feature degrades gracefully when the list is empty or the LLM finds no match.

---

## Task Breakdown

### Task 1 — Proto: add `known_repo_paths` to request and `suggested_repo_path` to draft
**File:** `proto/session/v1/backlog.proto`

- Add `repeated string known_repo_paths = 3;` to `ParseBacklogItemIntentRequest`.
- Add `string suggested_repo_path = 5;` to `ParsedBacklogItemDraft`.

After editing, run `make build` (triggers `buf generate`) to regenerate Go + TS bindings.

---

### Task 2 — Go: update `BuildBacklogIntentUserPrompt` to inject known repos
**File:** `session/backlog_intent.go`

- Change the system prompt `backlogIntentSystemPrompt` to include `suggested_repo_path` in the JSON schema: `{"title":"...","description":"...","acceptance_criteria":[...],"confidence":0.9,"suggested_repo_path":""}`. Add a rule: "6. suggested_repo_path: if a `Known repos:` list is provided below, set this to the exact path from that list that best matches the task's described repo, or an empty string if none matches. Never invent a path not in the list."
- Update `BuildBacklogIntentUserPrompt(message, repoPath string)` signature to `BuildBacklogIntentUserPrompt(message, repoPath string, knownRepoPaths []string) string`. Append a `Known repos:\n<path>\n<path>...` block when `len(knownRepoPaths) > 0`.
- Update `BacklogIntentDraft` struct to add `SuggestedRepoPath string \`json:"suggested_repo_path"\``.

---

### Task 3 — Go: thread `KnownRepoPaths` through the service handler
**File:** `server/services/backlog_service_intent.go`

- Pass `req.Msg.KnownRepoPaths` (capped to 20) to `BuildBacklogIntentUserPrompt`.
- Map `draft.SuggestedRepoPath` → `Draft.SuggestedRepoPath` in the response.

---

### Task 4 — Go tests for the updated intent logic
**Files:** `session/backlog_intent_test.go`, `server/services/backlog_service_intent_test.go`

- `backlog_intent_test.go`: add cases for `BuildBacklogIntentUserPrompt` with `knownRepoPaths` non-empty → prompt includes `Known repos:` block; empty list → no block.
- `backlog_service_intent_test.go`:
  - Add `TestParseBacklogItemIntent_should_PopulateSuggestedRepoPath_When_LLMSelectsOne`: mock headless call returns JSON with `suggested_repo_path` set; assert `resp.Draft.SuggestedRepoPath` matches.
  - Add `TestParseBacklogItemIntent_should_LeaveEmptySuggestedRepoPath_When_NoKnownRepos`: empty `KnownRepoPaths` list → `SuggestedRepoPath == ""`.

---

### Task 5 — TS client: update `ParsedBacklogItemDraft` interface and `parseBacklogItemIntent` signature
**File:** `web-app/src/lib/hooks/useBacklogService.ts`

- Add `suggestedRepoPath?: string` to the `ParsedBacklogItemDraft` interface.
- Update `parseBacklogItemIntent` signature to `parseBacklogItemIntent(message: string, knownRepoPaths?: string[]): Promise<ParsedBacklogItemDraft | null>`.
- In the implementation, send `knownRepoPaths: knownRepoPaths?.slice(0, 20) ?? []` in the RPC request.
- Map `resp.draft.suggestedRepoPath` in the return object.
- Update the type declaration on `useBacklogService`'s return interface.

---

### Task 6 — React: `BacklogItemIntentReview` collects repos and pre-populates form
**File:** `web-app/src/components/backlog/BacklogItemIntentReview.tsx`

- Import `useSessionRepoPaths`.
- In `useParseBacklogItemIntent`, call `useSessionRepoPaths()` and pass to `parseBacklogItemIntent`.
- In `draftToInitialValues`, map `draft.suggestedRepoPath` → `{ ...existing, repoPath: draft.suggestedRepoPath || undefined }`.
- Add `suggestedRepoPath?: string` prop to `BacklogItemIntentReview` internal state and thread it to `BacklogItemForm`.

---

### Task 7 — React: `BacklogItemForm` accepts and renders a repo suggestion chip
**File:** `web-app/src/components/backlog/BacklogItemForm.tsx`

- Add optional `autoSuggestedRepoPath?: string` prop.
- When prop is set and `repoPath === autoSuggestedRepoPath`, render a dismissible `[Suggested from your sessions ✕]` chip adjacent to the `RepoPathInput`.
- Clicking ✕ calls `setRepoPath("")` and clears the suggestion (user opted out).

---

### Task 8 — Frontend tests
**Files:**
- `web-app/src/components/backlog/BacklogItemIntentReview.test.tsx`
- `web-app/src/components/sessions/__tests__/Omnibar.backlogIntentReview.test.tsx`

- Add test: when `parseBacklogItemIntent` mock returns `suggestedRepoPath = "/home/user/code/my-repo"`, `BacklogItemForm` renders with that repo path pre-populated.
- Add test: suggestion chip renders when `autoSuggestedRepoPath` prop is set.
- Add test: clicking ✕ on the suggestion chip clears the repo path field.

---

## Pattern Decisions

### Why server-side LLM selection, not client-side fuzzy match?
The LLM already parses the free text — it can simultaneously select the matching repo from the list without an extra round-trip. Client-side fuzzy matching (e.g. Fuse.js on basenames) would be cheaper but less accurate for ambiguous names, abbreviations, or non-English text. Since the LLM is already invoked, the marginal cost of adding a JSON field to its output is low.

### Why add `known_repo_paths` to the request (not just infer from `message`)?
The LLM doesn't know about the user's local filesystem. A pure inference approach would return arbitrary guesses — the design explicitly prohibits inventing a path not in the provided list (constraint in the system prompt). This makes the behavior deterministic and safe.

### Why cap at 20 repos?
A typical installation has 1–5 repos. Sending 50+ short paths to the LLM only adds ~1000 tokens with no accuracy benefit. The cap prevents runaway token cost without affecting the common case.

### System prompt stability
Changing the system prompt breaks prefix caching. This is a one-time cost that re-warms in one call. The trade-off is acceptable because (a) the updated prompt is stable for all future calls, and (b) `suggested_repo_path` is only populated when `known_repo_paths` is non-empty, so the new field adds zero noise for empty-list callers.

---

## Adversarial review

**Risk: LLM hallucinates a path not in the list.**
- Mitigation: the system prompt rule says "Never invent a path not in the list." In practice, even if the LLM violates this, `draftToInitialValues` can validate `suggestedRepoPath` against `knownRepoPaths` before using it. Add a client-side guard: if the returned path is not in `knownRepoPaths`, treat it as empty.

**Risk: Proto field numbers collide.**
- Verified: `ParseBacklogItemIntentRequest` has fields 1–2 so `3` is free. `ParsedBacklogItemDraft` has fields 1–4 so `5` is free.

**Risk: Breaking the existing `TestParseBacklogItemIntent_should_ThreadRepoPathIntoPrompt_When_RepoPathSet` test.**
- `BuildBacklogIntentUserPrompt` signature changes. The existing test must be updated to pass `nil` as `knownRepoPaths`. This is a mechanical fix (add one argument to every existing call site — there are exactly 2: the test and the service handler).

**Risk: `useSessionRepoPaths()` called inside `useParseBacklogItemIntent` (a non-component context).**
- `useParseBacklogItemIntent` is a hook (called with `use` prefix), so calling other hooks inside it is valid. No issue.

**Risk: Chip layout breaks on mobile.**
- The chip is dismissible inline text next to the label. It must respect the existing `RepoPathInput` layout. Use a `<span>` with `data-testid` rather than a floating element to avoid layout shift.
