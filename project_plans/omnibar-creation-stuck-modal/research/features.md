# Research: Similar async-submit-close flows, and edge cases (Agent 2 — Features)

## 1. Where `<Omnibar>` actually lives and what `onClose` really does

`<Omnibar>` is rendered in exactly one place in production code:
`web-app/src/lib/contexts/OmnibarContext.tsx:275-279`:

```tsx
<Omnibar
  isOpen={isOpen}
  onClose={close}
  onCreateSession={handleCreateSession}
  onNavigateToSession={handleNavigateToSession}
  ...
/>
```

`close` (`OmnibarContext.tsx:148-152`) is trivial and **unconditional** — no gating, no
early return, no async work, no dependency on other state:

```tsx
const close = useCallback(() => {
  setIsOpen(false);
  setInitialInput(undefined);
  setInitialTitle(undefined);
}, []);
```

`isOpen` is plain `useState` on `OmnibarProvider`, which wraps the whole app tree (see
`web-app/src/app/Providers.tsx`) — it is not conditionally mounted/unmounted per route, so
`close`'s identity and the `setIsOpen` setter are stable for the app's lifetime. Inside
`Omnibar.tsx` itself, visibility is a plain conditional unmount, not a portal/CSS-driven
modal: `Omnibar.tsx:1203` — `if (!isOpen) return null;`. There is no `createPortal`, no
CSS transition gating dismissal, no z-index/animation that could "look open" while
`isOpen` is actually `false`.

**Conclusion: `onClose()` cannot legitimately fail or silently no-op from gating, a race
with other component state, or a thrown exception inside `close` itself** — it's a
one-line synchronous state setter. This narrows "why onClose() might not dismiss the
modal" down to two real mechanisms, both consistent with a "mobile timing quirk":

1. **The call is never reached** — an exception thrown between `await onCreateSession(...)`
   succeeding and the `onClose()` line (e.g. `addRecentShellCommand(shellCommand)` at
   `Omnibar.tsx:1020` throwing, or a stale/removed-history write). In the SpawnShell and
   Alias branches this exception falls into the same `catch` that already resets
   `isSubmitting` — so today this actually shows an error banner and re-enables the form,
   NOT a silently-stuck spinner. This isn't the bug being fixed but is worth confirming
   doesn't regress.
2. **The call happens, but React's state update / re-render is delayed or dropped** on a
   backgrounded/frozen mobile tab (e.g., user alt-tabs to another app mid-`await` while the
   RPC is in flight; Safari/iOS throttles timers and can defer paint on backgrounded tabs).
   When the tab regains focus, if `onClose()` already ran, `isOpen` is `false` and the
   component should have unmounted — so a *visibly* stuck "Creating..." button in this
   scenario would only be possible if `onClose()` was never invoked in the first place
   (case 1), or if something re-opened the omnibar afterward. There is no code path found
   that re-opens it automatically. This means the reported "stuck modal" symptom is most
   plausibly explained by **case 1 in disguise** (an unhandled exception, or the
   client-side RPC promise appearing to fail/hang while the server-side action actually
   succeeded — see §3 below) rather than `onClose()` itself misfiring.

## 2. Other async-submit → close-modal flows in the codebase (comparison patterns)

Searched `web-app/src/components/**/*.tsx` for `async`/`isSubmitting`/`onClose`/`finally`
patterns similar to Omnibar's three branches:

- **`web-app/src/components/sessions/SessionWizard.tsx:238-253`** (`onSubmit`) —
  **has the same anti-pattern** as the two buggy Omnibar branches: `setIsSubmitting(true)`
  before `try`, but `setIsSubmitting(false)` is only called in `catch`, no `finally`. It
  doesn't call `onClose()` itself (delegates dismissal to the parent via `onComplete`), so
  the failure mode is slightly different, but it's the same class of bug: if the parent's
  post-`onComplete` navigation/close doesn't actually unmount `SessionWizard`, the wizard's
  submit button stays disabled forever. **Not in scope for this bug fix** (out of the
  stated file), but worth flagging as the same defect pattern recurring — consistent with
  `.claude/rules/fix-flaky-tests-dont-defer.md`'s sibling principle of not re-excusing a
  known shape. Recommend filing a follow-up rather than silently fixing inline, since
  `plan.md` scope is explicitly `Omnibar.tsx`.
- **`web-app/src/components/onboarding/OnboardingModal.tsx`** — `handleInstallHooks`
  (`~108-134`) *does* use `try/catch/finally` correctly (`finally` resets its loading
  flag), and separately calls `onClose()` unconditionally after other handlers
  (`~131-184`) without wrapping the whole thing in `isSubmitting`-style guarding — it's a
  simpler flow (no risk of stuck-disabled-button because `onClose()` calls aren't gated by
  a loading state in the same way).
- No other component under `web-app/src/components/` combines
  `isSubmitting` + `await <async action>` + `onClose()` in the same handler outside these
  two files. `Omnibar.tsx`'s own default/third branch (`~1073-1169`) is the **canonical
  correct pattern already in this file** — `try { ...; onClose(); } catch { setError }
  finally { setIsSubmitting(false) }` — so the fix is to make the other two branches match
  it exactly, not invent a new pattern.

## 3. Root-cause candidate: RPC promise settling after the modal already closed, or
   appearing to fail when it didn't

`useSessionService.ts` (`~170`, `~835-899`) uses an `AbortController` per in-flight
request (`abortControllerRef`) and explicitly treats an abort as a silent early return
(`~899`: `return; // ConnectRPC abort (e.g. AbortController signal)`). This matters for
two edge cases:

