# Research 02 — UX Patterns for Rule Builders

## 1. Tag-Input UI Patterns

### How it Works
A tag input presents entered values as dismissible "chips" or "pills" inside the input field. The user types a value, presses **Enter** or **comma** to commit it as a chip, and presses **Backspace** on an empty input to delete the last chip.

### Best Practices for Keyboard-First Entry
- **Enter or comma to add**: Both triggers should work; comma is conventional for comma-separated lists (programs, flags).
- **Backspace-to-delete last**: When the text input is empty and Backspace is pressed, delete the most recently added chip — avoids requiring mouse click to remove.
- **Click-chip-X to delete**: Each chip has an `×` button for mouse users.
- **Duplicate prevention**: Silently de-duplicate on add (same behavior as GitHub label picker).
- **Empty-string prevention**: Trim whitespace; do not add empty strings.
- **Visual focus state**: The chip-container border should pulse/highlight when the inner input is focused, making the whole unit feel like a single input field.
- **Placeholder text**: Show descriptive placeholder in the empty state: "e.g. git, python3" for Programs; "push, commit" for Subcommands.
- **Paste handling**: Splitting comma-separated paste (`git,python3`) into multiple chips on paste events improves power-user speed.

### Recommendation for Rule Builder
Each `CommandCriteria` array field (Programs, Subcommands, BlockedSubcommands, RequiredFlags, ForbiddenFlags) should be a dedicated `<TagInput>` component. Trigger: Enter or comma. Delete: Backspace on empty or click-×. No additional libraries needed — build custom with vanilla-extract recipe for chip styling.

---

## 2. Visual Query Builders (AND/OR Logic, Field Variations, Validation)

### How They Work
Mature rule builders (GitHub Actions `if:`, Datadog monitors, Zapier filters, Retool query builders) share a common visual grammar:

- **Row = one condition**: Each row has [field selector] [operator selector] [value input].
- **AND/OR grouping**: A "+" button adds a new row at the same level; a "Group" button wraps rows in a parenthesized sub-group. Visual indentation communicates nesting.
- **Field-type-aware inputs**: Selecting a boolean field hides the value input; selecting an enum field replaces free text with a dropdown; selecting a list field shows a tag input.
- **Inline validation**: Red border + message appears immediately when a regex is invalid or a required field is empty.
- **Live operator descriptions**: The operator dropdown label is written in plain language: "matches any of", "does not include", "starts with".

### For the Rule Builder
The `CommandCriteria` fields have fixed semantics (AND between fields, OR within each array). There is no need for a general-purpose AND/OR tree builder. Instead, adopt a **fixed-field form** where each criteria field appears as a labeled section with its own tag input or toggle. This is simpler than a row-based query builder and better matches the known, finite schema.

The one exception is `Decision`: the three options (Auto-Allow, Auto-Deny, Escalate) can be shown as large segmented-button/radio controls at the top of the form for visual prominence.

### Validation Pattern
Validate on `blur` rather than on every keystroke. Exception: regex fields should validate in real time (200 ms debounce) so users see errors while typing. Error messages should be specific: "Invalid regex: unterminated `[`" rather than "Invalid input".

---

## 3. Progressive Disclosure: Simple vs Advanced/Regex Mode

### The Pattern
Progressive disclosure hides complexity until the user signals they need it. It is used by GitHub Actions (basic event filter → full expression), Stripe (webhook filter presets → custom regex), and Netlify (simple redirect → advanced rules).

**Implementation approaches:**
1. **Mode toggle** (Simple / Advanced): A segmented control or link at the top of the form switches between two views. Simple shows structured fields; Advanced shows a single regex textarea. The mode is remembered for the session.
2. **Accordion expansion**: Advanced options are hidden under a "Show advanced options ▾" disclosure triangle that expands inline.
3. **Escape hatch link**: Simple mode shows a "Switch to regex mode" link. The reverse link ("Use structured builder") appears in regex mode.

### When to Use Each
- **Mode toggle**: Best when the two modes use entirely different inputs (structured fields vs raw regex). This is the case here because a user who wants regex probably wants only that, not a hybrid form.
- **Accordion**: Best for optional supplementary fields that extend the main form (e.g., `RequiredFlagPrefixes` which is rarely needed).

### Recommendation
Use a **two-segment mode control** at the top of the rule builder: "Structured" (default) | "Regex". In Structured mode, show all `CommandCriteria` fields. In Regex mode, show only the `commandPattern` textarea and the `toolName`/`toolPattern` fields. The two modes write to mutually exclusive proto fields — the builder should clear the other mode's fields on switch (with a confirmation if fields are filled).

