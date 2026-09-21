# UX Design: CLI Flag Discovery

Inputs: `../requirements.md`, `../research/ux.md`, `../implementation/plan.md` (Flagged Choices 2, 3, 5, 6, 7, 9, 11, 12). Element ids and testids follow the plan (`prog-command-input`, `prog-command-status`, `prog-command-check`, `prog-flags-input`, `prog-flags-warning`, `omnibar-flags-warning`, `preset-program-warning`).

Principles: the probe never blocks saving or launching; warnings are soft ("is not listed in `--help`", never "invalid"); nothing is conveyed by color alone; nothing is conveyed by hover alone; probe results never steal focus; a transport failure is never shown as "not found"; a script is never run without an explicit Check.

## Surface inventory

| # | Surface | Kind | Where |
|---|---------|------|-------|
| S1 | Command field + `ProbeStatusBadge` | Interactive | Program Config (`ProgramsManager.tsx`) |
| S2 | `FlagCombobox` (Default CLI Flags) | Interactive | Program Config |
| S3 | Unknown-flag warning | Read-only text | Program Config |
| S4 | `FlagInfoButton` disclosure | Interactive | Warnings list and "Available flags" disclosure |
| S5 | Session-creation badge + saved-flag warning | Interactive | `OmnibarCreationPanel.tsx` via `ProgramProbeSection` |
| S6 | Status-to-copy map (probe result vocabulary) | Non-interactive | Shared by S1, S5 |
| S7 | Probe audit log line | Non-interactive | Server log |

## Shared: Probe state to UI copy (S6, condensed)

Single source of wording, used by both forms. Icon shapes differ (color is redundant). Copy matches plan AC5 and Task 2.1.2a.

| `ProbeUiState` / status | Icon (aria-hidden) | Text | Tone |
|---|---|---|---|
| idle | none | (nothing rendered) | - |
| checking | spinner (static under reduced-motion) | `Checking...` (the live-region text is announced only after 300ms; a fast probe announces just the result) | neutral |
| FOUND_PARSED | check | `Found: /usr/bin/claude` + ` 14 flags detected`; detail `Checked on this server only` | success |
| FOUND_NO_FLAGS | check | `Found: /usr/bin/claude. Couldn't read flags from --help; flag suggestions unavailable.` | neutral |
| NEEDS_CONFIRM (script, or picker resolve-only) | check (found) + info | `Found: <path>. Not checked for flags yet. Check runs `<program> --help` on this server.` (`<program>` is the command token, for example `claude`) + `Check` control; the `Check` control is a full-width 44px row under the text below 640px, inline after the text at 640-1024px | neutral |
| TIMEOUT | check (found) + info | `Found: /path. Timed out reading flags — try Check again.` (distinct from "Couldn't read flags") | neutral |
| wrapper (`is_wrapper`) | check + info | `Wrapper command (env): flags for the wrapped program are not checked.` | neutral |
| ERROR / BUSY | info | `Couldn't check right now.` + Retry | neutral |
| NOT_FOUND | warning triangle | `Not found as an executable on the server's PATH. Shell aliases and functions are not checked; the program may still work when launched from your shell.` | warning |
| transportError (network, `permission_denied`/403 from the guard) | warning triangle | `Couldn't check right now.` + one sentence that the server may refuse probes when it listens on a non-loopback address without auth + Retry | warning |

Rules: text plus icon always; suffix line `Checked: claude` shows the first token that was probed (so `python -m x` reads "Checked: python"); long paths truncate with ellipsis, full path revealed by the badge's expand toggle (44px). Wording says "the server's PATH" because the check runs on the stapler-squad host, not the browser machine. "Checked on this server only" is shown in the detail of every found variant (Decision D2).

Sample (S6 is a vocabulary, so one sample):

```
[check] Found: /usr/local/bin/claude  ·  14 flags detected
        Checked on this server only · Checked: claude   [ Show full path ]
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
         Checked on this server only · Checked: claude   [Show full path]
Default CLI Flags
+---------------------------------------------+
| ...                                         |
```

Mobile (375px, single column, everything wraps under the field):

