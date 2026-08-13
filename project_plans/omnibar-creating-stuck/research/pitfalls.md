# Pitfalls Research: Omnibar creation modal stuck on "Creating..."

Scope: risks specific to applying the `try/catch/finally` fix to the SpawnShell
(`web-app/src/components/sessions/Omnibar.tsx:1037-1061`) and Alias
(`:1072-1105`) branches of `handleSubmit`, matched against the existing Default
branch (`:1107-1203`), plus the `!isOpen` reset effect (`:622-633`) and new
regression tests.

## 1. Does moving `onClose()` inside `try` risk mis-attributing an `onClose()` throw as "Failed to create session"?

**Yes — and the default branch already has this exact latent bug, undocumented.**

Default branch, `:1176-1197`:
```ts
try {
  await onCreateSession(sessionData);
} catch (err) {
  // R2: Directory mode with non-existent path → show confirmation dialog
  if (...) { ...; return; }
  throw err;
}
// Persist the chosen path to history for future completions.
if (isPathInput && detection?.localPath && sessionType !== "one_off") {
  saveHistory(detection.localPath);
}
onClose();          // ← inside the outer try; a throw here falls into the outer catch
} catch (err) {
  const message = err instanceof Error ? err.message : "Failed to create session";
  setError(message);      // would show "Failed to create session" even though creation succeeded
} finally {
  setIsSubmitting(false);
}
```

There is **no special-casing** in the default branch to distinguish "creation
failed" from "`onClose()` (or `saveHistory()`) threw after creation succeeded."
Both land in the same `catch`, both produce the same generic "Failed to create
session" fallback message (since `onClose: () => void` carries no
information distinguishing it, and any thrown value that isn't an `Error`
falls back to the hardcoded string).

Precedent = **not handled**, not "handled a specific way." Applying the exact
same shape to SpawnShell/Alias reproduces this behavior faithfully (item 3 in
requirements: "No behavior change on the happy path") but does **not** fix a
pre-existing risk of mis-attribution if `onClose` throws.

**Is this a real risk in practice?** `onClose` is caller-supplied
(`OmnibarProps.onClose: () => void`, `Omnibar.tsx:51`); its actual
implementation across call sites should be checked, but nothing in `Omnibar.tsx`
suggests it can throw synchronously in normal operation (it's expected to be a
simple `setState`/close callback in the parent). `addRecentShellCommand()` in
the SpawnShell branch and `saveHistory()` in the default branch are similarly
unlikely-but-unverified throw sources.

**Recommendation for the fix**: since requirement 3 says "no behavior change on
the happy path," and the default branch already accepts this risk, the
SpawnShell/Alias branches should match it exactly (don't invent new
error-isolation logic not asked for — that would be scope creep beyond the
"mechanical pattern match" this task is scoped as). If tightening this is
wanted, it's a separate, explicit follow-up (candidate for the same "file a
follow-up" pattern requirement 7 already calls for), not part of this fix.
**Do not** silently improve error isolation only in the two new branches while
leaving the default branch inconsistent — that would create a
three-branches-now-disagree situation instead of resolving it.

## 2. Does adding `isSubmitting: false` to the `!isOpen` reset effect risk thrashing or a race with the `finally` block?

**No meaningful risk — the effect only fires on `isOpen` transitioning to
`false`, and `finally` only ever sets `false`, never `true`.**

The reset effect (`:622-633`):
```ts
useEffect(() => {
  if (!isOpen) {
    setInput("");
    setDetection(null);
    setFormState(INITIAL_FORM_STATE);
    setUIState({ showAdvanced: false, dropdownIndex: -1, dropdownDismissed: false, resultHighlightIndex: -1, atSuggestIndex: -1 });
    setError(null);
    lastSuggestedNameRef.current = "";
    prevDetectionTypeRef.current = null;
    dispatchMode({ kind: "reset_to_discovery" });
  }
}, [isOpen, dispatchMode]);
```

- **Dependency array**: only `[isOpen, dispatchMode]`. Adding a
  `setIsSubmitting(false)` call inside the `if (!isOpen)` block does not
  require adding `isSubmitting` (or its setter) to the deps — `setIsSubmitting`
  is a stable dispatch function from `useState`, exactly like the other
  setters already called unconditionally in this effect without being in the
  deps array (`setInput`, `setDetection`, `setFormState`, `setUIState`,
  `setError`). No dependency-array lint violation.
