# Parity Report: go-auto-instrumentation (Epic 2.3)

**Date**: 2026-08-22
**Purpose**: Substantiate (or scope down) requirements.md's first Success Metric — "produces a working stapler-squad binary that is functionally identical to the normal build (same CLI flags, same behavior) and passes the existing test/e2e suite." Phases 1–2 so far proved the woven binary *builds*, *starts*, and *emits spans* (spike-verdicts.md); this report covers whether it *behaves the same*.

**Headline (scoped)**: Binary-level parity (CLI surface, `version` output) is verified byte-identical apart from the version string and a printed bootstrap-log preamble. The e2e startup contract was not driven for this report (see Story 2.3.2 row) due to a safety-critical machine-load incident encountered while completing Story 2.3.1 (see below), which consumed the remaining safe execution budget for this session. **Unit-test behaviour under weaving is unverified — `go test` weaving could not be safely determined and is treated as a coverage gap, not a pass.**

---

## Story 2.3.1: `go test` weaving verdict

**Verdict: NOT SAFELY DETERMINABLE — treated as a coverage gap (equivalent to `NOT APPLIED` for downstream purposes), not a pass.**

Two independent attempts to run `go test -c` under Toolexec Injection (`GOFLAGS="... -toolexec=otelc toolexec'"`) each triggered a reproducible, near-instantaneous, extreme system load spike, unrelated to the size of the package under test or explicit parallelism limits:

