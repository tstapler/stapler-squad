# UX Research: Launcher Presets

## 0. Key finding up front

This repo already ships a feature that is functionally identical to what Launcher Presets
needs: **aliases** (`@alias-name` in the Omnibar, `web-app/src/components/ui/AliasPalette.tsx`,
`web-app/src/lib/omnibar/detectors/AliasDetector.ts`, `web-app/src/lib/hooks/useAliases.ts`).
An alias is a named, user-authored shortcut (path, program, session type, branch, autoYes,
namePrefix) that resolves to a **prefilled-but-editable** creation form, not an instant launch.
Launcher Presets should be built as a near-copy of this pattern rather than a new interaction
model — reusing it directly answers most of the questions below and keeps the Omnibar's mental
model consistent (the user already knows "type `@`, pick a shortcut, review the form, hit
Enter").

## 1. Comparable UX patterns already in the Omnibar

`OmnibarCreationPanel.tsx` (`web-app/src/components/sessions/OmnibarCreationPanel.tsx`) and
`Omnibar.tsx` establish a consistent interaction model:

- **Progressive disclosure, not one giant form.** `SESSION_TYPES` splits into `PRIMARY_TYPES`
  (shown by default) and `ADVANCED_TYPES` (behind a "▾ More" toggle,
  `OmnibarCreationPanel.tsx:69-112`). "Advanced Options" (program, category, auto-yes) is a
  separate collapsible section (`OmnibarCreationPanel.tsx:762-813`). A new "Presets" section
  should follow this same principle: don't clutter the primary form; presets are a fast path
  layered on top of it, not a new tier of complexity.
- **Selection drives prefill, not replacement.** Selecting a `RadioGroup` option, an alias, or a
  detected input type never submits anything by itself — it only updates `formState` via
  `setFormField`, and the existing Create Session button remains the single point of submission
  (`Omnibar.tsx:1073-1170`). This is the load-bearing precedent for Success Criteria #2's
  prefill-then-review requirement.
- **Detection → confirmation chip → editable form** is the exact three-step flow aliases use:
  typing `@name` shows `AliasPalette` (a `role="listbox"` overlay), selecting one sets
  `detection.type === InputType.Alias`, which (a) shows a small resolved-state chip
  (`Omnibar.tsx:1335-1345`, `data-testid="alias-resolution-chip"`, `role="status" aria-live="polite"`)
  confirming *what* was applied ("Alias resolved: @foo · main · label"), and (b) auto-fills
  `program`, `sessionType`, `autoYes`, `branch` in the same effect that runs detection
  (`Omnibar.tsx:508-546`) — but only when the corresponding field is still empty or was itself
  auto-suggested (`!programRef.current || programRef.current === lastSuggestedProgramRef.current`).
  This "don't clobber a value the user already typed" guard is important to replicate for
  presets: if a user has hand-edited the program field before picking a preset, the preset
  should not silently overwrite it.
- **Keyboard-first, mouse-optional.** `RadioGroup` (`web-app/src/components/ui/RadioGroup.tsx`)
  implements roving tabindex + arrow-key-selects (not arrow-key-then-Space) — moving focus *is*
  selecting, matching users' expectation that arrow keys in a native radio group both navigate
  and choose. `AliasPalette` supports ArrowUp/Down + Tab/Enter to select + Escape to back out
  one level (`Omnibar.tsx:701-732`). A `preset:<id>` shorthand (the Nice-to-Have `DetectorRegistry`
  entry) should follow the same Tab-to-accept / Enter-to-accept-and-imply-submit convention as
  `AliasDetector` and `WorkflowDetector`.
- **Errors surface inline, not as toasts or console-only logs.** `AliasPalette` renders a
  `role="alert" aria-live="assertive"` block titled "Alias config failed to load" with the raw
  error message when `useAliases()` fails (`AliasPalette.tsx:29-41`), and a distinct
  `role="status"` empty state ("No aliases yet… Add aliases in Settings → Aliases to launch
  sessions faster") when the list is empty but loaded successfully (`AliasPalette.tsx:43-54`).
  These two states are already differentiated by both visual treatment and ARIA role
  (`alert`/assertive for real failures, `status`/polite for benign empty state) — presets should
  reuse exactly this split.

## 2. Reconciling "one-click feel" with "review before launch"

The tension named in the research question is real but the alias precedent shows it's already
been resolved in this codebase, and the resolution is sound:

- **The "click" is not the launch — it's the fill.** From the user's perspective, selecting a
  preset *feels* like one action (click/Enter on a list item), and the visible cost of the
  "review" step is just glancing at a form that's already correct. The friction budget is spent
  on confirmation, not re-entry. This matches Nielsen's recognition-over-recall heuristic: the
  user recognizes their own preset's effect on the form instead of having to recall what flags
  they'd normally type.
- **Minimize the review step's friction, don't remove it.** Two concrete levers, both already
  precedented:
  1. **Resolution chip** (`data-testid="alias-resolution-chip"` pattern) — echo the resolved
     preset's argv/program/working-dir in one line above the form so the user can verify at a
     glance without reading every field. For a preset this line is especially valuable because
     argv arrays (e.g. `ssh -t host 'cd ~/repo && exec claude'`) are exactly the kind of thing a
     user wants to visually confirm before it runs, per Success Criteria #3's quoting-safety
     concern.
  2. **Cmd/Ctrl+Enter submits from anywhere** (`Omnibar.tsx:863-865`) — a power user who trusts
     a given preset can select it and immediately hit Ctrl+Enter without ever touching the
     mouse or tabbing to the Create button. This gives the "true one-click" users the shortcut
     they want while still defaulting everyone else through the review screen.
- **Do not add a second "instant launch" affordance that bypasses the form.** A "run immediately"
  button next to each preset would directly violate Success Criteria #2 and would also break
  parity with how aliases work today — two different levels of trust for two structurally
  identical shortcut mechanisms would be confusing and inconsistent to explain in the UI copy.
  If a truly zero-review flow is wanted later, that's a separate, explicitly-opted-into feature
  (e.g. a per-preset "skip review" flag), not the v1 default.

## 3. Accessibility

Match the two existing patterns exactly rather than inventing new ARIA semantics:

- **List of preset options** → same shape as `AliasPalette`: `role="listbox"` container,
  `role="option"` rows, `aria-selected`, `aria-activedescendant` on the listbox pointing at the
  current row's `id`, `tabIndex={isSelected ? 0 : -1}` (roving tabindex), and `onKeyDown`
  handling Enter/Space to select (`AliasPalette.tsx:134-159`). Reuse `data-testid="alias-row"`-style
  naming (`data-testid="preset-row"`) so E2E specs can use `getByTestId`/`getByRole` per
  `.claude/rules/e2e-test-conventions.md` (CSS class selectors are disallowed in this repo's
  Playwright specs).
- **If instead exposed as a `RadioGroup`-style selector** (single-select, small fixed set) reuse
  `web-app/src/components/ui/RadioGroup.tsx` directly rather than hand-rolling ARIA — it already
  implements `role="radiogroup"`/`role="radio"`/`aria-checked`/roving tabindex/arrow-key-cycling
  correctly and is exercised by existing tests. Whether presets should be a listbox (searchable,
  scales to many presets) or a radiogroup (small, fixed set, matches session-type styling)
  is a call for Phase 3 (Plan) — likely a listbox, since the config file can hold an arbitrary
  number of presets and a listbox scrolls/filters better than a wrapping button row.
- **Resolution/confirmation state** → `role="status" aria-live="polite"` (matches the alias
  resolution chip and spawn-shell chip, both non-error confirmations).
- **Error state (malformed config)** → `role="alert" aria-live="assertive"`, matching
  `alias-config-error`. This is a genuine failure the user must notice, not a passive status.
- **Keyboard access to the whole flow** — must work with the existing global handlers: Escape
  should back out one level as `AliasPalette`'s callers already implement, Tab/Enter accepts the
  highlighted item, and none of the new handlers should intercept keys currently owned by
  `isDropdownVisible`/`isAtDropdownVisible`/`activeDropdown === "alias"` blocks in
  `Omnibar.tsx:handleKeyDown` — a new "presets" dropdown-active state needs its own branch,
  following the same `if (activeDropdown === "presets" && ...)` early-return shape already used
  for `"alias"` (`Omnibar.tsx:702-732`) so it doesn't fight the other dropdowns for key events.
- **CSS** — per `.claude/rules/css-architecture.md`, any new preset list/row styling must be a
  `.css.ts` vanilla-extract file colocated with the component (`AliasPalette.css.ts` is the
  existing sibling to copy the shape of), not a new `.module.css` file.

## 4. Error and edge-case handling

| Case | Recommended UX | Precedent |
|---|---|---|
| No presets configured (file absent or `presets: []`) | Non-error empty state: "No presets yet. Add one in `~/.stapler-squad/launcher-presets.json`." Optionally link/mention the doc rather than blocking any flow — the Omnibar continues to function normally without presets. | `AliasPalette.tsx:43-54` "No aliases yet…" empty state, `role="status"` |
| Preset references a program not currently on `PATH`/`AvailablePrograms` | Do **not** silently drop the preset from the list (the user typed it deliberately and may install the binary later, or may be using an absolute path the auto-detector doesn't recognize). Instead, prefill the form as normal but show a non-blocking inline warning near the Program field ("`codex` not found in PATH — check it's installed") using the same soft-warning treatment as `pathDoesNotExist`/`showCreateRepoNotice` (visually distinct from a hard error, doesn't disable Create Session). This must be a **frontend-computed check against the existing `AvailablePrograms`/`useAvailablePrograms()` list** (`OmnibarCreationPanel.tsx:250`) at render/selection time, not a startup-time backend rejection — availability is host-environment-dependent and can change after the config loads. |
| Config file has a JSON syntax error | Fail loudly per Success Criteria #4, **and surface it to the user in the UI**, not just server logs — a hand-edited JSON file is exactly the kind of error a user will trip on repeatedly if the only signal is a log line they don't know to check. Reuse the `alias-config-error` chip shape: `role="alert"`, red/warning styling, showing the actual parse error message (line/col if the JSON parser provides it) so the user can find and fix the typo without spelunking logs. The RPC (`GetLauncherPresets`) should return this as a structured error/empty-list-with-error field rather than a generic 500, so the frontend can render it inline instead of a blank silent failure. |
| Duplicate preset `id` in config | Same "fail loudly" treatment as JSON syntax errors — reject the whole file per the Must-Have requirement ("do not partially apply"), surfaced with a message identifying which `id` collided, so the user isn't left guessing which of two presets with the same name is missing. |
| Preset with empty/missing `argv` | Same category — reject at load time with a specific field-level message ("preset `foo` has no `argv`"), not a runtime crash when the user later tries to launch it. |

General principle across all four: **the failure mode for a hand-edited config file must be
diagnosable from the UI alone.** A user who edits JSON by hand and gets it wrong will not
reflexively check `~/.stapler-squad/logs/stapler-squad.log` — surfacing errors only server-side
effectively makes broken presets invisible/undebuggable to non-technical or in-a-hurry users.

## 5. Jobs-to-be-done

- **Functional job:** "Get me into my daily/recurring workflow (a specific program + flags +
  directory) without re-typing or re-selecting the same three things every time." This is
  identical to the functional job aliases already serve for path+program+session-type; presets
  extend it to cover full `argv` (multi-flag invocations, remote-exec via `ssh -t host '...'`)
  that a single `program` string can't express.
  Example done today by hand: opening the Omnibar, typing a path, opening Advanced Options,
  selecting a program from a dropdown, and manually typing flags into whatever mechanism exists
  for extra args — a preset collapses this into one selection.
- **Emotional job:** Reduce the low-grade friction/annoyance of repetitive setup ("not this
  again") and the small anxiety of complex multi-arg commands with fragile quoting (a
  mistyped `ssh -t host '...'` either fails cryptically or, worse, runs the wrong thing) —
  Success Criteria #3's insistence on `argv`-not-shell-strings is itself an emotional-job
  requirement: trust that the exact command they saved is the exact command that runs, with
  zero shell-quoting surprises.
  It also gives a small sense of control/mastery — "I've encoded my setup once, and the tool
  respects it" — versus a system that only ever recomputes defaults algorithmically.
- **Social job:** Presets are file-based and hand-edited (Out of Scope explicitly excludes a
  UI editor), which enables a **shareable-artifact** job: a user can hand a coworker their
  `launcher-presets.json` (or a snippet of it) to replicate a launch setup, or commit a
  team-standard preset file to a shared dotfiles repo — the same social pattern this project's
  own `cfgcaddy`/dotfiles-as-shareable-config culture already reflects. This differentiates
  presets from aliases (which, per the RPC evidence above, live inside the main `config.json`/
  `ListAliases` RPC rather than a standalone hand-editable file) — presets are explicitly meant
  to be portable, inspectable, version-controllable text, which is itself a large part of their
  appeal to this user base (engineers who already keep their environment in dotfiles).

## Sources consulted

- `project_plans/launcher-presets/requirements.md` (full read)
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx` (full read)
- `web-app/src/components/sessions/Omnibar.tsx` (full read)
- `web-app/src/components/ui/RadioGroup.tsx` (full read)
- `web-app/src/components/ui/AliasPalette.tsx` (full read)
- `web-app/src/lib/hooks/useAliases.ts` (partial read, RPC + state shape)
- `web-app/src/lib/omnibar/detectors/AliasDetector.ts` (grep: priority = 36)
- Backend alias config handling: `server/services/session_service.go`,
  `server/services/defaults_service.go` (grep only, to confirm aliases live in `config.json`
  rather than a standalone file — contrast noted in §5)