- **Ordering / double-set**: this effect only runs when `isOpen` flips to
  `false`. The `finally` block in `handleSubmit` runs synchronously as part of
  the async submit flow, which is only reachable while the modal is open
  (`canSubmit`/`isSubmitting` guard at `:1019` — though note `isSubmitting` is
  read, not necessarily requiring `isOpen`, so submission is possible even if a
  caller somehow flips `isOpen` false mid-flight, but that's an existing
  question independent of this fix). If `onClose()` synchronously flips
  `isOpen` to `false` before the `finally` runs (e.g. parent's `onClose` is a
  synchronous `setState` call and React batches/re-renders before the async
  function's `finally` line executes — not the normal case since `finally`
  runs on the same microtask before yielding, but worth naming), both the
  effect and `finally` would call `setIsSubmitting(false)` — a harmless
  same-value double-set, no thrash (React bails out of re-render for same
  primitive state value).
- **No interaction with other state in the same effect**: `isSubmitting` reset
  is orthogonal to the input/detection/form/UI resets already there — none of
  those setters read or depend on `isSubmitting`.

This is genuinely defense-in-depth as requirement 6 frames it: the component
instance is long-lived (never unmounts between open/close cycles per the
requirements doc), so this catches any future/other path that leaves
`isSubmitting` stuck `true` when the modal is dismissed, independent of the
`finally` fix. Safe to add.

## 3. React 18 StrictMode / async-state-after-unmount pitfalls for `setIsSubmitting` after `onClose()`

- **No unmount risk here** — confirmed by requirement 6's own framing: "the
  Omnibar component instance is long-lived... never unmounts between open/close
  cycles." `onClose()` (per the bug this whole fix addresses) does not
  synchronously unmount the modal in the broken case — that's the root cause,
  not a side effect to guard against. So calling `setIsSubmitting(false)` in
  `finally` after `onClose()` runs against a still-mounted component in both
  the broken and working cases. No "setState on unmounted component" warning
  risk.
- **StrictMode double-invocation**: relevant to *render* and certain effects
  (intentionally double-invoked in dev to surface impure logic), not to
  event-handler-triggered async callbacks like `handleSubmit`. `handleSubmit`
  is a `useCallback`-wrapped click/keydown handler, not a render-phase or
  effect-phase function — StrictMode does not double-invoke it. No pitfall
  here for this specific change.
- **Effect double-invocation on the reset effect**: StrictMode *does*
  double-invoke effects (mount → cleanup → mount) in dev. The `!isOpen` reset
  effect has no cleanup function, so double-invocation just calls the same
  idempotent setters twice — already true today for the existing setters, and
  adding `setIsSubmitting(false)` is equally idempotent (same-value set is a
  no-op re-render bail-out). No new risk introduced.
- **Test environment**: RTL's `render` doesn't wrap in `<StrictMode>` by
  default, so this is a theoretical/production concern, not something the new
  Jest tests need to work around.

## 4. Jest/RTL pitfalls for testing the two async submit branches

The existing `__tests__/Omnibar.alias.test.tsx` already exercises the Alias
branch's happy path (submit via Ctrl+Enter → asserts `onCreateSession` args)
under the exact harness the new regression tests must reuse. Key patterns and
risks to carry over:

- **Fake timers are mandatory and already standard practice.** `beforeEach`
  calls `jest.useFakeTimers()`; the 150ms input-detection debounce
  (`Omnibar.tsx:601`, comment: `// 150ms debounce`) means typing into the
  combobox does nothing detection-wise until timers advance. The established
  helper:
  ```ts
  async function typeAndDetect(input: Element, value: string) {
    fireEvent.change(input, { target: { value } });
    await act(async () => {
      jest.advanceTimersByTime(200); // > 150ms debounce
    });
  }
  ```
  Must be reused (or duplicated) for both new branches — omitting the
  `advanceTimersByTime` step will leave `detection` `null`/stale and the
  SpawnShell/Alias code path in `handleSubmit` simply won't be entered
  (`detection?.type === InputType.SpawnShell && detection.confidence === 1.0`
  guard at `:1037` will fail).
- **`afterEach` must drain pending timers before switching back to real
  timers**: existing pattern is
  ```ts
  afterEach(() => {
    act(() => { jest.runOnlyPendingTimers(); });
    jest.useRealTimers();
    jest.clearAllMocks();
    resetDefaultRegistry();
  });
  ```
  Skipping `runOnlyPendingTimers()` before `useRealTimers()` can leave a
  dangling fake-timer-scheduled debounce callback that fires against real
  timers unexpectedly in a later test, a known source of cross-test flakiness.
