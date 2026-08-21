# Findings: Architecture

## Summary

Four architectural decisions shape this feature. Each has been analyzed against the actual codebase
topology: Omnibar.tsx, OmnibarCreationPanel.tsx, detector.ts, useModeReducer.ts, and the
OmnibarContext/useSessionService pipeline.

The central theme: all four features sit at the boundary between the detection pipeline (150ms
debounce, returns DetectionResult) and the React form state (synchronous, drives UI). Decisions
made here govern UX responsiveness, testability, and future extensibility of the omnibar as a
command surface.

Key grounding observations from code:
- Detection fires at 150ms debounce in a `useEffect` on `input` (Omnibar.tsx lines 329-384).
- Auto-fill of session name is gated on `result.suggestedName` (lines 361-366); if SessionSearch
  returns `suggestedName: ""` (detector.ts line 300), the fill never happens — this is the exact
  gap to close for Q3.
- Slash-command prefix detection (`new/` shorthand) lives in `NewSessionDetector` at priority 35
  and feeds a `new_prefix_typed` mode action — this is the established pattern for prefix commands.
- `OmnibarFormState` has no `firstPrompt`; `OmnibarSessionData.prompt` is defined; OmnibarContext
  already passes `prompt: data.prompt` to the RPC. The field exists at every layer except the form
  state interface and the panel UI.
- `handleSubmit` (Omnibar.tsx lines 639-641) builds `finalPrompt` from attached images only.
  Adding `firstPrompt` to OmnibarFormState without updating this produces silent data loss.

---

## Options Surveyed

### Q1: Where should inline `>` separator parsing happen?

**Option A: onChange handler (live split)**
Parse on every keystroke. As soon as the user types `>`, split the input string in place:
left side → `sessionName`, right side → `firstPrompt`. The main input field then holds only
the path/search portion; `firstPrompt` is populated immediately.

Implications: The `>` character is consumed from the primary input field. Reversal (backspacing
past `>`) requires detecting that `firstPrompt` is empty and recombining — adds state machine
complexity. Path detection (`/foo/bar > start coding`) must receive only the path-portion, which
requires the split to already have happened before detection runs.

**Option B: Derived value for display, split on submit (compute-on-read)**
Store the raw unsplit string in `input`. Compute `parsedName` and `parsedPrompt` as derived
values at the point of use: in `canSubmit`, `handleSubmit`, and the session name preview label.
Detection pipeline receives the full `my-name > first prompt` string — no path/URL pattern contains
`>`, so it falls through to SessionSearch, which then triggers the bare-text auto-fill path
described in Q3. The parse is a pure function `parseInputWithSeparator(input)`.

Implications: `input` is the single source of truth. No state recombination needed on backspace.
The pure function is trivially testable. Visual feedback can be provided by showing a parsed
preview in the session name field label. Clean separation between parse-time and effect-time.

**Option C: Submit handler only**
No live feedback. Parse in `handleSubmit` just before calling `onCreateSession`.

Implications: User cannot verify the split before committing. Fails progressive disclosure UX
principle for power syntax. Acceptable only for an MVP, not a finished feature.

---

### Q2: Where should slash-command prefix parsing happen?

**Option A: New Detector in DetectorRegistry (priority < 10)**
Register a `SlashCommandDetector` at priority 5. Matches `/oneoff ...`, `/worktree ...`.
Returns a DetectionResult with a new `InputType.SlashCommand` variant carrying `commandName`
and `remainder` in `metadata`.

Implications: Consistent with how `NewSessionDetector` handles `new/` prefix at priority 35.
Fully testable in isolation. BUT: creates a coupling problem. The modeReducer currently
cannot set `sessionType` in formState — it only controls display mode. A SlashCommand
InputType would need to somehow communicate `sessionType = "one_off"` to `OmnibarFormState`,
which lives outside the detection pipeline. This requires either (a) a new action type in
modeReducer that also carries a formState patch, or (b) a side-effect in the detection
useEffect that reads `result.metadata.command` and calls `setFormField`. Option (b) is the
path of least resistance but muddles the responsibilities of the detection useEffect.

Critically: the `LocalPathDetector` at priority 100 accepts any string starting with `/`
that contains multiple slashes. `/oneoff hello world` has only one slash so would not match
LocalPathDetector — but a priority-5 `SlashCommandDetector` with an open-ended pattern like
`/\w+` would also match `/tmp/myproject`, stealing it from LocalPathDetector. This requires
tight allowlisting in the detector.

**Option B: Pre-processing step before DetectorRegistry (input preprocessing)**
In the 150ms debounce `useEffect`, before calling `detect(input)`, check if input starts
with `/` followed by a known command keyword. If matched, call `setFormField("sessionType", ...)`
immediately, strip the command prefix, and pass the remainder to `detect()`.

