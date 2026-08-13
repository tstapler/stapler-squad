# Pitfalls Research — Omnibar Creation Stuck Modal

Agent 4 (Pitfalls), SDD research phase. Scope: what commonly goes wrong with
"async action → close modal" React patterns, and what this specific fix
(`web-app/src/components/sessions/Omnibar.tsx`) needs to be designed against.

## 1. The code-confirmed root cause: `isSubmitting` is never reset on close

This is the load-bearing finding — it changes the risk calculus for AC6.

- `handleSubmit` (`Omnibar.tsx:984`) has three creation branches:
  - SpawnShell (`Omnibar.tsx:1003-1027`) — `setIsSubmitting(false)` only in `catch`.
  - Alias (`Omnibar.tsx:1038-1071`) — same shape, only in `catch`.
  - Default/manual (`Omnibar.tsx:1073-1169`) — already has `try/catch/finally`
    (`finally { setIsSubmitting(false); }` at `Omnibar.tsx:1167-1169`).
- `onClose` is the `close` callback from `OmnibarContext.tsx` (wired at
  `OmnibarContext.tsx:277` → `Omnibar.tsx:50` prop), and it is **literally
  `setIsOpen(false)`** (`Omnibar.tsx:148-150`, mirrored in `OmnibarContext.tsx`).
  It is synchronous, cannot throw, and cannot silently no-op under normal
  React semantics.
- `Omnibar.tsx:1203`: `if (!isOpen) return null;` — this makes the component
  render nothing when closed, but **it does not unmount the component
  instance**. The parent (`OmnibarContext.tsx:275`) always renders
  `<Omnibar ... />` unconditionally; only the *output* is conditionally
  `null`. All `useState` in `Omnibar` — including `isSubmitting` — survives
  across close/reopen cycles.
- There is a dedicated "reset state when closed" effect
  (`Omnibar.tsx:587-599`) that resets `input`, `detection`, `formState`,
  `uiState`, `error`, and dispatches `reset_to_discovery` — **but it does not
  reset `isSubmitting`.**
- The submit button reads `isSubmitting` directly:
  `disabled={!canSubmit || isSubmitting}` and label `isSubmitting ? "Creating…" : "Create Session"`
  (`Omnibar.tsx:1588-1590`).

**Consequence:** in the SpawnShell/Alias branches, a *successful* session
creation sets `isSubmitting = true`, then `onClose()` correctly closes the
modal (React unmounts nothing, state persists) — but `isSubmitting` is never
flipped back. The **next time** the user opens the omnibar, the reset-on-close
effect has already run (clearing everything except `isSubmitting`), so the
modal opens already showing a disabled "Creating…" button, permanently, for a
session that already succeeded and already closed correctly. This matches the
bug report's literal wording ("modal permanently stuck showing Creating…")
without requiring `onClose()` itself to be broken.

This is a strong, code-grounded alternative explanation for "why didn't
`onClose()` dismiss the modal on mobile": it likely *did* dismiss it — the
stuck state the user saw may have been the *next* open, not the same one,
which is easy to misreport as "the modal wouldn't close."

## 2. Secondary, independently real risk: `position: fixed` without a portal

- `Omnibar.css.ts:21-22`: the overlay uses `position: "fixed"` directly, with
  no `createPortal` in `Omnibar.tsx` (confirmed: no `react-dom` `createPortal`
  import anywhere in the file).
- This repo's own `.claude/rules/css-architecture.md` documents this exact
  anti-pattern as a known break: `position: fixed` overlays without
  `createPortal(..., document.body)` silently misbehave when any ancestor has
  a CSS `transform`, `filter`, or `will-change` — common on mobile browsers
  during virtual-keyboard show/hide, page-transition animations, or PWA
  viewport handling (iOS Safari in particular applies visual-viewport
  transforms around keyboard events).
- If this is in play, the failure mode is different from #1: the overlay can
  become visually mispositioned or fail to actually disappear even though
  React's `isOpen` state is correctly `false` and the component correctly
  returns `null` on next render — i.e., a rendering/compositing problem, not
  a state problem.
- This is plausible but **unverified** for the specific mobile report — there
  is no attached repro, screenshot, or browser/OS detail in the bug summary
  to confirm which of #1 or #2 (or both) explains the reported session. Flag
  as UNVERIFIED; do not present as the confirmed cause.

## 3. Why "just add `finally`" is not equally risky in both directions

The research question asks whether blindly adding `finally { setIsSubmitting(false) }`
could produce a *worse* UX than today if `onClose()` "truly" fails (i.e. #2 above)
— resetting `isSubmitting` on a modal that's still visually stuck would flip
the button from disabled ("Creating…") to enabled ("Create Session"), inviting
a duplicate-submission click against a session that already exists.

That risk is real but narrow:
- It only manifests under hypothesis #2 (CSS-driven non-dismissal), not under
  #1 (state leak) — and #1 is the mechanism actually confirmed to exist in
  this codebase today, with a direct code path from "success" to "stuck
  forever," independent of any CSS/mobile quirk.
- The `finally` fix is a strict improvement for #1 (closes a real, provable
  bug) and is neutral-to-positive for #2 as long as it's paired with the
  existing default-branch pattern's other safety property: `onCreateSession`
  is `await`ed *before* `onClose()` is called, so by the time `finally` runs,
  the RPC has already resolved (success or failure) — `finally` never fires
  concurrently with an in-flight create. The risk window described in the
  question (re-enabling the button while a duplicate create could race) does
  not apply to the `await`ed call itself, only to a *second, independent*
  click after the button re-enables.