The Python-mode checkboxes within Structured mode are a further layer of progressive disclosure: they appear only when the Programs tag input contains a recognized Python interpreter name.

---

## 4. Live Preview Patterns

### Reference Implementations
- **regex101.com**: As you type a pattern, matched text lights up instantly in the test string panel. Match details appear in the sidebar.
- **Stripe webhook filter**: Shows a count badge "3 events match" that updates as you refine the filter.
- **GitHub Codespaces port forwarding**: Shows "This will match requests to..." as you fill in path patterns.

### Key Design Decisions
1. **Client-side only**: Network round-trips add latency and require a running server. For the preview, a TypeScript port of `CommandCriteria.Matches()` runs in the browser. The logic is simple enough to port: array prefix matching, exact token matching, subcommand extraction.
2. **Two lists**: "Would match" (3–5 green examples) and "Would not match" (2–3 red boundary examples). Static example banks are pre-defined per Programs value (e.g. if Programs contains "git", draw from a git example bank).
3. **Debounce at 200 ms**: Avoid re-running on every keystroke.
4. **Empty state**: When no criteria are set, show placeholder text: "Add criteria above to see matching examples."
5. **Placement**: Below the criteria fields, above the Submit button — visually part of the form flow.

### Recommendation
Implement a `useRulePreview(criteria: CommandCriteria): PreviewResult` hook that:
- Takes the current form state.
- Runs against a hardcoded set of `~30 example commands` per known program (git, python3, npm, docker, etc.).
- Returns `matches[]` and `nonMatches[]` for display.
- Is called via a `useDeferredValue` or `useTransition` to keep the UI responsive.

The TS port only needs to implement: `matchesProgram`, `extractSubcommand`, `Subcommands allow-list`, `BlockedSubcommands deny-list`, `RequiredFlags exact match`, `ForbiddenFlags exact match`. Skip `SafePythonImportsOnly` and `RedirectionPattern` in the preview — they require deeper AST analysis.

---

## 5. Template/Preset Libraries

### Presentation Patterns
- **Card grid** (Vercel, Railway, Netlify): 3–4 columns of cards, each with an icon, title, and one-line description. A search box filters cards. Clicking a card pre-fills the form and closes the dialog.
- **Categorized list** (GitHub Actions marketplace): Categories in a left sidebar; templates on the right. Good for large libraries (20+ templates); overkill for 8–12 templates.
- **Dropdown with preview** (Datadog monitors): A `<select>` dropdown; selecting an entry shows a preview panel before confirming. Minimal chrome but poor discoverability.

### When to Use Each
For 8–12 templates (the requirements scope), a **card grid** inside a Radix Dialog is ideal. Cards are scannable; no sidebar overhead needed. A search box becomes useful at 15+ templates.

### Card Design
Each template card should show:
- **Icon** (emoji or simple SVG): language/tool icon (python logo, git branch icon, docker whale, etc.)
- **Title**: "Python script/module" — short noun phrase
- **Description**: One sentence: "Allow python3 running a .py file or -m module. Does not match inline -c code."
- **Decision badge**: Auto-Allow / Escalate / Auto-Deny badge (same design as the table)

### Template Data Structure
```typescript
interface RuleTemplate {
  id: string;
  title: string;
  description: string;
  icon: string; // emoji or SVG path
  decision: AutoDecision;
  riskLevel: string;
  criteria: Partial<CommandCriteria>;
  // Pre-fills reason/alternative too
  reason?: string;
  alternative?: string;
  tags?: string[]; // for search
}
```

Templates are defined as a static array in the frontend — no network request needed. This matches the requirements' constraint that the feature works without backend changes for US-1–US-5.

### Recommendation
Open the template picker via a Radix `<Dialog>`. Render templates as a 3-column card grid. On select: close dialog, pre-fill form fields (switch to Structured mode), allow user to edit. Add a "Start from scratch" option at the bottom of the grid to dismiss without pre-filling.

---

## Summary Recommendations

| Pattern | Recommended Approach |
|---------|---------------------|
| Tag input | Custom `<TagInput>` component, Enter/comma to add, Backspace to delete last |
| Query builder | Fixed-field form (not row-based), AND between fields, OR within arrays |
| Progressive disclosure | Two-segment mode toggle: "Structured" / "Regex" |
| Python-mode reveal | Conditional section appearing when Programs contains a Python interpreter |
| Live preview | Client-side TS port of `CommandCriteria.Matches()`, 200 ms debounce, example bank |
| Template picker | Radix Dialog, 3-column card grid, static data, no network request |
