# UX research: "Create Backlog Item" in the Omnibar footer

Scope: `OmnibarCreationPanel.tsx` footer (lines ~1023-1046), `OmnibarResultList.tsx` listbox row, `Omnibar.tsx` (`openBacklogCompose` ~L1030, compose view ~L1748). Evidence labels: VERIFIED = read in repo; INFERRED = from general UX/WCAG knowledge, no source fetched this session (web research not run per tool budget).

## Current state (VERIFIED)
- Discovery mode: `role="listbox"` with "+ New Session" and "Create Backlog Item (AI)" `role="option"` rows (`OmnibarResultList.tsx:135-172`); keyboard nav via `aria-activedescendant`, input keeps focus.
- Creation mode: footer = [Create Backlog Item (secondary)] [Cancel (secondary)] [Create Session (primary)] (`OmnibarCreationPanel.tsx:1023-1046`). The new button is rendered only when `onCreateBacklog` is passed; `Omnibar.tsx:2084,2127` pass it in both call sites.
- Compose mode replaces the whole card (`backlogComposeText !== null`, `Omnibar.tsx:1748`) with `aria-labelledby="backlog-compose-title"`; Escape handling is suppressed while composing (`Omnibar.tsx:1294`).
- `openBacklogCompose` seeds the compose text from the typed input (strips `backlog:`), so the footer button preserves user input.

## Comparable patterns (INFERRED)
- Raycast / VS Code palette / Linear: alternate creation targets appear as sibling *commands* in the result list, not as footer buttons. Footers carry the action hint for the highlighted row (Enter to run, Cmd+K for secondary actions).
- GitHub "New" menu, Notion "New page" vs "New database", Slack composer: one surface, multiple sibling creators; choosing one swaps the form (a mode switch of the same card), and the typed text carries over.
- Conclusion: list row (discovery) + footer button (form state) is consistent with the pattern; the footer button is a "switch what I'm creating" affordance.

## Mental model
Sibling of Create Session at the *intent* level, but mechanically a mode switch: it does not submit the session form, it discards the session form view and opens compose. So: it is a sibling action, not a primary submit. Label should stay a noun phrase ("Create Backlog Item") not imply submission. Risk: user has filled session fields (path, branch, program) and loses them on click. VERIFIED partially: only input text is carried over; I did not verify whether session form state survives returning. Recommend confirming a back path from compose restores creation panel (or documenting that it does not).

## Button order and styling
- Current order puts the mode switch first (leftmost), then Cancel, then primary. Reasonable: destructive/secondary actions left, primary right-most, matching dialog conventions. Recommendation: keep it left-aligned and visually separated from Cancel/Create (e.g. `margin-right: auto` on the footer button) so it reads as "alternative path" and cannot be mis-hit as Cancel. Currently it shares `buttonSecondary` with Cancel, making two identical-weight neighbors; a text/ghost (tertiary) variant is better.
- Keep Create Session as the only primary.

## Accessibility
- Footer button is a native `<button type="button">`: correct semantics (it performs an action/mode change, not selection). WCAG 4.1.2 satisfied by native role and visible name. Keep the visible text equal to the accessible name (2.5.3 Label in Name); do not add an aria-label that differs.
- Listbox row vs button: the `option` role is right in the listbox (the input owns focus; options are not tabbable). Do not add `tabIndex` to options. Footer buttons are in the normal tab order after the form fields and before Cancel/Create: logical DOM order = visual order (2.4.3).
- Disabled state: Create Session is `disabled` when invalid; backlog button must stay enabled (it needs no session fields). Do not gate it on `canSubmit`.
- Focus management on switching to compose (INFERRED best practice, WAI-ARIA APG dialog pattern): after the swap, move focus to the compose textarea (first interactive element/heading) because the clicked button unmounts; otherwise focus falls to `body` and screen-reader users lose context. Verify `BacklogComposeWidget` autofocuses; announce the change via the compose container's `aria-labelledby` title (already present). On cancel/back from compose, return focus to the invoking control (footer button or input) if it still exists.
- Escape: compose suppresses the global Escape close (`Omnibar.tsx:1294`), so Escape must be handled inside the widget (back to omnibar vs close). Specify one behavior and test it.
- Keyboard shortcut: none today for the footer button. Optional: show a hint chip only if a shortcut is added; do not add one for this task.

## Discovery mode footer: recommendation
AC says visible in every state the card shows. Discovery already exposes the action as a listbox row, so the action is *present* in both states; the footer is not required. Recommend: do NOT add a footer to discovery mode for this change. Reasons: (1) duplicates the row two lines above it, (2) a footer there would add a second tab stop and a different (button vs option) semantic for the same action in one view, (3) discovery rows are the established pattern and keyboard-reachable. Caveat: the row is `Create Backlog Item (AI)` while the footer reads `Create Backlog Item`; align labels (drop or add "(AI)" in both) so users and tests recognize one action. If a reviewer reads the AC literally as "footer button in discovery", the minimal alternative is a read-only hint footer in discovery (e.g. "Enter to select / Esc to close"), not a duplicate action; flag as an owner decision.

## Error and edge states
- Click with empty input: compose opens with empty text; the widget should show its own empty-state/required validation, not fail silently.
- Submitting state: while `isSubmitting` (session being created) disable the backlog button too, to avoid switching modes mid-request (currently only Create Session is disabled; INFERRED gap, VERIFIED that only `canSubmit || isSubmitting` guards Create Session).
- Backlog feature unavailable (service down, feature-flag off): `onCreateBacklog` is optional, so the button hides cleanly; prefer hiding over a dead button. Compose-time RPC failure belongs to the widget (`role="alert"`, like `omnibar-create-error`).
- Data loss: switching from a filled session form discards the form view; consider preserving creation-panel state in `Omnibar` state (it is lifted there, so likely preserved; unverified).
- Narrow viewport: three footer buttons may wrap; check flex-wrap so primary stays right-most and the backlog button doesn't push Create Session off-card.
- Test hooks: `omnibar-create-backlog-button` testid exists; add an e2e assertion that focus lands in the compose textarea.

## Recommendations summary
1. Keep footer button (native button, enabled regardless of form validity, disabled during `isSubmitting`).
2. Style as tertiary/left-separated; keep Create Session the sole primary.
3. Move focus into compose on switch; define Escape/back behavior.
4. No new footer in discovery mode; unify label text across row and button; flag the literal-AC reading to the owner.
