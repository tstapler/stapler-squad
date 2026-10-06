# Research: Stack (Agent 1)

## React / TypeScript versions

- React `^19.0.0`, `react-dom` `^19.0.0`, TypeScript `^5.9.3` (`web-app/package.json`).
- No use of `useTransition`/`useActionState`/`useOptimistic` in `Omnibar.tsx` — submit state is
  plain `useState` (`isSubmitting`, `error`) managed by hand, consistent with the rest of the
  `web-app/src/components/sessions/` directory (no React 19 Actions API adoption yet in this file).

## Async submit + close pattern used elsewhere in the codebase

Searched every component using `setIsSubmitting` under `web-app/src/components/`:

- `web-app/src/components/sessions/Omnibar.tsx`
- `web-app/src/components/sessions/SessionWizard.tsx`
- `web-app/src/components/sessions/CheckpointButton.tsx`
- `web-app/src/components/sessions/ResumeSessionModal.tsx`
- `web-app/src/components/sessions/NewShellDialog.tsx`

**No shared hook or utility exists** for "async submit with guaranteed loading-state reset" — every
one of these components hand-rolls its own local `isSubmitting` state plus an inline
`try { ... } finally { setIsSubmitting(false) }` block. There is no `useAsyncSubmit`,
`withLoadingState`, `runAsync`, or similar generic wrapper anywhere in `web-app/src/lib/hooks/`
(confirmed via grep across `lib/` and `components/` for `with(Loading|Async)`/`run(Loading|Async)`
function/const declarations — zero hits).

This confirms the requirements doc's framing is correct: Omnibar.tsx's own **default branch**
(lines ~1073–1169) already has the right pattern in the same file —

```ts
setIsSubmitting(true);
setError(null);
try {
  // ... build session data, call service ...
  onClose();
} catch (err) {
  const message = err instanceof Error ? err.message : "Failed to create session";
  setError(message);
} finally {
  setIsSubmitting(false);
}
```

The fix should replicate this exact local pattern into the SpawnShell (lines 1003-1027) and
Alias-invocation (lines 1038-1071) branches, not introduce a new shared hook/abstraction. A
generic wrapper would be premature for a 3-callsite, single-file duplication and doesn't match
any existing convention in the codebase (see `.claude/rules/interface-pollution-checklist.md`'s
Go-specific guidance against speculative abstractions — same spirit applies here: use the
concrete, already-proven local pattern rather than inventing a hook with one real caller).

## Testing stack and Omnibar test conventions

- Jest `^30.2.0`, `ts-jest` `^29.4.11`, `jest-environment-jsdom` `^30.2.0`.
- `@testing-library/react` `^16.3.0`, `@testing-library/jest-dom` `^6.9.1`,
  `@testing-library/user-event` `^14.5.2`.
- Existing Omnibar test files (all under `web-app/src/components/sessions/__tests__/` — note:
  NOT colocated as `Omnibar.test.tsx` directly beside `Omnibar.tsx`):
  - `Omnibar.alias.test.tsx` — alias `namePrefix` population tests (the closest existing
    precedent for testing the alias-invocation code path this bug lives in)
  - `Omnibar.discovery.test.tsx`
  - `Omnibar.pathcompletion.test.tsx`
  - `OmnibarCreationPanel.attach.test.tsx`
- Convention from `Omnibar.alias.test.tsx`: mocks `next/navigation`, `ThemeContext`,
  `@/lib/config`, and hooks (`usePathCompletions`, `usePathHistory`, `useAliases`) via
  `jest.mock(...)` with local `jest.fn()` doubles (`mockUsePathCompletions`, etc.), and mocks the
  `@/lib/omnibar` barrel's `detect` export while passing through to the real implementation by
  default (`jest.requireActual`). Uses `render`, `screen`, `fireEvent`, `act` from
  `@testing-library/react`.
- New regression test (per acceptance criterion 5) should follow this same convention: a new
  file such as `web-app/src/components/sessions/__tests__/Omnibar.aliasSubmit.test.tsx` (or an
  added `describe` block inside `Omnibar.alias.test.tsx` if scope is tight enough), mocking
  `onClose` as a no-op (`jest.fn()` that does nothing, simulating the reported failure mode where
  `onClose()` doesn't synchronously unmount) and asserting `isSubmitting`/button-disabled state
  resets after the success path resolves.

## Summary of what to reuse vs. hand-roll

There is no existing "guaranteed loading-state reset" hook to reuse — the correct move is to
copy the try/catch/finally shape already used by Omnibar.tsx's own default branch (and mirrored
independently in `SessionWizard.tsx`, `CheckpointButton.tsx`, `ResumeSessionModal.tsx`,
`NewShellDialog.tsx`) into the two broken branches, not to invent a new shared utility.
