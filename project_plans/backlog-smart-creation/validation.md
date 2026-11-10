# Validation Plan: Backlog Smart Creation

## AC-to-Test Mapping

### AC-1 — Repo path auto-detection

| Test | Type | File |
|---|---|---|
| `BacklogComposeWidget` renders a repo path combobox pre-filled with the first `suggestedRepoPaths` entry | Unit | `BacklogComposeWidget.test.tsx` |
| `BacklogComposeWidget` with no `suggestedRepoPaths` renders an empty editable field | Unit | `BacklogComposeWidget.test.tsx` |
| Omnibar passes `useSessionRepoPaths()` result to `BacklogComposeWidget` as `suggestedRepoPaths` | Integration | `Omnibar.backlogCompose.test.tsx` |
| User can clear the pre-filled path and submit with empty string | Unit | `BacklogComposeWidget.test.tsx` |
| User can select a different path from the dropdown | Unit | `BacklogComposeWidget.test.tsx` |

---

### AC-2 — `triageTriggered` truthful notification

| Test | Type | File |
|---|---|---|
| Notification says "triage underway" when `triageTriggered === true` | Unit | `Omnibar.backlogCompose.test.tsx` |
| Notification says "add a repo path" when `triageTriggered === false` AND `repoPath === ""` | Unit | `Omnibar.backlogCompose.test.tsx` |
| Notification says "check the repo path" when `triageTriggered === false` AND `repoPath !== ""` | Unit | `Omnibar.backlogCompose.test.tsx` |
| Error notification is shown when `createBacklogItemFromChat` returns null | Unit | `Omnibar.backlogCompose.test.tsx` |

---

### AC-3 — Background AI enrichment

| Test | Type | File |
|---|---|---|
| `parseBacklogItemIntent` is called with the submitted text and `knownRepoPaths` when `triageTriggered === false` | Unit | `Omnibar.backlogCompose.test.tsx` |
| `updateBacklogItem` is called with the enriched title/description/ACs when parsing succeeds | Unit | `Omnibar.backlogCompose.test.tsx` |
| "Backlog item enriched" notification fires after successful enrichment | Unit | `Omnibar.backlogCompose.test.tsx` |
| No enrichment call when `triageTriggered === true` | Unit | `Omnibar.backlogCompose.test.tsx` |
| Parse failure is silent — no error notification fired | Unit | `Omnibar.backlogCompose.test.tsx` |

---

### AC-4 — Brainstorm mode

| Test | Type | File |
|---|---|---|
| "Explore with AI" button is visible in the compose widget | Unit | `BacklogComposeWidget.test.tsx` |
| Clicking the button renders `BrainstormPanel` | Unit | `BacklogComposeWidget.test.tsx` |
| Clicking again hides the panel | Unit | `BacklogComposeWidget.test.tsx` |
| Submitting a question in the panel calls `parseBacklogItemIntent` with the question + textarea text | Unit | `BrainstormPanel.test.tsx` |
| Panel does not block the submit button while an AI call is in flight | Unit | `BrainstormPanel.test.tsx` |
| Previous turns remain visible after a new turn is submitted | Unit | `BrainstormPanel.test.tsx` |

---

### AC-5 — Repo path persists to triage

| Test | Type | File |
|---|---|---|
| `createBacklogItemFromChat` proto call includes `repo_path` from the compose widget submission | Unit | `useBacklogService.test.ts` |
| Backend `CreateBacklogItemFromChat` handler passes non-empty `RepoPath` to `CreateBacklogItem` | Unit | `backlog_service_chat_test.go` |
| `MaybeTriggerTriage` returns true when `repoPath` is non-empty (existing test, verify unchanged) | Unit | `backlog_service_triage_test.go` |

---

### AC-6 — No regression

| Test | Type | File |
|---|---|---|
| Typing `backlog: some text` in the omnibar still opens `BacklogItemIntentReview` (not compose widget) | Regression | `Omnibar.backlogIntentReview.test.tsx` |
| `BacklogItemIntentReview` component tests all pass unchanged | Regression | `BacklogItemIntentReview.test.tsx` |
| Items created via the backlog form (`CreateBacklogItem` RPC directly) are unaffected | Regression | `backlog_service_lifecycle_test.go` |

---

### AC-7 — Notification accuracy (covered by AC-2 above, additional edge cases)

| Test | Type | File |
|---|---|---|
| `useBacklogCreationNotifier` does not fire enrichment notification AND triage-complete notification on the same item (dedup check) | Unit | `useBacklogCreationNotifier.test.ts` |
| Notification contains a deep link `onView` handler navigating to `/backlog?item=<id>` | Unit | `Omnibar.backlogCompose.test.tsx` |

---

## Edge Cases and Error Scenarios

### Edge Case 1 — Proto field default
When existing callers (e.g. automated scripts, MCP tools calling `CreateBacklogItemFromChat`)
omit `repo_path`, the proto3 default of `""` applies. `MaybeTriggerTriage` receives `""` and
returns false — same behavior as before. No regression possible.

### Edge Case 2 — `useSessionRepoPaths` returns empty list
The compose widget shows an empty repo path field. User must type manually or leave blank.
Notification correctly says "Add a repo path to enable triage" (not a misleading "triage starting").

### Edge Case 3 — User edits the item between creation and enrichment completion
The conditional update guard in `EnrichBacklogItem` / `UpdateBacklogItem` checks whether the
current field values match what was created (not what was enriched). If the user has edited
the description, the guard fails and enrichment skips that field. Title and ACs can still be
enriched independently.

### Edge Case 4 — Network failure during enrichment
`parseBacklogItemIntent` is fire-and-forget. A network error throws, is caught in the
`.catch()`, and is logged to console. No user-visible error notification. The item is still
accessible via the creation notification link.

### Edge Case 5 — `BacklogComposeWidget` called from a context without sessions
If `suggestedRepoPaths` is empty, the combobox renders with an empty placeholder. The user
can type a path manually or leave it blank. All notification paths still work correctly.

## Proto Regeneration Checklist

After Track A-1 (proto change):
```bash
make proto-gen          # regenerate Go + TS bindings
go build ./...          # confirm backend compiles
cd web-app && bun run build --no-minify 2>&1 | head -50  # confirm TS types compile
```

If Track B-2 adds a new RPC:
```bash
make registry-generate  # update feature registry JSON
# Verify the new RPC's JSON file has tested:true or add testIds
make ci                 # full CI pipeline
```
