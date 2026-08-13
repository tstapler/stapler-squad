# ADR-001: Omnibar Inline Shorthand Language

Status: Accepted
Date: 2026-05-05
Deciders: Tyler Stapler

---

## Context

The Stapler Squad omnibar is a keyboard-first launcher for creating and
navigating AI agent sessions. As the omnibar gains more creation modes and
configuration options (session type, branch, first prompt, program), there is
pressure to surface these options without adding form fields that increase visual
complexity and require mouse interaction.

Comparable tools have solved this with inline shorthand syntax:

- **Todoist** uses `>` to denote projects and `:` for labels inline in the task
  name field. Users learn one input surface rather than two.
- **Linear** uses `!priority` and `@assignee` prefixes inline in issue creation.
- **Raycast** uses space-separated command extensions on its command bar.

The principle behind these designs: the primary input field is the expert
interface. Modal pickers and form fields are the discoverable fallback. Users
who invest in learning the inline syntax get zero mouse friction.

The question this ADR answers: what shorthand language should the Stapler Squad
omnibar speak, and how should it be architecturally wired so that future
additions are low-cost and low-risk?

---

## Decision

The omnibar will support a minimal inline shorthand language with two
constructs:

### Construct 1: `>` Separator

```
<session name> > <first prompt>
```

The `>` character (first occurrence only) splits the input into a session name
part and a first-prompt part. This is only active when the detection type is
`SessionSearch` (free-text input, not a path or GitHub URL). For all other
detection types, `>` is treated as a literal character in the input.

Implementation: a pure function `parseInputWithSeparator(input)` that returns
`{ name, prompt }`. Called at `canSubmit` evaluation and at `handleSubmit` —
never as a Detector.

### Construct 2: Slash-Command Prefix

```
/<command> <remainder>
```

A leading slash followed by a known keyword switches the active session type
and strips itself from the input before detection. The remainder is passed to
the normal detection pipeline.

Known commands are defined in a single allowlist map:

```ts
const KNOWN_SLASH_COMMANDS: Record<string, SessionType> = {
  "/oneoff":    "one_off",
  "/one-off":   "one_off",
  "/worktree":  "new_worktree",
  "/dir":       "directory",
  "/directory": "directory",
  "/existing":  "existing_worktree",
};
```

Unknown slash prefixes (e.g. `/foobar`) are passed through unchanged. The
allowlist is the sole extension point — adding a new command requires editing
only this map.

Implementation: a pure function `parseSlashCommand(input)` that returns
`{ matched, sessionType, remainder }`. Called before `detect()` in the 150ms
debounce effect — never as a Detector.

### Extension Contract

To add a new slash command in the future:

1. Add one entry to `KNOWN_SLASH_COMMANDS` in
   `web-app/src/lib/omnibar/parseSlashCommand.ts`.
2. Add test cases for the new prefix to
   `web-app/src/lib/omnibar/parseSlashCommand.test.ts`.
3. No other files change.

Slash commands that alias an **existing** session type do not trigger the
7-touchpoint session-creation-registry checklist. They simply set the session
type in `OmnibarFormState`. If a slash command needs to enable a **new** session
type that does not exist yet, that new type must go through the full 7-touchpoint
checklist independently — the slash command is added after the type exists.

To add a new separator construct (e.g. `@category`):

1. Decide whether it is gated on detection type (like `>`) or unconditional.
2. Add a new pure function in `web-app/src/lib/omnibar/parseInput.ts` or a
   new file if the logic is substantially different.
3. Wire it at the debounce effect level, not in the Detector layer.
4. Document it in this ADR under "Construct N".

---

## Consequences

### Positive

- **Zero form fields added.** Both constructs are parsed from the primary input
  field. The creation panel remains as compact as it is today.
- **Low coupling.** Both constructs are pure functions with no external
  dependencies. They are easy to test in isolation and easy to delete or modify.
- **Discovery via the separator.** When the `>` separator is detected, the UI
  auto-expands the firstPrompt textarea and shows a split preview below the
  input. Users discover the shorthand through its effect, not through
  documentation.
- **LocalPath collision resolved.** By pre-processing slash commands before
  `detect()`, the `LocalPathDetector` (which fires on any leading `/`) never
  sees a valid slash command. No change to `LocalPathDetector` is required.
- **Minimal footprint.** Two new files (`slugify.ts`, `parseSlashCommand.ts`),
  one extended file (`parseInput.ts`), and a one-line change in `detector.ts`.

### Negative / Trade-offs

- **`>` is not universal.** The `>` separator is active only for `SessionSearch`
  detection. Users who type a local path (e.g. `~/code/myapp`) cannot use `>`
  to attach a first prompt inline — they must use the textarea. This is
  acceptable because the `>` character appears legitimately in path contexts.
- **Slash commands are not discoverable without documentation.** Unlike the `>`
  separator (which shows a visual split preview), slash commands have no inline
  affordance. Users must know the commands exist. This is consistent with how
  power-user shortcuts work in Todoist and Linear.
- **The allowlist is hard-coded.** Commands cannot be registered dynamically.
  This is a deliberate choice — the allowlist is the contract, and unknown
  slashes fall through unchanged rather than failing silently.

---

## Alternatives Rejected

### Alternative A: New `Detector` classes for slash commands

The initial research considered adding a `SlashCommandDetector` to the
`DetectorRegistry`. This was rejected for two reasons:

1. Detectors return a `DetectionResult` with a `type` field — but a slash
   command does not create a new detection type, it mutates the session type in
   `OmnibarFormState`. The Detector architecture has no mechanism to mutate form
   state as a side effect of detection.
2. A `SlashCommandDetector` at any priority would require coordination with
   `LocalPathDetector` (priority 100), which also fires on leading `/`. The
   priority ordering would need to encode slash-command awareness, coupling the
   two detectors.

Pre-processing before `detect()` is simpler and has no coupling to the registry.

### Alternative B: Ctrl+Tab for session type cycling

Browser-level key interception prevents `Ctrl+Tab` from being reliably
`preventDefault`-ed in Chrome and Firefox. The shortcut fires a tab-switch
action at the browser chrome level before any JavaScript keydown handler runs.
Testing in Chrome confirmed this. Tab (without modifier) is used instead,
gated on `!isDropdownVisible` to avoid conflicts with the path-completion
dropdown.

### Alternative C: Separate form fields for all omnibar configuration

Adding explicit dropdowns or pickers for session type and first prompt inside
the omnibar modal was rejected on the grounds of visual complexity. The omnibar
is designed to be a single-input surface. Form fields work against this design
and increase the number of interaction steps for experienced users. The
expandable firstPrompt textarea (accessed via disclosure triangle or `>`
shorthand) is the minimum viable escape hatch for users who prefer the
visual approach.

### Alternative D: Parsing the `>` separator in the Detector layer

An alternative was to have `SessionSearchDetector` split on `>` and return
both `suggestedName` (from the name part) and a new `suggestedPrompt` field in
`DetectionResult`. This was rejected because `DetectionResult` is a stable
interface shared across all detectors, and extending it for a feature specific
to `SessionSearchDetector` would add noise for all callers. The pure function
approach keeps the parsing isolated to the consumption site (Omnibar.tsx).