- If a **second `createSession` call reuses/replaces** `abortControllerRef.current` while
  a first call is still pending (e.g., a double-submit — see §4), the first call's promise
  can reject with an abort error that the caller (Omnibar's `handleSubmit`) still catches
  as a generic failure, showing "Failed to create session" even though a session was (or
  will be) created by the second, overlapping call. This is a duplicate-session risk, not
  just a UI-freeze risk.
- No `isMounted`/cleanup-ref guard exists anywhere in `Omnibar.tsx` or
  `OmnibarContext.tsx` for "component unmounted mid-await" — not a crash risk in React 18
  (state updates on unmounted components are silently dropped, no warning), but confirms
  there's no existing convention in this file to mirror for "ignore late state updates
  after the user already closed/moved on."

## 4. Edge cases the fix should explicitly handle

1. **Double-submit / rapid re-click**: `handleSubmit`'s guard
   `if (!canSubmit || isSubmitting) return;` (`Omnibar.tsx:985`) reads `isSubmitting` from
   the closure of the *current* render. Two synchronous invocations in the same event-loop
   tick (a known mobile Safari "ghost click"/double-tap-before-repaint pattern) can both
   observe `isSubmitting === false` before either `setIsSubmitting(true)` commits and
   disables the DOM button (`OmnibarCreationPanel.tsx:828`:
   `disabled={!canSubmit || isSubmitting}`). This is a pre-existing risk independent of the
   try/finally bug, but the fix's regression test should include a "rapid double-submit
   only creates one session" case if feasible, or at minimum note it as a known residual
   risk if out of scope.
2. **Component/provider unmount mid-`await`**: not currently guarded anywhere in this file;
   don't introduce a new pattern (e.g. `isMountedRef`) unless the other branches already
   need it — the existing default branch doesn't have one either, so parity, not
   over-engineering, is the right bar.
3. **Rapid successive alias/shell submissions** (submit → success → immediately reopen
   omnibar → submit again before prior state settles): `close()` resets `initialInput`/
   `initialTitle` but each submit path is independent local state inside `Omnibar` that
   resets via the mount/unmount cycle (`isOpen` false → `return null` unmounts and drops
   all local `useState`), so a full close between submissions is safe. The risk is only
   real if `onClose()` doesn't fire (case 1 above) and the user retries inside the same
   still-mounted instance.
4. **Network call resolving after the user manually closed the modal** (e.g., hits Escape
   — handled at `Omnibar.tsx:908-915` — or clicks outside, while a submit is in flight):
   `onClose`/`close` doesn't cancel the in-flight `onCreateSession` promise, and there's no
   guard preventing the `.then`/`catch` continuation from calling `setError`/
   `setIsSubmitting` on an already-unmounted component. Harmless in React 18 (no-op), but
   worth a one-line acknowledgment in the plan since the ACs mention "failure path still
   works" — the failure path must still be exercised correctly even if the modal was
   already dismissed by other means.

## 5. Unstated need: user-visible fallback if create succeeds but the modal doesn't close

Given `close()` is unconditional and synchronous, the only realistic way a user sees a
permanently-stuck "Creating..." button *after a successful create* is if the exception
path (§1 case 1) is hit — meaning **the current code already shows an error banner in that
case, not a silent freeze** (both buggy branches already have `setError(...)` in `catch`).
The literal ACs (try/finally parity + regression test) are necessary but not sufficient to
address the live-reported symptom unless the actual trigger is identified. Two concrete,
low-effort additions beyond the literal ACs, consistent with `NotificationContext`
already existing in this codebase (`web-app/src/lib/contexts/NotificationContext.tsx`,
`addNotification`, already wired into session-related flows like
`useSessionNotifications.ts`):

- If `onCreateSession` succeeds but a *subsequent* step throws (e.g.
  `addRecentShellCommand`), surface a toast via `addNotification` rather than (or in
  addition to) the inline form error — the session **was** created, so "Failed to create
  session" is actively misleading and invites a duplicate re-submit. This is the
  concrete duplicate-session risk the requirements' "unstated needs" question is pointing
  at.
- Consider logging (`console.error` at minimum, matching the existing pattern in
  `OmnibarContext.tsx:248` and `SessionWizard.tsx:246`) whenever the post-success cleanup
  step throws, so a "modal didn't close after successful create" report can actually be
  correlated with a browser console error in the future instead of being unreproducible.

## Files referenced

- `web-app/src/components/sessions/Omnibar.tsx:984-1170` (three branches under review)
- `web-app/src/components/sessions/Omnibar.tsx:1203` (`if (!isOpen) return null;`)
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx:827-830` (submit button, `disabled={!canSubmit || isSubmitting}`)
- `web-app/src/lib/contexts/OmnibarContext.tsx:130-153` (`open`/`close`/`toggle`, all unconditional `setIsOpen`)
- `web-app/src/lib/contexts/OmnibarContext.tsx:275-283` (`<Omnibar onClose={close} ...>`)
- `web-app/src/components/sessions/SessionWizard.tsx:238-253` (same try/catch-without-finally anti-pattern, different file — flag as follow-up, not in scope)
- `web-app/src/lib/hooks/useSessionService.ts:170,835-899` (AbortController reuse; silent-abort-return path relevant to duplicate-session risk)
- `web-app/src/lib/contexts/NotificationContext.tsx` (existing toast system usable for the "succeeded but modal stuck" fallback)
