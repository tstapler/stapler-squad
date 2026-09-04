# Validation Plan: session-notes

**Date**: 2026-08-06

## Happy Path Scenario

Given a running session with no note set (`session.note == ""`), when the user opens the
session detail view's Info tab, clicks "Add note", types a markdown note, and clicks Save,
then the note persists — visible as rendered markdown in `NotePanel`'s read mode and as a
`📝` badge on the session's `SessionCard` in the list — and survives a page reload (proxy
for a server restart, since both discard the in-memory `Instance` and rebuild it from the
same ent/SQLite-backed `LoadInstances()` read path).

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC1 — attach note (Story 1.1: ent schema) | `session/ent/schema/session.go` (generated) | *(no dedicated test — verified transitively)* | N/A | `go build ./session/...` after `ent generate` confirms `session.FieldNote` exists; covered by AC1's downstream tests below. |
| AC1 — attach note (Story 1.2: Instance round trip) | `session/instance_test.go` | `TestInstance_Note_RoundTripsThroughSerialization` | Unit (happy) | `Instance` with `Note` set via `SetNote("left this waiting on CI")` → `ToInstanceData()` → `FromInstanceData()` → reconstructed `Instance.Note == "left this waiting on CI"`. Directly exercises the Risk Control mitigation for the "missing touchpoint" risk (plan Risk Control row 1). |
| AC1 — attach note (Story 1.3/1.4: `UpdateSession` happy path) | `server/services/session_service_test.go` | `TestUpdateSession_NoteUpdate` | Unit (happy) | Mirrors `TestUpdateSession_TagsUpdate` (lines 505-536): seed a paused session, call `UpdateSession(Note: proto.String("left this waiting on CI"))`, assert `response.session.note` and the storage-reloaded instance both carry it. |
| AC1 — attach note (Story 1.3/1.4: max-length rejection) | `server/services/session_service_test.go` | `TestUpdateSession_NoteExceedsMaxLength_ReturnsInvalidArgument` | Unit (error path) | Send a 10,001-char `note`; assert `connect.CodeInvalidArgument` and that the session's stored note is unchanged (no partial write). |
| AC1 — attach note (Story 1.4: clear-note regression, guarded-vs-unconditional `SetNote`) | `server/services/session_service_test.go` | `TestUpdateSession_NoteCleared_PersistsAsEmptyAcrossReload` | Integration (ent/SQLite storage round trip) | Seed a session with `Note: "stale reminder"`, call `UpdateSession(Note: proto.String(""))`, reload via `fix.storage.LoadInstances()`; both the response's `session.note` and the reloaded instance's `Note` must be `""`. Must fail against the guarded (`if data.Note != ""`) `Update` path and pass against the unconditional one (Task 1.2.6(b)). This is the one test in the suite that actually exercises the ent repository + SQLite file, not just in-memory state — hence Integration rather than Unit. |
| AC1 — attach note (Story 2.1: NotePanel save happy path) | `web-app/src/components/sessions/NotePanel.test.tsx` | `NotePanel_should_CallOnSaveWithTypedValue_When_SaveClicked` | Unit (happy) | Render `NotePanel` with `note=""`, click "Add note", type into `data-testid="session-note-textarea"`, click `data-testid="session-note-save-button"`; assert `onSave("...")` called and panel returns to read mode. |
| AC1 — attach note (Story 2.1: NotePanel save error path) | `web-app/src/components/sessions/NotePanel.test.tsx` | `NotePanel_should_PreserveTextareaAndShowAssertiveError_When_OnSaveRejects` | Unit (error path) | `onSave` mock rejects; assert textarea still shows the typed text, panel stays in edit mode, and an `aria-live="assertive"` error element renders. |
| AC1 — attach note (Story 2.1: wiring) | `web-app/src/lib/hooks/useSessionService.test.ts` (existing file, extend) | `updateSession_should_IncludeNoteInRequestBody_When_UpdatesContainNote` | Unit (happy) | Calls the hook's `updateSession({ note: "x" })`; asserts the ConnectRPC call body includes `note: "x"` — regression guard against the "unlisted whitelist key silently dropped" failure mode the plan calls out in Task 2.1.4. |
| AC2 — renders as markdown in read mode (Story 2.1) | `web-app/src/components/sessions/NotePanel.test.tsx` | `NotePanel_should_RenderMarkdownElements_When_NoteContainsGfmSyntax` | Unit (happy) | `note = "**Blocked** — see [PR #482](https://x)"`; assert `data-testid="session-note-rendered"` contains a `<strong>` and an `<a href="https://x">`. |
| AC2 — heading remap (a11y regression guard, Pattern Decision 8) | `web-app/src/components/sessions/NotePanel.test.tsx` | `NotePanel_should_RemapHeadingToNonPageLevelTag_When_NoteContainsH1Syntax` | Unit (error/edge path) | `note = "# Heading"`; assert the rendered output does NOT contain a raw `<h1>`/`<h2>` element (remapped per Pattern Decision 8's `components` prop). |
| AC2 — round trip preserves content for rendering (integration angle) | *(covered by `TestUpdateSession_NoteUpdate` above — no separate integration test needed)* | — | N/A | Rendering is a pure frontend concern once `session.note` is correctly persisted/read; the persistence round trip is already covered under AC1. No data-store call originates from the render path itself. |
| AC3 — SessionCard indicator, non-empty note (Story 2.2) | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_RenderNoteBadge_When_SessionNoteIsNonEmpty` | Unit (happy) | Session fixture with `note: "waiting on CI"`; assert `data-testid="badge-has-note"` renders. |
| AC3 — SessionCard indicator, whitespace/empty gating (Story 2.2) | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_NotRenderNoteBadge_When_SessionNoteIsWhitespaceOrEmpty` | Unit (error/edge path) | Two fixtures: `note: "   \n"` and `note: ""`; assert neither renders `data-testid="badge-has-note"` — this is the "most likely to regress silently" case per `design/ux.md` Surface 1 edge cases. |
| AC3 — tooltip plain-text truncation | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_TruncateNoteBadgeTooltipToOneHundredTwentyChars_When_NoteIsLong` | Unit (happy) | Note >120 chars; assert `Tooltip`'s `label` prop equals `truncateGoal(note.trim(), 120)`. |
| AC3 — indicator sourced from persisted data (integration angle) | *(covered by `TestUpdateSession_NoteUpdate` / `TestUpdateSession_NoteCleared_PersistsAsEmptyAcrossReload` above)* | — | N/A | `SessionCard` renders from already-fetched `session.note`; no direct store/API call in the component itself. |
| AC4 — persists across server restarts (Go persistence layer) | `server/services/session_service_test.go` | `TestUpdateSession_NoteUpdate` (reload assertion) and `TestUpdateSession_NoteCleared_PersistsAsEmptyAcrossReload` | Integration | Both tests reload via `fix.storage.LoadInstances()` — the same ent/SQLite read path a real server restart exercises (in-memory `Instance` is discarded and rebuilt identically either way). This is the test-level proof of AC4's actual guarantee. |
| AC4 — persists across server restarts (end-to-end proxy) | `tests/e2e/session-notes.spec.ts` | `session-notes should_PersistNoteAndBadgeAcrossPageReload` | E2E | Add a note, `page.reload()`, re-assert both the rendered note (`getNoteRenderedBody()`) and the `SessionCard` badge (`badge-has-note`) survive. Documented in the plan (Story 3.2) as a reasonable proxy given the Playwright harness spins up one server per run — a literal restart isn't practical there; the Go-level integration tests above cover the literal persistence guarantee. |

**N/A — ent auto-migration, no manual migration.** `plan.md`'s "Migration Plan" section states
ent's auto-migration (`client.Schema.Create(ctx)` at startup) handles the new `note` column;
there is no manual/reversible migration file to test. No migration test step is included in
this validation plan.

## UX Acceptance Tests

Source: `design/ux.md` "UX Acceptance Criteria" (17 criteria: task completion 1-4, error/edge
5-9, accessibility 10-16, cross-panel disambiguation 17). These are human-verifiable behavioral
scenarios distinct from the unit/integration tests above — most are automated via Playwright
against the real rendered app (per the `ui-playwright` implementation model and this repo's
`tests/e2e/` conventions: `data-testid`/ARIA locators only, no `waitForTimeout`); a few are
either better suited to Jest/RTL (fixture-driven component checks) or are explicitly flagged
in the design doc as non-automatable human judgment calls.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. Attach note in ≤3 interactions | `tests/e2e/session-notes.spec.ts` | `should_AttachNoteInThreeInteractions_When_UserAddsNoteFromDetailView` | Playwright | Open session detail (already open) → click "Add note" (1) → type text → click Save (2, typing not counted) → assert saved/read mode. Count only click/keypress *actions*, not typing. |
| 2. View existing note in 1 click (details open by default) | `tests/e2e/session-notes.spec.ts` | `should_ShowRenderedNoteWithoutExtraExpandStep_When_DetailViewOpened` | Playwright | Open a session with an existing note → assert `getNoteRenderedBody()` is visible immediately, with no additional `<summary>` click needed (`<details open>`). |
| 3. Discard in-progress edit in 1 click, no confirm dialog | `tests/e2e/session-notes.spec.ts` | `should_DiscardDraftInOneClick_When_CancelClicked` | Playwright | Enter edit mode, type a draft, click `Cancel` once → assert textarea/panel reverts to prior read-mode content, no dialog/modal appeared, and no `onSave` call occurred (via a route/network assertion). |
| 4. Identify noted sessions from card grid alone | `tests/e2e/session-notes.spec.ts` | `should_ShowNoteBadgeInSessionListWithoutOpeningDetail_When_SessionHasNote` | Playwright | From the session list view, assert `badge-has-note` is visible on the target card without navigating into detail view. |
| 5. Save failure retains exact typed text | `tests/e2e/session-notes.spec.ts` | `should_RetainExactTypedText_When_SaveRequestFails` | Playwright | Intercept the `UpdateSession` RPC route to fail (`page.route(...).fulfill({status: 500})` or abort), type text, click Save, assert textarea value is unchanged before/after the rejected call. |
| 6. Inline `aria-live="assertive"` error adjacent to textarea | `tests/e2e/session-notes.spec.ts` | `should_ShowInlineAssertiveErrorAdjacentToTextarea_When_SaveFails` | Playwright | Same failure-injection setup as #5; assert an element with `aria-live="assertive"` is visible and located adjacent to the textarea (not a dismissed/separate toast region). |
| 7. No dead ends — retry and exit both available in error state | `tests/e2e/session-notes.spec.ts` | `should_KeepSaveAndCancelBothEnabled_When_InErrorState` | Playwright | After a failed save, assert both `session-note-save-button` and the Cancel button are visible and enabled (not disabled), and that clicking Cancel exits edit mode cleanly. |
| 8. Badge never renders for whitespace-only/empty note | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_NotRenderNoteBadge_When_SessionNoteIsWhitespaceOrEmpty` | Jest/RTL | (Same test as the AC3 unit test above — fixture-driven, cheaper and more precise as a component test than a full e2e flow; deliberately reused rather than duplicated in Playwright.) |
| 9. Overflow cannot be saved as literal overflow | `tests/e2e/session-notes.spec.ts` | `should_BlockTypingPastCapAndRouteServerRejectionThroughErrorPath_When_NoteExceedsMaxLength` | Playwright | Attempt to type/paste >10,000 chars; assert textarea value length is capped at 10,000 (native `maxLength`). Separately (same test or a sibling case), simulate a bypassed-cap server rejection via route interception returning `InvalidArgument`, and assert it surfaces through the same Surface 5 error path as #6, not a crash or silent truncation. |
| 10. Full keyboard navigation, no mouse required | `tests/e2e/session-notes.spec.ts` | `should_SupportFullKeyboardOnlyFlow_When_NoMouseUsed` | Playwright | Using only `page.keyboard.press("Tab"/"Enter"/"Space")`, reach and activate Add note/Edit, type into the textarea, reach and activate Save and (in a separate run) Cancel — assert each transition completes without any `page.click`/`page.mouse` call. |
| 11. Focus management: edit→textarea, exit→Edit button | `tests/e2e/session-notes.spec.ts` | `should_MoveFocusToTextareaThenBackToEditButton_When_EnteringAndExitingEditMode` | Playwright | Click Edit → assert `page.evaluate(() => document.activeElement.dataset.testid)` equals the textarea's testid; click Save (success path) → assert active element is the Edit button, not `<body>`. Repeat for the Cancel exit path. |
| 12. Programmatic labels present (textarea, badge) | `tests/e2e/session-notes.spec.ts` | `should_ExposeAccessibleLabelsOnTextareaAndBadge_When_Rendered` | Playwright | Assert textarea has `aria-label="Session note (markdown)"` and `aria-describedby` pointing to a hint element containing "Markdown supported"; assert the badge has `role="img"` and `aria-label="Has a note"`. |
| 13. Distinct live regions for polite vs assertive | `tests/e2e/session-notes.spec.ts` | `should_UseDistinctAriaLiveRegionsForSuccessAndError_When_SaveSucceedsOrFails` | Playwright | Trigger a successful save and a failed save in separate runs; assert the `aria-live="assertive"` error node and any `aria-live="polite"` success node (if present) are different DOM elements, never the same node reused for both severities. |
| 14. Heading hierarchy preserved (a11y CI gate) | `tests/e2e/session-notes.spec.ts` + axe CI | `should_PreserveHeadingHierarchy_When_NoteContainsH1Syntax` | Playwright + axe-core (existing UX-analysis CI job) | Save a note containing `# Heading`; assert the rendered tag is not `<h1>`/`<h2>` (matches Pattern Decision 8's remap); rely on the repo's existing PR-level Axe Core CI check (blocks on WCAG AA violations) against this page as the authoritative a11y gate rather than reimplementing axe rules in the spec. |
| 15. Color contrast via existing theme tokens only | *(static/manual — no runtime test)* | `manual: grep NotePanel.css.ts and NotePanel.tsx for hardcoded colors` | Manual code review + `.claude/rules/css-architecture.md` | Reviewer greps the new `.css.ts`/`.tsx` files for hex literals or `var(--undefined-token)` usage; CI's `lint:css` step (per `css-architecture.md`) also fails the build on any undefined CSS var, providing an automated backstop even though this specific criterion isn't a bespoke test. |
| 16. Touch targets ≥44×44px on mobile | `tests/e2e/session-notes.spec.ts` | `should_MeetMinimumTouchTargetSize_When_ViewedOnMobileViewport` | Playwright (mobile viewport) | Set a mobile viewport (e.g. `page.setViewportSize({width: 375, height: 667})`), locate Edit/Add note/Save/Cancel buttons, assert each `boundingBox()` has `width >= 44` and `height >= 44`. |
| 17. Notes-vs-Goal disambiguation (cross-panel) | *(not automated)* | `manual: design QA checklist — GoalPanel vs NotePanel side-by-side review` | Manual design review | Explicitly flagged in `design/ux.md` as "a design-review-time human judgment call, not an automatable test" — checked before implementation ships by visually confirming `NotePanel` never gains a status chip/progress fraction and retains its distinct "Notes" label + Edit affordance versus `GoalPanel`'s read-only, agent-framed presentation. |

## Test Stack

- **Unit**: Go (`go test ./server/services`, `go test ./session/...`) for backend handler/instance/serialization logic; Jest/RTL (`cd web-app && npx jest --no-coverage --testPathPatterns="NotePanel.test|SessionCard.test|useSessionService.test"`) for component and hook logic.
- **Integration**: Go tests that round-trip through the real ent/SQLite repository via `fix.storage.LoadInstances()` (`TestUpdateSession_NoteUpdate`'s reload assertion, `TestUpdateSession_NoteCleared_PersistsAsEmptyAcrossReload`) — these are the tests that actually exercise the persistence layer rather than in-memory state alone.
- **E2E / UX**: Playwright (`cd tests/e2e && npx playwright test session-notes.spec.ts`), following `.claude/rules/e2e-test-conventions.md` (feature annotation `// @feature session:update, session-notes-panel, session-notes-card-indicator`, `data-testid`/ARIA locators only, no `waitForTimeout`). Two UX criteria (8, 17) are deliberately satisfied by existing/adjacent artifacts (a Jest test, a manual design-QA checklist) rather than new Playwright cases, to avoid duplicating cheaper, more precise coverage or attempting to automate an explicitly-human judgment call.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods: happy path + error paths covered — `UpdateSession`'s note-handling
  branch has both (`TestUpdateSession_NoteUpdate`, `TestUpdateSession_NoteExceedsMaxLength_ReturnsInvalidArgument`,
  `TestUpdateSession_NoteCleared_PersistsAsEmptyAcrossReload`).
- All external integrations: unit mocked + at least one integration test — the only "external"
  dependency here is the ent/SQLite repository; `TestUpdateSession_NoteCleared_PersistsAsEmptyAcrossReload`
  and the reload assertion in `TestUpdateSession_NoteUpdate` are the integration tests satisfying
  this; no other external call (no network/3rd-party service) is introduced by this feature.
- UX acceptance criteria: each of the 17 criteria in `design/ux.md` has a corresponding test or
  manual step in the table above (15 automated, 2 explicitly manual — criteria 8 via an existing
  Jest test reused rather than duplicated, criterion 17 flagged by the design doc itself as a
  non-automatable human judgment call).
- Feature registry: per `.claude/rules/feature-registry.md` (Story 3.1), `docs/registry/features/backend/session/update.json`
  must list `TestUpdateSession_NoteUpdate` and `TestUpdateSession_NoteExceedsMaxLength_ReturnsInvalidArgument`
  in `testIds`; the two new frontend registry entries (`session-notes-panel`, `session-notes-card-indicator`)
  must list their corresponding `NotePanel.test.tsx`/`SessionCard.test.tsx` test names — run
  `make registry-generate` and confirm `docs/registry/coverage-gaps.json`'s count does not grow.
