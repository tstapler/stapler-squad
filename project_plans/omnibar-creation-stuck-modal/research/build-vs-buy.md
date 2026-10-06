# Build vs. Buy — omnibar-creation-stuck-modal

## Question

Should the fix for the stuck-loading-modal bug in `Omnibar.tsx`'s `handleSubmit` reach for an
existing async-state library, or is a local hand-fix (try/finally, copied from the file's own
third branch) correct?

## What's actually broken

`handleSubmit` (`web-app/src/components/sessions/Omnibar.tsx`) has three session-creation
branches, confirmed by reading lines 1005–1169:

1. **One-off/terminal branch** (`web-app/src/components/sessions/Omnibar.tsx:1016-1026`) —
   `setIsSubmitting(true)` → `try { await onCreateSession(...); onClose(); } catch { setError(...); setIsSubmitting(false); }`.
   `setIsSubmitting(false)` only runs in the `catch`. The success path never resets it —
   it relies entirely on `onClose()` unmounting the Omnibar so the stale `isSubmitting=true`
   state stops mattering.
2. **Alias-invocation branch** (`web-app/src/components/sessions/Omnibar.tsx:1061-1070`) —
   identical shape/bug to branch 1.
3. **Default branch** (`web-app/src/components/sessions/Omnibar.tsx:1073-1169`) — the
   already-correct reference implementation: `try { ... } catch (err) { setError(...); }
   finally { setIsSubmitting(false); }`. This is the exact pattern branches 1 and 2 are
   missing, already proven in the same file.

Root cause of the *user-visible* stuck modal: when `onClose()` doesn't actually dismiss (e.g.
a parent re-render races the close, or the close handler itself throws/no-ops under some
condition), branches 1 and 2 have no fallback path to clear `isSubmitting` — nothing else in
the component ever will. Branch 3 doesn't have this failure mode because `finally` clears the
flag unconditionally regardless of what `onClose()` does.

## 1. Existing OSS library check

Checked `web-app/package.json` `dependencies` + `devDependencies` for anything that already
subsumes "async action state machine" (loading/error/success flags around an async call):

```
$ python3 -c "... filter deps for async/query/swr/use-/react-use/ahooks/effect/rxjs ..."
(no output — zero matches)
```

No `react-use`, `swr`, `@tanstack/react-query`/`react-query`, `ahooks`, `rxjs`, or similar
already in the dependency tree. The project's existing pattern for this exact problem is
already hand-rolled `useState` + `try/catch/finally`, used consistently across ~30+ hooks in
`web-app/src/lib/hooks/` (`useApprovalRules.ts`, `useWorkflows.ts`, `useBacklogService.ts`,
etc. — none of them import an async-state library either). There is no existing
"useAsync"/"useAsyncFn"-style hook in `web-app/src/lib/hooks/` to reuse; the closest thing —
`isSubmitting` + `error` state — is inlined per-component/hook throughout the codebase, and
`Omnibar.tsx` itself already has the correct version of the exact pattern needed, three times
over (lines 1018, 1063, 1076, plus a fourth instance at line 1550-1559 inside a nested
callback, also using proper try/finally).

## 2. Is a bespoke 2–3 line try/finally fix appropriate here?

Yes. This is not a case of inventing a new pattern — it's applying a pattern the file already
uses correctly in two other places (three, counting line 1550) to the two places that are
missing it. The fix is:

- Wrap the `await onCreateSession(sessionData)` calls in branches 1 and 2 with the same
  `try { ... } catch (err) { setError(...); } finally { setIsSubmitting(false); }` shape
  branch 3 already has.
- No new abstraction, no new dependency, no new hook needed — copying an in-file, in-repo,
  already-reviewed pattern verbatim is the textbook case for "just fix it locally" per
  `.claude/rules/interface-pollution-checklist.md`'s smell #5 (unjustified generic/abstraction
  for a problem a concrete fix already solves) and smell #4 (don't introduce a
  wrapper/hook layer with no added behavior over what's already there).

Introducing `react-use`'s `useAsyncFn` (or similar) here would mean: adding a new dependency,
rewriting all three branches (not just the two broken ones) to fit that hook's API, and
absorbing whatever behavioral differences it has from the current `isSubmitting`/`error`
state pair (which also drives UI beyond just the loading spinner — e.g. `setError` messages
shown inline). That's a disproportionate blast radius for a 2-line-per-branch fix, and it
doesn't address the actual root cause anyway — see below.

## 3. Root-cause note: `finally` alone isn't the whole story

Adding `finally` to branches 1 and 2 fixes the *state-desync* failure mode (loading flag never
clears on success). But per the bug summary, part of the fix scope is also investigating *why*
`onClose()` sometimes doesn't dismiss the modal after a successful creation — that's a
separate, real root cause (something in the close/dismiss path, not the async-state pattern)
and needs its own fix regardless of whether `isSubmitting` is correctly reset. A library
wouldn't touch that half of the bug at all — it's UI-dismissal logic, not async-state
management. This confirms the fix is inherently local: part state-machine hygiene (mechanical,
copy the in-file pattern), part app-specific modal-lifecycle bug (nothing a generic library
addresses).

## 4. Anything to fork/adapt?

No. There's no third-party code involved in either half of the fix (the try/finally state
hygiene, or the `onClose()` dismissal investigation). Nothing to fork or vendor.

## Verdict

**Local hand-fix, no new dependency.** Confirmed by checking `package.json` (no async-state
library present) and `web-app/src/lib/hooks/` (no existing `useAsync`-style hook to reuse
instead). The correct pattern already exists in the same file
(`web-app/src/components/sessions/Omnibar.tsx:1076-1169`) and should be copied verbatim into
the two branches missing it (`:1016-1026` and `:1061-1070`). Adopting a library here would add
a dependency, force a rewrite of an already-correct third branch, and still leave the
`onClose()`-doesn't-dismiss root cause unaddressed since that's app-specific modal logic, not
generic async-state management.
