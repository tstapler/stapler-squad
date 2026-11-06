# Research: AI Backlog Creation Prompt

## Root Cause — Confirmed

Commit `4a85c12c5` ("dedicated compose widget for AI backlog items", Oct 11) changed
`BacklogComposeWidget.onSubmit` in `Omnibar.tsx` from routing through the AI review step
to calling `createBacklogInBackground` (fire-and-forget, no LLM).

### Exact regression site

**File:** `web-app/src/components/sessions/Omnibar.tsx`, lines 1762–1767

```tsx
// CURRENT (buggy)
onSubmit={(text) => {
  setBacklogComposeText(null);
  onClose();                            // closes omnibar immediately
  void createBacklogInBackground(text); // fire-and-forget; NO AI parsing
}}
```

The fix is three lines:

```tsx
// FIXED
onSubmit={(text) => {
  setBacklogComposeText(null);
  setBacklogReviewText(text); // hand off to BacklogItemIntentReview
  // intentionally no onClose() — BacklogItemIntentReview.onDone() closes
}}
```

The `backlogReviewText !== null` render branch (lines 1723–1746) already exists and
already has a correct `onDone` handler that closes the omnibar and navigates to the
new item:

```tsx
onDone={(item: BacklogItem) => {
  setBacklogReviewText(null);
  onClose();
  router.push(`/backlog?item=${item.id}`);
}}
```

## Two Parallel Flows (Current State)

| Flow | Trigger | AI? | Outcome |
|---|---|---|---|
| `backlog: <msg>` prefix + Enter | `ChatBacklogItemDetector` → `setBacklogReviewText` | ✅ `parseBacklogItemIntent` (LLM) | User reviews structured draft, then `createBacklogItem` |
| "Create backlog item" button + compose | `openBacklogCompose()` → `BacklogComposeWidget` → `createBacklogInBackground` | ❌ None | Raw text → `deriveChatItemTitle` (first ≤80 chars as title) |

## AI Intent Pipeline Details

`BacklogItemIntentReview` calls `parseBacklogItemIntent` via the `useParseBacklogItemIntent`
hook:

- LLM invoked via `headlessPool.CallBlocking` with feature key `FeatureKeyBacklogIntentParse`
  (`server/services/backlog_service_intent.go`)
- System prompt in `session/backlog_intent.go` (`backlogIntentSystemPrompt`): generates
  structured title, expands description, produces 2–5 testable acceptance criteria, returns
  a confidence score, picks a repo path from the known-repos list
- On parse failure: `parseFailed = true`, fallback fills `title = first line`, `description = full text`
- Loading state: `ParsingIndicator` spinner shown during LLM call
- After parsing: user sees pre-filled `BacklogItemForm` and can edit before saving

## Secondary Issue: Triage Never Fires for Compose-Created Items

`CreateBacklogItemFromChat` in `backlog_service_lifecycle.go` calls
`MaybeTriggerTriage(ctx, created.ID, req.Msg.SkipTriage, created.RepoPath)` where
`created.RepoPath` is always `""` (the RPC request has no `repoPath` field), so
`backlog_service_triage.go` line ~2826 returns `false` immediately:

```go
if skipTriage || repoPath == "" || s.headlessPool == nil { return false }
```

After the fix, items will go through `BacklogItemIntentReview` → `createBacklogItem`
(which does accept a `repoPath` populated by the AI suggestion), so triage will fire
for items where the AI correctly identified a repo. This secondary issue is resolved
as a side effect of the primary fix.

## Existing Tests

| File | Tests | Impact of Fix |
|---|---|---|
| `Omnibar.backlogCompose.test.tsx` line 224 | `"closes immediately on Create, then notifies..."` | Validates *buggy* behavior — must be replaced |
| `Omnibar.backlogCompose.test.tsx` line 282 | `"still opens the intent review for backlog: prefix (no regression)"` | Must keep passing |
| `Omnibar.backlogIntentReview.test.tsx` | End-to-end review flow | No change needed — already covers correct path |
| `BacklogItemIntentReview.test.tsx` | Parse success/failure, repo suggestion | No change needed |

## Risk Assessment

**Low risk.** The fix is a 3-line change in `Omnibar.tsx`. Both code paths already exist
and are tested in isolation. Only the submit handler changes — the compose-widget UX and
the `backlog: <msg>` omnibar-prefix path are both unaffected. Updating one test that
validates the old behavior is the only non-trivial work.

The `BacklogComposeWidget` prop contract (`onSubmit: (text: string) => void`) requires no
change — the caller's side just changes what it does with the text.
