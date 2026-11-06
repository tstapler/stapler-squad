# Research: stack and state coverage for the Omnibar backlog button

Paths relative to `web-app/src/components/sessions/`. Line numbers are from HEAD `dfc17acf5` (verified via grep/Read).

## Stack and conventions
- React + vanilla-extract (`Omnibar.css.ts`, `OmnibarCreationPanel.css.ts`; footer style `Omnibar.css.ts:238`, flex-end, gap 8). Buttons use `buttonClass + buttonSecondary`.
- Jest + testing-library; component tests colocated (`OmnibarCreationPanel.test.tsx:55` already has a `omnibar-create-backlog-button (footer action)` describe; `OmnibarResultList.test.tsx` covers the list row).
- data-testid: `omnibar-create-backlog-button` (footer), `omnibar-create-session-button`, `omnibar-create-error`, `backlog-compose-textarea`, `backlog-compose-widget`. The discovery list row has id `${id}-create-backlog` (`OmnibarResultList.tsx:62,156`), no testid; e2e finds it by text "Create Backlog Item (AI)".
- e2e (`tests/e2e/`): spec header `// @feature ...`, no `waitForTimeout`, testid/ARIA locators only, helpers under `tests/e2e/pages/`. Existing: `omnibar-backlog-compose.spec.ts` (opens via list row, V1_DESKTOP + V2_PORTRAIT, axe), `omnibar-backlog-intent-confirm.spec.ts` (`backlog:` prefix).
- Per CLAUDE.md, OmnibarAction/DetectorRegistry registries only matter for new actions/detectors; this change adds neither (same `openBacklogCompose`, `Omnibar.tsx:1030`).

## Current state (already implemented in HEAD commit)
`OmnibarCreationPanel` takes optional `onCreateBacklog` (`:153`) and renders the footer button first in the footer (`:1024-1033`); `Omnibar.tsx:2127` passes `openBacklogCompose`. Unit test exists.

## Which states show what
`modeState.type` is `discovery | creation | creation_with_repo` (`Omnibar.tsx:608,511`). Backlog compose/review are separate early returns, not modes.

| State | Where | Footer? | Backlog entry point |
|---|---|---|---|
| Closed | `:1716` returns null | n/a | n/a |
| Backlog review (`backlogReviewText != null`) | `:1721` early return, BacklogItemForm | no Omnibar footer | n/a (already inside backlog flow) |
| Backlog compose (`backlogComposeText != null`) | `:1748` early return, BacklogComposeWidget | own widget buttons | n/a |
| Discovery, normal | `:2066` OmnibarResultList | NO (panel gated by `!isDiscoveryMode`, `:2121`); only the bottom shortcuts bar | list row (last index, `dispatchHighlightedResultAction` `:1047`) |
| Discovery + alias browse/completion (`:635`) | same list branch (not excluded) | no | list row |
| Discovery + `@` command (`isAtDropdownVisible`, `:616`) | `:1910` at-dropdown; list suppressed by `!isAtDropdownVisible` (`:2066`) | no | NONE: row hidden, no footer |
| Creation / creation_with_repo (incl. path dropdown, one_off) | `:2121` panel | YES (unconditional in panel, `:1023`) | footer button |

## Gaps / what is needed for "reachable in every state the card shows"
1. Discovery mode has no footer, so the new button does not appear there; the list row remains the only path. Acceptable per requirements ("visible in discovery (list row)"), and keyboard nav depends on the row.
2. Real gap: discovery + `@` command hides both the list row and the footer, so no backlog entry exists. Options: (a) render a footer-row button in discovery too (a small row in the shortcuts bar, `Omnibar.tsx:~2200`, shown when `isAtDropdownVisible` or always) with the same testid; (b) accept as out of scope. Option (a) with a single shared button component covers every state with one testid.
3. Wording differs: footer says "Create Backlog Item", list row says "Create Backlog Item (AI)". Keep consistent if e2e matches by text.
4. Footer button seeds compose with `input.trim()` minus `backlog:`; in creation mode `input` is the path/URL text, so it will prefill the path. Confirm that is wanted (or pass empty).
5. Tests to add: Omnibar-level test that discovery/`@`-command state exposes a button and creation state's footer click opens `backlog-compose-textarea`; an e2e case clicking `omnibar-create-backlog-button` in creation mode (existing spec only uses the list row). No existing Omnibar.test.tsx backlog test found.
6. Mobile: footer is `flex-end` with 3 buttons; check wrapping at V2_PORTRAIT and the 44px touch-target assertion the compose e2e uses.
