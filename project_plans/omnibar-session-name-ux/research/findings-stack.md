# Findings: Stack

## Summary

Two independent gaps to fill:

1. **Session name auto-fill for bare text input**: `SessionSearchDetector` always returns `suggestedName: ""`, so when the user types free text (no path or URL), the session name field stays blank. The fix is to slugify the omnibar input text and set it as `suggestedName` inside `SessionSearchDetector`.

2. **First prompt UI wiring**: `initial_prompt` (field 15) exists in the proto, is generated in TypeScript bindings (`initialPrompt: string` in `CreateSessionRequest`), and is used in `SessionWizard`. However, the omnibar path (`OmnibarFormState → OmnibarSessionData → OmnibarContext → useSessionService → RPC`) does not thread it at all: `OmnibarFormState` has no `firstPrompt` field, `OmnibarCreationPanel` has no textarea for it, `useSessionService.createSession` does not pass `initialPrompt` to the RPC, and `OmnibarContext.handleCreateSession` does not forward it.

No slug library is installed. The codebase uses inline simple `replace(/\//g, "-")` patterns throughout `detector.ts`.

---

## Options Surveyed

### Q1: Slugify approach for bare-text → session name

**Option A: Inline custom function**

A small pure function colocated in `detector.ts` or a new `web-app/src/lib/omnibar/slugify.ts`:

```ts
export function toSessionSlug(input: string): string {
  return input
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, "")   // strip non-alphanumeric except spaces/hyphens
    .replace(/\s+/g, "-")            // spaces → hyphens
    .replace(/-{2,}/g, "-")          // collapse multiple hyphens
    .replace(/^-|-$/g, "")           // trim leading/trailing hyphens
    .slice(0, 50);                   // reasonable length cap
}
```

Consistent with the existing pattern of inline `replace` chains throughout `detector.ts`. Covers the stated use case (2–8 words, ASCII-dominant).

**Option B: `npm slugify`**

~4 KB gzipped. Supports locale-aware transliteration (é→e, ü→u). Overkill for session name input; adds a dependency the size-limit tooling must track.

**Option C: `@sindresorhus/slugify`**

Pure ESM, ~2 KB gzipped. Strong Unicode support. Same verdict: unnecessary for this use case.

---

### Q2: Wiring `initial_prompt` through the omnibar path

The gap is a multi-step addition across 4 files:

1. Add `firstPrompt: string` to `OmnibarFormState` and `INITIAL_FORM_STATE` in `Omnibar.tsx`
2. Add `firstPrompt?: string` to `OmnibarSessionData` in `Omnibar.tsx`
3. Add a `<textarea>` for first prompt in `OmnibarCreationPanel.tsx`
4. Pass `initialPrompt: data.firstPrompt` in `OmnibarContext.tsx handleCreateSession`
5. Add `initialPrompt: request.initialPrompt` to the RPC call body in `useSessionService.ts`

`SessionWizard` already implements the full pattern (`initialPrompt`, zod schema, character counter at 10,000, react-hook-form registration) — that is the reference design.

---

## Trade-off Matrix

| Criterion | Inline slug fn | npm slugify | @sindresorhus/slugify |
|---|---|---|---|
| Bundle size impact | 0 KB | ~4 KB gzip | ~2 KB gzip |
| Unicode/emoji support | No (ASCII-only) | Yes | Yes |
| Correctness for use case | Sufficient | Over-engineered | Over-engineered |
| Consistency with codebase | High (matches existing) | Low | Low |
| Maintenance burden | Minimal | Dep updates | Dep updates |

---

## Risk and Failure Modes

**Slug collision**: Two different inputs can produce the same slug. Not a problem — session names are labels, not unique keys.

**Empty slug**: Input like "!!! ???" collapses to `""` after stripping. Guard: if result is empty, return `""` (leave field blank, same as current behavior). Do not default to `"session"`.

**`prompt` vs `initialPrompt` confusion**: These are distinct proto fields with different semantics:
- `prompt` (field 7): pre-session uploaded image paths (space-joined). Populated from `attachedImagePathsRef`.
- `initial_prompt` (field 15): injects via CLAUDE.md; no size limit; shell-safe. This is the correct field for the new textarea.

Do not mix them. The new textarea must populate `initialPrompt`, not `prompt`.

**handleSubmit data loss**: `handleSubmit` (Omnibar.tsx lines 639-641) builds `finalPrompt` from attached images only. Adding `firstPrompt` to OmnibarFormState without updating this line produces silent data loss. Fix: `[formState.firstPrompt?.trim(), ...imagePaths].filter(Boolean).join("\n") || undefined`.

