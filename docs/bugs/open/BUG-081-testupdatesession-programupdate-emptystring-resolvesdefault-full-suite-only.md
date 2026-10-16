# BUG-081: `TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault` fails only in the full `server/services` suite, passes in isolation [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-20 (during PR #538's code-review-fix gate, `dynamic-rule-reload`)
**Impact**: Intermittent CI noise on `server/services` — the failure is not reproducible on
demand, which erodes trust in red CI for this package (same class as BUG-076, see
`.claude/rules/fix-flaky-tests-dont-defer.md`).

## Problem Description

Running the full package (`go test ./server/... ./pkg/classifier/...`) occasionally produces:

```
--- FAIL: TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault (0.01s)
    session_service_program_test.go:184:
        Error:      Should NOT be empty, but was
        Test:       TestUpdateSession_ProgramUpdate_EmptyString_ResolvesDefault
        Messages:   test assumption: a default program must be configured
```

The failing assertion (`server/services/session_service_program_test.go:184`) is:

```go
defaultProgram := config.LoadConfig().DefaultProgram
assert.NotEmpty(t, defaultProgram, "test assumption: a default program must be configured")
```

`config.LoadConfig()` (`config/config.go:849`) re-reads `config.json` from `GetConfigDir()` on
every call — no in-memory cache — so an empty `DefaultProgram` here means either (a) another
test wrote a fresh `DefaultConfig()` to the same shared config path underneath this test, or
(b) `GetConfigDir()` resolved to a different directory than the one another concurrent/prior
test populated (e.g. a `t.Setenv`-driven `STAPLER_SQUAD_TEST_DIR`/`HOME`/instance-scoped path
that isn't as test-isolated as assumed).

Re-running the same test in isolation immediately after passed cleanly, and re-running the
full `server/services` suite 3 more times back-to-back reproduced it 0/3 times — consistent
with state leaking from another test's config-path/env-var setup rather than a logic bug in
program-default resolution itself.

Confirmed unrelated to PR #538's diff (`dynamic-rule-reload` — fsnotify watcher event-mask
fix, MCP handler test coverage, project-dir-created-late retry, and claude-settings `Deny`
rule enforcement; none of those touch `config.LoadConfig`, `DefaultProgram`, or program
resolution) — verified by running `go test ./server/services/...` on the pre-fix commit
(`a2941af3f`) which passed cleanly, and by this test passing consistently in isolation both
before and after the fix commits. Filed per the blast-radius exception in
`.claude/rules/fix-flaky-tests-dont-defer.md` rather than root-caused here, since bisecting
shared config-path/env-var state across the full `server/services` suite is out of scope for
a rules-hot-reload feature.

## Fix Approach

Same approach as BUG-076 (which diagnosed the equivalent leak for
`TestCreateWorkflow_WebhookSecret_RoundTripsThroughHMACVerification`'s encryption-key
resolution):

- Bisect which preceding test(s) in `server/services` mutate the shared config path/env vars
  (`STAPLER_SQUAD_TEST_DIR`, `HOME`, `STAPLER_SQUAD_INSTANCE`) that `config.LoadConfig()`
  depends on, particularly any test using `t.Parallel()` alongside `t.Setenv` on those vars.
- Once identified, isolate the mutated resource per-test (e.g. inject an explicit config path
  instead of relying on process-global env vars + shared `GetConfigDir()`), following the
  precedent set by BUG-076 and `TestRemoveHooksConfig_should_StripOnlyTheNamedHook_When_
  MultipleHooksPresent`'s fix (`server/services/hook_injector.go:60`).

## Related Tasks

Found during Phase 7 code-review-fix verification of PR #538 (`dynamic-rule-reload`). Not
fixed in that PR — out of scope (unrelated config-loading test infra vs. a claude-settings
rule-reload feature).
