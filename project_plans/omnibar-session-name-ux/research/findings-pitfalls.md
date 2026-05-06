# Findings: Pitfalls

## Summary

Four feature areas — slug generation, inline `>` separator, Tab/Ctrl+Tab keyboard shortcuts, and slash-command prefix detection — each carry distinct failure modes. The keyboard shortcut design has the highest severity: `Ctrl+Tab` is a hard browser-level conflict with no workaround; Tab inside the omnibar is already claimed by path-completion autocomplete dropdown cycling. Slug generation has the widest surface area of edge cases but all are solvable with a standard normalization pipeline. The `>` separator is low-risk on supported platforms. Slash-command priority injection into the DetectorRegistry is a one-line fix but must happen at the right layer.

**Prioritized pitfall list (highest to lowest):**

1. **[CRITICAL] Ctrl+Tab is a browser/OS conflict — abandon it**
2. **[HIGH] Tab key dual-purpose collision** (already used for path completion cycling in `Omnibar.tsx`)
3. **[HIGH] `/oneoff` detected as LocalPath** before slash-command detector runs
4. **[MEDIUM] Slug degenerates to empty string** on all-emoji, all-special-chars, or whitespace-only input
5. **[MEDIUM] `>` separator fires on path-detected inputs** — should only run in bare-text `SessionSearch` mode
6. **[LOW] Leading-number slug** — valid in git but unusual
7. **[LOW] Long-input slug truncation** — produces trailing hyphen; needs trimming
8. **[LOW] Multiple `>` characters** — need a clear rule (first split wins)

---

## Options Surveyed

### Slug Generation Edge Cases

| Input | Expected Slug | Risk |
|---|---|---|
| `MY FEATURE` | `my-feature` | None — `.toLowerCase()` handles it |
| `123-test` | `123-test` | Git branch warning (valid but unusual) |
| `  ` (spaces only) | `""` (empty) | Must handle: block submit with inline error |
| `!!!` (specials only) | `""` (empty) | Must handle: block submit with inline error |
| `🚀 launch` | `launch` | Emoji stripped; remaining text slugified |
| `🚀` (emoji only) | `""` (empty) | Degenerate: block submit, not silent fallback |
| `你好世界` (CJK only) | `""` (empty after latin-strip) | Degenerate: no pinyin romanization; block submit |
| `a > b > c` | slug:`a`, prompt:`b > c` | Take first `>` only; rest is prompt verbatim |
| 120-character input | truncated to ~50 chars at word boundary | Trailing-hyphen risk after truncation |
| `$(rm -rf /)` | `rm-rf` | No injection risk; shell chars stripped, residual slugified |

### Inline `>` Separator Platform Considerations

`>` is a valid character in macOS/Linux filenames but a shell redirection operator. Since the omnibar input is a UI text field (not a shell), there is no shell interpretation risk. On Windows `>` is forbidden in filenames, but session names are not used as filenames — they become slugified branch names and UI labels. `>` is safe as a separator on all supported platforms. The only risk is a user genuinely wanting `>` in their session name, which is a documented trade-off.

### Keyboard Shortcut Options

| Shortcut | Conflict | Verdict |
|---|---|---|
| Tab | Path-completion cycling already in `Omnibar.tsx`; browser focus trap | Context-dependent: usable only when `isDropdownVisible === false` |
| Ctrl+Tab | Browser tab-switching — **not deliverable to JS event listeners** in Chrome/Firefox/Safari | Abandon entirely |
| Alt+Tab | macOS application switcher; Windows window switcher | Abandon entirely |
| Ctrl+, | VS Code "open settings" — only fires if omnibar is the active focus | Viable |
| Ctrl+[ / Ctrl+] | Not universally claimed; matches VS Code panel navigation | Strongest alternative candidate |
| Shift+Tab | Browser reverse-focus cycle; works with `preventDefault` but harms a11y | Avoid |

**VS Code precedent:** Ctrl+Shift+P command palette uses Tab for autocomplete, Ctrl+Tab for editor tab cycling — in separate keyboard contexts. Linear uses prefix keys not Tab. Raycast uses Tab for a "secondary action" (open-in vs. copy), not for mode cycling.

### Slash-Command vs. Path Detection Conflict