**Prompt length**: `SessionWizard` caps at 10,000. A shorter limit (2,000) is more appropriate for quick-create context.

---

## Migration and Adoption Cost

No proto changes, no schema migrations. All fields already exist in generated TypeScript (`initialPrompt: string` confirmed at line 233 of `/web-app/src/gen/session/v1/session_pb.ts`).

Touch points:
- `/web-app/src/components/sessions/Omnibar.tsx`: add `firstPrompt` to `OmnibarFormState` and `OmnibarSessionData`
- `/web-app/src/components/sessions/OmnibarCreationPanel.tsx`: add textarea
- `/web-app/src/lib/contexts/OmnibarContext.tsx`: pass `initialPrompt: data.firstPrompt`
- `/web-app/src/lib/hooks/useSessionService.ts`: add `initialPrompt: request.initialPrompt` to RPC call
- `/web-app/src/lib/omnibar/detector.ts` (`SessionSearchDetector`): set `suggestedName: toSessionSlug(trimmed)`
- New: `web-app/src/lib/omnibar/slugify.ts` (~15 lines) with unit test

Total: 5–6 file edits, 0 new npm dependencies.

---

## Operational Concerns

**Size-limit**: No new packages, so the JS bundle limit is unaffected.

**Tests**: The slug function should have a unit test. `SessionSearchDetector` change should add a case to `detector.test.ts`. The omnibar firstPrompt round-trip should have a Jest/RTL test. The `session:create` entry in `docs/registry/backend-features.json` should have its `lastModified` bumped.

---

## Prior Art and Lessons Learned

**Existing slug patterns in `detector.ts`** (lines 70, 139, 185): These are minimal inline replacements (slash → hyphen) on already-clean identifiers from URLs/paths. Free-text input needs additionally: `toLowerCase()`, non-alphanumeric stripping, and space-to-hyphen conversion.

**`SessionWizard` `initialPrompt` pattern** is the reference implementation. Key detail: it passes `initialPrompt` directly as a named field; the omnibar version can simplify by skipping zod validation if desired.

**`suggestedName` as the auto-fill hook**: The auto-fill logic in `Omnibar.tsx` lines 361–364 reads `result.suggestedName` and writes to `sessionName`. Putting the slug in `suggestedName` inside `SessionSearchDetector` is the correct hook — no `Omnibar.tsx` auto-fill logic changes needed.

---

## Open Questions

1. Should the first-prompt textarea appear in the main form body or inside "Advanced Options"? Quick-create context suggests main form (bottom, above Advanced), not buried.
2. What is the character limit for the omnibar first-prompt textarea? `SessionWizard` uses 10,000; 2,000 is more appropriate here.
3. Should the slug cap at 50 chars or follow a session name validation limit? 50 chars is a reasonable default.

---

## Recommendation

**For slug generation**: Implement an inline `toSessionSlug` pure function in a new `web-app/src/lib/omnibar/slugify.ts`. Do not add an npm package. Update `SessionSearchDetector.detect()` to return `suggestedName: toSessionSlug(trimmed)` instead of `""`. This matches codebase style and has zero bundle impact.

**For first prompt wiring**: Follow the `SessionWizard` `initialPrompt` pattern exactly. Add `firstPrompt: string` to `OmnibarFormState` and `OmnibarSessionData`, add a textarea to `OmnibarCreationPanel`, pass it through `OmnibarContext` as `initialPrompt`, and add `initialPrompt: request.initialPrompt` to `useSessionService.createSession`. Do not confuse `prompt` (image paths) with `initialPrompt` (text injection). Patch `handleSubmit` to incorporate `firstPrompt` into `finalPrompt`.

---

## Web Search Results

Web search not performed — codebase evidence was sufficient for all decisions.

Key file paths referenced:
- `web-app/src/lib/omnibar/detector.ts` — `SessionSearchDetector`, existing slug patterns
- `web-app/src/components/sessions/Omnibar.tsx` — `OmnibarFormState`, `OmnibarSessionData`, auto-fill logic (lines 361–364)
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx` — form fields, no first-prompt textarea
- `web-app/src/lib/contexts/OmnibarContext.tsx` — `handleCreateSession`, `prompt: data.prompt` present, `initialPrompt` absent
- `web-app/src/lib/hooks/useSessionService.ts` — `createSession`, `initialPrompt` field absent from RPC call
- `web-app/src/gen/session/v1/session_pb.ts` line 233 — `initialPrompt: string` confirmed in generated `CreateSessionRequest`
- `web-app/src/components/sessions/SessionWizard.tsx` — reference implementation of `initialPrompt` textarea pattern
