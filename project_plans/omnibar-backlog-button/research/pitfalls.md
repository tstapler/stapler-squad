# Pitfalls: Create Backlog Item footer button (OmnibarCreationPanel)

Note: commit dfc17acf5 already implements the button (OmnibarCreationPanel.tsx:1024-1033, Omnibar.tsx:2127, test at OmnibarCreationPanel.test.tsx:55-68). Findings below are checked against that code. Static reading only; nothing was run.

## 1. Keyboard / focus / Escape / Enter (highest risk)

- `handleKeyDown` is attached to the modal wrapper div (`Omnibar.tsx:1799`), so it sees keydowns bubbling from footer buttons. Its fallthrough `else if (e.key === "Enter") handleSubmitRef.current()` (`Omnibar.tsx:1241-1246`) does not check `e.target`. Pressing Enter on the focused Create Backlog button therefore fires the button's click (opens compose) AND bubbles to `handleSubmit`, which can submit a session when `canSubmit` is true. Cancel already has the same latent issue; the new button makes it reachable. Fix: `onKeyDown={(e) => e.stopPropagation()}` on the footer buttons, or an `e.target`-is-button guard in `handleKeyDown`. Needs a test: Enter on the button does not call `onSubmit`.
- Escape: creation mode Escape goes back to discovery (`Omnibar.tsx:1226-1237`) and calls `stopImmediatePropagation` to suppress the global listener (`:1293-1295`). The global listener skips close while `backlogComposeText !== null`. The button only sets compose text, so no new Escape path. The widget's Escape returns to the input and focus is restored by the effect at `:1281-1287`, but that effect focuses `inputRef` in the creation form, not the footer button. Focus return to the originating button is lost (a11y nit).
- No focus trap exists in Omnibar (grep for focusTrap/FocusTrap: none). The new button only adds a tab stop. Tab key handling: cycles session type only when no dropdown is open and the target is the input path (`:1216-1223`); verify it does not hijack Tab when focus is on the button, since the handler is on the wrapper.
- Seed text: `openBacklogCompose` seeds from `input.trim()` (`Omnibar.tsx:1030-1034`). In creation mode `input` is the path/URL field, so clicking the footer button pre-fills the compose textarea with a path like `/home/x/repo`. Decide whether this is wanted or whether creation-mode should seed empty. Not covered by any test.
- In creation mode the click discards unsaved form state? Compose mode returns early from the render (`:1748`), so the creation panel unmounts. Whether formState survives is held in `formState` hook state (likely retained), but compose Cancel only refocuses the input; confirm form fields are still populated after cancel.

## 2. Footer layout at narrow widths

- `footer` style (`Omnibar.css.ts:238-245`): `display:flex; justifyContent:flex-end; gap:8; padding:16`, no `flexWrap`. The modal has `overflowX: hidden` (`:61`). Three buttons ("Create Backlog Item", Cancel, "Create Session") at 14px with `8px 16px` padding need roughly 400px. At phone widths (V2_PORTRAIT, <768) the left edge will be clipped, not scrolled, which is the same failure the comment at `:34` warns about. Add `flexWrap: "wrap"` (or a column layout under a media query). Consider `marginRight: "auto"` on the backlog button so it sits left, apart from the primary actions.
- Touch target: existing e2e `omnibar-backlog-compose.spec.ts` asserts >=44px targets and no horizontal overflow for compose widget at V1_DESKTOP and V2_PORTRAIT. A similar assertion for the footer is advisable; the shared `button` class is `padding 8px 16px, fontSize 14` (about 35px tall), likely below 44px on phone.

## 3. Existing tests asserting on footer buttons / counts

- Unit: `OmnibarCreationPanel.test.tsx:48-68` (testid presence; new backlog test). No `getAllByRole("button")` count assertions found for the panel in the grep (only these hits).
- `Omnibar.backlogCompose.test.tsx` mocks `OmnibarResultList` only; it does not mock `OmnibarCreationPanel`. Its `getByText`/`getByRole("button", {name: /create backlog/i})` style queries could now match two elements in creation mode. Run it to confirm (not run here).
- E2E: `omnibar-backlog-compose.spec.ts:22` uses `page.getByText('Create Backlog Item (AI)')` (list row, discovery mode); the footer label is "Create Backlog Item" (no "(AI)"), and `getByText` default is substring/case-insensitive but the string with "(AI)" will not match the footer, so no strict-mode ambiguity. `omnibar-create-session-button` users (`session-creation-async.spec.ts:136,153`, `remote-workspaces.spec.ts:119`, `pages/SessionsPage.ts:17`) locate by testid and are unaffected.
- No e2e covers the new footer button; consider adding one (needs `// @feature` header, testid locator, no `waitForTimeout`).

## 4. jscpd gate (`web-app/.jscpd.json`: minLines 20, minTokens 200, threshold 0.14%)

- The change adds ~13 lines in the component and a ~12-line test; below the 20-line/200-token clone size, so low risk. The new test's `render(<OmnibarCreationPanel {...buildProps(...)}/>)` pattern reuses `buildProps`. Adding a second test file with a copied jest.mock block would be the actual risk. Run `pnpm run lint:duplicates` in `web-app/` to confirm; the global threshold is a ratchet and not diff-scoped.

## 5. Registry docs

- `docs/reference/feature-testing-registry.md:19` already describes "the omnibar's 'Create Backlog Item' button/row" opening compose. Update to say "list row (discovery) and footer button `omnibar-create-backlog-button` (creation)". No OmnibarAction/detector change needed (button routes through the existing handler, so the exhaustive-switch registry is untouched).
- `docs/reference/session-creation-registry.md` has no backlog mention and covers session creation modes only; no update needed.
- Feature registry: `web-app/src/lib/features/features/backlog.ts:39` has a 'Create Backlog Item' entry; `make registry-generate` (CLAUDE.md) only needs a rerun if a `+feature:` marker or component moved. `OmnibarCreationPanel.tsx` has no new marker; run `make registry-diff` to confirm no changes.
