# Research Synthesis: Omnibar Session Name UX

## Decision Required

How to implement smart session name auto-population, a first-prompt textarea, an inline separator shorthand (`>`), slash-command session type switching, and Tab-based type cycling in the Stapler Squad omnibar — with each feature integrated into the existing detection pipeline, form state, and RPC layer.

## Context

The omnibar currently auto-fills session names only for detected paths/URLs (via `suggestedName` in `DetectionResult`). When a user types plain text (e.g., `implement oauth`), the `SessionSearchDetector` returns `suggestedName: ""` and the session name field stays blank. When switching to one-shot mode, the user must re-type the name they already wrote.

The `initial_prompt` field (#15) already exists in `CreateSessionRequest` proto and is threaded through the full server stack, but the omnibar UI path is missing: no `firstPrompt` in `OmnibarFormState`, no textarea in `OmnibarCreationPanel`, no forwarding in `OmnibarContext` or `useSessionService`.

Three features are entirely new: the `>` inline separator, `/command` prefix switching, and Tab cycling for session types.

Web search confirmed: **Ctrl+Tab cannot be delivered to JavaScript in Chrome** (browser reserves it for tab switching). This is the single hardest constraint.

## Options Considered

| Feature | Option A | Option B | Option C |
|---|---|---|---|
| Slug source | npm slugify | @sindresorhus/slugify | **Inline pure function** |
| Slug hook | New `useEffect` | **SessionSearchDetector returns suggestedName** | Preview-only state |
| `>` separator parse | onChange live split | **Derive on read (pure function, gated)** | Submit only |
| Slash command routing | New Detector + InputType | **Pre-process before detect()** | Panel interceptor |
| Keyboard cycling | Ctrl+Tab (abandoned) | **Tab (gated on !isDropdownVisible)** | Ctrl+[ / Ctrl+] |
| ADR format | **Single ADR** | Two ADRs | Rules file update only |

## Dominant Trade-off

**Parsing complexity vs. state simplicity.** Option A approaches (live split, new Detector, new InputType enum) provide immediate responsiveness but introduce bidirectional state management and deep coupling into the detection architecture. Option B approaches (pure functions, pre-processing, derive on read) are slightly less "live" but produce zero new state, remain trivially testable, and respect the existing separation between detection and form state.

All recommendations land on Option B — defer computation to the narrowest point of use.

## Recommendation

### 1. Slug generation — Inline `toSessionSlug` pure function

**Choose: Custom inline function in `web-app/src/lib/omnibar/slugify.ts`**

```ts
export function toSessionSlug(input: string): string {
  return input
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, "")
    .replace(/\s+/g, "-")
    .replace(/-{2,}/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 50);
}
```

Zero bundle impact. Consistent with existing codebase style. Return `""` for degenerate inputs (emoji-only, all-specials) — never substitute a literal like `"session"`. Show an inline error when the slug is empty and submit is blocked.

**Hook into: `SessionSearchDetector.detect()`** — single line change: `suggestedName: toSessionSlug(trimmed)`. The existing `lastSuggestedNameRef` guard in `Omnibar.tsx` handles manual-edit protection automatically.

Accept these costs: no Unicode transliteration (emoji, CJK produce empty slug). This is acceptable — session names are ASCII-dominant in practice; degenerate inputs show an error.

### 2. First prompt — Wire `initialPrompt` through the omnibar path

**Choose: Follow the SessionWizard pattern exactly**

4 precise touchpoints:
1. Add `firstPrompt: string` to `OmnibarFormState` and `INITIAL_FORM_STATE`
2. Add `firstPrompt?: string` to `OmnibarSessionData`
3. Add optional expandable textarea in `OmnibarCreationPanel` below session name (main form body, above Advanced Options)
4. Patch `handleSubmit` — **critical**: current line produces silent data loss if not updated: `finalPrompt = [formState.firstPrompt?.trim(), ...imagePaths].filter(Boolean).join("\n") || undefined`

Then thread through `OmnibarContext` (`initialPrompt: data.firstPrompt`) and `useSessionService` (`initialPrompt: request.initialPrompt`).

**Do not confuse `prompt` (image paths, field 7) with `initialPrompt` (text injection, field 15).** They are distinct proto fields.

Accept these costs: character limit TBD (recommend 2,000 for quick-create context vs. SessionWizard's 10,000).

### 3. `>` inline separator — Derive on read, gated on SessionSearch

**Choose: Pure function `parseInputWithSeparator`, called at canSubmit + handleSubmit + preview label**

```ts
function parseInputWithSeparator(s: string): { name: string; firstPrompt: string } {
  const idx = s.indexOf(">");
  if (idx === -1) return { name: s, firstPrompt: "" };
  return { name: s.slice(0, idx).trim(), firstPrompt: s.slice(idx + 1).trim() };
}
```

**Gate on `detection?.type === InputType.SessionSearch` only.** Path inputs and URL inputs never enter SessionSearch type — the separator cannot fire on `/foo/bar` or `https://...`.

Split on first `>` only. `a > b > c` → name: `a`, firstPrompt: `b > c`.

**Visual feedback**: When input contains `>` and type is SessionSearch, show a preview label: `"Session name: my-feature"` below the input. This is the critical discoverability hook — without it, the separator is invisible.

Accept these costs: users who want `>` in a session name cannot use it. This is documented in the ADR.

### 4. Slash command prefix — Pre-process before DetectorRegistry

**Choose: `parseSlashCommand()` pure function, called at the top of the 150ms debounce `useEffect` before `detect()`**

```ts
const KNOWN_SLASH_COMMANDS: Record<string, SessionTypeValue> = {
  oneoff: "one_off",
  worktree: "new_worktree",
  dir: "directory",
  existing: "existing_worktree",
};

function parseSlashCommand(input: string): { sessionType: SessionTypeValue; remainder: string } | null {
  const match = /^\/([a-z]+)(?:\s+(.*))?$/i.exec(input.trim());
  if (!match) return null;
  const cmd = match[1].toLowerCase();
  if (!KNOWN_SLASH_COMMANDS[cmd]) return null;
  return { sessionType: KNOWN_SLASH_COMMANDS[cmd], remainder: match[2]?.trim() ?? "" };
}
```

In the debounce `useEffect`: call `parseSlashCommand(input)` first. If matched, call `setFormField("sessionType", result.sessionType)`, then call `detect(result.remainder)` on the remainder. Do NOT call `detect()` on the full input including the slash command.

**Why not a new Detector**: A `SlashCommandDetector` in `DetectorRegistry` would need to communicate `sessionType` back to `OmnibarFormState` — a separate state tree from `DetectionResult`. Pre-processing keeps the coupling minimal: one pure function, one state mutation, one pass to `detect()`.

Accept these costs: slash commands are not discoverable without in-UI hints. A ghost-text placeholder in the creation panel (e.g., "Type /oneoff to switch to one-off mode") is the recommended minimum.

### 5. Tab cycling — Gated on `!isDropdownVisible`

**Choose: Tab key in the omnibar input, conditional on `!isDropdownVisible`**

```ts
// In Omnibar.tsx handleKeyDown, inside creation mode branch:
if (e.key === "Tab" && !isDropdownVisible && modeState.type !== "discovery") {
  e.preventDefault();
  const types = SESSION_TYPES.map(t => t.value);
  const idx = types.indexOf(sessionType);
  const next = types[(idx + 1) % types.length];
  setFormField("sessionType", next);
}
```

**Do not implement Ctrl+Tab.** Confirmed by web search: Chrome does not deliver Ctrl+Tab to JavaScript keydown listeners — the browser intercepts it for tab switching before the event reaches the page. No workaround exists.

Show a hint in the creation panel footer: `Tab: cycle type`. This is the minimum discoverability for a power-user shortcut.

Accept these costs: Tab cannot be used for form field navigation while creation mode is open (it cycles session types instead). This is acceptable — Tab is already non-standard in this modal context.

### 6. ADR

Write `ADR-NNN: Omnibar Inline Shorthand Language` as a single ADR. Required sections:
- Context (why the omnibar is becoming a command surface, not just a search field)
- Decision (adopt `>` separator and `/command` prefix as the omnibar's inline command language)
- Consequences (discoverability requirements, known limitations)
- Extension Contract (how to add new slash commands: add to `KNOWN_SLASH_COMMANDS`, add hint text, add test)
- Alternatives Rejected (multiple input fields, separate mode selector, Ctrl+Tab)

The Extension Contract section is the critical piece — it prevents future ad-hoc `if (input.startsWith("/"))` proliferation.

## Implementation Order (critical path)

1. **`toSessionSlug` + SessionSearchDetector** — unblocks everything (name auto-fill fixes the core UX bug; slug function is reused by `>` separator)
2. **`firstPrompt` threading** — independent; wire proto field through to textarea
3. **`>` separator (derive on read)** — depends on step 1 (slug function) and step 2 (firstPrompt in formState)
4. **Tab cycling** — independent; pure event handler addition
5. **`/command` prefix pre-processing** — most complex; implement last; depends on slug function

## Open Questions Before Committing

- [ ] What character limit for the first-prompt textarea? (2,000 recommended; needs decision)
- [ ] Should `>` separator apply in one-off mode (where input == session name, not a path)? Likely yes — needs UX decision
- [ ] What replaces Ctrl+Tab in the shortcut legend? Tab (gated) is the answer, but the footer hint text needs finalization

These are product decisions, not blockers — all have clear defaults that can be confirmed during implementation.

## Sources

- [findings-stack.md](./findings-stack.md) — slug generation options, `initialPrompt` threading gap analysis
- [findings-features.md](./findings-features.md) — Todoist/Raycast/Linear/Slack prior art, separator and slash-command UX
- [findings-architecture.md](./findings-architecture.md) — 4 architecture decisions with trade-off matrices and code-grounded analysis
- [findings-pitfalls.md](./findings-pitfalls.md) — Ctrl+Tab browser constraint (confirmed), edge cases, priority conflicts
- Web search: [Todoist natural language UX](https://thesweetsetup.com/using-natural-language-with-todoist/) — confirms inline parsing + real-time feedback as the key UX principle
- Web search: [Chrome Ctrl+Tab behavior](https://www.robin-drexler.com/2015/07/07/overriding-default-browser-shortcuts) — confirms browser-level interception; confirmed by [Chromium bug](https://bugzilla.mozilla.org/show_bug.cgi?id=1052569)
- Web search: [Command palette prior art](https://destiner.io/blog/post/designing-a-command-palette/) — confirms `/` prefix as established convention
