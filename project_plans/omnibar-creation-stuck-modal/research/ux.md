# UX Research: Omnibar Creation Stuck Modal

Agent 5 (UX) — SDD research phase for `omnibar-creation-stuck-modal`.

## Code grounding

`web-app/src/components/sessions/Omnibar.tsx`'s `handleSubmit` (lines 984–1210) has three
near-identical async submission branches, and only one of them is written correctly:

- **SpawnShell branch** (lines 1016–1026): `setIsSubmitting(true)` → `try { await
  onCreateSession(...); onClose(); } catch { setError(...); setIsSubmitting(false); }` — no
  `finally`.
- **Alias branch** (lines 1061–1070) — this is the exact path the bug report hit
  (`@pw retirement`): same shape, same missing `finally`.
- **Default/manual branch** (lines 1073–1169): wraps the whole body in `try { ... }
  catch (err) { setError(...) } finally { setIsSubmitting(false) }` — this is the correct
  pattern, and it already exists in the same file.

Root cause is a **copy-paste inconsistency**, not a missing pattern the codebase needs to
invent. All three branches should converge on the `try/catch/finally` shape already used by
the default branch.

A second, non-obvious detail: the component does **not unmount** when the modal closes. Line
1203, `if (!isOpen) return null;`, means `Omnibar` stays mounted for the lifetime of the app
(rendered once in `OmnibarContext.tsx:275`) and just renders `null` while closed. React
preserves function-component state across a `return null` render. So if `onClose()` is ever a
no-op (state update swallowed, stale closure, etc.), `isSubmitting` doesn't just stay stuck
for the current open instance — it **persists into the next time the user opens the omnibar**,
since it's the same component instance with the same `useState` cell. A finally-block fix
alone does not address this; the state also needs to be reset on the `isOpen` transition
itself (there's already a `useEffect` keyed on `isOpen` at lines 582–599 doing analogous
reset-on-open work — `setIsSubmitting(false)` belongs there too, as defense in depth
independent of whichever code path created the session).

## Findings

### 1. Timeout fallback vs. finally-block fix alone

The finally-block fix is necessary but not sufficient by itself, given the state-persistence
behavior above. Recommended layering, cheapest fix first:

1. **Fix the missing `finally`** in both broken branches — this is the actual bug and the
   majority of the fix.
2. **Reset `isSubmitting` (and `error`) in the existing `isOpen`-keyed effect** (lines
   582–599) — this bounds the damage to "one submission cycle" even if a future code path
   reintroduces the same mistake, and specifically fixes the cross-session persistence found
   above.
3. **Do not add a timeout-based auto-dismiss.** A modal that vanishes on its own N seconds
   after a background action is a worse pattern here: the session was already created
   successfully, so silently closing the modal without confirming the user saw that is just a
   variant of the same "no feedback" problem, and picking N is guesswork (mobile network
   latency to `onCreateSession` varies far more than desktop). A timeout is the right tool for
   "the action might still be pending and we don't know," not for "the action succeeded and we
   just failed to act on it."
4. **Do add a manual escape hatch that's always available**, independent of the bug: the
   overlay `onClick={onClose}` (line 1216) and `Escape` key handler (line 908–915) already
   exist as manual dismiss paths — verify neither one is gated on `!isSubmitting`. A quick
   grep of `handleKeyDown`/the overlay click handler should confirm the user was never
   actually fully trapped, only trapped as far as the primary CTA. If either *is* gated on
   `isSubmitting`, un-gate it: a disabled primary button should never be the only way out of a
   modal.

### 2. Mobile-specific races

This bug was reported on a phone, and there are two well-documented mobile-web mechanisms that
make "action succeeded but the UI didn't update" more likely than on desktop:

- **Virtual keyboard show/hide triggers `visualViewport` resize events**, which many mobile
  browsers fire *asynchronously* relative to focus/blur, sometimes hundreds of ms after the
  triggering interaction. If any layout/animation code in this modal (or a parent) is wired to
  viewport-resize (common for "shrink modal above the keyboard" patterns), a resize firing
  mid-submission can interrupt or race a state update. Grep found no `visualViewport` listener
  in `Omnibar.tsx` itself, but it's worth checking `OmnibarContext.tsx` and any shared modal
  wrapper for one before ruling it out.
- **Tap-and-hold-to-dismiss-keyboard is a two-step gesture on mobile**: tapping "Create
  Session" on a phone often first dismisses the keyboard (a browser-level action, not a React
  event) and only registers the actual tap on a second gesture, or registers a `touchend`
  significantly after `touchstart`. If `handleSubmit` is wired to a fast-path event
  (`onClick` is fine; the risk is elsewhere) combined with `disabled={!canSubmit ||
  isSubmitting}` flipping mid-gesture, a double-tap can be captured as two rapid submissions
  before the first `setIsSubmitting(true)` re-render commits — React batches this correctly in
  18+, but it's the class of bug worth a regression test for on this exact button.
- **iOS Safari suspends timers/network in background tabs more aggressively than desktop
  Chrome.** If the user backgrounds the tab (switches apps to check something) between tapping
  "Create Session" and the RPC resolving, `onCreateSession`'s promise can resolve much later
  than expected, well after the user has forgotten they submitted — this doesn't cause the bug
  reported here (which is a synchronous logic bug, not a timing one) but reinforces why a
  silent success + failed-to-close modal is especially bad on mobile: the user has less
  working memory of "I just submitted this" by the time they notice the stuck spinner.