Implications: Synchronous — no debounce lag on the type switch. No new InputType enum needed.
Cannot be tested via DetectorRegistry unit tests, but testable as a pure `parseSlashCommand()`
function. Has one explicit branching point rather than requiring modeReducer/formState
coordination with a new InputType. More direct for this use case: slash commands mutate
form state, not detection type.

**Option C: OmnibarCreationPanel interceptor**
Parse in the creation panel. Rejected: the panel only receives `formState`, not `input`.
Threading raw input down as a prop violates the existing boundary.

---

### Q3: Where should bare-text → kebab slug auto-fill happen?

**Option A: SessionSearchDetector returns suggestedName**
Change `SessionSearchDetector.detect()` to compute a slug:
`suggestedName: toKebabSlug(trimmed)` instead of `suggestedName: ""`.
The auto-fill logic in Omnibar.tsx lines 361-366 fires because `result.suggestedName` is now
non-empty. The existing `lastSuggestedNameRef` guard (line 362-365) prevents overwriting a
manually edited name. Zero new state, one-line change in detector.ts.

Implications: Consistent with how every other detector supplies `suggestedName`. The guard
(`sessionName === ""` OR `sessionName === lastSuggestedNameRef.current`) correctly skips
auto-fill if the user already typed a name. This is the minimal change with maximum leverage.

**Option B: Separate useEffect on input**
Add a new `useEffect` watching `input` and detection type; if type is SessionSearch, compute
slug and call `setSessionName`.

Implications: Duplicates the manual-edit guard that already exists. More surface area, more
tests. No benefit over Option A.

**Option C: Preview-only state, commit on Tab/Enter**
Show computed slug as a greyed placeholder in the session name field. Commit only on Tab or
on typing something different.

Implications: Requires "suggested but uncommitted" vs "user typed" distinction in state.
The existing `lastSuggestedNameRef` already provides this distinction implicitly via Option A.
Option C adds complexity for the same UX outcome.

---

### Q4: ADR structure for inline-shorthand concept

**Option A: Single ADR covering the full inline shorthand concept**
Title: "ADR-NNN: Omnibar Inline Shorthand Language"
Covers both the `>` separator and `/command` prefix as a unified "command language" design
decision. Sections: Context, Decision, Consequences, Alternatives Rejected, Extension Contract
(how to add new slash commands). This documents the design intent and prevents ad-hoc extension.

**Option B: Separate ADRs per feature**
One ADR for `>` separator parse location, one for slash command routing. Granular but risks
over-formalization for what are essentially two implementation choices within one UX feature.

**Option C: No ADR — extend existing registry rules**
Update `.claude/rules/session-creation-registry.md` and `.claude/rules/feature-testing-registry.md`
with inline shorthand conventions. Skips the ADR format entirely.

---

## Trade-off Matrix

| Decision | Option | Responsiveness | Testability | Code Surface | State Complexity | Safe Extension |
|---|---|---|---|---|---|---|
| Q1 `>` separator | A: onChange split | Instant | Medium | Medium | High (recombine) | Low |
| Q1 `>` separator | **B: derive on read** | Instant (computed) | High (pure fn) | Low | None | High |
| Q1 `>` separator | C: submit only | None | High | Low | None | N/A |
| Q2 slash command | A: Detector + InputType | 150ms lag | High | Medium | High (coupling) | Low |
| Q2 slash command | **B: Pre-processing** | Instant | High (pure fn) | Low | None | Medium |
| Q2 slash command | C: Panel interceptor | Instant | Low | Medium | High | Low |
| Q3 bare-text slug | **A: SessionSearchDetector** | 150ms lag | High | Minimal | None | High |
| Q3 bare-text slug | B: Separate useEffect | 150ms lag | Medium | Medium | Low | Medium |
| Q3 bare-text slug | C: Preview-only state | Instant | High | High | High | Medium |
| Q4 ADR | **A: Single ADR** | N/A | N/A | N/A | N/A | High |
| Q4 ADR | B: Separate ADRs | N/A | N/A | N/A | N/A | Medium |
| Q4 ADR | C: No ADR | N/A | N/A | N/A | N/A | Low |

---

## Risk and Failure Modes

**Q1 — `>` in non-bare-text inputs**
Paths never contain `>` (shell would expand it). GitHub URLs don't contain `>`. However,
pasted content (git commit messages, bash heredocs) might. Mitigation (Option B): only apply
the separator parse when `detection?.type === InputType.SessionSearch`. Path and URL inputs
are never in SessionSearch type — the parse is gated by detection type, not by character presence.