```
Command
+-----------------------------------+
| claudee                           |
+-----------------------------------+
+-----------------------------------+   <- 44px
|            Check                  |
+-----------------------------------+
 [!] Not found as an executable on
     the server's PATH. Shell aliases
     and functions are not checked;
     the program may still work when
     launched from your shell.
     Checked: claudee
```

Inputs carry `autoCapitalize="off" autoCorrect="off" spellCheck={false}`. The `Check` button is visible on both form factors (secondary style on desktop, full-width 44px on mobile) because on mobile "blur" means dismissing the keyboard, which users do not always do before tapping Save.

### Interaction flow

| Step | User | System |
|---|---|---|
| 1 | Types in command field | Badge unchanged (no per-keystroke probing). If a prior result exists for a different command, badge switches to idle (stale result hidden) |
| 2 | Blurs field | Implicit probe: badge shows `Checking...`; a native binary is run with `--help`, a script is not (step 3e). Field stays editable; focus not moved |
| 2b | Presses `Check` | Explicit probe: the only action that confirms running a script; also bypasses a cached TIMEOUT. The `Check` button stays mounted and keeps focus, is `disabled` with `aria-busy="true"` while the run is in flight (no double submit), and is relabelled `Check again` afterwards |
| 2c | Presses Enter in the field while it is non-empty | Immediate implicit probe (same as blur, no waiting for blur). A native binary runs; a script shows the NEEDS_CONFIRM hint and is **not** run. The form is never submitted |
| 3a | Server returns found | Badge shows found variant; flags field becomes autocomplete-capable (S2) |
| 3b | Server returns NOT_FOUND | Warning variant; Save stays enabled |
| 3c | Response arrives for an older command (user kept typing) | Discarded; badge remains idle/checking for the current command |
| 3d | RPC fails | transportError variant with Retry |
| 3e | Server returns NEEDS_CONFIRM (the target is a script, not a native binary) | Badge shows the NEEDS_CONFIRM variant; nothing was executed. `Check` runs it |
| 4 | Presses Retry / Check | Returns to step 2b |
| 5 | Clicks Save at any time, even mid-check | Saves; in-flight probe is aborted; no confirmation dialog |
| 6 | Clears the field | Badge disappears (idle); no probe on empty command |
| Exit | Cancel, Save, or navigating away | Aborts in-flight probe on unmount; no leftover state |

