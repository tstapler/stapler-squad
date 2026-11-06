# Pitfalls & Risks: fix-benchmark-ci-failures

Research date: 2026-11-04

## 1. Pinning `ubuntu-24.04`

**Risk level: Low — but watch the deprecation timeline**

GitHub guarantees `ubuntu-24.04` as an explicitly versioned label (not a floating alias), so it will not silently migrate the way `ubuntu-latest` did when it jumped from Ubuntu 22 → 26 on 2026-10-19. GitHub's policy is to give 60 days' notice before removing a pinned runner image, so there is no immediate cliff.

Practical pitfalls:
- When GitHub eventually retires `ubuntu-24.04` (likely once Ubuntu 28 ships as LTS), all pinned jobs will need an update — the same kind of change needed now. This is expected and manageable; it is not a reason to avoid pinning.
- `ubuntu-24.04` ships Chromium/Playwright dependencies from the Ubuntu 24 apt pool. Those versions may differ from the ones available on whatever `ubuntu-latest` resolves to in the future. The risk is that a future Playwright version breaks compatibility with `ubuntu-24.04`'s system libs — but that is a narrower, traceable failure mode compared to the current silent OS migration.
- The three Playwright-dependent jobs (`e2e-latency`, `frontend-throughput`, `e2e-perf`) already work around apt-mirror instability with a 3-attempt, 8-minute-per-attempt retry loop. That loop continues to protect against transient network flakes regardless of the pinned image.

**No action needed beyond the pin itself.** Add a calendar note to revisit when Ubuntu 26 LTS ships (expected ~April 2026 — already shipped; Ubuntu 28 is the next LTS after that).

## 2. Adding buf/ent-gen to Tier 2

**Risk level: Medium — runtime cost must stay within the 50-minute cap**

Current state: `go-tier2` has no code-gen steps. It runs `go test -bench=. -run='^$' ./... -timeout=30m` and then saves the baseline. If `./...` tries to compile packages that import generated code (proto stubs, ent ORM), the build silently fails partway through the pipeline because there is no `set -o pipefail` and `tee` swallows the non-zero exit code.

**buf generate risks:**
- The workflow pins buf at `v1.41.0` (curl from GitHub releases). That binary is already downloaded fresh each job run — there is no caching of the buf binary itself. On a cold runner the download adds ~5 seconds; on a warm cache it is repeated anyway. No version mismatch risk as long as `buf.gen.go-only.yaml` stays compatible with v1.41.0.
- buf does **not** require network access after the binary is installed: it reads local `.proto` files and generates output entirely offline. No proxy or external registry is involved.
- The go-only template (`buf.gen.go-only.yaml`) is used by `go-tier1` and `e2e-latency` already — it's proven and skips the web-app `protoc-gen-es` plugin that requires bun/node, so it is safe to add to `go-tier2` without installing bun.

**ent generate risks:**
- `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` does **require module download** (`-mod=mod`) on a cold cache. `actions/setup-go` with `cache: true` caches the Go module download cache, so subsequent runs are fast — but on a first post-change run, `entgo.io/ent` needs to be fetched. That is consistent with `go-tier1` and `e2e-latency`, which already do the same step.
- The ent generate step itself takes ~10-20 seconds (the Go toolchain builds the codegen binary the first time). This is well within the 50-minute cap; `go-tier1` already does it inside a 20-minute cap with no issue.
- **Generated code must not be committed** (`.gitignore` excludes `session/ent/*.go` other than `schema/`). The generate step runs at job start and writes to the working tree; that is fine since the working tree is ephemeral on a runner.

**Timing impact estimate:** Adding buf (10s) + ent-gen (20s) = ~30 seconds extra. The 30-minute `go test` timeout and 50-minute job timeout remain far from exhaustion.

## 3. Terminal-throughput timeout tuning (30s `waitForTestCompletion`)

**Risk level: Medium — a too-generous timeout can mask real regressions**

The spec passes `30_000` ms to `waitForTestCompletion` on line 64. The test then runs 10 times in a loop, each with its own 30-second ceiling, inside a 300-second total test timeout. On a slow Ubuntu 26 runner the canvas renderer (GPU disabled in CI headless mode) can be 2–3× slower than local WebGL runs.

