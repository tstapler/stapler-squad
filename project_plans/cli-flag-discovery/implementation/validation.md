# Validation: cli-flag-discovery

Coverage map derived from `plan.md` (task IDs refer to it). No fake `CommandExecutor` and no sleeping-script binaries: the prober uses injected function fakes; the runner uses a re-exec'd test binary through the unexported `runWith` seam (plan Task 1.1.2b-c).

| AC | Proof | Where |
|---|---|---|
| 1 | Handler test with a real `Prober` and fake `lookPath`/`run` returning aider fixture text; handler tests with fake prober; `found` derivation table test | 1.2.2c, 1.2.2e |
| 2 | Prober test with `WithHome("/home/tyler")`: env prefix skipped, `~` expanded, `NOT_FOUND` not an RPC error, nothing cached | 1.1.1b, 1.1.4c |
| 3 | Runner via helper modes `bigout`, `hang`, `flood`, `orphan` (`runWith`, limits 80ms/256KiB); `DefaultLimits()` asserts 3s/256KiB; prober fake-`stat`/`run` tests for cache by path+mtime, TTLs, singleflight, BUSY, leader-only semaphore (10 concurrent same-binary probes all real), cancelled callers do not free slots, flight exits after all callers cancel (goleak/Eventually), cancel-then-reprobe, limits plumbing | 1.1.2d, 1.1.4c |
| 4 | Parser golden fixtures (claude, aider, gh, gh-pr-list, rg, uv, gemini, agy; negatives git, tmux), aliases, fuzz, adversarial 256KB input | 1.1.3c-d |
| 5 | Jest: `useProbeProgram` (stale, transport, `PermissionDenied`/403 -> "Couldn't check" never "not found", ERROR/BUSY), `ProbeStatusBadge` variants, `ProgramsManager` blur/Enter/Check; Playwright e2e | 2.1.1b, 2.1.2b, 2.2.1b, 5.1b |
| 6 | Jest: `flagTokens`, `FlagCombobox` (ARIA, same-node focus preservation), `validateFlags`, warning wiring; wrapper suppression | 3.1.1b, 3.1.2b, 4.1.1b, 4.1.2a |
| 7 (scoped down: saved `cli_flags`; `extraFlags` not validated) | Jest `ProgramProbeSection` (badge, saved-flag warning, no duplicate warning); panel diff limited to one element | 2.3.2c, 4.1.2b |
| 8 | Resolve table (relative paths, literal `claude;`); prober tests (dir, non-exec, world-writable); runner `printenv` (poisoned `runSpec.parentEnv`, through real `runWith`)/`sid`/cwd/`helpSpec` args/`probeEnv` no-leak; `ProbeGuard` tests (method, Host, Origin, non-loopback bind, lazy origins, real Connect handler); per-listener chain tests (`Start()` chain has guard and 403s wrong Host on :8543; `StartRemote()` chain has none); audit-line-per-call slog capture; e2e wrong-Host 403 | 1.1.1b, 1.1.2d, 1.1.4c, 1.2.3b-c, 5.1b |
| 9 | Playwright with `hasTouch`, 375x667, tap on `prog-flag-info-button`, `aria-expanded`, 44x44 bounding box. Jest `FlagInfoButton` is a unit substitute only | 5.1b (proof), 4.2.1b |
| 10 | `go test ./config/clihelp ./server/services ./server/middleware -race`; jest patterns per plan AC10; `make ready`; `pnpm run lint:duplicates` | 5.2.1a |

## Pre-mortem (risks and where covered)
- Hanging or noisy binaries: `hang`/`flood`/`orphan` runner tests, early kill, `WaitDelay=200ms`.
- Cancelled or busy probe poisoning the cache: cancel-then-reprobe and BUSY-not-cached tests.
- False "not found" from PATH mismatch: login-shell PATH derivation test (1.1.4e: failure not cached, `$SHELL` unset fallback, bash early-return, background-helper shell) plus alias-only NOT_FOUND with explanatory copy; guard 403 shown as "Couldn't check".
- Wrapper false warnings: wrapper skip tests and the built-in Proxy entry case.
- Parser false positives: warn-only, false-negative bias, zero-flags suppression.
- Unauthenticated listener abuse: target rules plus `ProbeGuard`; owner sign-off PENDING (ADR-001).
- proto `gen/` output not committed (gitignored); jscpd ratchet (shared mock, `useListboxNav` extraction, no new reference doc).