**Q2 — slash command collides with local paths**
`/tmp/foo` is a valid local path. A greedy SlashCommandDetector at priority 5 would intercept
it. Mitigation for both options: require known command keywords in an explicit allowlist
(`KNOWN_SLASH_COMMANDS = ["oneoff", "worktree", "directory"]`). Never match `/tmp`, `/usr`,
`/home`, etc. Check `input.startsWith("/" + knownCommand + " ")` or
`input.startsWith("/" + knownCommand + "\n")` — require a space or end-of-input after the keyword.

**Q3 — slug update races with manual editing**
The existing `lastSuggestedNameRef` guard (Omnibar.tsx:362-365) handles this: if
`sessionName !== lastSuggestedNameRef.current`, auto-fill is skipped. A slug from
SessionSearchDetector flows through the same guard. No new race condition introduced.

**firstPrompt threading gap in handleSubmit**
Current code (Omnibar.tsx:639-641): `finalPrompt = imagePaths.length > 0 ? imagePaths.join(" ") : undefined`. Adding `firstPrompt` to OmnibarFormState without patching this line produces a silent
data loss bug. The correct logic: `finalPrompt = [formState.firstPrompt?.trim(), ...imagePaths].filter(Boolean).join("\n") || undefined`.

**ADR without extension contract**
If the shorthand concept is underdocumented, future developers add new slash commands as
ad-hoc `if (input.startsWith("/"))` checks scattered in Omnibar.tsx's onChange handler.
The ADR must specify: "new slash commands are added to `KNOWN_SLASH_COMMANDS` in
`parseSlashCommand.ts`, not as inline conditionals."

---

## Migration and Adoption Cost

**firstPrompt field — 4 precise touchpoints:**
1. `OmnibarFormState` interface (Omnibar.tsx:36-50) — add `firstPrompt: string`
2. `INITIAL_FORM_STATE` (Omnibar.tsx:52-66) — add `firstPrompt: ""`
3. `handleSubmit` (Omnibar.tsx:639-641) — incorporate `formState.firstPrompt` into `finalPrompt`
4. `OmnibarCreationPanel` — add expandable textarea below session name field

These are additive, non-breaking. No proto change needed (`initial_prompt` field 15 already
exists in the proto and is threaded through the full stack).

**Slug auto-fill — 1 touchpoint:**
- `SessionSearchDetector.detect()` — change `suggestedName: ""` to `suggestedName: toKebabSlug(trimmed)`

**`>` separator (Option B) — 3 touchpoints:**
1. New pure function `parseInputWithSeparator(input: string): { name: string; firstPrompt: string }`
2. `canSubmit` — call the function to extract name for validation preview
3. `handleSubmit` — call the function to extract `firstPrompt` (merged with image paths)

**Slash command preprocessing (Option B) — 2 touchpoints:**
1. New pure function `parseSlashCommand(input: string): { command: string; remainder: string } | null`
2. Top of the 150ms debounce `useEffect` in Omnibar.tsx — call `parseSlashCommand`, set
   `formField("sessionType", ...)` if matched, pass remainder to `detect()`

**Registry updates (per `.claude/rules/`):**
- No new RPC method → no `backend-features.json` entry
- No new session creation mode lifecycle → 7-touchpoint checklist NOT triggered for slash
  commands that alias existing modes (one_off, worktree)
- New UI component (firstPrompt textarea) → add entry to `frontend-features.json`

---

## Operational Concerns

**Detection debounce and slug freshness**: Slug updates from SessionSearchDetector fire after
150ms. If the user types fast and submits immediately with Cmd+Enter before debounce fires,
the session name reflects pre-debounce state. This is identical behavior to existing branch/path
auto-fill — not a new regression.

**Expandable textarea height**: The first prompt textarea will sit below the session name field
in an already-constrained modal. Start at 1 row; expand on focus or content overflow.
`field-sizing: content` is the modern CSS approach [TRAINING_ONLY - verify browser support];
fallback is a `rows` adjustment on `onChange` using `scrollHeight`.

**Slash command visual feedback**: When user types `/oneoff `, the session type switches to
`one_off`, the path input disappears (already hidden for one_off mode), and the mode badge
changes. The existing one_off hint ("Directory will be created in your one-off base directory…")
displays correctly as long as `sessionType === "one_off"` is set. No additional feedback logic
needed beyond what already exists.

**`>` separator feedback**: In Option B (derive on read), the user has no live signal that the
split will happen. Add a computed session name preview label: when input contains `>` and
detection type is SessionSearch, show "Session name: `<computed-slug>`" below the input field.
This is additive to the existing detection badge.

---

## Prior Art and Lessons Learned

