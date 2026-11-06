# Requirements: Backlog AI Repo Suggestion

**Item ID:** 5b404ff3-0b24-4e40-9f5d-037d9bd07e98  
**Date:** 2026-10-11  
**Project name:** backlog-ai-repo-suggestion

---

## Problem Statement

When a user types `backlog: <text>` in the Omnibar to create a backlog item via the AI intent-review flow, the resulting `BacklogItemIntentReview` form's `repoPath` field is always blank. The LLM that parses the free-text message also has no repo context, so its generated title/description/ACs cannot be repo-specific.

The user often names the relevant repo in their text (e.g. `backlog: fix broken auth in stapler-squad`), but the system ignores this signal entirely. It has no mechanism to:
1. Extract a repo mention from the text
2. Match it against the known session repos the system already tracks
3. Feed it back to the LLM for richer, repo-aware drafts
4. Pre-populate the `repoPath` field in `BacklogItemForm` so the item is associated with the correct repo without manual entry

---

## Context

### Current flow

1. User types `backlog: fix foo in owner/repo` in the Omnibar → `ChatBacklogItemDetector` fires.
2. On submit, `Omnibar.tsx` sets `backlogReviewText` to the parsed text and renders `BacklogItemIntentReview`.
3. `useParseBacklogItemIntent` calls `parseBacklogItemIntent(text)` in `useBacklogService.ts`.
4. `useBacklogService.ts` comment (line 983-985) explicitly notes that `repo_path` is omitted because "the omnibar has no current repo context."
5. The RPC calls `ParseBacklogItemIntent` with only `{ message }`.
6. LLM produces a draft (title, description, ACs, confidence) with no repo awareness.
7. `draftToInitialValues` maps the draft to `BacklogItemForm.initialValues`, omitting `repoPath`.
8. User must manually type the repo path into the `RepoPathInput`.

### What exists that this feature can build on

- `useSessionRepoPaths()` hook already extracts deduplicated `repoRoot` strings from all known sessions (used in `RepoPathInput.tsx`).
- `ParseBacklogItemIntentRequest` already has an optional `repo_path` field (proto field #2).
- `BuildBacklogIntentUserPrompt` already appends `"\n\nRepo: <path>"` when `repoPath != ""`.
- `ParsedBacklogItemDraft` proto message can gain a new `suggested_repo_path` field with no breaking change.

---

## Functional Requirements

### FR-1 — Client collects known repos and sends them in the intent-parse request
When the user triggers the `backlog:` flow, the client collects the list of known repo root paths (from `useSessionRepoPaths()`) and includes them in the `ParseBacklogItemIntentRequest` (as a new repeated field `known_repo_paths`). The client must dedup and cap the list to a reasonable size (≤20) to bound token cost.

### FR-2 — Server LLM picks the best-matching repo from the known list
`ParseBacklogItemIntent` on the backend passes the known repo list to the LLM prompt. The LLM is instructed to select the best-matching repo path from the list (or return empty string if none fits). The selected path is returned in a new `suggested_repo_path` field on `ParsedBacklogItemDraft`.

### FR-3 — Client pre-populates repoPath in BacklogItemForm from the suggestion
If `ParsedBacklogItemDraft.suggested_repo_path` is non-empty, `draftToInitialValues` (in `BacklogItemIntentReview.tsx`) maps it to `initialValues.repoPath`. The user sees the repo pre-filled and can accept, change, or clear it.

### FR-4 — Server uses suggested_repo_path as LLM prompt context
When a repo is suggested, the same repo path is appended to the user prompt (the existing `Repo: <path>` line in `BuildBacklogIntentUserPrompt`), letting the LLM generate more targeted ACs and descriptions.

### FR-5 — Graceful degradation when no match is found
If no known repos match (list empty, or LLM selects none), the behavior is identical to today — `repoPath` remains blank in the form. No error surfaced.

### FR-6 — UI shows a confidence indicator when a repo was auto-suggested
`BacklogItemIntentReview` should visually indicate when `repoPath` was auto-suggested (e.g. a small dismissible chip or note on the field), so the user knows it was inferred rather than entered. This keeps the auto-fill transparent.

---

## Non-Functional Requirements

- **Token cost**: The additional prompt text (repo list) should be bounded — send only repo basenames or short paths, not full absolute paths, to the LLM when the list is long, while still returning the full path in the draft response.
- **Latency**: No new network round-trips; the repo list is available synchronously on the client from the Redux store.
- **Proto compatibility**: New fields (`known_repo_paths` on the request, `suggested_repo_path` on the draft) must be optional/repeated with zero values that preserve the current (no-op) behavior.

---

## Out of Scope

- Extracting repo suggestions from text without a known list (pure LLM inference of an arbitrary path).
- Suggesting repos not present in the user's current sessions.
- Automatically filing to GitHub Issues in the suggested repo.
- Changing how the Omnibar's main session-creation flow handles repo suggestions.
