# Requirements: omnibar-creation-stuck-modal

## Source

Backlog item `a6c87dbf-2ebb-4c6c-8fab-032d76fef1e7` — "bug: Omnibar alias/shell session
creation gets stuck on Creating after successful session creation". Reported live via
mobile screenshot on 2026-08-06.

## Problem

The Omnibar new-session modal got stuck on a disabled "Creating..." button indefinitely
after submitting `@pw retirement` (alias invocation resolving to
`~/Documents/personal-wiki`, session type "Existing folder"). The session was in fact
created successfully server-side (`personal-wiki-retirement`, status Active, path
`/home/tstapler/Documents/personal-wiki`, `created_at 2026-08-06T10:12:57-07:00`,
matching the phone status bar in the screenshot, and it ran fine for hours afterward).
The bug is purely frontend: the modal never dismissed and the submit button never left
its loading state.

## Root cause (confirmed by reading code)

`handleSubmit` in `web-app/src/components/sessions/Omnibar.tsx` has three
near-identical session-creation branches:

1. **SpawnShell branch** (`web-app/src/components/sessions/Omnibar.tsx:1003-1027`) —
   `>shell` command detection.
2. **Alias-invocation branch** (`web-app/src/components/sessions/Omnibar.tsx:1038-1071`)
   — the one `@pw retirement` hits.
3. **Default branch** (`web-app/src/components/sessions/Omnibar.tsx:1073-1169`) —
   directory / new_worktree / existing_worktree / one_off / new_project / autonomous.

Branches 1 and 2 call `setIsSubmitting(true)`, then on success call `onClose()` with no
`finally` block — `setIsSubmitting(false)` only runs inside `catch`. Branch 3 correctly
wraps its reset in `try { ... } finally { setIsSubmitting(false) }`
(`web-app/src/components/sessions/Omnibar.tsx:1167-1169`).

If `onClose()` fails to synchronously unmount/dismiss the modal for any reason (parent
re-render race, mobile browser timing quirk — the report came from a phone browser),
branches 1 and 2 leave `isSubmitting` stuck `true` forever with the modal still visible,
even though session creation itself already succeeded server-side.

## In scope

- Make the SpawnShell and Alias-invocation branches resilient to `onClose()` not
  dismissing the modal, using the same `try/catch/finally` pattern already proven in the
  default branch, so `isSubmitting` cannot get stuck `true` after a successful
  `onCreateSession` call regardless of what `onClose()` does.
- Investigate why `onClose()` did not dismiss the modal on the reporting mobile browser
  and fix that root cause if a concrete mechanism is found (this is the actual bug the
  user hit; the finally-block change alone is a defense-in-depth symptom guard on top of
  it, not a fix for it).

## Out of scope

- Any change to backend session-creation logic (already correct — the session was
  created successfully).
- Redesigning the Omnibar's three-branch structure into a single shared code path
  (worth flagging as a follow-up refactor suggestion, not doing it as part of this bug
  fix — keep the diff minimal per repo convention).

## Acceptance criteria

1. After a successful alias-invocation session creation (`@alias ...` submit), if
   `onClose()` does not result in the modal unmounting, the submit button/loading state
   still resets to non-submitting (button re-enabled, no permanently stuck
   "Creating...").
2. Same guarantee for the SpawnShell (`>shell ...`) branch.
3. Existing successful-path behavior (modal closes normally, no error shown) is
   unchanged when `onClose()` works correctly — no new error toast/flash appears on the
   happy path.
4. Failure path (`onCreateSession` throws) in both branches still surfaces the error
   message and re-enables the button, as it does today.
5. A regression test (Jest/RTL, colocated with existing Omnibar tests) covers: alias
   branch success path resets `isSubmitting` even when `onClose` is mocked as a no-op.
6. If a concrete root cause for `onClose()` not dismissing the modal is found, it is
   fixed or filed as a tracked follow-up with the specific mechanism named (not "mobile
   timing" left as a vague guess).