Current `LocalPathDetector` (priority 100) fires on any input where `trimmed.startsWith("/")`. A slash-command like `/oneoff` starts with `/` → `isAbsolute = true` → returns `InputType.LocalPath` with `suggestedName: "oneoff"`. The command is silently misinterpreted.

A new `SlashCommandDetector` must run at priority < 10 (before all existing detectors). It must match only the exact known command strings (`/oneoff`, `/worktree`, `/dir`, `/existing`) and return null for everything else (so `/opt/homebrew` is not intercepted).

---

## Trade-off Matrix

| Feature | Option A | Option B | Winner |
|---|---|---|---|
| Slug degenerate fallback | Return `""` — block submit with error | Return `"session"` literal | Block submit + show inline error. Silent `"session"` creates naming collisions |
| `>` separator scope | Fire on all input modes | Fire only in `SessionSearch` / bare-text mode | Bare-text only — paths and URLs must not be split on `>` |
| Tab for session type cycling | Use Tab always | Gate Tab on `!isDropdownVisible` | Gate on `!isDropdownVisible` — path completion behavior is load-bearing |
| Ctrl+Tab | Intercept with `e.preventDefault()` | Abandon | Abandon — browser does not deliver this event to JS |
| Multiple `>` in input | First split: name=`a`, prompt=`b > c` | Last split: name=`a > b`, prompt=`c` | First split — leftmost `>` is the separator; rest is prompt verbatim |
| SlashCommandDetector priority | Priority 5 (before all) | Priority 8 | Priority 5; exact value matters less than being first |

---

## Risk and Failure Modes

**RF-1 [MEDIUM]: Slug degenerates to empty string.** Trigger: emoji-only, CJK-only, or special-char-only input. Effect: submit gated on `!!sessionName.trim()` silently blocks; no feedback. Mitigation: show inline validation error ("Session name is empty after cleaning special characters").

**RF-2 [CRITICAL]: Ctrl+Tab delivers no event to JS.** Trigger: user presses Ctrl+Tab. Effect: browser switches tabs; omnibar context and in-progress session data are discarded. Mitigation: do not implement Ctrl+Tab. Use Ctrl+[ / Ctrl+] or Tab (gated).

**RF-3 [HIGH]: Tab cycles session type while path completion dropdown is open.** Trigger: user types a path, dropdown appears, presses Tab expecting to autocomplete the path — but Tab was re-bound to "cycle session type." Effect: path completion bypassed; session type changes unexpectedly. Mitigation: gate the session-type-cycling Tab shortcut on `!isDropdownVisible`. The `isDropdownVisible` boolean already exists in `Omnibar.tsx`.

**RF-4 [HIGH]: `/oneoff` detected as LocalPath.** Trigger: user types `/oneoff`. Effect: `LocalPathDetector` fires (priority 100 but `startsWith("/")` guard triggers); mode not switched; local-path badge appears. Mitigation: register `SlashCommandDetector` at priority 5 in `createDefaultRegistry()`.

**RF-5 [MEDIUM]: `>` fires on path inputs.** Trigger: user types `/Users/foo/project > ` accidentally. Effect: input is split; path before `>` becomes session name slug; text after `>` becomes first prompt; path is silently lost. Mitigation: only apply `>` splitting when `detection.type === InputType.SessionSearch`.

**RF-6 [LOW]: Trailing hyphen after slug truncation.** Trigger: long input truncated at word boundary ends in `-`. Effect: ugly slug. Mitigation: `slug.replace(/-+$/, "")` after truncation.

**RF-7 [LOW]: Leading-digit slug.** Trigger: `123 fix auth`. Result: `123-fix-auth`. Git allows it; some older CI tools may warn. Mitigation: document in ADR; no code change required.

**RF-8 [LOW]: Slug auto-fill race with manual session name edit.** Trigger: user manually clears and re-types session name; continuing to edit the main input triggers a new `suggestedName` that overwrites the manual value. Mitigation: the existing `lastSuggestedNameRef` guard in `Omnibar.tsx` lines 360–364 already handles this — confirm the new bare-text slug path uses the same guard.

---

## Migration and Adoption Cost