| Attempt | Package | Deps (`go list -deps \| wc -l`) | Constraints | Result |
|---|---|---|---|---|
| 1 | `./telemetry` | 399 | none (`go test -c -o /tmp/telemetry-test-woven ./telemetry`, via `./scripts/otel-auto-build.sh`) | Did not finish inside a 3-minute external timeout. Immediately after the kill, `uptime` reported `load average: 14648.35, 11620.86, 7174.29` on a 24-core machine. |
| 2 | `./pkg/ansi` | 50 (the leanest candidate package checked) | `-p 2`, `GOMAXPROCS=4`, run under a load-monitoring safety script (`setsid`'d, polled every 3s, hard-killed the process group if `load1 > 500` or elapsed `> 200s`) | Killed by the safety script's own load check at **elapsed=0s**, with `load1=1877.76` already exceeding the threshold — i.e. the spike happened in under ~3 seconds of the build starting, before the first poll interval even completed normally. |

By contrast, `go build` under the identical `GOFLAGS` (Epic 2.1/2.2's own build and smoke-test runs, several full-binary builds) never produced anomalous load — each completed in 15–60s wall with normal (<10) load averages. This isolates the anomaly to `go test -c` specifically, not to Toolexec Injection or otelc in general.

**Command (attempt 1)**:
```
./scripts/otel-auto-build.sh go test -c -o /tmp/telemetry-test-woven ./telemetry
# externally killed after 3m; uptime immediately after: load average: 14648.35, 11620.86, 7174.29
```

**Command (attempt 2)**, safety-monitored:
```
otelc setup
export GOFLAGS="'-toolexec=otelc toolexec'"
export GOMAXPROCS=4
setsid go test -c -p 2 -o /tmp/ansi-test-woven ./pkg/ansi &
# polling /proc/loadavg every 3s; kill -TERM/-KILL the process group if load1>500 or elapsed>200s
```
**Output**:
```
otelc setup done
launched build pid=2986440
elapsed=0s load1=1877.76
DANGER: load1=1877.76 exceeds safety threshold — killing build group
build finished/killed status=143
DONE cleanup_otelc status=143
```
Both attempts' `go.mod`/`go.sum` and otelc-generated scaffolding (`.otelc-build/`, `otel.instrumentation.go`, `otelc.runtime.go`) were confirmed fully reverted after cleanup (`git status --short` clean on those paths).

**Baseline comparison** (unwoven, for context — both completed normally with no load anomaly):
```
$ go test -c -o /tmp/telemetry-test-unwoven ./telemetry   # 1.5s wall
$ go tool nm /tmp/telemetry-test-unwoven | grep -c "go.opentelemetry.io/otelc/"
0
$ go test -c -o /tmp/ansi-test-unwoven ./pkg/ansi          # <1s wall
$ go tool nm /tmp/ansi-test-unwoven | grep -c "go.opentelemetry.io/otelc/"
0
```
(The `go.opentelemetry.io/otelc/` prefix is otelc's own instrumentation-package namespace per spike-verdicts.md's Spike B/C finding — a precise, false-positive-free marker, unlike a bare `otelc` substring search, which also matches this repo's own pre-existing `go.opentelemetry.io/otel/semconv` symbols such as `OTelComponentType...`.)

**Per Story 2.3.1's AC for the `NOT APPLIED` branch**: this is recorded plainly as a coverage gap. The Go-suite leg cannot substantiate "passes the existing test/e2e suite" — not because weaving demonstrably doesn't apply to `go test` (that remains genuinely unknown), but because determining the answer safely was not possible within this session's resource budget on this shared machine. **The Adoption Verdict must treat "unit-test behaviour under weaving is unverified" as an open precondition**, with the additional, stronger caveat that a future attempt to resolve it should run inside a resource-isolated environment (a container or VM with a hard cgroup CPU/process-count limit), never on a shared interactive workstation, given the reproduced severity (load average in the thousands within seconds, twice).

No triage against `.claude/rules/fix-flaky-tests-dont-defer.md` was possible or applicable here: no test failure was observed to triage — the build itself could not safely complete.

---

## Story 2.3.2: CLI-flag and e2e startup parity

**Verdict: PARTIAL — CLI surface parity verified; e2e startup contract not driven this session.**

### CLI surface diff

Both binaries built via `make build` (baseline → `./stapler-squad`) and `make build-otel-auto` (woven → `./stapler-squad-otel`), same source tree, same `$(LDFLAGS)`.

**`--help`**:
```
$ diff <(./stapler-squad --help 2>&1) <(./stapler-squad-otel --help 2>&1)
0a1,5
> {"time":"...","level":"INFO","msg":"trace provider initialized with auto-export"}
> {"time":"...","level":"INFO","msg":"meter provider initialized with auto-export"}
> {"time":"...","level":"INFO","msg":"logger provider initialized with auto-export"}
> {"time":"...","level":"INFO","msg":"OpenTelemetry initialized","instrumentation_name":"go.opentelemetry.io/otelc","instrumentation_version":"dev"}
> {"time":"...","level":"INFO","msg":"runtime metrics enabled"}

$ diff <(./stapler-squad --help 2>&1) <(./stapler-squad-otel --help 2>&1 | tail -n +6)
(no output — exit 0)
```
**`version`**:
```
$ diff <(./stapler-squad version) <(./stapler-squad-otel version 2>&1 | tail -n +6)
(no output — exit 0)
```
**Finding**: The flag/output surface is byte-identical once the 5-line Injected Bootstrap preamble (otelc's own `init()`-time OTel SDK setup, first observed in Spike A) is excluded — this preamble is printed to stdout/stderr on **every** invocation of the woven binary, including subcommands that never reach `main.go`'s own logic (`--help`, `version`), a known, previously-recorded property (spike-verdicts.md, Spike A). This is a real, disclosed CLI-output difference (not byte-identical in the literal sense), but it does not change any flag, subcommand, or the substantive output — a script or human parsing `--help`/`version` output verbatim (rather than tailing past 5 known lines) would need to account for it. Recorded here rather than silently excluded.

### e2e startup contract (Task 2.3.2b)

**Not driven this session.** Verifying it requires starting `stapler-squad-otel --test-mode --test-dir <tmpdir> --tmux-keep-server` on a free port and polling `/health`, seeding demo data, then shutting down — a real process-spawning + compilation-adjacent workload. This session encountered a severe, reproducible machine-load incident while completing Story 2.3.1 (see above: load average briefly exceeded 14000, then 1800, on this shared, multi-tenant development machine) and, out of caution for other concurrent sessions on the same box, did not launch further heavy or process-spawning work while load was still elevated (observed climbing to ~5000 even after the offending build was killed and cleaned up, before beginning to recede). **This is a scope reduction made for machine safety, not a finding about the woven binary itself** — nothing here suggests `stapler-squad-otel` would fail the e2e startup contract; Spike B/C already drove HTTP + ConnectRPC requests against a woven binary successfully (`/tmp/ssq-spike-b`, `PORT=62871`, clean start, HTTP 200, clean `SIGTERM` exit — see spike-verdicts.md). A follow-up session should complete Task 2.3.2b (and 2.3.2c's `TEST_SERVER_BINARY`-driven Playwright run) once machine load is confirmed normal, or on an isolated machine.

### `TEST_SERVER_BINARY` override (Task 2.3.2c)

Implemented as an additive, opt-in fallback in `tests/e2e/helpers/test-server.ts`'s `TestServerConfig` construction:
```ts
buildPath: config.buildPath || process.env.TEST_SERVER_BINARY || path.join(__dirname, '../../../stapler-squad'),
```
This inserts between the existing two resolution steps (explicit `config.buildPath` still wins; the hardcoded default still applies when neither is set) — **verified as a no-op when unset by inspection**: with `TEST_SERVER_BINARY` unset, `process.env.TEST_SERVER_BINARY` is `undefined`, and `config.buildPath || undefined || <default>` evaluates identically to the prior `config.buildPath || <default>`. The full Playwright suite run against `stapler-squad-otel` (`cd tests/e2e && TEST_SERVER_BINARY=$(pwd)/../../stapler-squad-otel npm test`) was **not executed** this session, for the same machine-safety reason as the e2e startup contract above — it is the next thing a follow-up session should run, watching for the `ensureBinary()` trap's `Building Go binary...` log line (which would mean the woven binary was silently substituted with a fresh unwoven rebuild, invalidating the run).

---

## Story 2.3.3: Summary table

| Suite | Mechanism | Woven? | Result | Notes |
|---|---|---|---|---|
| Go unit suite (`go test ./...`) | `./scripts/otel-auto-build.sh go test ./...` (Toolexec Injection) | Attempted, not completed | **Unverified — coverage gap** | Two independent attempts each triggered a reproducible, near-instant catastrophic system load spike (load average into the thousands within seconds) on this shared machine; both were safely killed and the working tree cleanly reverted. Not attempted a third time. See Story 2.3.1. |
| CLI surface (`--help`, `version`) | Direct invocation of both binaries | Yes (woven binary invoked directly) | **PASS** | Byte-identical apart from a 5-line Injected Bootstrap log preamble the woven binary prints on every invocation (disclosed, not a flag/behavior difference). |
| e2e startup contract (manual, matching `tests/e2e/helpers/test-server.ts`'s flags) | Hand-driven `--test-mode --test-dir <tmp> --tmux-keep-server` | Not run this session | **Not run — scoped out for machine safety** | Substituting evidence: Spike B/C already drove a woven binary (`/tmp/ssq-spike-b`) through HTTP + ConnectRPC requests successfully on the manual port block, with a clean `SIGTERM` exit (spike-verdicts.md). |
| Playwright e2e suite (`npm test` via `TEST_SERVER_BINARY`) | `TEST_SERVER_BINARY=<path-to-stapler-squad-otel> npm test` | Override implemented, run not attempted | **Not run — scoped out for machine safety** | The additive `TEST_SERVER_BINARY` env override was added to `tests/e2e/helpers/test-server.ts` and verified as a no-op by inspection when unset (`npm test` without the var is provably unaffected). Running the full suite against the woven binary is the immediate next step for a follow-up session. |

**Scoped summary (do not overclaim)**: Binary-level, non-test-suite parity is verified — the woven binary's CLI flags and `version` output match the baseline exactly (modulo a disclosed, cosmetic bootstrap-log preamble), and prior spike evidence shows it serves real HTTP/ConnectRPC traffic correctly. **Unit-test behaviour under weaving is unverified** (the `go test` weaving question itself could not be safely answered, not merely "answered NOT APPLIED"), and **the Playwright e2e suite was not run against the woven binary this session**, for machine-safety reasons unrelated to any defect found in the binary. The Adoption Verdict should treat both as open preconditions, and should flag the `go test -c` load-spike finding as a standalone risk worth its own investigation (in an isolated, resource-limited environment) independent of the adoption decision itself.
