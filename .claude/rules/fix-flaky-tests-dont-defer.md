# Fix Flaky Tests When You Find Them — Don't Just Defer Them

When a test fails intermittently and you recognize it as "a known pre-existing flake," root-cause and fix it in the same session, or file it as a tracked bug immediately — don't just note it as unrelated and move past it again.

**Wrong:**
```
FAIL	github.com/tstapler/stapler-squad/server/services	84.844s
--- FAIL: TestRemoveHooksConfig_should_StripOnlyTheNamedHook_When_MultipleHooksPresent (0.00s)
```
> "Known pre-existing flake, unrelated to this diff — re-ran the job, it passed, moving on."

**Right:**
```
FAIL	github.com/tstapler/stapler-squad/server/services	84.844s
--- FAIL: TestRemoveHooksConfig_should_StripOnlyTheNamedHook_When_MultipleHooksPresent (0.00s)
```
> Confirmed unrelated to the current diff (re-run passed), but still root-caused and fixed
> before moving on — filed as its own fix if it can't be resolved in the same sitting, not
> silently re-excused the next time it appears.

## Why

This repo's own audit history (`docs/tasks/backlog-feature-improvement.md`) has repeatedly
named "known pre-existing flake, confirmed unrelated" as an acceptable reason to route around
a failure rather than fix it. `session/tmux`'s `TestEnsureServerRunning_NoOp` and
`TestKillOrphanedControlModeClients` were cited this way across multiple unrelated PRs without
being fixed. Every re-excusal is a missed root-cause opportunity, and it erodes the value of CI
red as a signal — a reviewer who sees "known flake, unrelated" stops checking whether that's
still true.

`server/services`' `TestRemoveHooksConfig_should_StripOnlyTheNamedHook_When_MultipleHooksPresent`
is the worked example of doing this right: after being re-excused across several PRs, it was
root-caused (a strict-prefix URL match bug in `hookCommandReferencesURL`, see the doc comment
at `server/services/hook_injector.go:60`) and fixed with a deterministic regression test
(`Test_hookCommandReferencesURL_should_NotMatchWhenURLIsAStrictPrefixOfAnother` in
`server/services/hook_injector_test.go`) instead of being deferred again.

## How to apply

- If you can root-cause and fix it in the current session (most flakes are a missing
  synchronization point, shared mutable state, or a fixed/non-unique temp resource name — see
  `TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON`'s doc
  comment in `server/services/hook_injector_test.go` for another worked example), fix it.
- If it's out of scope for the current change, don't just move on — file it as its own bug
  (or route it to `sdd:fix-bug`) so it gets picked up on its own, rather than letting the next
  person re-discover and re-excuse the same flake.
- Only exception: when fixing it now would meaningfully expand the current change's blast
  radius (e.g. it requires refactoring shared test infrastructure other in-flight PRs also
  touch) — say so explicitly and file the follow-up immediately, don't let "later" mean
  "never."