All mitigations are additive. No existing session data or user preferences require migration. The `lastSuggestedNameRef` guard already exists. The `SlashCommandDetector` is a new class in `detector.ts` — no existing tests break unless the priority ordering is wrong. `Ctrl+Tab` removal is a non-change (it was never shipped).

Estimated file touchpoints:
- `/web-app/src/lib/omnibar/detector.ts` — add `SlashCommandDetector`, register at priority 5
- `/web-app/src/lib/omnibar/detector.test.ts` — new describe block for `SlashCommandDetector`
- `/web-app/src/components/sessions/Omnibar.tsx` — gate Tab shortcut on `!isDropdownVisible`; add `>` parsing gated on `SessionSearch` type; slug normalization call
- New pure function module: slug normalization + tests (no component coupling)

---

## Operational Concerns

**Slug empty-state UX:** If the slug pipeline returns empty and `canSubmit` is blocked, the creation panel shows a silently disabled submit button with no explanation. Inline validation message is required.

**IME (Input Method Editor) on CJK keyboards:** macOS/Windows CJK IMEs fire `compositionstart`/`compositionend` events. During composition, `onChange` fires with partial phonetic input. The 150ms debounce in the existing detection effect is likely sufficient to avoid mid-composition slugification, but the slug pipeline should be aware that composed characters may yield empty output.

**`>` in pasted clipboard content:** A user pasting from a terminal error like `Build failed: step > overflow` will see the `>` separator fire and split their intended session name. This is acceptable behavior in bare-text mode as long as the session name field shows the split result immediately (live preview feedback).

---

## Prior Art and Lessons Learned

**Todoist inline syntax**: key lesson is **visible real-time feedback** — users must see the split result as they type, not only on submit.

**Linear command palette**: uses `/` prefix for command mode — identical to what slash-commands propose here. Vocabulary must be a closed set.

**VS Code Tab in command palette**: Tab completes autocomplete selection. It does NOT cycle result modes. VS Code uses separate shortcuts for mode switching. **Do not multiplex Tab across two conflicting functions in the same input element.**

**Browser Ctrl+Tab behavior** [TRAINING_ONLY - verify]: Ctrl+Tab is intercepted at the OS message pump level before the JS event listener receives it in Chrome, Firefox, and Safari. No JS workaround exists.

---

## Open Questions

1. **What shortcut replaces Ctrl+Tab?** Ctrl+[ and Ctrl+] are the strongest candidates. Needs a decision before implementation.

2. **Should `/oneoff` switch the session type globally (mutating the creation panel radio state), or produce a transient badge that only applies on submit?**

3. **How does `>` separator interact with the `lastSuggestedNameRef` guard?** If the user typed `my feature > do something` and previously manually edited the session name field, the guard would prevent auto-fill. The `>` separator parse must decide whether to bypass or respect the guard.

4. **What is the slug truncation limit?** Recommend 50-char soft limit with 100-char hard cap.

5. **Should `SlashCommandDetector` return a new `InputType.SlashCommand` enum value, or mutate mode state directly via `dispatchMode`?**

---

## Recommendation

Do not implement Ctrl+Tab. Use Tab (gated on `!isDropdownVisible`) as the session-type cycling shortcut, with a labeled hint in the creation panel footer.

Gate Tab-for-session-type-cycling on `!isDropdownVisible`. The existing `isDropdownVisible` boolean in `Omnibar.tsx` is the correct insertion point.

Register `SlashCommandDetector` at priority 5 in `createDefaultRegistry()`. Match only the exact known command strings. Return null for all other `/`-prefixed input.

Gate `>` separator parsing on `detection.type === InputType.SessionSearch` (bare text only).

Implement slug normalization as a pure function. Return `""` for degenerate inputs and surface an inline validation error rather than substituting `"session"`.

---

## Pending Web Searches

1. `site:chromium.org ctrlTab keydown preventDefault browser` — verify whether any modern Chromium version delivers Ctrl+Tab to JS keydown listeners
2. `"ctrl+tab" site:bugzilla.mozilla.org javascript keydown` — same for Firefox
3. `Linear command palette slash commands keyboard UX 2026` — verify Linear's exact slash-command behavior
4. `Raycast keyboard mode cycling shortcut design` — verify Raycast's session-type cycling UX pattern
5. `React controlled input IME compositionend onChange CJK` — confirm React's IME handling for CJK slug generation
