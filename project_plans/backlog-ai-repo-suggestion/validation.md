# Validation: Backlog AI Repo Suggestion

**Date:** 2026-10-11

---

## Acceptance Criteria — testability review

| # | Criterion | How to verify | Status |
|---|---|---|---|
| AC-1 | When the user types `backlog: fix foo in owner/repo` and known sessions include that repo, `BacklogItemForm.repoPath` is pre-populated with the matched path | Unit test in `BacklogItemIntentReview.test.tsx`: mock `parseBacklogItemIntent` returning `suggestedRepoPath`, assert field value | Testable |
| AC-2 | When no known repo matches the text, `repoPath` remains blank (no regression) | Unit test with `suggestedRepoPath: ""` or null return | Testable |
| AC-3 | The suggestion chip is visible and correctly labeled when `repoPath` was auto-suggested | Unit test: render `BacklogItemForm` with `autoSuggestedRepoPath` set, assert `data-testid="repo-suggestion-chip"` visible | Testable |
| AC-4 | Clicking ✕ on the suggestion chip clears the `repoPath` field | Unit test: click the chip dismiss button, assert `repoPath` → `""` | Testable |
| AC-5 | `ParseBacklogItemIntent` RPC: with `KnownRepoPaths` set and a matching repo in the message, `Draft.SuggestedRepoPath` is populated | Go unit test in `backlog_service_intent_test.go` with mock headless call | Testable |
| AC-6 | `ParseBacklogItemIntent` RPC: with empty `KnownRepoPaths`, `Draft.SuggestedRepoPath` is `""` | Go unit test | Testable |
| AC-7 | `BuildBacklogIntentUserPrompt` with non-empty `knownRepoPaths` produces a prompt containing a `Known repos:` block | Go unit test in `backlog_intent_test.go` | Testable |
| AC-8 | Client-side guard: if the returned `suggestedRepoPath` is not in `knownRepoPaths`, it is ignored (treated as empty) | Unit test in `BacklogItemIntentReview.test.tsx`: mock returns path not in known list, assert `repoPath` stays blank | Testable |

---

## Risk — re-review after adversarial pass

### 1. `useSessionRepoPaths` inside `useParseBacklogItemIntent` — hook rule concern
`useParseBacklogItemIntent` is defined as a custom hook (called at the top of `BacklogItemIntentReview`). React's rules-of-hooks allow calling other hooks inside custom hooks. Calling `useSessionRepoPaths()` from `useParseBacklogItemIntent` is valid.

**However**, `useSessionRepoPaths` returns the Redux store snapshot at call time. The repo list is captured once at mount and passed to `parseBacklogItemIntent` inside the `useEffect`. Since the parse runs once on mount and the repo list doesn't change mid-parse, this is correct.

### 2. `known_repo_paths` token cost
With 20 paths of average length 40 characters, the `Known repos:` block is ~900 characters (~225 tokens). Added to the existing user prompt (~200 tokens) and system prompt (~400 tokens), the total call budget stays comfortably inside the existing `backlogIntentCallBudget = 90s` timeout. No change to the timeout needed.

### 3. `suggested_repo_path` in the JSON schema — LLM compliance
Adding a field to the JSON schema the LLM must produce is a safe change. The LLM has been producing this exact JSON shape reliably (evidenced by existing tests passing). Adding one more optional string field with an explicit "empty string if no match" fallback is a well-understood LLM instruction. Worst case: the LLM omits the field → `json.Unmarshal` leaves `SuggestedRepoPath = ""` → same as no suggestion. No hard failure.

### 4. Interface declaration change in `useBacklogService.ts`
The `parseBacklogItemIntent` type on the returned interface must be updated from `(message: string) => ...` to `(message: string, knownRepoPaths?: string[]) => ...`. All callers must be checked:
- `BacklogItemIntentReview.tsx` — sole caller; will be updated in Task 6.
- `BacklogItemIntentReview.test.tsx` — must mock the new signature.
- `dispatch.ts` / `Omnibar.tsx` — do NOT call `parseBacklogItemIntent` directly; no change needed.

### 5. Makefile touchpoint: `make build` required
After editing `.proto`, `gen/` must be regenerated with `make build` (which runs `buf generate`). This is the standard workflow for any proto change in this repo. The implementation session must do this before any Go or TS compilation step.

---

## Go / TS call-site audit

**Go call sites of `BuildBacklogIntentUserPrompt`:** (searched `backlog_intent.go`, `backlog_service_intent.go`)
- `server/services/backlog_service_intent.go:46` — the only production call. Must be updated to pass `req.Msg.KnownRepoPaths`.
- `server/services/backlog_service_intent_test.go:52` — test call. Must be updated to pass `nil` / `[]string{}` to maintain existing semantics.

**TS call sites of `parseBacklogItemIntent`:** (searched `web-app/src`)
- `web-app/src/components/backlog/BacklogItemIntentReview.tsx:65` — the only production call. Will be updated in Task 6.
- `web-app/src/components/backlog/BacklogItemIntentReview.test.tsx` — mock; must be updated to match new signature.
- `web-app/src/lib/contexts/__tests__/OmnibarContext.test.tsx` — may mock this hook; must be updated if it does.

All call sites are local (same package or same codebase) — no public API surface affected.

---

## OmnibarContext test file check

Need to verify `OmnibarContext.test.tsx` mocks `parseBacklogItemIntent`:
- File path: `web-app/src/lib/contexts/__tests__/OmnibarContext.test.tsx`
- If it mocks the entire `useBacklogService` return shape, the mock must include the new signature.

---

## Feature registry update

Per `CLAUDE.md`: after adding an RPC field change, run `make registry-generate` and commit changed files. The `ParseBacklogItemIntent` RPC already exists; this change modifies its request/response shape, not its name, so no new registry entry is needed — but `make registry-generate` should still be run to confirm no drift.

---

## Final verdict

The plan is sound. All risks are addressed. The change is minimal (2 proto fields, ~40 lines of Go, ~60 lines of TS/TSX) and fully backward-compatible. No existing test semantics are broken — only signatures must be updated to accept the new optional parameter.

Proceed to implementation.