- **`await act(async () => { ... })` around every state-changing event**,
  including the submit keydown itself — the existing tests wrap
  `fireEvent.keyDown(input, { key: "Enter", ctrlKey: true })` in
  `await act(async () => { ... })` specifically because `handleSubmit` is
  `async` and the promise chain (`await onCreateSession(...)`, then
  `onClose()`/`setIsSubmitting`) needs to flush inside `act` or RTL will warn
  about state updates outside `act` (and, worse, assertions can race the
  pending promise and read stale state).
- **The new tests' entire point is asserting behavior *after* `onClose` is a
  no-op mock** (per requirement 5: "asserting isSubmitting resets even when
  onClose is a mocked no-op"). This means:
  - Use `onClose: jest.fn()` (default no-op, i.e. explicitly do *not* let it
    flip `isOpen`) rather than a mock that calls a real close handler — the
    existing `renderOmnibar` helper already defaults `onClose` to
    `props.onClose ?? jest.fn()`, and the test must pass `isOpen={true}`
    statically (it's not wired to react to the mock being called) — this is
    exactly the setup needed to reproduce "onClose doesn't synchronously
    unmount/close" without needing a real modal-dismiss integration.
  - Assert `isSubmitting`'s effect on the DOM (e.g. a disabled submit button,
    "Creating..." label, or `aria-busy` — whatever the actual DOM signal is;
    inspect `OmnibarCreationPanel.tsx`'s submit button rendering for the
    exact testid/text to assert against) rather than trying to read internal
    state directly, since `isSubmitting` is component-internal state, not a
    prop. **Action item for implementer**: check what
    `OmnibarCreationPanel.tsx` renders when `isSubmitting` is true (button
    text/`disabled` attribute/`data-testid`) to know what to assert
    post-reset — this file did not need to be read for this pitfalls
    question but must be read before writing the actual test assertions.
  - Because `onCreateSession` is awaited before `onClose()`/`finally` run,
    `mockResolvedValue(undefined)` (as used throughout the existing alias
    tests) is sufficient to drive the happy path without needing manual
    Promise control (`new Promise(resolve => ...)` + manual `resolve()`)
    unless a test specifically wants to assert the *in-flight* (`isSubmitting
    === true`, spinner visible) state before resolution — that would need an
    unresolved promise held in a variable and resolved mid-test inside a
    second `act()`.
  - Regression test must **not** rely on the modal actually closing/unmounting
    to pass — asserting on `isSubmitting`-driven DOM state while
    `isOpen={true}` throughout is the whole point; a test that also checks
    `onClose` was called is fine/expected, but the reset-to-non-submitting
    assertion must hold independent of that.
- **`detect` mock note**: `Omnibar.alias.test.tsx` mocks the `@/lib/omnibar`
  module's `detect` export only to simulate *detection throwing*; it otherwise
  passes through to the real implementation. The new SpawnShell test does not
  need this mock unless it wants to test a detection-throws edge case — reuse
  is optional there, but the `resetDefaultRegistry()` /
  `getDefaultRegistry().register(...)` pattern from `beforeEach` is *not*
  needed for SpawnShell since `CommandDetector` (which matches `>shell ...`)
  is already part of `createDefaultRegistry()` per
  `.claude/rules/feature-testing-registry.md`'s detector priority table
  (priority 5) — unlike `AliasDetector`, which is a *dynamic* detector
  registered only via `OmnibarContext.tsx` in production and thus needs manual
  registration in tests that render `Omnibar` directly without that context
  wrapper.
- **Mock surface is large**: any new test file colocated in
  `__tests__/Omnibar.*.test.tsx` will need to replicate (or import/share) the
  ~15 `jest.mock(...)` calls at the top of `Omnibar.alias.test.tsx`
  (`next/navigation`, `ThemeContext`, `lib/config`, `usePathCompletions`,
  `usePathHistory`, `useSessionSearch`, `useWorktreeSuggestions`,
  `useAliases`, `useAliasSuggestions`, `useAtCommandSuggestions`,
  `useAvailablePrograms`, `useSlashCommands`, `useSlashCommandSuggestions`,
  `lib/store`, `lib/store/sessionsSlice`, `OmnibarResultList`) — missing any
  one of these is a likely source of unrelated test failures/flakiness
  (e.g. a real hook trying to hit the network or read `next/navigation`
  context that isn't present in the JSDOM test env), not something specific
  to this fix but a real setup cost worth flagging since it affects how
  fast/reliable the two new regression tests will be to get green.
