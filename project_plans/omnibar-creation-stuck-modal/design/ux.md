# UX Design: Omnibar Creation Stuck Modal

SDD planning phase — design artifact for `omnibar-creation-stuck-modal`, built on
`project_plans/omnibar-creation-stuck-modal/research/ux.md` (root cause, mobile-race
analysis, accessibility gaps, duplicate-resubmission risk already established there — not
re-derived here).

## Surfaces in scope

Three user-facing surfaces in `web-app/src/components/sessions/Omnibar.tsx`, all driven by
the same `isSubmitting`/`error` state:

| # | Surface | Location | Current behavior |
|---|---|---|---|
| A | Submit button (idle / loading / error) | `Omnibar.tsx:1584–1591` | `disabled={!canSubmit \|\| isSubmitting}`, label toggles `"Create Session"` / `"Creating…"`; no `aria-busy`, not wrapped in `aria-live` |
| B | Modal dismiss affordances | Escape handler `Omnibar.tsx:908–910`, overlay-click `Omnibar.tsx:1216` | Both call `onClose()` directly — confirmed **not** gated on `isSubmitting` today |
| C | Accessibility announcement | none yet on the button; existing local pattern at `Omnibar.tsx:1338` (`role="status" aria-live="polite"` on the alias-resolution chip) | Button state changes are silent to AT; other parts of the form already use this pattern |

Also relevant but not a distinct "surface": the `isOpen`-keyed reset effect at
`Omnibar.tsx:588–599`, which resets `error` but not `isSubmitting` on close — this is where
the second line of defense goes (research finding, section "Timeout fallback vs.
finally-block fix alone", step 2).

## Flow diagrams

### Surface A — submit button state machine

**Before fix — SpawnShell/Alias branches, `onClose()` works:**

```
[idle: "Create Session"]
        │ click
        ▼
[loading: "Creating…", disabled]
        │ onCreateSession() resolves
        ▼
   onClose() called ──► modal unmounts (isOpen→false)
                         (isSubmitting never explicitly reset, but it doesn't
                          matter — the isOpen-keyed effect wipes form state,
                          and the button isn't rendered while closed)
```
No visible bug in this path — this is why the bug was hard to spot in normal testing.

**Before fix — SpawnShell/Alias branches, `onClose()` fails to dismiss (the reported bug):**

```
[idle: "Create Session"]
        │ click
        ▼
[loading: "Creating…", disabled]
        │ onCreateSession() resolves (session created server-side ✓)
        ▼
   onClose() called ──► modal does NOT unmount (isOpen stays true)
        │
        ▼
[loading: "Creating…", disabled]   ◄── STUCK. No finally. isSubmitting
                                        never reset. Session already exists;
                                        user has no way to know or retry.
```

**After fix — either `onClose()` outcome:**

```
[idle: "Create Session"]
        │ click
        ▼
[loading: "Creating…", disabled, aria-busy="true"]
        │ onCreateSession() resolves
        ▼
   finally { setIsSubmitting(false) }   ◄── always runs, regardless of onClose()
        │
        ├── onClose() worked  → modal unmounts, button moot
        │
        └── onClose() failed  → modal still visible, but button is back to
                                 [idle: "Create Session", aria-busy="false"]
                                 (re-enabled — see duplicate-resubmission
                                 note below for why this alone is not enough)
```

**After fix — failure path (`onCreateSession` throws), unchanged from today's correct
default-branch behavior:**

```
[idle: "Create Session"]
        │ click
        ▼
[loading: "Creating…", disabled, aria-busy="true"]
        │ onCreateSession() throws
        ▼
   catch { setError(msg) }
        │
        ▼
   finally { setIsSubmitting(false) }
        │
        ▼
[idle: "Create Session", error banner visible]   ◄── button re-enabled, error shown
```

### Surface A (extended) — success-confirmation beat, closing the duplicate-resubmission gap

The bare `finally` fix above re-enables the button on a still-visible modal with unchanged
form contents whenever `onClose()` fails — a user who reads the re-enabled button as "it
must have failed" can tap again and create a **second** session with identical parameters
(research finding §4). Recommended intermediate state:

```
[idle] → click → [loading: "Creating…"] → onCreateSession() resolves
                                                   │
                                                   ▼
                                    [confirmed: "Created ✓", still disabled]
                                                   │
                                    onClose() called (may or may not dismiss)
                                                   │
                              ┌────────────────────┴────────────────────┐
                              ▼                                         ▼
                    modal unmounts (isOpen→false)          modal still visible after a
                    → next isOpen-keyed effect run           short beat: isSubmitting
                      wipes isSubmitting/error/form           resets to false, but the
                      for the *next* open                     form is visually marked
                                                                "already submitted" (not
                                                                a fresh retry target) —
                                                                Escape/overlay-click
                                                                remain the way out
```

This "Created ✓" beat is a UX acceptance criterion below (AC-8) — it directly answers the
research doc's open question and is cheap (one more `useState`, no timers, no guessing at a
duration threshold).

### Surface B — dismiss affordances (Escape / overlay-click)

**Both before and after fix — must be identical, this is a regression guard, not a change:**