Enter behavior: Enter in the command field runs the implicit probe only and never submits the form (prevents accidental save on mobile keyboards' "Go"). It is deliberately not consent to execute a script, because on mobile "Go" would consent silently; consent is the explicit `Check` button.

### Error and edge states

| Situation | User sees | Exit path |
|---|---|---|
| Not found | Warning variant | Fix command and re-check, or Save anyway |
| Command has only env assignments / unbalanced quote / starts with `-` / relative path | NOT_FOUND text (server maps every `ResolveError` to not-found) | Edit and re-check, or Save anyway |
| Program is a script (shebang) | NEEDS_CONFIRM variant ("Not checked for flags yet. Check runs `<program> --help` on this server."); not run on blur or Enter; the flags field shows the hint "Suggestions appear after Check reads the flags." | `Check` to run, or Save anyway |
| Probe times out | TIMEOUT variant (found retained if path resolved), "try Check again" | Check again; flags typed by hand |
| Server unreachable or guard refuses | transportError + Retry | Retry or Save anyway |
| Semaphore saturated / spawn failure | ERROR/BUSY variant ("Couldn't check right now.") + Retry | Retry, Save anyway |
| Command with wrapper (`npx claude`, `env ... claude`) | `Wrapper command (npx): flags for the wrapped program are not checked.` | Explains scope; no suggestions or warnings |
| Program used for remote targets | Detail `Checked on this server only` (shown on every found variant, D2) | Save anyway |

### Acceptance criteria

- UX-1: A user can learn whether a typed command exists in 1 action after typing (blur, Enter, or one tap on `Check`; a script is only resolved by blur/Enter, and read for flags by `Check`), with no extra navigation.
- UX-2: The badge is a `role="status"` (`aria-live="polite"`) region with a stable id referenced by the command input's `aria-describedby`; `aria-invalid` is never set by a probe result.
- UX-3: Save is enabled and clickable in every probe state, including `checking`, NOT_FOUND and transportError; no confirmation modal is introduced.
- UX-4: A slow response for a previous command never overwrites the state for the current command (stale drop).
- UX-5: The `Check` button and the Retry button are at least 44x44 CSS px and reachable by Tab in DOM order after the command input.
- UX-6: Probe results never move keyboard focus or scroll the page.
- UX-7: On a 375px viewport the badge text wraps; no horizontal scroll appears; full path is reachable via a 44px toggle.
- UX-8: Under `prefers-reduced-motion` the spinner is static and the text `Checking...` is still present.
- UX-42: A script (shebang) is never executed on blur, on Enter in the command field, or on picker selection; it runs only after the `Check` button, and the badge says the flags have not been checked yet and what Check does.

---

## S2. Program Config: `FlagCombobox` (Default CLI Flags)

### Wireframes

Desktop (list anchored under the input; the active option shows its description inline, rows contain text only, no buttons):

```
Default CLI Flags
+---------------------------------------------+
| --yes --mo|                                 |  role=combobox
+---------------------------------------------+
+---------------------------------------------+
| --model <value>                             |  <- active (aria-activedescendant)
|   Model to use for the main chat            |     description inline
| --model-settings <value>                    |
| --model-metadata-file                       |
+---------------------------------------------+
```

Mobile (list rendered in-flow below the input, pushing content down, max-height ~40vh with scroll, rows 44px):

```
Default CLI Flags
+-----------------------------------+
| --yes --mo|                       |
+-----------------------------------+
| --model <value>                   |  44px, active
|   Model to use for the main chat  |
| --model-settings <value>          |  44px
| --model-metadata-file             |  44px
+-----------------------------------+
(list is in normal flow, not a floating overlay)
```

Descriptions for flags that are not the active option are reachable through the "Available flags (N)" disclosure under the field (S4). Suggestions appear only when a probe returned flags (`FOUND_PARSED`); otherwise the field is a plain input (silently), and a hint under the field reads `Check the command above to enable flag suggestions` only when no probe has run yet.

### Interaction flow

| Step | User | System |
|---|---|---|
| 1 | Types a token starting with `-` | Listbox opens with prefix matches, then substring matches, for the token at the caret only; max 8 visible then scroll |
| 2 | Down / Up | Moves active option (wraps); `aria-activedescendant` updates; the active option's description shows inline and is exposed via `aria-describedby` on the input; DOM focus stays in the input |
| 3 | Enter (list open, option active), or Tab **only if the user has already arrowed to an option** | Replaces the token at caret with the flag; appends a trailing space; list closes. Value-taking flags do not add `=` |
| 4 | Escape | Closes list; text unchanged; focus stays |
| 5 | Alt+Down | Opens list for the current token |
| 6 | Taps an option (touch) | Option selected via pointer-down (input does not blur, so no re-probe); list closes; keyboard stays open |
| 7 | Types a non-dash token or presses Space after a complete flag | List closes; typing is never intercepted |
| Exit | Any time: Escape, blur, or typing a non-flag token | List closes, value preserved |

### Error and edge states

| Situation | User sees | Exit path |
|---|---|---|
| No probe yet / NOT_FOUND / NEEDS_CONFIRM / zero flags / TIMEOUT / transportError / wrapper | Plain text input (no popup, no error). Hint under the field, referenced by `aria-describedby`: "Check the command above" when no probe has run, "Suggestions appear after Check reads the flags." when NEEDS_CONFIRM | Type flags by hand |
| Filter matches nothing | List closes silently (no "no results" row while typing) | Continue typing |
| Command changed after flags were fetched | Suggestions stay tied to the last found command until the new probe settles; a stale-result note is not shown; combobox reverts to plain input while checking | Re-check |
| Tab pressed with list closed | Normal focus move | - |
| Tab with list open and no option arrowed to (`aria-activedescendant` not set by the user; the first option is never auto-active) | Normal focus move; typed text untouched; list closes | - |
| Tab with list open after the user arrowed to an option | Accepts that option (same as Enter) | Escape first to leave without accepting |

### Acceptance criteria

- UX-9: Given probed flags and a `--mo` token, a user can insert `--model` in 2 keystrokes (Down, Enter) or 1 tap on the option.
- UX-10: The input exposes `role="combobox"`, `aria-autocomplete="list"`, `aria-expanded`, `aria-controls` and `aria-activedescendant`; the listbox uses `role="listbox"` with `role="option"` children carrying stable ids and `aria-selected`; options contain no interactive children.
- UX-11: Screen reader announces each option as flag name and whether it takes a value, with the description via the input's accessible description (for example "--model, takes a value" then "Model to use").
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
 [!] --verbos is not listed in `claude --help`. It may still work
     (hidden or subcommand flags).
     Available flags (14)  v
```

Mobile: same text, wraps. Multiple unknowns join in one line: `--verbos, --foo are not listed in ...`, capped at 3 names then `and 2 more`.

### Interaction flow

| Step | User | System |
|---|---|---|
| 1 | Finishes typing a flag (on blur of flags field or when Space follows the token) | Validation runs locally against the probed flag set; no RPC |
| 2 | Warning appears below the field in `prog-flags-warning`, referenced by the input's `aria-describedby` | Polite live announcement is not used (the text is reachable when the input is focused via describedby) |
| 3 | Opens "Available flags (N)" (optional) | Lists probed flags, each with a `FlagInfoButton` (S4) |
| 4 | Ignores the warning and saves | Saves unchanged; no dialog |
| Exit | Edit the token to a known flag, or dismiss by saving | Warning never blocks |

Suppression rules: no warnings when the probe yielded zero flags, timed out, is NEEDS_CONFIRM, or the state is not FOUND_PARSED; no warning for the value token after a value-taking flag, for `--x=value`, bundled shorts that cannot be judged, `--no-<known>`, or anything after `--`; none when the probed command is a wrapper.

### Error and edge states

| Situation | User sees | Exit path |
|---|---|---|
| Probe re-run produces a different flag set | Warning recomputed silently | - |
| Known-valid flag the parser missed (hidden or unparsed) | Only the soft "is not listed" warning; never blocks | Ignore and save |
| Very long flag text | Warning wraps; token names are code-styled and truncated at 40 chars with ellipsis | - |

### Acceptance criteria

- UX-16: Warning text is `<flag> is not listed in \`<program> --help\`. It may still work (hidden or subcommand flags).` and never uses the words "invalid" or "error".
- UX-17: The warning has no `role="alert"`, no `aria-invalid` on the input, and is not red-error styled; it uses the `warning` tokens with a triangle icon.
- UX-18: The input's `aria-describedby` includes the warning element id while a warning is visible.
- UX-19: Save remains enabled while a warning is visible; no confirmation is added.
- UX-20: No unknown-flag warning appears when the probe returned zero flags.
- UX-21: (withdrawn; number retained so UX-22 to UX-42 references stay stable.)

---

## S4. `FlagInfoButton` (tap-to-reveal description)

### Wireframe

```
Collapsed:  --model <value>                 [ i ]     (44x44 hit area)
Expanded:   --model <value>                 [ i ]  aria-expanded=true
            Model to use for the main chat. (inline row, in flow)
```

Used in the "Available flags (N)" disclosure and the warnings list, always outside `role=listbox` and `role=option`. There is no Radix `Tooltip` and no hover handler: every fact is available by tap, click or keyboard. Inside the combobox listbox only the active option's description is shown inline (S2, Decision D3).

### Interaction flow

| Step | User | System |
|---|---|---|
| 1 | Taps or clicks `[i]`, or focuses it and presses Enter/Space | Description row appears inline; `aria-expanded="true"`; button label updates to "Hide description for --model" |
| 2 | Taps again | Row collapses |
| Exit | Tap `[i]` again, or close the disclosure | State is not persisted |

### Error and edge states

| Situation | User sees | Exit path |
|---|---|---|
| Flag has no description | `[i]` not rendered (no empty disclosure) | - |
| Very long description (>300 chars, already capped) | Truncated with ellipsis, full text on expand | Tap to collapse |
| Narrow viewport | Description wraps in flow, never floats or clips | - |

### Acceptance criteria

- UX-22: A mobile user can read any flag's description with one tap; no hover is required anywhere.
- UX-23: The button is at least 44x44 CSS px, is a real `<button type="button">` with `aria-expanded` and `aria-controls` pointing to the description element, and a label naming the flag.
- UX-24: Tapping `[i]` does not change the input value and does not trigger a probe.
- UX-25: The active option's description is visible without any gesture for keyboard users.
- UX-26: Flags without descriptions render no disclosure control.

---

## S5. Session creation: status badge and saved-flag warning

Scope (Flagged Choice 2): no new flags input. The panel (through `ProgramProbeSection`) shows the selected program's status and validates that program's saved `cli_flags`. Alias `extraFlags` are not validated.

### Wireframes

Desktop and mobile share one layout (badge is below the native `<select>`, never inside options):

```
Program
+---------------------------------------------+
| aider                                    v  |   <select id=omnibar-program>
+---------------------------------------------+
 [check] Found: /usr/bin/aider · 42 flags detected
 [!] --bogus is not listed in `aider --help`. It may still work
     (hidden or subcommand flags).
```

Script not yet checked (selection never runs a program):

```
 [check] Found: /usr/local/bin/aider. Not checked for flags yet.
         Check runs `aider --help` on this server.
 +---------------------------------------------+
 |                   Check                     |   <- full-width 44px row at 375px;
 +---------------------------------------------+      inline after the text at 640-1024px
```

Not found (replaces the old `preset-program-warning`; same testid retained on this variant, only one message ever rendered):

```
 [!] Not found as an executable on the server's PATH. Shell aliases and
     functions are not checked; the program may still work when launched
     from your shell.
```

### Interaction flow

| Step | User | System |
|---|---|---|
| 1 | Opens Advanced Options / picker shows a program | Panel resolves the option's `command` and sends a resolve-only request: shows `Checking...`, then found, not found, or NEEDS_CONFIRM. Nothing unconfirmed is executed. A program the user already checked on this server (same path, mtime and size, this server process) is served from the cache or re-probed by the server and **never regresses to NEEDS_CONFIRM after the 10-minute cache TTL** |
| 2 | Changes selection | Previous request aborted; new resolve-only request fires; badge resets to checking for the new program |
| 3 | Sees found / not found / needs-confirm / no flags / timeout / transport variants | Same vocabulary as S6 |
| 4 | Selects `Check` on a NEEDS_CONFIRM badge | Explicit probe runs the program with `--help`; flags and saved-flag warning then appear |
| 5 | Creates the session anyway | Creation is never blocked or delayed by the probe |
| Exit | Change program or continue creating | No modal, nothing to dismiss |

Results are memoized per command for the session so re-selecting a program does not re-probe. NEEDS_CONFIRM returns for an already-checked program only when its file changed (new mtime or size: the earlier consent covered different bytes) or the server restarted (the confirmed set is in memory; persisting execution consent to disk is a security-model change and is deferred, see plan Unresolved Questions). The `Check` button stays mounted and focused, disabled with `aria-busy` during a run.

### Error and edge states

| Situation | User sees | Exit path |
|---|---|---|
| Built-in/static option without `command` | Uses the option `value` as the command | - |
| Probe RPC unavailable | transportError variant ("Couldn't check right now."); no fallback check | Retry, create anyway |
| Program exists but saved flags empty | Only badge; no flag warning | - |
| Saved flags contain unknown flags but probe found zero flags, or state is NEEDS_CONFIRM | No flag warning (suppression rule) | - |
| Fast selection changes | Only the latest selection's result renders | - |

### Acceptance criteria

- UX-27: Selecting a program with a missing binary shows the NOT_FOUND badge below the picker without any additional click; the old `preset-program-warning` text is not rendered at the same time (single message).
- UX-28: The badge is `role="status"`; the picker `<select>` references it through `aria-describedby`.
- UX-29: Session creation is never disabled or delayed by probe state (create button remains enabled during `checking`, NOT_FOUND, NEEDS_CONFIRM, transportError).
- UX-30: The flag warning (`omnibar-flags-warning`) lists only flags from saved `cli_flags` that are absent from the parsed set, using the same wording as UX-16.
- UX-31: On a 375px viewport the badge and warning appear directly under the picker and wrap without horizontal scroll.
- UX-32: Probe does not fire per keystroke and does not fire on panel mount for programs when the section is closed (only when the picker's program is visible or changes).

---

## S7. Probe audit log line (condensed, non-interactive)

Sample (`~/.stapler-squad/logs/staplersquad.log`, JSON line; matches plan Observability):

```json
{"level":"INFO","msg":"program_probe","resolved_path":"/usr/bin/aider","command_token":"aider","status":"FOUND_PARSED","flags":42,"duration_ms":210,"cache_hit":false,"truncated":false,"is_wrapper":false}
```

Acceptance criteria:
- UX-33: Every probe attempt emits one `program_probe` line with `resolved_path`, `status`, `flags`, `duration_ms`, `cache_hit`, `truncated`.
- UX-34: Every outcome logs at Info (NOT_FOUND and cache hits included, since the line is the audit record); BUSY (semaphore saturation) logs at Warn.
- UX-35: The line never contains command arguments or environment-assignment values.

---

## Cross-cutting accessibility and form-factor criteria

- UX-36: Every interactive control in S1-S5 is reachable and operable by keyboard alone in a logical order: command input, Check, badge toggle, flags input (list navigation by arrows), "Available flags" disclosure.
- UX-37: The Program Config form and the omnibar picker pass Axe (WCAG 2.1 AA) with no violations in found, not-found, checking, and warning states; e2e locators use `data-testid` or ARIA roles only.
- UX-38: All colored states pair color with a distinct icon shape and text; text contrast at least 4.5:1 in both themes.
- UX-39: Touch targets (`Check`, Retry, `[i]`, option rows, path toggle) are at least 44x44 CSS px.
- UX-40: Both command and flags inputs disable auto-capitalization, autocorrect and spellcheck so mobile keyboards do not alter `--flags`.
- UX-41: Time-to-save for the config form is unchanged: no probe or warning adds a required step.
- UX-43: The `Check` button stays mounted and keeps keyboard focus through a run and after the result; it is `disabled` with `aria-busy="true"` while a run is in flight (no double submit); the "Checking..." text in the live region is announced only if the run exceeds 300ms.
- UX-44: In the flags combobox Tab never changes typed text unless the user has arrowed to an option first; Enter accepts the active option.
- UX-45: At 375px the `Check` control is a full-width row at least 44px high; at 640-1024px it sits inline and wraps without clipping; with the on-screen keyboard open (about 300px of visual viewport) the active option and Save stay reachable by scroll.

Total UX acceptance criteria: 45 (UX-1 to UX-45; UX-21 withdrawn), plus the 5 in the S6 vocabulary block.

## Flow completeness check (no dead ends)

| Flow | Error state | Exit path |
|---|---|---|
| S1 probe | not found, needs-confirm, timeout, ERROR/BUSY, transportError | Edit and re-check, Check, Retry, or Save anyway |
| S2 autocomplete | absent flags, empty match | Falls back to plain input; Escape |
| S3 warning | unknown flag | Edit, or Save anyway |
| S4 disclosure | missing description | Control not rendered; tap to collapse |
| S5 picker | not found, needs-confirm, transport, no flags | Check, Retry, or create anyway |

## Decisions (status against the plan)

- D1: `Enter` in the command field never submits the form and runs the implicit probe; for a script it shows the NEEDS_CONFIRM hint and does not run it (only the `Check` button confirms). Adopted with this change (plan task 2.2.1a; the original D1 made Enter equal to Check).
- D2: Copy `Checked on this server only` in the badge detail of every found variant. Adopted (plan task 2.1.2a).
- D3: Keyboard access to descriptions: inline description on the active option; `FlagInfoButton` only outside options (warnings list, "Available flags" disclosure); no Radix tooltip. Adopted (plan task 4.2.2a).
- D4: A suggestion action in the warning and an "edit in Program Config" link in the panel. Dropped; no AC depends on them.
- D5: `Check` button visible on desktop and mobile. Adopted (plan task 2.2.1a).
