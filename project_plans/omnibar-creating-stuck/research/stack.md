# Stack Research: Omnibar creation modal stuck on "Creating..."

## Versions (from `web-app/package.json`)

| Package | Version |
|---|---|
| react / react-dom | `^19.0.0` |
| typescript | `^5.9.3` |
| jest | `^30.2.0` |
| ts-jest | `^29.4.11` |
| @testing-library/react | `^16.3.0` |
| @testing-library/jest-dom | `^6.9.1` |
| @testing-library/user-event | `^14.5.2` (present as devDep; existing Omnibar tests do not actually use it — see below) |
| @types/react | `^19` |
| @types/jest | `^30.0.0` |
| packageManager | `pnpm@10.27.0` |

Test runner: `jest.config.js` at `web-app/jest.config.js`. Run via `pnpm test` (= `jest`) or targeted:
```
cd web-app && npx jest --no-coverage --testPathPatterns="Omnibar.alias"
```

## Target file confirmed

`web-app/src/components/sessions/Omnibar.tsx` (1659 lines). `handleSubmit` (`useCallback`, starts line 1018) has exactly the three branches requirements.md describes:

- **SpawnShell** branch: lines 1037–1061. `setIsSubmitting(true)` (1050) → `try { await onCreateSession(...); ...; onClose(); } catch (err) { setError(...); setIsSubmitting(false); }` — no `finally`, success path never resets `isSubmitting`.
- **Alias** branch: lines 1072–1105. Same shape — `setIsSubmitting(true)` (1095) → `try { await onCreateSession(sessionData); onClose(); } catch (err) { setError(...); setIsSubmitting(false); }` — no `finally`.
- **Default** branch: lines 1107–1203, already correct — `try {...} catch (err) {...} finally { setIsSubmitting(false); }` (finally at line 1201–1203). This is the shape to copy into the other two branches.