```
        modal open, any isSubmitting value (true OR false)
                          │
        ┌─────────────────┼─────────────────┐
        ▼                                    ▼
  press Escape                        click overlay (outside modal)
        │                                    │
        ▼                                    ▼
   onClose() invoked                   onClose() invoked
   (never checks isSubmitting)         (never checks isSubmitting)
        │                                    │
        ▼                                    ▼
   modal unmounts (assuming isOpen→false propagates from parent)
```

Confirmed by reading the code (`Omnibar.tsx:908–910` and `:1216`): neither handler is
gated on `isSubmitting` today. This satisfies the "no dead ends" property structurally
already — the design's job is to (a) add a regression test pinning this so a future edit
can't accidentally add `disabled={isSubmitting}` or an equivalent guard to either path, and
(b) not introduce any new gating when adding the "Created ✓" beat in Surface A above (the
confirmed state must not disable Escape/overlay-click, only the primary CTA).

### Surface C — accessibility announcement

**Before fix:**

```
sighted user: sees "Create Session" → "Creating…" (visual only)
screen reader user: taps button → *silence* → (if stuck) *continued silence forever*
                     no aria-busy, no aria-live region, text-node change inside an
                     already-rendered button is not reliably announced
```

**After fix:**

```
sighted user: sees "Create Session" → "Creating…" → "Created ✓" (or error text)
screen reader user: taps button →
    aria-busy="true" announced (state change) →
    aria-live="polite" region announces "Creating session…" →
    on resolve: region announces "Session created" (or the error message on failure) →
    aria-busy="false"
```
Pattern to reuse verbatim: `role="status" aria-live="polite"` as already used for the
alias-resolution chip (`Omnibar.tsx:1338`) and two other existing states
(`:1284`, `:1429`) — this is applying an established local convention, not inventing a new
one.

## UX acceptance criteria

Each is independently verifiable by a human tester (manual click-through or screen-reader
pass); items 1–4 restate requirements.md's ACs 1–4 in UX-observable terms, items 5–9 add the
research doc's accessibility/no-dead-ends/duplicate-resubmission recommendations.

1. **AC-1 (alias branch, stuck-modal recovery).** Submit `@alias ...`. If the modal does not
   visually close after creation succeeds, the submit button is NOT stuck on
   "Creating…" — within one render it shows either "Created ✓" (confirmation beat) or has
   returned to "Create Session" (re-enabled), never a permanently disabled spinner state.
2. **AC-2 (SpawnShell branch, stuck-modal recovery).** Same as AC-1, reproduced via `>shell
   ...` submission.
3. **AC-3 (happy path unchanged).** When the modal *does* close normally after success, no
   error banner/toast flashes at any point during the transition, and no "Created ✓" state
   lingers visibly after unmount (a human tester should not be able to see or screenshot it
   under normal desktop-speed interaction).
4. **AC-4 (failure path unchanged).** Submitting via either branch when `onCreateSession`
   rejects: error message appears in the existing error-banner location, button returns to
   "Create Session" (re-enabled), modal stays open. No regression from current behavior.
5. **AC-5 (no dead ends — Escape).** With the button in the "Creating…" state (`isSubmitting
   === true`), pressing Escape still closes the modal. Verified for both an in-flight
   submission and a stuck one.
6. **AC-6 (no dead ends — overlay click).** Same as AC-5, via clicking outside the modal
   instead of pressing Escape.
7. **AC-7 (screen-reader announcement).** With a screen reader running (VoiceOver/NVDA
   acceptable for manual check), tapping "Create Session": (a) the button's busy state is
   announced (`aria-busy` transition), and (b) a live region announces "Creating…" on start
   and either the success confirmation or the error message on completion — a screen-reader
   user gets equivalent information to what a sighted user sees, without having to re-focus
   the button.
8. **AC-8 (duplicate-resubmission guard).** After a successful creation where the modal does
   *not* auto-close, the form is visibly in a "just submitted" state (e.g. "Created ✓",
   inputs disabled or visually inert) for a short beat rather than instantly reverting to an
   untouched, freshly-resubmittable "Create Session" form — a user cannot mistake the stuck
   modal for "nothing happened yet" and blindly resubmit the identical alias/shell command.
9. **AC-9 (cross-session state isolation).** Open the omnibar, submit a session that
   reproduces the stuck-modal condition (or simulate via a mocked `onClose` no-op in a dev
   build), manually dismiss via Escape, then reopen the omnibar fresh. The newly opened
   modal starts in a clean idle state — "Create Session" enabled, no leftover "Creating…" /
   "Created ✓" from the previous cycle. (Guards the `isOpen`-keyed reset effect actually
   covers `isSubmitting`, per the research doc's state-persistence finding.)

## Summary

3 surfaces designed (submit button state machine, dismiss affordances, accessibility
announcement region), 9 UX acceptance criteria written (AC-1–4 mirror requirements.md's ACs
1–4 in human-testable form; AC-5–9 add the no-dead-ends guarantee, screen-reader parity,
duplicate-resubmission guard, and cross-session state isolation from research/ux.md).
