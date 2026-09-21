# UX Design: CLI Flag Discovery

Inputs: `../requirements.md`, `../research/ux.md`, `../implementation/plan.md` (Flagged Choices 2, 3, 5, 6, 7). Element ids and testids follow the plan (`prog-command-input`, `prog-command-status`, `prog-command-check`, `prog-flags-input`, `prog-flags-warning`, `omnibar-flags-warning`, `preset-program-warning`).

Principles: the probe never blocks saving or launching; warnings are soft ("not listed in --help", never "invalid"); nothing is conveyed by color alone; nothing is conveyed by hover alone; probe results never steal focus; a transport failure is never shown as "not found".

## Surface inventory

| # | Surface | Kind | Where |
|---|---------|------|-------|
| S1 | Command field + `ProbeStatusBadge` | Interactive | Program Config (`ProgramsManager.tsx`) |
| S2 | `FlagCombobox` (Default CLI Flags) | Interactive | Program Config |
| S3 | Unknown-flag warning | Interactive (read + act on suggestion) | Program Config |
| S4 | `FlagInfoButton` disclosure | Interactive | Combobox rows and warnings |
| S5 | Session-creation badge + saved-flag warning | Interactive | `OmnibarCreationPanel.tsx` |
| S6 | Status-to-copy map (probe result vocabulary) | Non-interactive | Shared by S1, S5 |
| S7 | Probe audit log line | Non-interactive | Server log |

## Shared: Probe state to UI copy (S6, condensed)

Single source of wording, used by both forms. Icon shapes differ (color is redundant).

| `ProbeUiState` / status | Icon (aria-hidden) | Text | Tone |
|---|---|---|---|
| idle | none | (nothing rendered) | - |
| checking | spinner (static under reduced-motion) | `Checking...` | neutral |
| FOUND_PARSED | check | `Found: /usr/bin/claude` + ` 14 flags detected` | success |
| FOUND_NO_FLAGS | check | `Found: /usr/bin/claude. Couldn't read flags from --help; flag suggestions unavailable.` | neutral |
| TIMEOUT | check (found) + info | `Found: /path. Timed out checking flags (3s); flag suggestions unavailable.` | neutral |
| ERROR (probe failed to run) | info | `Couldn't check flags for this program. You can still save.` + Retry | neutral |
| NOT_FOUND | warning triangle | `Not found on the server's PATH. Sessions using this program may fail to start.` | warning |
| transportError | warning triangle | `Couldn't check this program (server unreachable). You can still save.` + Retry | warning |

Rules: text plus icon always; suffix line `Checked: claude` shows the first token that was probed (so `python -m x` reads "Checked: python"); long paths truncate with ellipsis, full path revealed by the badge's expand toggle (44px). Wording says "the server's PATH" because the check runs on the stapler-squad host, not the browser machine.

Sample (S6 is a vocabulary, so one sample):

```
[check] Found: /usr/local/bin/claude  ·  14 flags detected
        Checked: claude                       [ Show full path ]
```

Acceptance criteria (S6):
- Every `ProbeUiState` variant maps to exactly one row above; no variant renders empty text.
- Transport failure never uses the NOT_FOUND text.
- Icon shape differs between success (check), warning (triangle) and neutral (info); each is `aria-hidden` with text carrying the meaning.
- All text/background pairs meet 4.5:1 in light and dark themes (verify with Axe).
- Copy contains no "invalid", "error" or "failed" for warning-tone states.

---

## S1. Program Config: command field and status badge

### Wireframes

Desktop (form column, ~480px):

```
Command
+---------------------------------------------+  [ Check ]
| claude                                      |
+---------------------------------------------+
 [check] Found: /usr/local/bin/claude · 14 flags detected      <- role=status
         Checked: claude                        [Show full path]
Default CLI Flags
+---------------------------------------------+
| ...                                         |
```

Mobile (375px, single column, everything wraps under the field):

```
Command
+-----------------------------------+
| claude                            |
+-----------------------------------+
+-----------------------------------+   <- 44px
|            Check                  |
+-----------------------------------+
 [!] Not found on the server's PATH.
     Sessions using this program may
     fail to start.
     Checked: claudee
```

Inputs carry `autoCapitalize="off" autoCorrect="off" spellCheck={false}`. The `Check` button is visible on both form factors (secondary style on desktop, full-width 44px on mobile) because on mobile "blur" means dismissing the keyboard, which users do not always do before tapping Save.

