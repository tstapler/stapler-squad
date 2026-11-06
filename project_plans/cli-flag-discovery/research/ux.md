# UX Research: CLI Flag Discovery

Confidence: repo facts VERIFIED by reading files (paths cited). External-pattern claims are from general knowledge of those products, INFERRED, not re-fetched.

## 0. Existing UI to reuse (VERIFIED)

- `web-app/src/components/settings/ProgramsManager.tsx:313-336`: `prog-command` and `prog-flags` are plain `<input type="text">` with `<label htmlFor>`; state in `formData`. No validation, no `aria-describedby`.
- Session-creation picker is NOT free text: `components/sessions/OmnibarCreationPanel.tsx:936-960` is a `<select id="omnibar-program">` fed by `useAvailablePrograms()`. Session creation has no flags input, so AC7 "flag validation" at creation applies only to the program's saved `cli_flags` (show them read-only with warnings), unless a flags field is added. This is a scope question for the plan.
- An existing missing-binary warning exists at `OmnibarCreationPanel.tsx:350,951`: `isProgramRecognized` (name not in available list) renders `<span presetListStyles.programWarning data-testid="preset-program-warning">"x" not found in PATH ...`. It has no `role`, no icon. Extend/replace with the probe result instead of adding a second warning (avoid two conflicting messages).
- Reusable: `components/ui/Tooltip.tsx` (Radix Tooltip, hover/focus only; 400ms delay; does not open on tap reliably), `components/ui/AutocompleteInput.tsx` (has `role="listbox"`/`role="option"`/`aria-selected` but the input itself lacks `role="combobox"`/`aria-expanded`/`aria-activedescendant`, and it is single-token oriented), `components/history/HistorySearchInput.tsx:174` (a correct `role="combobox"` + `aria-controls` + `aria-autocomplete="list"` reference), `components/ui/AliasPalette.tsx:63,103` and `Omnibar` (`aria-activedescendant` usage). `@radix-ui/react-popover` is NOT installed (grep of package.json returned nothing); a disclosure button is implementable without it.
- Styling: vanilla-extract `*.css.ts` beside the component (e.g. `ProgramsManager.css.ts`), theme tokens in `styles/theme.css.ts` (`warning`, `warningBg`, `warningText` at lines 100-102).

## 1. Comparable patterns

| Pattern | What works | Lesson for us |
|---|---|---|
| IDE/terminal flag hints (VS Code launch.json, shell completion) | Suggestions appear inline as you type; description sits next to each candidate, not hidden behind hover; unknown tokens get a squiggle, never block | Show flag + one-line description in the suggestion row; warn, never block (matches AC6) |
| GitHub Actions editor | Validation is lazy and passive (inline annotation with message text); unknown keys are flagged but save is allowed for schema-lag reasons | Parsed `--help` is incomplete by nature (hidden flags, subcommand flags), so "unknown" must read as "not seen in --help", not "invalid" |
| Warp / Fig autocomplete | Token-aware completion (completes the token under the caret, not the whole field); description panel for highlighted item; Tab/Enter accepts | Complete only the caret's token; show description of the active option in the popup |

Why they work: feedback is at the point of typing, descriptions are visible without a separate gesture, and failure is soft. Where we diverge: probe runs on blur/select (cost + safety, findings.md risk), so flag hints appear only after the command has been probed. Flags field should be disabled-looking-not-disabled ("Check the command first" hint) until a probe result exists; autocomplete silently absent when flags list is empty.

## 2. Mental models

- Users think "I am pointing at a program I already have", not "registering a binary". "Not found" should be phrased in terms of their machine ("isn't on the server's PATH"), and must make clear that the check runs on the stapler-squad server host, which may differ from the shell they use (very common: PATH differs under systemd; `~` expansion; remote hosts).
- Flag knowledge is recall-not-recognition: users half-remember `--dangerously-skip-perms`. Autocomplete converts recall to recognition; the description matters more than the name.
- "Unknown flag" warnings are trusted only if rarely wrong. False positives (parser misses flags) train users to ignore the warning. Prefer wording "not listed in --help" and suppress the warning entirely when parse yielded zero flags or low count.
- Tokens after the command in `prog-command` (e.g. `python -m myagent`) count as command; flags belong in `prog-flags`. Users will put flags in the command field; the probe uses the first token only (AC2), so surface "Checked: python" so the scope of the check is visible.

## 3. Accessibility (WCAG 2.1 AA; Axe blocks CI)

