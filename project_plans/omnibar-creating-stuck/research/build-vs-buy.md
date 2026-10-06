# Build vs. Buy Research — Omnibar Stuck "Creating..." Modal

Scope: `handleSubmit` in `web-app/src/components/sessions/Omnibar.tsx`, lines 1017-1230.

## 1. How much of the 3 branches is actually shared boilerplate?

Read directly (`web-app/src/components/sessions/Omnibar.tsx:1037-1204`). Three branches call
`onCreateSession`:

| Branch | Lines | Has try/catch/finally? |
|---|---|---|
| SpawnShell | 1037-1061 | try/catch only — `setIsSubmitting(false)` is called **inside the catch**, missing on the success path's early `return` after `onClose()` (line 1055-1060) |
| Alias | 1072-1105 | Same shape — try/catch only, `setIsSubmitting(false)` only on the catch path (1095-1103) |
| Default | 1107-1203 | Full `try { ... } catch { ... } finally { setIsSubmitting(false) }` (1110, 1198, 1201) — this is the correct shape |

The shared boilerplate across all three is exactly 4 lines each:
```ts
setIsSubmitting(true);
setError(null);
try {
  await onCreateSession(sessionData);
  ...
} catch (err) {
  setError(err instanceof Error ? err.message : "Failed to create session");
  setIsSubmitting(false);  // present in all 3, but only as the catch-path reset
}
```
The bug is structural, not a typo: SpawnShell and Alias reset `isSubmitting` only in the
`catch` block, relying on `onClose()` synchronously unmounting the modal to make the
"stuck" `setIsSubmitting(false)` unnecessary on the success path. When `onClose()` doesn't
synchronously unmount (parent defers, or the modal stays mounted across a state update),
the button stays showing "Creating..." forever. The Default branch avoids this entirely by
using `finally`.

Everything else in each branch is **branch-specific and non-trivial**:
- SpawnShell: builds `sessionData` from `shellDir`/`shellCommand` detection metadata,
  calls `addRecentShellCommand`.
- Alias: builds `sessionData` from alias metadata (`aliasName`, `branch`, `extraFlags`),
  branch-name derivation logic (`useTitleAsBranch`).