### Interaction flow

| Step | User | System |
|---|---|---|
| 1 | Types in command field | Badge unchanged (no per-keystroke probing). If a prior result exists for a different command, badge switches to idle (stale result hidden) |
| 2 | Blurs field, or presses `Check`, or presses Enter in the field while it is non-empty | Badge shows `Checking...`; field stays editable; focus not moved |
| 3a | Server returns found | Badge shows found variant; flags field becomes autocomplete-capable (S2) |
| 3b | Server returns NOT_FOUND | Warning variant; Save stays enabled |
| 3c | Response arrives for an older command (user kept typing) | Discarded; badge remains idle/checking for the current command |
| 3d | RPC fails | transportError variant with Retry |
| 4 | Presses Retry / Check | Returns to step 2 |
| 5 | Clicks Save at any time, even mid-check | Saves; in-flight probe is aborted; no confirmation dialog |
| 6 | Clears the field | Badge disappears (idle); no probe on empty command |
| Exit | Cancel, Save, or navigating away | Aborts in-flight probe on unmount; no leftover state |

Enter behavior: Enter in the command field triggers `check` only; it does not submit the form (prevents accidental save on mobile keyboards' "Go").

### Error and edge states

| Situation | User sees | Exit path |
|---|---|---|
| Not found | Warning variant | Fix command and re-check, or Save anyway |
| Command has only env assignments / unbalanced quote / starts with `-` | NOT_FOUND text (server maps all `ResolveReason` failures to not-found) | Edit and re-check, or Save anyway |
| Probe > 3s | TIMEOUT variant (found retained if path resolved) | Continue; flags typed by hand; Retry |
| Server unreachable | transportError + Retry | Retry or Save anyway |
| Semaphore saturated / spawn failure | ERROR variant + Retry | Retry, Save anyway |
| Command with wrapper (`npx claude`) | Found: `npx`; suffix "Checked: npx" | Explains scope; no flags (or npx flags only) |
| Remote host session | Text reads `Checked on this server only` when the program is used for remote targets (open question, Decision D2) | Save anyway |

### Acceptance criteria

- UX-1: A user can learn whether a typed command exists in 1 action after typing (blur, Enter, or one tap on `Check`), with no extra navigation.
- UX-2: The badge is a `role="status"` (`aria-live="polite"`) region with a stable id referenced by the command input's `aria-describedby`; `aria-invalid` is never set by a probe result.
- UX-3: Save is enabled and clickable in every probe state, including `checking`, NOT_FOUND and transportError; no confirmation modal is introduced.
- UX-4: A slow response for a previous command never overwrites the state for the current command (stale drop).
- UX-5: The `Check` button and the Retry button are at least 44x44 CSS px and reachable by Tab in DOM order after the command input.
- UX-6: Probe results never move keyboard focus or scroll the page.
- UX-7: On a 375px viewport the badge text wraps; no horizontal scroll appears; full path is reachable via a 44px toggle.
- UX-8: Under `prefers-reduced-motion` the spinner is static and the text `Checking...` is still present.

---

## S2. Program Config: `FlagCombobox` (Default CLI Flags)

### Wireframes

Desktop (list anchored under the input, description of active option in a side row):

```
Default CLI Flags
+---------------------------------------------+
| --yes --mo|                                 |  role=combobox
+---------------------------------------------+
+---------------------------------------------+
| --model <value>            [i]              |  <- active (aria-activedescendant)
|   Model to use for the main chat            |
| --model-settings <value>   [i]              |
| --model-metadata-file      [i]              |
+---------------------------------------------+
```

Mobile (list rendered in-flow below the input, pushing content down, max-height ~40vh with scroll, rows 44px):

```
Default CLI Flags
+-----------------------------------+
| --yes --mo|                       |
+-----------------------------------+
| --model <value>              [i]  |  44px
| --model-settings <value>     [i]  |  44px
| --model-metadata-file        [i]  |  44px
+-----------------------------------+
(list is in normal flow, not a floating overlay)
```

The `[i]` control is S4. Suggestions appear only when a probe returned flags (`FOUND_PARSED`); otherwise the field is a plain input (silently), and a hint under the field reads `Check the command above to enable flag suggestions` only when no probe has run yet.

### Interaction flow

| Step | User | System |
|---|---|---|
| 1 | Types a token starting with `-` | Listbox opens with prefix matches, then substring matches, for the token at the caret only; max 8 visible then scroll |
| 2 | Down / Up | Moves active option (wraps); `aria-activedescendant` updates; DOM focus stays in the input |
| 3 | Enter or Tab (list open, option active) | Replaces the token at caret with the flag; appends a trailing space; list closes. Value-taking flags do not add `=` |
| 4 | Escape | Closes list; text unchanged; focus stays |
| 5 | Alt+Down | Opens list for the current token |
| 6 | Taps an option (touch) | Option selected via pointer-down (input does not blur, so no re-probe); list closes; keyboard stays open |
| 7 | Types a non-dash token or presses Space after a complete flag | List closes; typing is never intercepted |
| Exit | Any time: Escape, blur, or typing a non-flag token | List closes, value preserved |

### Error and edge states

| Situation | User sees | Exit path |
|---|---|---|
| No probe yet / NOT_FOUND / zero flags / TIMEOUT / transportError | Plain text input (no popup, no error) | Type flags by hand |
| Filter matches nothing | List closes silently (no "no results" row while typing) | Continue typing |
| Command changed after flags were fetched | Suggestions stay tied to the last found command until the new probe settles; a stale-result note is not shown; combobox reverts to plain input while checking | Re-check |
| Tab pressed with list closed | Normal focus move | - |
| Tab with list open and an option active | Accepts option (documented in hint text for screen readers via `aria-describedby`) | Escape first to leave without accepting |

### Acceptance criteria

- UX-9: Given probed flags and a `--mo` token, a user can insert `--model` in 2 keystrokes (Down, Enter) or 1 tap on the option.
- UX-10: The input exposes `role="combobox"`, `aria-autocomplete="list"`, `aria-expanded`, `aria-controls` and `aria-activedescendant`; the listbox uses `role="listbox"` with `role="option"` children carrying stable ids and `aria-selected`.
- UX-11: Screen reader announces each option as flag name, whether it takes a value, and description (for example "--model, takes a value: Model to use").
- UX-12: Escape closes the list without clearing text; typing is never prevented by the component.
- UX-13: Option rows are at least 44px tall on touch viewports; the list renders in normal document flow on viewports narrower than 640px so the on-screen keyboard cannot cover it.
- UX-14: Selecting an option by tap or click does not trigger a probe (no blur race).
- UX-15: When no flags are available, the field is visually and behaviorally identical to the previous plain input, with no error styling.

---

## S3. Program Config: unknown-flag warning

### Wireframe

```
Default CLI Flags
+---------------------------------------------+
| --verbos --model x                          |   (no aria-invalid)
+---------------------------------------------+
 [!] --verbos isn't listed in `claude --help`. It may still work
     (hidden or subcommand flags).  Did you mean --verbose?  [Use it]
```

Mobile: same text, wraps; `Use it` is a 44px button below the text. Multiple unknowns join in one line: `--verbos, --foo aren't listed in ...`, capped at 3 names then `and 2 more`.

### Interaction flow

| Step | User | System |
|---|---|---|
| 1 | Finishes typing a flag (on blur of flags field or when Space follows the token) | Validation runs locally against the probed flag set; no RPC |
| 2 | Warning appears below the field in `prog-flags-warning`, referenced by the input's `aria-describedby` | Polite live announcement is not used (the text is reachable when the input is focused via describedby) |
| 3 | Clicks `Use it` (only when edit distance <= 2 to exactly one known flag) | Replaces that token with the suggested flag; warning recomputes |
| 4 | Ignores the warning and saves | Saves unchanged; no dialog |
| Exit | Edit the token to a known flag, click `Use it`, or dismiss by saving | Warning never blocks |

Suppression rules: no warnings when the probe yielded zero flags, timed out, or the state is not FOUND_PARSED; no warning for the value token after a value-taking flag, for `--x=value`, bundled shorts that cannot be judged, `--no-<known>`, or anything after `--`.

### Error and edge states

| Situation | User sees | Exit path |
|---|---|---|
| Probe re-run produces a different flag set | Warning recomputed silently | - |
| Suggestion equidistant to two flags | No `Did you mean` text, only the base warning | Edit by hand or use combobox |
| Very long flag text | Warning wraps; token names are code-styled and truncated at 40 chars with ellipsis | - |

### Acceptance criteria

- UX-16: Warning text is `<flag> isn't listed in \`<program> --help\`. It may still work (hidden or subcommand flags).` and never uses the words "invalid" or "error".
- UX-17: The warning has no `role="alert"`, no `aria-invalid` on the input, and is not red-error styled; it uses the `warning` tokens with a triangle icon.
- UX-18: The input's `aria-describedby` includes the warning element id while a warning is visible.
- UX-19: Save remains enabled while a warning is visible; no confirmation is added.
- UX-20: No unknown-flag warning appears when the probe returned zero flags.
- UX-21: `Did you mean` appears only for a unique match within edit distance 2 and is a 44px control on touch.

---

## S4. `FlagInfoButton` (tap-to-reveal description)

### Wireframe

```
Collapsed:  --model <value>                 [ i ]     (44x44 hit area)
Expanded:   --model <value>                 [ i ]  aria-expanded=true
            Model to use for the main chat. (inline row, in flow)
```

Used in S2 option rows and S3 warnings. Desktop additionally shows the existing Radix `Tooltip` on hover/focus as an enhancement only; every fact available on hover is also available through the button.

### Interaction flow

| Step | User | System |
|---|---|---|
| 1 | Taps or clicks `[i]`, or focuses it and presses Enter/Space | Description row appears inline; `aria-expanded="true"`; button label updates to "Hide description for --model" |
| 2 | Taps again | Row collapses |
| 3 | Selects another option / closes the list | Expanded rows collapse with the list |
| Exit | Tap `[i]` again, Escape, or close the list | State is not persisted |

Inside the combobox listbox the button is not in the tab order (`tabIndex={-1}`); keyboard users get the active option's description inline automatically (Decision D3) and screen-reader users hear it via the option's accessible description. Tapping `[i]` does not select the option and does not blur the input.

### Error and edge states

| Situation | User sees | Exit path |
|---|---|---|
| Flag has no description | `[i]` not rendered (no empty disclosure) | - |
| Very long description (>300 chars, already capped) | Truncated with ellipsis, full text on expand | Tap to collapse |
| Narrow viewport | Description wraps in flow, never floats or clips | - |

### Acceptance criteria

- UX-22: A mobile user can read any flag's description with one tap; no hover is required anywhere.
- UX-23: The button is at least 44x44 CSS px, is a real `<button type="button">` with `aria-expanded` and `aria-controls` pointing to the description element, and a label naming the flag.
- UX-24: Tapping `[i]` does not change the input value, does not close the listbox, and does not trigger a probe.
- UX-25: The active option's description is visible without any gesture for keyboard users.
- UX-26: Flags without descriptions render no disclosure control.

---

## S5. Session creation: status badge and saved-flag warning

Scope (Flagged Choice 2): no new flags input. The panel shows the selected program's status and validates that program's saved `cli_flags` (plus alias `extraFlags` if already plumbed).

### Wireframes

Desktop and mobile share one layout (badge is below the native `<select>`, never inside options):

```
Program
+---------------------------------------------+
| aider                                    v  |   <select id=omnibar-program>
+---------------------------------------------+
 [check] Found: /usr/bin/aider · 42 flags detected
 [!] --bogus isn't listed in `aider --help`. It may still work
     (hidden or subcommand flags).
     Saved flags: --yes-always --bogus       [Edit in Program Config]
```

Not found (replaces the old `preset-program-warning`; same testid retained on this variant, only one message ever rendered):

```
 [!] "aidr" not found on the server's PATH. Sessions using this
     program may fail to start.        [Edit in Program Config]
```

### Interaction flow

| Step | User | System |
|---|---|---|
| 1 | Opens Advanced Options / picker shows a program | Panel resolves the option's `command`, calls the probe once, shows `Checking...` |
| 2 | Changes selection | Previous probe aborted; new probe fires; badge resets to checking for the new program |
| 3 | Sees found / not found / no flags / timeout / transport variants | Same vocabulary as S6 |
| 4 | Clicks `Edit in Program Config` | Navigates to that program's settings entry; unsaved omnibar input is preserved by existing behavior (open in the same route without clearing draft); if navigation would lose input, link opens in new tab |
| 5 | Creates the session anyway | Creation is never blocked or delayed by the probe |
| Exit | Change program, continue creating, or open Program Config | No modal, nothing to dismiss |

Results are memoized per command for the session so re-selecting a program does not re-probe.

### Error and edge states

| Situation | User sees | Exit path |
|---|---|---|
| Built-in/static option without `command` | Uses the option `value` as the command | - |
| Probe RPC unavailable | transportError variant; falls back to the old list-membership check text only if the RPC never responds | Retry, create anyway |
| Program exists but saved flags empty | Only badge; no flag warning | - |
| Saved flags contain unknown flags but probe found zero flags | No flag warning (suppression rule) | - |
| Fast selection changes | Only the latest selection's result renders | - |

### Acceptance criteria

- UX-27: Selecting a program with a missing binary shows the NOT_FOUND badge below the picker without any additional click; the old `preset-program-warning` text is not rendered at the same time (single message).
- UX-28: The badge is `role="status"`; the picker `<select>` references it through `aria-describedby`.
- UX-29: Session creation is never disabled or delayed by probe state (create button remains enabled during `checking`, NOT_FOUND, transportError).
- UX-30: The flag warning (`omnibar-flags-warning`) lists only flags from saved `cli_flags` (and alias `extraFlags` where available) that are absent from the parsed set, using the same wording as UX-16.
- UX-31: On a 375px viewport the badge and warning appear directly under the picker, wrap without horizontal scroll, and `Edit in Program Config` is a 44px target.
- UX-32: Probe does not fire per keystroke and does not fire on panel mount for programs when the section is closed (only when the picker's program is visible or changes).

---

## S7. Probe audit log line (condensed, non-interactive)

Sample (`~/.stapler-squad/logs/staplersquad.log`, JSON line):

```json
{"level":"INFO","msg":"program probe","resolved_path":"/usr/bin/aider","status":"FOUND_PARSED","flags":42,"duration_ms":210,"cache_hit":false,"truncated":false}
```

Acceptance criteria:
- UX-33: Every completed probe emits one line with `resolved_path`, `status`, `flags`, `duration_ms`, `cache_hit`, `truncated`.
- UX-34: NOT_FOUND logs at Debug; semaphore saturation logs at Warn.
- UX-35: The line never contains command arguments or environment-assignment values.

---

## Cross-cutting accessibility and form-factor criteria

- UX-36: Every interactive control in S1-S5 is reachable and operable by keyboard alone in a logical order: command input, Check, badge toggle, flags input (list navigation by arrows), warning actions.
- UX-37: The Program Config form and the omnibar picker pass Axe (WCAG 2.1 AA) with no violations in found, not-found, checking, and warning states; e2e locators use `data-testid` or ARIA roles only.
- UX-38: All colored states pair color with a distinct icon shape and text; text contrast at least 4.5:1 in both themes.
- UX-39: Touch targets (`Check`, Retry, `[i]`, option rows, `Use it`, `Edit in Program Config`, path toggle) are at least 44x44 CSS px.
- UX-40: Both command and flags inputs disable auto-capitalization, autocorrect and spellcheck so mobile keyboards do not alter `--flags`.
- UX-41: Time-to-save for the config form is unchanged: no probe or warning adds a required step.

Total UX acceptance criteria: 41 (UX-1 to UX-41), plus the 5 in the S6 vocabulary block.

## Flow completeness check (no dead ends)

| Flow | Error state | Exit path |
|---|---|---|
| S1 probe | not found, timeout, ERROR, transportError | Edit and re-check, Retry, or Save anyway |
| S2 autocomplete | absent flags, empty match | Falls back to plain input; Escape |
| S3 warning | unknown flag, ambiguous suggestion | Edit, `Use it`, or Save anyway |
| S4 disclosure | missing description | Control not rendered; tap to collapse |
| S5 picker | not found, transport, no flags | Retry, Edit in Program Config, create anyway |

## Decisions the plan should confirm

- D1: `Enter` in the command field runs `Check` and does not submit the form (recommended; not in the plan).
- D2: Remote-host label: recommend the copy `Checked on this server only` for programs used against remote targets, because the requirements do not cover remote probing (`research/ux.md` section 4). Needs a plan task if accepted; otherwise omit.
- D3: Keyboard access to descriptions inside the combobox: inline description on the active option (recommended) versus Right-arrow to reach the `[i]` button. Plan task 4.2.2a leaves this open; this design picks inline-on-active.
- D4: `Edit in Program Config` link in S5 and the `Use it` action in S3 are not in the plan; both are optional polish and can be dropped without breaking any AC (UX-21, UX-30 partly, UX-31 partly).
- D5: `Check` button visible on desktop as well as mobile (recommended for parity and testability; plan task 2.2.1a mentions it as mobile-motivated).
