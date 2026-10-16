# BUG-076: `TestCreateWorkflow_WebhookSecret_RoundTripsThroughHMACVerification` fails only in the full `server/services` suite, passes in isolation [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-15 (during PR #503's code-review-fix gate)
**Impact**: Intermittent CI noise on `server/services` — the failure is not reproducible on demand, which erodes trust in red CI for this package (see `.claude/rules/fix-flaky-tests-dont-defer.md`).

## Problem Description

Running `CI=true go test ./server/services/... ./session/tmux/...` (full package) produced:

```
[FAIL] TestCreateWorkflow_WebhookSecret_RoundTripsThroughHMACVerification
   workflow_service_test.go:750:
   Error: Received unexpected error:
```

(failure is at `require.NoError(t, err)` after `decryptWorkflowSecret(infra.cfg, wf)` — `server/services/workflow_service_test.go:749-750`)

Running the same test in isolation immediately after:

```
CI=true go test ./server/services/... -run TestCreateWorkflow_WebhookSecret_RoundTripsThroughHMACVerification -v
Go test: 1 passed in 1 packages
```

passed cleanly. This points to state leaking from another test in the `server/services` package into `newWebhookTestInfra(t)`'s setup or `decryptWorkflowSecret`'s encryption-key resolution (`infra.cfg`) — e.g. a shared config file, environment variable, or encryption key material mutated by a preceding test — rather than a bug in the webhook HMAC logic itself, which passes reliably alone.

Confirmed unrelated to PR #503's diff (that PR only touches `session/tmux/tmux.go` DEBUGTMP log removal and `docs/bugs/open/BUG-072-*.md` documentation) — filed per the blast-radius exception in `.claude/rules/fix-flaky-tests-dont-defer.md` rather than root-caused in that PR, since diagnosing shared config/key state across `server/services` test infra is out of scope for a tmux-logging cleanup.

## Fix Approach

- Bisect which preceding test(s) in the package mutate shared state that `newWebhookTestInfra`/`decryptWorkflowSecret` depend on (likely `infra.cfg`'s encryption key path or a shared temp config file — grep for global/package-level test fixtures touched by both this test and workflow/webhook-adjacent tests).
- Once identified, isolate the mutated resource per-test (e.g. `t.TempDir()`-scoped config instead of a shared one), following the same pattern used to fix `TestRemoveHooksConfig_should_StripOnlyTheNamedHook_When_MultipleHooksPresent` (see `server/services/hook_injector.go:60`).

## Related Tasks

Found during code review of PR #503. Not fixed in that PR — out of scope (unrelated workflow/webhook test infra vs. tmux logging cleanup).