Known failure modes on headless CI:
- **CPU throttling by the hypervisor.** GitHub-hosted runners share physical CPU. A bursty neighbor can extend a 100 KB render from ~3s to 15–25s. The 30s per-run timeout provides some headroom, but back-to-back slow runs can push total wall time past 300s and trip the per-test `test.setTimeout`.
- **Canvas renderer path is slower than WebGL.** The spec acknowledges this in its header comment. The 30s timeout was sized for canvas-renderer CI runs; if Ubuntu 26 runners have meaningfully lower CPU throughput than Ubuntu 24 runners, the existing 30s may not be sufficient.
- **Masking regressions.** Raising `waitForTestCompletion` to, say, 60s means a genuinely slow render (e.g. a 5× throughput regression) still passes the timeout — the regression only shows up in the benchmark numbers, not as a timeout failure. That is acceptable because the spec already compares numbers against a baseline with `fail-on-regression: 50`; a timeout failure before numbers are captured is strictly worse (no data, no comparison, just a red CI job with no actionable signal). Therefore, **raising the timeout is the right trade-off** — it trades a noisy hard failure for a soft regression signal.
- **Re-runs inflate variance.** Each re-navigation (`await setupStressTestPage(page)`) re-runs the onboarding-tour init script, re-initializes xterm.js, and re-JITs the renderer. On a cold runner the first few runs are slower; the spec already discards `WARMUP_RUNS = 2`, which covers this.

**Recommended ceiling:** 60s per run is a safe upper bound for a 100 KB payload on canvas CI. At 10 runs × 60s = 600s maximum, the per-test `test.setTimeout(300_000)` (5 min) would need to be raised to at least 650s (11 min) to avoid the outer timeout becoming the new failure mode. The job's 45-minute timeout is not affected.

## 4. `set -o pipefail` in Tier 2

**Risk level: Low — one real gotcha with `go test ... ./... | tee`**

The current `go-tier2` run step uses:
```bash
go test \
  -bench=. \
  -run='^$' \
  -benchmem \
  -count=8 \
  -timeout=30m \
  ./... | tee tier2-bench.txt
```

Without `set -o pipefail`, if `go test` exits non-zero (e.g. a build failure because generated code is missing), `tee` exits 0 and the entire pipeline reports success. This is the root cause of the "silent build failure" described in the requirements.

**Adding `set -o pipefail`:**
- In bash, `set -o pipefail` makes the pipeline exit with the rightmost non-zero exit code, so a `go test` failure propagates through `tee`.
- **Gotcha:** GitHub Actions run `run:` steps under `/bin/sh` by default, not bash. `set -o pipefail` is a **bash-ism**; POSIX sh ignores or rejects it. The fix requires either (a) adding `shell: bash` to the step, or (b) prepending `#!/bin/bash` as a shebang — but (b) doesn't work in GHA `run:` steps. The `go-tier1` job already uses `set -o pipefail` correctly (line 85 of `benchmark.yml`) with no explicit `shell:` override, which means GitHub Actions is already running those steps as bash on ubuntu runners (GHA defaults to `bash` on Linux runners, not `/bin/sh`, despite the docs sometimes saying sh). Verify this assumption: `go-tier1` currently passes, confirming bash is in use. Adding `set -o pipefail` to `go-tier2` will work the same way.
- **Second gotcha:** `tee` itself exits 0 when the write succeeds. With `pipefail`, the pipeline's exit code is the highest non-zero exit code from any command — `go test`'s non-zero will propagate. This is the desired behavior.
- **Third gotcha:** Some packages in `./...` may legitimately have no benchmarks. `go test -bench=. ./some/pkg` exits 0 even when no benchmarks match; this is not affected by `pipefail`.
- **No risk of false positives** from `pipefail` for packages that simply have no tests: `go test` exits 0 for those as well.

## Summary

| Fix | Risk | Key concern |
|---|---|---|
| Pin `ubuntu-24.04` | Low | Will need updating when Ubuntu 28 ships; that is expected and manageable |
| Add buf/ent-gen to Tier 2 | Medium | Adds ~30s to job runtime; ent-gen needs network on cold cache; both are already proven in `go-tier1` |
| Raise `waitForTestCompletion` timeout | Medium | A higher per-run timeout requires raising `test.setTimeout` proportionally to avoid the outer timeout becoming the new failure mode |
| `set -o pipefail` in Tier 2 | Low | Must confirm `shell: bash` semantics — already works in `go-tier1` without an explicit `shell:` because GHA Linux runners default to bash |