- **AC6 (investigate the real `onClose` root cause) is still worth doing**,
  but the evidence gathered here downgrades it from "equally critical to
  AC1/AC2" to "important follow-up, not a blocking prerequisite" — the
  `finally` fix independently closes a confirmed bug that fully explains the
  reported symptom, whether or not the CSS hypothesis also contributes.
  Recommend scoping AC6 as: reproduce on an actual mobile browser/PWA build,
  check for ancestor `transform`/`filter` at the time of the report, and only
  escalate to a `createPortal` migration if reproduced independently of the
  `isSubmitting` leak (i.e., after the `finally` fix ships, does the report
  still recur?).

## 4. Defense-in-depth recommendation

Add `isSubmitting` to the existing "reset state when closed" effect
(`Omnibar.tsx:587-599`), in addition to the `finally` fix in both branches.
Rationale: the reset-on-close effect already exists specifically as a
belt-and-suspenders cleanup for stale state across the close boundary — every
other piece of submit-adjacent state is listed there except this one. This
means *any future branch* that forgets a `finally` (a fourth creation path,
a refactor that drops the `finally`) fails safe instead of silently
reintroducing this exact bug. This is cheap and directly addresses the class
of bug, not just the instance — consistent with this repo's
`quality:reflect-and-fix` philosophy (see `.claude/rules/fix-flaky-tests-dont-defer.md`
for the same "fix the class, not the instance" reasoning applied elsewhere).

## 5. Other common pitfalls checked and ruled out / not applicable

- **`addRecentShellCommand` / `saveHistory` throwing (mobile Safari private-mode
  localStorage quota)** — checked `web-app/src/lib/omnibar/recentShellCommands.ts:16-27`
  and `web-app/src/lib/hooks/usePathHistory.ts:42-48`: both already wrap
  `localStorage` access in `try/catch` and fail silently. Ruled out as a
  throw-between-await-and-onClose risk.
- **Stale closures over `onClose`/`onCreateSession`** — `handleSubmit`'s
  `useCallback` dependency array (`Omnibar.tsx:1170-1196`) correctly includes
  both `onClose` and `onCreateSession`. `close`/`onCreateSession` are
  themselves stable `useCallback`s in `OmnibarContext.tsx`. No stale-closure
  risk found.
- **Double-submission via un-debounced click** — mitigated the same way in
  all three branches: `setIsSubmitting(true)` is called synchronously before
  the `await`, and the button is `disabled={... || isSubmitting}`. This is
  pre-existing shared behavior, not something the `finally` fix changes.
- **React StrictMode double-invocation** — no evidence found of StrictMode
  being enabled for this component tree in a way that would double-fire
  `handleSubmit` (it's an event-handler callback, not a render-phase or
  effect-phase function, so StrictMode's double-invoke behavior — which only
  targets render, effects, and a few lifecycle-adjacent functions — does not
  apply here).
- **setState-after-unmount warnings** — not applicable in the way it usually
  is, because `Omnibar` never actually unmounts across open/close (see #1);
  the risk here is the opposite of the usual pitfall — state persisting
  *too* long, not being lost too early.

## 6. Git history / prior incidents

- No prior bug doc under `docs/bugs/` mentions Omnibar modal dismissal,
  `isSubmitting`, or a stuck "Creating…" state (searched fixed/open bug docs).
- One directly relevant prior incident: commit `80f80da22` ("fix(web-app,e2e):
  fix Omnibar creation-mode race + stabilize repo-path-picker-parity e2e
  spec", 2026-08-01) fixed a related-but-distinct bug in the same file: an
  input-detection `useEffect` was unconditionally dispatching
  `reset_to_discovery` on every empty-input re-render — including spurious
  re-runs from async alias/workflow refetches — silently stomping an
  explicitly-selected creation mode before the user typed anything. That
  fix (`prevDetectionInputRef` guard) is a sibling example of the same root
  class of bug this one is: **state that should only reset on a genuine
  user-driven transition instead resets (or fails to reset) on the wrong
  trigger.** Worth citing as precedent that this file has a recurring
  "effect timing / stale-vs-fresh state" failure mode, reinforcing that the
  defense-in-depth fix in #4 is worth doing now rather than waiting for a
  fifth branch to reintroduce it.
- No other commits reference "stuck modal," "onClose," or similar for
  Omnibar specifically; the many "stuck" commits in history are unrelated
  (backlog-item stuck-state detection, a different subsystem).

## Summary for the plan phase

1. AC1/AC2 (`finally` in SpawnShell + Alias branches) is a confirmed, provable
   bug fix — ship it as designed.
2. Add `setIsSubmitting(false)` to the existing reset-on-close effect
   (`Omnibar.tsx:587-599`) as defense-in-depth against the same class
   recurring in a future branch.
3. AC6 (investigate real `onClose` failure) should be scoped as a follow-up
   reproduction task (mobile browser + ancestor transform/filter check,
   `position: fixed` vs `createPortal`), not a blocking co-requirement of
   AC1/AC2 — the state-leak mechanism found here already fully explains the
   reported symptom on its own.
4. Add a regression test asserting `isSubmitting` returns to `false` after a
   successful SpawnShell/Alias creation, and ideally one asserting it resets
   on the next `isOpen` transition even if a `finally` were hypothetically
   missing (this covers the defense-in-depth item, not just the branch fix).