Actionable takeaway for the fix: don't rely on any viewport/resize/focus event as a signal for
modal lifecycle; the `isOpen` boolean and the `finally` block are the only two things this fix
needs to touch. Add a mobile-viewport Playwright check (`browser_resize` to a phone viewport)
to whatever E2E test covers this bug, per `.claude/rules/e2e-test-conventions.md`.

### 3. Accessibility of the "Creating…" state

Current markup (lines 1584–1591):
```tsx
<button type="button" className={createButton} onClick={handleSubmit} disabled={!canSubmit || isSubmitting}>
  {isSubmitting ? "Creating…" : "Create Session"}
</button>
```

Gaps found:
- **No `aria-busy`** on the button. `aria-busy="true"` should be set while `isSubmitting` is
  true — this is the correct semantic for "in progress," distinct from `disabled` (which just
  means "not interactive right now," with no implication of *why*).
- **No `aria-live` region wrapping the button or its label.** A `disabled` attribute change and
  a text content change inside a button are **not reliably announced** by screen readers on
  their own — `aria-disabled`/`disabled` state changes are inconsistently announced across
  AT/browser combos, and plain text-node changes inside an already-rendered button are not
  automatically treated as a live region. A screen reader user who taps "Create Session" may
  hear nothing confirming the tap registered, then nothing when it gets stuck. Fix: either add
  `aria-live="polite"` to a small status element near the button announcing "Creating
  session…" / "Session created" / the error text (the file already uses this pattern
  elsewhere — `aria-live="polite"` at lines 1284, 1338, 1429 for other states — so this is
  applying an existing local convention, not introducing a new one), or add `role="status"`
  the same way the alias-resolution chip does at line 1338.
- **Once fixed, does re-enabling clearly signal "you can retry"?** Text reverting from
  "Creating…" back to "Create Session" plus the button becoming non-disabled is a standard and
  legible pattern sighted-user-side. For screen-reader users, the same `aria-live` region
  fix above should announce the return to the ready state (or the error, if one occurred) so
  it isn't only conveyed by the disabled-state flip.

### 4. Risk of accidental duplicate resubmission after the fix

This is the most important open question, and the finally-block fix alone actively makes it
*worse* in one specific scenario, not better:

- **Before the fix**: session created, modal stuck open forever. Bad UX (no feedback,
  no escape), but *zero* risk of a duplicate session — the button stays disabled forever.
- **After a bare finally-block fix**: if `onClose()` throws or silently no-ops (the "genuinely
  fails to dismiss" case from the research question), the `finally` still fires and
  `isSubmitting` resets to `false`, re-enabling "Create Session" — on a modal that's still
  showing the *same pre-fill form state* (title, path, alias, etc.), because nothing cleared
  the form. A user who assumes the stuck spinner meant failure, sees the button re-enable, and
  taps it again will create a **second, duplicate session** with identical parameters. This is
  strictly worse than the original bug for that one edge case (stuck-forever is annoying but
  safe; silently-resettable-into-a-duplicate is a data-integrity issue), even though it's
  correct behavior for the far more common case (network/RPC failure, no session was created).

  Recommended mitigation, in order of cost:
  1. **Cheap, do it regardless of scope**: on success, before calling `onClose()`, clear or
     disable the form inputs (or set a short-lived `justSucceeded` flag that keeps the button
     disabled with "Created ✓" for a beat) so that *even if* `onClose()` fails, the retry
     surface no longer looks like a fresh unsubmitted form. This directly answers the research
     question: yes, show a brief success confirmation (checkmark / "Session created" label) as
     an intermediate state between "Creating…" and "idle, ready to resubmit" — it costs one
     extra `useState` and a couple hundred ms, and it converts the worst-case failure mode from
     "silent duplicate" to "user sees confirmation, then has to notice the modal didn't close
     and manually dismiss it," which is recoverable.
  2. **If `onClose()` failing to dismiss is itself fixed as part of this project** (rather than
     just working around it), the duplicate-resubmission risk mostly evaporates, since success
     will reliably close the modal and there's nothing left to resubmit against. Prioritize
     finding *why* `onClose()` can fail to dismiss (stale closure over `setIsOpen`? an
     intervening `isOpen` prop feeding back from a parent effect?) over building UX to route
     around it — but ship the success-confirmation affordance anyway as defense in depth, since
     "the async close call didn't do what we expected" is a class of bug, not a one-off.

## Summary of recommendations for the plan phase

1. Add `finally { setIsSubmitting(false); setError(null on success path is fine, but don't
   clear on error) }`-equivalent to the SpawnShell and Alias branches, matching the existing
   default-branch pattern (`Omnibar.tsx:1167-1169`).
2. Also reset `isSubmitting`/`error` inside the existing `isOpen`-keyed effect
   (`Omnibar.tsx:582-599`) as a second line of defense against the no-unmount state-leak found
   above.
3. Do not add a timeout-based auto-dismiss; do verify Escape/overlay-click dismiss paths are
   never gated on `isSubmitting`.
4. Add `aria-busy` to the submit button while submitting, and route the "Creating…" / success /
   error text through an `aria-live="polite"` region, consistent with the pattern already used
   at lines 1284/1338/1429.
5. Add a brief success-confirmation state (disable + "Created ✓" or similar, held for a short
   beat) before/around the `onClose()` call on every success path, so a failed-to-dismiss modal
   never looks like an untouched, freshly-resubmittable form.
6. Cover this with a mobile-viewport Playwright test (per `e2e-test-conventions.md`) exercising
   the `@alias` submission path specifically, since that's the reproduction case.
