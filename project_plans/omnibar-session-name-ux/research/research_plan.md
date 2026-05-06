# Research Plan: Omnibar Session Name UX

## Context (grounded in codebase as of 2026-05-05)

Key discoveries from codebase survey:
- `initial_prompt` field (#15) already exists in `CreateSessionRequest` proto
- `useSessionService.ts` passes `prompt: request.prompt` to the RPC — pipeline exists
- `OmnibarContext.tsx` passes `prompt: data.prompt` — pipeline exists
- BUT `OmnibarFormState` has no `firstPrompt` field; `OmnibarCreationPanel` has no prompt textarea
- Session name auto-fill only works for detected paths/URLs via `suggestedName`; bare text typed
  as a session name (SessionSearch type) does NOT auto-populate the name field
- `SessionTypeRadioGroup` already supports arrow-key cycling, but only inside the creation panel
- No inline slash-command or separator parsing exists anywhere

## Subtopics

### 1. Stack — slug generation + first_prompt threading
Scope: How to convert arbitrary user input to a smart kebab-case slug. Whether to use a
library (slugify, deburr+parameterize, custom) or implement inline. Confirm the full
`first_prompt` threading path from UI → OmnibarFormState → OmnibarSessionData →
OmnibarContext → useSessionService → RPC.
Search cap: 3 searches
Trade-off axes: bundle size, Unicode handling, edge case correctness, maintenance burden

### 2. Features — Todoist-style inline shorthand + launcher UX patterns
Scope: Survey Todoist's inline syntax model (dates, labels, priorities as inline text).
Survey Linear, Raycast, Spotlight for session-type cycling UX. Understand the principles
behind "inline command syntax" in launcher interfaces.
Search cap: 4 searches
Trade-off axes: discoverability, parsing complexity, reversibility, power-user ceiling

### 3. Architecture — inline parsing location + formState shape + ADR design
Scope: Where exactly to parse the "name > prompt" separator: input onChange handler vs.
a derived computed value vs. submit time. How to add `firstPrompt` to OmnibarFormState
and thread it to OmnibarSessionData. ADR structure for the inline-shorthand concept.
Where to handle slash-command prefix detection: before or after DetectorRegistry.
Search cap: 3 searches
Trade-off axes: UX responsiveness, state complexity, testability

### 4. Pitfalls — edge cases + keyboard conflicts + parsing ambiguity
Scope: Slug generation edge cases (emoji, all-caps, special chars, >40 chars).
Separator parsing ambiguity (path inputs contain `>` ? No, but worktree labels might).
Tab key conflict (Tab already used for path completion dropdown cycling in Omnibar).
Ctrl+Tab browser/OS level conflicts.
Search cap: 3 searches
Trade-off axes: safety, UX correctness, conflict surface

## Output files
- research/findings-stack.md
- research/findings-features.md
- research/findings-architecture.md
- research/findings-pitfalls.md
- research/synthesis.md (parent agent, after all findings complete)