**`NewSessionDetector` at priority 35**: Already the canonical example of prefix-command
shortcut in this codebase. Matches `new/` prefix, returns `InputType.NewSession` with remainder
in `parsedValue`. The modeReducer dispatches `new_prefix_typed` which sets `creation_with_repo`
mode with the remainder as the query. The key difference for slash commands: `NewSessionDetector`
does not need to set `formState.sessionType` because `creation_with_repo` mode implies a new
worktree. Slash commands like `/oneoff` DO need to set `sessionType` in formState. This is the
coupling gap that makes pre-processing (Q2 Option B) preferable to a new Detector (Q2 Option A).

**`lastSuggestedNameRef` guard pattern**: The existing guard (Omnibar.tsx:169, used at lines
362-365) is a mature solution to the "auto-fill without stomping manual edits" problem. Any
new slug auto-fill must go through this same guard. SessionSearchDetector returning `suggestedName`
gets this protection automatically — no new guard logic needed.

**Image path prompt concatenation**: Current `finalPrompt` is `imagePaths.join(" ")`. Adding
`firstPrompt` must prepend text before image paths. The forward-compatible format:
`[textPrompt, ...imagePaths].filter(Boolean).join("\n")`. This is backward-compatible because
when `firstPrompt` is empty, the result equals the existing behavior.

**`one_off` session mode pattern**: One-off mode already demonstrates the "flag on existing
session type" pattern: `sessionType === "one_off"` in formState maps to `SessionType.DIRECTORY`
in the proto + `oneOff: true` flag. Slash commands that activate existing modes (e.g., `/oneoff`)
simply set `formState.sessionType` to the existing string value — no new proto field or session
type constant needed.

---

## Open Questions

1. **Separator character choice**: Is `>` the right separator? Alternatives: `|`, `//`, `::`.
   `>` has Todoist precedent but evokes shell redirection. `|` evokes Unix pipes. `//` is least
   ambiguous but requires two characters. Decision needed before implementation — it affects the
   pure function signature and the ADR.

2. **Slug character limit**: Should `toKebabSlug` cap output length? Tmux session names and git
   branch names have practical limits. Suggested cap: 40 characters. The ADR should specify this.

3. **Slash command discovery**: No current mechanism shows available slash commands. A lone `/`
   typed by the user could show a contextual command dropdown. Out of scope for initial feature
   but the ADR extension contract should reserve this UX slot.

4. **`>` parse scope**: Should the separator apply only in SessionSearch mode, or also when the
   user has manually switched to one-off mode (where the input is the session name, not a path)?
   In one-off mode, the input field is currently the session name field — the separator might be
   welcome there too. Needs UX decision.

5. **firstPrompt field naming**: `OmnibarFormState` will use `firstPrompt`; `OmnibarSessionData`
   uses `prompt`. The ADR should codify this naming convention to prevent confusion with "AI
   assistant prompts" vs "initial session start prompts."

---

## Recommendation

**Q1 — `>` separator parsing: Option B (derive on read)**

Parse `input` with a pure function `parseInputWithSeparator(s: string): { name: string; firstPrompt: string }`
at the point of use — in `canSubmit`, `handleSubmit`, and in a session name preview label.
Apply only when `detection?.type === InputType.SessionSearch` to prevent false fires on path
inputs. Zero new state. Testable as a pure function. Add a visible preview label when the
separator is detected.

**Q2 — slash command prefix: Option B (pre-processing before DetectorRegistry)**

Add a `parseSlashCommand(input: string)` pure function with an explicit known-command allowlist.
Call at the top of the 150ms debounce `useEffect` in Omnibar.tsx, before `detect(input)`.
If matched, call `setFormField("sessionType", ...)` immediately, pass remainder to `detect()`,
update the mode badge. This avoids the InputType/modeReducer/formState coupling problem and
keeps slash-command logic in one testable, auditable function.

**Q3 — bare-text slug auto-fill: Option A (SessionSearchDetector returns suggestedName)**

Change the single line in `SessionSearchDetector.detect()` from `suggestedName: ""` to
`suggestedName: toKebabSlug(trimmed)`. The existing `lastSuggestedNameRef` guard in Omnibar.tsx
handles manual-edit protection automatically. Minimal change, maximum leverage.

**Q4 — ADR structure: Option A (single ADR)**

Write `ADR-NNN: Omnibar Inline Shorthand Language` covering both the `>` separator and
`/command` prefix as a unified design decision. Required sections: Context, Decision,
Consequences, Extension Contract (exactly how to add new slash commands), Alternatives Rejected.
The extension contract is the most important section — it prevents future ad-hoc proliferation.

---

## Pending Web Searches

1. `field-sizing content CSS property browser support 2026` — verify browser support for
   the auto-expanding textarea approach before recommending it in implementation
2. `inline command language UX patterns omnibar launcher Raycast Linear` — confirm prior
   art for `/command` prefix in production launcher interfaces
3. `tmux session name maximum length limit` — confirm tmux session name character limit to
   inform slug truncation default