- Default: ~90 lines — `new_project` vs. regular session split, autonomous-mode
  composition, GitHub URL resolution, the R2 path-confirmation dialog special case
  (`setPendingSessionData`/`setShowPathConfirmation`, which intentionally does NOT reset
  `isSubmitting` via `finally` — it sets it explicitly at line 1187 before an early
  `return`, bypassing the `finally` block's redundant-but-harmless second call), and
  history persistence (`saveHistory`).

So the *sessionData construction* differs completely per branch (three different shapes,
three different source data), and the *error-handling shell* is the only truly identical
part — call `setIsSubmitting(true)`/`setError(null)` before, `try/catch/finally` with
`setIsSubmitting(false)` in `finally`, `onClose()` on success. That shell is ~6 lines.

## 2. Is a shared helper worth doing now, or is it separate refactoring scope?

**Recommendation: patch in place, do not extract a helper in this PR.**

Reasons:
- The fix per broken branch is genuinely 1 line each: replace the catch-only
  `setIsSubmitting(false)` with a `finally` block (or restructure to match the Default
  branch's shape). That's a 2-6 line diff per branch, ~10-15 lines total, entirely inside
  the existing function.
- A `submitSession(sessionData, { onSuccessExtra })` extraction would need to parameterize:
  the success side-effect (`addRecentShellCommand` for SpawnShell vs. `saveHistory` for
  Default vs. nothing for Alias), the special-case R2 path-confirmation early-return in the
  Default branch (which deliberately skips `onClose()`/full-finally semantics), and the
  differing `sessionData` construction — none of which is boilerplate, so the "shared
  helper" would end up being a thin `try/finally` wrapper taking a callback, which doesn't
  meaningfully reduce the ~90-line Default branch and only saves ~4 lines each on the other
  two.
- This repo's own `.claude/rules/interface-pollution-checklist.md` calls out "unjustified
  generic ... used at a single call site that a concrete type or a plain loop would express
  more clearly" and prefers waiting for a second/third real use before generalizing. Here
  there are 3 call sites, but they're 3 branches of one function already co-located — the
  dedup value of a helper is marginal since a reviewer scanning `handleSubmit` top-to-bottom
  already sees all 3 shapes adjacent to each other for comparison.
- Requirements doc classifies this as Complexity-1 / quick task. Introducing a new
  `submitSession` helper would touch the same function bodies anyway (to call the helper
  instead of inlining) plus add a new function signature, tests for the helper in isolation,
  and a new indirection layer — expanding the diff and review surface disproportionate to
  fixing 2 missing `finally` blocks.
- Extracting now, before there's a second bug-driven reason to touch this code, is exactly
  the kind of speculative refactor the interface-pollution checklist and this repo's
  general minimal-diff bias argue against doing inside a bug-fix PR. If a 4th
  session-creation branch is added later (see `.claude/rules/session-creation-registry.md`),
  *that* PR is a better moment to extract the shared shell, once there's a concrete third
  or fourth caller motivating it.

Net: fix the two branches to use `try/catch/finally` matching the Default branch's proven
shape, plus the defense-in-depth reset in the modal's `!isOpen` effect the requirements doc
calls for. Leave deduplication as a follow-up, not blocking this fix.

## 3. Third-party library / existing hook check

No third-party library or SaaS applies — this is local React state (`useState` +
`useCallback` inside a single component), not a workflow orchestration or async-state
problem large enough to justify a dependency (e.g. TanStack Query mutations, XState). Pulling
in a state-machine library for one boolean flag would be disproportionate.

**No existing shared hook for this pattern exists in the codebase.** Searched
`web-app/src` for `useAsyncAction`, `useAsync`, `useSubmitState`, `useIsSubmitting`, and
`isSubmitting` generally:

```
grep -rniE "useAsyncAction|useAsync\b|useSubmitState|useIsSubmitting|isSubmitting" web-app/src --include="*.ts" --include="*.tsx" -l
```
Every hit is a component maintaining its own local `const [isSubmitting, setIsSubmitting] = useState(false)`:
- `web-app/src/components/sessions/CheckpointButton.tsx`
- `web-app/src/components/sessions/NewShellDialog.tsx`
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx`
- `web-app/src/components/sessions/Omnibar.tsx`
- `web-app/src/components/sessions/ResumeSessionModal.tsx`
- `web-app/src/components/sessions/SessionWizard.tsx`

`web-app/src/lib/hooks/` (85+ files) has no `useAsyncAction`, `useAsyncSubmit`, or similar
generic async-mutation-state hook — every submit-button component in this codebase
independently reimplements `useState(false)` + manual `setIsSubmitting` calls around its
async call. This is a pre-existing, repo-wide pattern, not something unique to Omnibar.

This means: (a) there's nothing existing to reuse for this fix, confirming the in-place
patch is the right scope for *this* PR, and (b) the *same* missing-`finally`-block class of
bug could plausibly exist in `ResumeSessionModal.tsx`, `NewShellDialog.tsx`, or
`SessionWizard.tsx` — worth a quick follow-up check (out of scope here per this task's
Complexity-1 sizing), and a stronger argument for eventually extracting a shared
`useAsyncSubmit`-style hook repo-wide rather than one-off inside `Omnibar.tsx` alone, once
that pattern needs fixing in a second file.

## Summary

1. Shared boilerplate across the 3 branches is ~4-6 lines (`setIsSubmitting`/try/catch/
   finally shell); everything else (session-data construction, side effects) is genuinely
   branch-specific.
2. Do the minimal in-place fix (match the Default branch's `try/catch/finally` shape in the
   other two branches, plus the `!isOpen` defense-in-depth reset) — a shared helper is not
   worth it in this PR given Complexity-1 sizing and the interface-pollution bias toward
   non-speculative changes; revisit if/when a 4th branch or a second broken file appears.
3. No third-party library applies. No existing `useAsyncAction`-style hook exists anywhere
   in `web-app/src` — every submit component (`NewShellDialog`, `ResumeSessionModal`,
   `SessionWizard`, `Omnibar`) reimplements local `isSubmitting` state independently, so
   there's nothing to reuse now, though it's a candidate for a future repo-wide extraction.