Combobox (flags input), ARIA 1.2 editable combobox with list autocomplete:
- Input: `role="combobox"`, `aria-autocomplete="list"`, `aria-expanded`, `aria-controls=<listbox id>`, `aria-activedescendant=<active option id>` (DOM focus stays in the input). Existing `AutocompleteInput` lacks the first, second (partial), and last; extending it or wrapping is required, otherwise Axe `aria-required-attr`/listbox-parent rules may flag it.
- Listbox `role="listbox"` with `role="option"` children, each with stable `id`; `aria-selected` on active. Option accessible name = flag; description as `aria-describedby` on option or included in the text ("--model, takes a value: Model to use").
- Keys: Down/Up move (wrap), Enter/Tab accept, Escape closes without clearing text, Alt+Down opens. Typing is never intercepted. Trigger completion on the current whitespace-delimited token starting with `-`.
- Live feedback: probe status region `role="status"` (`aria-live="polite"`) for "Checking...", "Found at /usr/bin/x", "Not found". Non-blocking warnings: text in an element referenced by the input's `aria-describedby` (plus `aria-invalid` is NOT set: it's a warning, not an error). Do not use `role="alert"` for routine warnings (the existing pi warning uses it because it's a safety issue; not comparable).
- Color: found/not-found must not rely on color alone (1.4.1). Use icon + text ("Found: /usr/bin/claude" / "Not found on server PATH") with distinct shapes (check vs. warning triangle), `aria-hidden` on icons. Text contrast 4.5:1; `warningText` on `warningBg` from `theme.css.ts` (#92400e on #fef3c7 is roughly 7:1, INFERRED, verify in dark theme with Axe).
- Tooltip / description on touch and keyboard: Radix Tooltip is hover/focus only and content is not reachable on tap. For each flag description use an info disclosure `<button type="button" aria-expanded aria-controls>` with min 44x44 CSS px hit area (WCAG 2.5.5 is AAA, 44px is repo/mobile convention; 24px is the 2.2 AA floor), toggling an inline description row. Inline expansion beats a floating popover on mobile (no viewport clipping, no dependency on new `@radix-ui/react-popover`). Also keep desktop hover via existing `Tooltip` as an enhancement only; no information available only on hover (1.4.13: dismissible, hoverable, persistent).
- In suggestion list on touch: option rows min height 44px; use `onPointerDown`/`onMouseDown` preventDefault so tapping an option doesn't blur the input and close the list before the click registers (the blur that triggers probing is the same event; ensure the flags-field blur doesn't re-probe).
- Focus: probe result must not steal focus; don't move focus on warning. Respect `prefers-reduced-motion` for the spinner (static "Checking..." text remains).
- Testing: add `jest-axe` or the existing e2e Axe run covering the Program Config form; e2e locators must be `data-testid`/ARIA only, e.g. `prog-command-status`, `prog-flags-warning`.

## 4. Error and edge states

| State | UI | Notes |
|---|---|---|
| Idle / never probed | No indicator | Do not show "not verified" noise |
| Probing | Inline spinner + "Checking..." (status region); input stays editable | Debounce not needed (blur-only); cancel/ignore stale result if command changed (compare returned command to current) |
| Found, flags parsed | "Found: /path" + "N flags detected" | Enables autocomplete |
| Found, zero flags parsed | Neutral (not warning): "Found: /path. Couldn't read flags from --help; flag suggestions unavailable." | Suppress unknown-flag warnings; never imply the flags are wrong |
| Not found | Warning: "Not found on the server's PATH. Sessions using this program may fail to start." Save still allowed | AC2 `found=false` is not an RPC error |
| Probe timeout (3s) | Neutral/warning: "Timed out checking flags (3s). The program may not support --help; flag suggestions unavailable." Found status still shown if LookPath succeeded | Distinguish binary-found from help-failed; RPC needs a `probe_status`/reason field so UI can differ (recommend `help_status` enum: ok, timeout, oversized, no_output) |
| Output oversized/unparseable | Same as zero flags | AC4 |
| RPC error / server unreachable | "Couldn't check this program (server unreachable). You can still save." with Retry button (44px) | Distinct from "not found"; never show "not found" on transport failure |
| Remote server (multi-host) | Label the check target: "on <host>" when the session targets a remote; if unsupported, say "Checked on this machine only" | Open question: probing remote hosts is not in requirements; avoid a false "found" for remote sessions |
| Unknown flag | Non-blocking, `aria-describedby` text: "--foo isn't listed in `git --help`. It may still work (hidden or subcommand flags)." | Never red error style; also list clickable "Did you mean --foo-bar?" only if edit distance small |

Wording rules: plain, blame-free, no "invalid"/"error" for warnings; say what happens next; keep under one line on mobile (truncate path with ellipsis, full path available on expand).

## 5. Job to be done

"When I add or launch an AI agent program, I want to know immediately that it will actually start and how to configure it, so I don't waste a session slot and a tmux round-trip finding out from a failed launch."
Sub-jobs: (a) confirm the binary is real (highest value, cheap, cover everywhere); (b) discover flags I don't remember (mid value, Program Config only); (c) catch typos in flags (lower value, highest false-positive risk; ship as gentle hint). Success metric candidates: fewer sessions failing at startup for command-not-found; unchanged time-to-save for the config form (probe must never block save).
Prioritize for slicing: binary check in both Program Config and picker first; flag autocomplete second; unknown-flag warnings and tooltips last.

## 6. Both form factors (project rule)

- Desktop: inline status under `prog-command`, combobox popup anchored to flags input, hover tooltip as enhancement plus keyboard focus.
- Mobile: status text wraps under the field; suggestion list rendered inline (in-flow, max-height with scroll) rather than absolutely positioned to avoid on-screen-keyboard occlusion; 44px rows/info buttons; use `autoCapitalize="off" autoCorrect="off" spellCheck={false}` on both inputs (otherwise mobile keyboards mangle `--flags`; add `inputMode="text"`, and note many mobile keyboards need easy access to `-`); info disclosure via tap; probe fires on blur which on mobile is dismissing the keyboard, so also offer explicit "Check" button as fallback (44px).
- Picker `<select>` on mobile opens a native sheet; put the warning below it (as existing code does), not inside options.

## 7. Recommendations / open questions for the plan

1. Add a shared `ProbeStatus` component (icon + text + `role=status`) used by ProgramsManager and OmnibarCreationPanel; replace `preset-program-warning` logic.
2. Extend `AutocompleteInput` (or new `FlagCombobox`) for combobox ARIA + token-at-caret completion; build off `HistorySearchInput` combobox attributes.
3. Add an inline-disclosure info button component; do not add a Popover dependency.
4. Proto response should include help status distinct from `found` (see table) so UI wording is accurate.
5. Decide: does session creation get a flags input, or only show saved flags' warnings? Requirement AC7 is ambiguous; current UI has none.
6. Decide remote-host behavior (label vs unsupported).
