# Requirements: Omnibar creation modal stuck on "Creating..."

Backlog item: a6c87dbf-2ebb-4c6c-8fab-032d76fef1e7 (SDD pipeline, non-interactive — derived
directly from item description + acceptance criteria, ideation interview skipped).

## Problem

`handleSubmit` in `web-app/src/components/sessions/Omnibar.tsx` has three session-creation
branches:

1. **SpawnShell** (`>shell ...`, ~line 1037-1061)
2. **Alias invocation** (`@alias ...`, ~line 1072-1105)
3. **Default** (path/repo/new-project, ~line 1107-1203)

Branches 1 and 2 call `setIsSubmitting(true)`, then on success call `onClose()` with no
`finally`; the failure path resets `isSubmitting` only inside `catch`. Branch 3 already
wraps the reset in `try/catch/finally`. If `onClose()` doesn't synchronously unmount the
modal (confirmed live: a `@pw retirement` alias session was created successfully server-side
— `personal-wiki-retirement`, Active, ran for hours — but the modal stayed open with the
submit button permanently stuck on "Creating..." on a mobile browser), branches 1 and 2 have
no path back to `isSubmitting = false`.

## Requirements (from acceptance criteria)

1. Alias-invocation branch: after a successful `onCreateSession` call, `isSubmitting` resets
   to non-submitting regardless of whether `onClose()` actually unmounts the modal.
2. SpawnShell branch: same guarantee — `isSubmitting` resets to `false` after a successful
   `onCreateSession` call regardless of `onClose()` behavior.
3. No behavior change on the happy path: when `onClose()` works normally, the modal still
   closes with no new error toast/flash.
4. Failure path unchanged: `onCreateSession` throwing in either branch still surfaces the
   error message and re-enables the button (this already works — must not regress).
5. Regression tests (Jest/RTL, colocated with existing Omnibar tests) for both branches:
   success path resets `isSubmitting` even when `onClose` is a mocked no-op.
6. Defense-in-depth: the reset-on-close effect at `Omnibar.tsx:622-633` (`if (!isOpen) { ... }`)
   also explicitly clears `isSubmitting`, since the `Omnibar` component instance is long-lived
   and persists across open/close cycles (it does not unmount between sessions).
7. Root cause of `onClose()` failing to dismiss the modal: code inspection confirms
   `Omnibar.css.ts`'s `overlay` style uses `position: "fixed"` (line 22) and the component is
   *not* rendered via `createPortal`. Per this repo's own CSS architecture rule
   (`.claude/rules/css-architecture.md`, "Never Do" list), `position: fixed` overlays without
   `createPortal(..., document.body)` silently break when any ancestor has `transform`,
   `filter`, or `will-change` — a plausible, specific mechanism for a modal that appears open
   but is unreachable/misplaced. This is a distinct, larger change (portal migration touches
   the render tree and any ancestor-transform styling) — file it as its own tracked backlog
   item naming this exact hypothesis, rather than fixing it inline here.

## Fix approach

Apply the same `try { ... } catch (err) { setError(...) } finally { setIsSubmitting(false) }`
shape already used in the default branch to the SpawnShell and Alias branches. Move the
`onClose()` call inside the `try`, drop the branch-local `setIsSubmitting(false)` from
`catch` (the `finally` now covers it unconditionally), and add `isSubmitting: false` to the
`!isOpen` reset effect.

## Out of scope

- Actually implementing the `createPortal` migration for the overlay (tracked as a follow-up
  item per requirement 7).
- Any other Omnibar behavior not related to the stuck-submitting-state bug.