Reset-on-close effect (req #6 target): lines 622–633, in a `useEffect` gated on `!isOpen`. Currently resets `input`, `detection`, `formState`, `uiState`, `error`, two refs, and dispatches `reset_to_discovery` — but does **not** call `setIsSubmitting(false)`. Adding it there is the "defense in depth" the requirement asks for.

Submit-button call sites keying off `isSubmitting` (both must be considered when asserting UI state in tests):
- `web-app/src/components/sessions/Omnibar.tsx:1625` — shortcuts-bar Create button, `disabled={!canSubmit || isSubmitting}`, label `isSubmitting ? "Creating…" : "Create Session"` (line 1627, **note: em dash `…`**, not `...`).
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx:850,852` — footer Create button, same `disabled` expression, but its label uses three literal dots: `isSubmitting ? "Creating..." : "Create Session"` (line 852). **The two buttons use different ellipsis characters** — a test asserting on the exact string must match whichever button it queries, not assume both render identical text. Both are queryable via `getAllByRole("button", { name: /create session/i })` while `isSubmitting` is false (the accessible name reverts to "Create Session"); while submitting, query by `/creating/i` or check `hasAttribute("disabled")` instead of exact text if covering both.

## Existing test file layout

Tests for `Omnibar.tsx` are split across `web-app/src/components/sessions/__tests__/`, one file per concern, not a single monolithic `Omnibar.test.tsx`:

- `Omnibar.alias.test.tsx` (472 lines) — alias `namePrefix` population + a detection-throw robustness test. **Most relevant precedent** — already exercises the Alias submission path end-to-end via `onCreateSession`/`onClose` mocks and asserts on Create-button `disabled` state.
- `Omnibar.discovery.test.tsx`
- `Omnibar.pathcompletion.test.tsx`
- `OmnibarCreationPanel.attach.test.tsx`

New tests for this fix should follow the `Omnibar.<topic>.test.tsx` naming convention, e.g. `Omnibar.submitReset.test.tsx` or similar, colocated in the same `__tests__/` directory — not a new top-level `Omnibar.test.tsx`.

## Conventions to match exactly (from `Omnibar.alias.test.tsx`)

**Mocking layer** — a large, copy-pasteable block of `jest.mock(...)` calls at the top of the file stubs every hook/context `Omnibar.tsx` pulls in, so the component can be rendered standalone:
`next/navigation`, `@/lib/contexts/ThemeContext`, `@/lib/config`, `@/lib/hooks/usePathCompletions`, `@/lib/hooks/usePathHistory`, `@/lib/hooks/useSessionSearch`, `@/lib/hooks/useWorktreeSuggestions`, `@/lib/hooks/useAliases`, `@/lib/hooks/useAliasSuggestions`, `@/lib/hooks/useAtCommandSuggestions`, `@/lib/hooks/useAvailablePrograms`, `@/lib/hooks/useSlashCommands`, `@/lib/hooks/useSlashCommandSuggestions`, `@/lib/store`, `@/lib/store/sessionsSlice`, `@/components/sessions/OmnibarResultList` (stubbed to `() => null`), `@/lib/api/transport`.

For SpawnShell-branch tests, no alias-specific mocking is needed beyond this baseline set (SpawnShell detection is registered in `createDefaultRegistry()` per `.claude/rules/feature-testing-registry.md`, so no extra `getDefaultRegistry().register(...)` call is required — unlike Alias, which needs `AliasDetector` registered manually since it's a dynamic/context-registered detector in production).

**`renderOmnibar` helper** (lines 166–182):
```ts
function renderOmnibar(
  props: { onClose?: jest.Mock; onCreateSession?: jest.Mock; onNavigateToSession?: jest.Mock } = {}
) {
  const onClose = props.onClose ?? jest.fn();
  const onCreateSession = props.onCreateSession ?? jest.fn().mockResolvedValue(undefined);
  const onNavigateToSession = props.onNavigateToSession ?? jest.fn();
  const utils = render(
    <Omnibar isOpen={true} onClose={onClose} onCreateSession={onCreateSession} onNavigateToSession={onNavigateToSession} />
  );
  const input = screen.getByRole("combobox", { name: /session source input/i });
  return { ...utils, input, onClose, onCreateSession, onNavigateToSession };
}
```
Reuse this pattern (or import/adapt it) rather than inventing a new render helper. Note `isOpen={true}` is hardcoded and never toggled by these tests — since `onClose` is a plain `jest.fn()` that doesn't flip `isOpen`, **the harness already simulates the exact bug scenario** (`onClose()` called but the modal doesn't unmount/close) with no extra setup needed. This is the natural place to assert `isSubmitting` resets to `false` post-success even though `onClose` fired.

**`typeAndDetect` helper** (lines 184–190) — types into the input and flushes the 150ms detect debounce under fake timers:
```ts
async function typeAndDetect(input: Element, value: string) {
  fireEvent.change(input, { target: { value } });
  await act(async () => {
    jest.advanceTimersByTime(200);
  });
}
```

**Timer setup** — every `describe` block uses `jest.useFakeTimers()` in `beforeEach` and `jest.useRealTimers()` + `jest.runOnlyPendingTimers()` + `jest.clearAllMocks()` (+ `resetDefaultRegistry()` when detectors were touched) in `afterEach`.

**Submission trigger** — tests submit via `fireEvent.keyDown(input, { key: "Enter", ctrlKey: true })` wrapped in `await act(async () => {...})` (Ctrl+Enter works in both discovery and creation mode), rather than clicking a Create button directly. This is the pattern to reuse for driving SpawnShell/Alias submission in the new tests.

**Async/import style** — plain `fireEvent` + `act` from `@testing-library/react`, no `@testing-library/user-event` despite it being a devDependency; don't introduce `userEvent` into these new tests, it would be an inconsistent style relative to every existing Omnibar test.

**Asserting on `onCreateSession`** — `expect(onCreateSession).toHaveBeenCalledWith(expect.objectContaining({...}))`, not exact-object equality, since `OmnibarSessionData` has many fields.

**Asserting on submit state** — the detection-throw test (lines 437–471) is the closest existing precedent for asserting `isSubmitting`/disabled behavior: it queries **both** Create buttons via `screen.getAllByRole("button", { name: /create session/i })`, maps `.hasAttribute("disabled")` on each, and compares before/after snapshots — because Omnibar renders two buttons that both key off the same `canSubmit || isSubmitting` condition (footer button in `OmnibarCreationPanel.tsx:850` and shortcuts-bar button in `Omnibar.tsx:1625`). New tests asserting the fix (`isSubmitting` returns to `false`/button re-enabled after a successful SpawnShell/Alias submission with a non-unmounting `onClose`) should query both buttons the same way rather than assuming only one is present — note SpawnShell/Alias submissions may stay in discovery mode (no `OmnibarCreationPanel` rendered, so only the shortcuts-bar button exists) — check which mode each branch's detection puts the component into before asserting on button count.

**Error-path testing pattern** — `jest.spyOn(console, "error").mockImplementation(() => {})` is used to suppress/assert React's console.error output when an intentional error path is exercised; restore via `consoleErrorSpy.mockRestore()` in `afterEach`.

## Summary of what the new tests need to do differently from existing coverage

No existing test currently asserts that `isSubmitting` resets to `false` after a **successful** SpawnShell or Alias submission — `Omnibar.alias.test.tsx`'s alias tests only assert on `onCreateSession`'s call args (title/branch), not on post-submit button/spinner state, and its one disabled-state assertion covers the **detection-throw** (pre-submission) error path, not the post-`onCreateSession`-success race this bug is about. The new tests are net-new coverage, not a modification of existing assertions.
