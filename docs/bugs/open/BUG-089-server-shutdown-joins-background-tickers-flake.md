# BUG-089: `TestServer_Shutdown_JoinsBackgroundTickers` flakes when run alongside the full `server` package suite [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-23
**Impact**: Intermittent CI failure in the `server` package's test suite — no production impact. Not caused by, or related to, PR #605's diff; this test is pre-existing code from `main` (merged into the PR #605 branch via a `main`-sync merge), not something that PR added or touched.

## Problem Description

`TestServer_Shutdown_JoinsBackgroundTickers` (in the `server` package — exact file not yet identified, found via `go test ./server/... -run 'TestServer_Shutdown_JoinsBackgroundTickers'` failing once) failed once when run as part of the full `go test ./server/... ./testutil/... -count=1` suite, but passed cleanly 20/20 times in isolation (`go test ./server/... -run 'TestServer_Shutdown_JoinsBackgroundTickers' -race -count=20`). This is the same *symptom shape* as BUG-087 (a test that's flaky only in the presence of other concurrently-running tests in the same package, not on its own) — likely another instance of shared-state interference (global logger, global config, or timing-sensitive ticker/shutdown-ordering assertions racing against unrelated tests in the same binary), though the specific mechanism hasn't been root-caused yet.

## Reproduction Steps

1. Run the full package: `go test ./server/... ./testutil/... -count=1`
2. Occasionally (observed once), `TestServer_Shutdown_JoinsBackgroundTickers` fails.
3. Run it in isolation: `go test ./server/... -run 'TestServer_Shutdown_JoinsBackgroundTickers' -race -count=20` — passes every time.

## Root Cause

Not yet investigated in depth — only confirmed non-reproducible in isolation, which points at cross-test interference (shared global state, or ordering/timing sensitivity under the full suite's concurrency) rather than a bug in the test's own logic. Needs the same kind of investigation BUG-087 received (identify what global/shared resource the test and some other concurrently-running test both touch).

## Files Likely Affected

- The `server` package file containing `TestServer_Shutdown_JoinsBackgroundTickers` (not yet located — `grep -rn "TestServer_Shutdown_JoinsBackgroundTickers" server/` will find it).

## Fix Approach

Unknown pending investigation — likely the same class of fix as BUG-087 (remove shared global-state mutation from the test, or isolate it properly), once the specific interfering test/resource is identified.

## Verification

After the fix: run `go test ./server/... -count=20` (full package, not just the isolated test) with no failures involving `TestServer_Shutdown_JoinsBackgroundTickers`.

## Related Tasks

Discovered while running PR #605's (`stapler-squad-web-transport` branch, project `web-transport-architecture-review`) local test gate, immediately after merging latest `main` into the branch (this test is `main`'s code, not PR #605's). See also `docs/bugs/open/BUG-087-captureLogs-global-slog-swap-races-under-t-parallel.md` for the same symptom shape found in the same investigation, and `docs/bugs/open/BUG-088-credential-chain-bypasses-test-dir-isolation.md` for a related test-isolation gap.
