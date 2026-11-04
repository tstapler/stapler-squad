# Compile-Time Auto-Instrumentation (always on in `stapler-squad`)

Every `stapler-squad` binary, whether from `make build`, `make install-service`, Bazel (`//:stapler-squad`) or a GoReleaser release, is built with `otelc` compile-time weaving. There is no separate `stapler-squad` binary any more (owner decision, [ADR-005 amendment](../../project_plans/bazel-conversion/decisions/ADR-005-permanent-exceptions-outside-bazel.md); this supersedes [ADR-003](../../project_plans/go-auto-instrumentation/decisions/ADR-003-structural-opt-in-separate-binary.md)). Plain `go build .` still works for a quick compile check but produces an uninstrumented binary that is not shipped.

`otelc` is an **external binary**, not a Go module dependency of this repo. `make build` installs the pinned version if it is missing (`make ensure-otelc`, which runs the command below):

```bash
go install go.opentelemetry.io/otelc/tool/cmd/otelc@v1.0.1   # pinned: OTELC_VERSION in the Makefile
make build                                                    # builds ./stapler-squad, woven (build-embedded / build-embedded-tymux weave too)
make otel-auto-smoke                                          # Collector Smoke Test: proves a db.system span actually arrived (needs Docker, local OTLP collector on :4317)
PORT=62871 STAPLER_SQUAD_INSTANCE=claude-otel-manual STAPLER_SQUAD_TMUX_SOCKET=ssq-otel-manual OTEL_ENABLED=true OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 OTEL_EXPORTER_OTLP_PROTOCOL=grpc ./stapler-squad --tmux-keep-server   # run it by hand on the manual port block (CLAUDE.md)
```

**Build cost.** A woven build runs `otelc setup` twice and then a toolexec `go build`, so it is slower and heavier than a plain build, and it temporarily rewrites `go.mod`/`go.sum` (restored from a byte backup on exit; `scripts/otel-auto-build.sh`'s Module Mutation Guard fails the build if the build step itself changes them). Run one woven build at a time per checkout: the rewrite is in place. Tests stay on plain `go test`; `make otel-auto-test` is the one designed woven test path and is not part of `ci`.

**Bazel.** `bazel build //:stapler-squad` runs the weave as a sandboxed, remote-cacheable action (`tools/bzl/otel_weave_hermetic.bzl`, `tools/bzl/otel_weave_hermetic.sh`, spike S15b) from a committed `otelc setup` overlay (`tools/bzl/otel_overlay/`; regenerate with `gen.sh` when `go.mod`/`go.sum`/otelc change), so the real tree's `go.mod` is never touched and `make build` (`scripts/otel-auto-build.sh`) is unchanged. `//:stapler-squad_plain` is the uninstrumented `go_binary`, kept only for the parity harness. See [bazel-targets-and-configs.md](../reference/bazel-targets-and-configs.md).

**Releases.** `.goreleaser.yaml` builds the `stapler-squad` id through `scripts/goreleaser-go-tool.sh` (the `tool:` shim), so run `goreleaser` with `--parallelism 1` and `otelc` on `PATH`. The release workflows install `otelc` and pass the flag.

Full findings, commands, and raw output: [`spike-verdicts.md`](../../project_plans/go-auto-instrumentation/implementation/spike-verdicts.md).

## Run Recipe

`OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_PROTOCOL`, and `DD_TRACE_ENABLED` are **runtime** variables — set them when *launching* `stapler-squad`, never when building it. `make build` / `scripts/otel-auto-build.sh` neither set nor can set them: that process exits before the binary ever runs, and the binary is a separate process that inherits nothing from the build shell (ADR-004).

**Tracing on** — the bare "auto-export" default does **not** reach a local collector; the endpoint and protocol must be set explicitly (Spike D):

```bash
OTEL_ENABLED=true \
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 OTEL_EXPORTER_OTLP_PROTOCOL=grpc \
PORT=62871 STAPLER_SQUAD_INSTANCE=claude-otel-manual ./stapler-squad --tmux-keep-server
```

**Tracing off** — leave `OTEL_ENABLED`/`DD_TRACE_ENABLED` unset (Spike D.2's Exporter Toggle remedy was N/A — no Injected Bootstrap leak was observed, so no additional `OTEL_TRACES_EXPORTER=none`/`OTEL_METRICS_EXPORTER=none` is needed):

```bash
PORT=62871 STAPLER_SQUAD_INSTANCE=claude-otel-manual ./stapler-squad --tmux-keep-server
```

This is the identical variable set `scripts/otel-auto-smoke.sh --suppression` uses, so this doc and the smoke test cannot drift out of sync.

## Operational consequences of the Spike Verdicts

| Consequence | Verdict |
|---|---|
| `-tags embed_tmux` support | Supported — Spike A's Story 1.2.2 passed; `make build-embedded` and `make build-embedded-tymux` weave too. See [Spike A](../../project_plans/go-auto-instrumentation/implementation/spike-verdicts.md#spike-a--build-flag-passthrough-toolexec-injection). |
| Instrumentation Scope discriminator | Woven Spans' scope always starts with `go.opentelemetry.io/otelc/instrumentation/` (e.g. `.../database/sql`, `.../net/http`). Manual Spans carry their own library's scope (`connectrpc.com/otelconnect`, `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`) — never `stapler-squad` directly; that scope is reserved for spans created via `telemetry.StartSpan`. This supersedes the plan's Domain Glossary, which assumed Manual Spans carry scope `stapler-squad`. See [Spike C](../../project_plans/go-auto-instrumentation/implementation/spike-verdicts.md#spike-c--coexistence-with-otelhttp--otelconnect). |
| Exporter Toggle pairing with `OTEL_ENABLED` | Not required. Spike D's suppression run passed with `OTEL_ENABLED`/`DD_TRACE_ENABLED` simply left unset — no leaked spans, so `OTEL_TRACES_EXPORTER=none`/`OTEL_METRICS_EXPORTER=none` are not needed. See [Spike D](../../project_plans/go-auto-instrumentation/implementation/spike-verdicts.md#spike-d--otel_enabledfalse-suppression). |
| `pitfalls.md` #736-class SQLite log noise | Absent — `invalid DSN` and `parsing DDL` were not observed in the binary's log or the collector output during Spike B. See [Spike B](../../project_plans/go-auto-instrumentation/implementation/spike-verdicts.md#spike-b--cgo-and-sqlite-weaving-the-gono-go-gate). |

## Subprocess instrumentation hook (`instrumentation/otelc/safeexec`)

`instrumentation/otelc/safeexec/hook.go` wraps `executor/safeexec.CommandContext` with a span (Story 5.1.3), gated behind the `otelcauto` build tag because it imports `go.opentelemetry.io/otelc/pkg/hook`, which only exists in `go.mod` during an `otelc setup` window. Its tests (`hook_test.go`) therefore have no repeatable way to run directly — `go test ./instrumentation/otelc/safeexec/...` fails outside that window. Run them with `make otel-auto-test`, which runs `scripts/otel-auto-test.sh` — a module-backup/GOFLAGS/cleanup lifecycle mirroring `scripts/otel-auto-build.sh`'s (see that script's header comment for why it needs its own two-`otelc setup`-call sequence rather than reusing `otel-auto-build.sh` directly) — and leaves `go.mod`/`go.sum` byte-identical afterward. This target is intentionally not a prerequisite of `ci`/`ready`/`quick-check`/`pre-commit`/`install-service`: a woven `go test` run is a machine-load risk.

## Known limitations

- **This repo's own manual OTel pipeline is currently broken, independent of otelc.** `telemetry/telemetry.go`'s `resource.Merge` call fails with a Schema URL conflict (semconv v1.24.0 pinned in `telemetry.go` vs. the resolved SDK's v1.41.0 default), so `telemetry.Initialize` errors out and this repo's own `TracerProvider` never installs — even with `OTEL_ENABLED=true`, even on a plain `go build` binary. Concretely: today, `OTEL_ENABLED=true` on `stapler-squad` only reliably produces Woven Spans plus Manual Spans that happen to get routed through *otelc's own* Injected Bootstrap provider (which installs first, at `init()` time) — not a fully-working, independently-configured manual pipeline. If this bug is fixed independently, Spike C's coexistence result should be re-verified, since two independently-configured providers would then race to call `otel.SetTracerProvider`. This is a pre-existing bug, out of scope for this doc — file it separately rather than fixing it here. See [Spike C](../../project_plans/go-auto-instrumentation/implementation/spike-verdicts.md#spike-c--coexistence-with-otelhttp--otelconnect).
- **`otelc setup`'s `go.mod`/`go.sum` mutation is build-time-only and auto-reverted.** `otelc setup` (run once per build by `scripts/otel-auto-build.sh`) does temporarily add `require`/`replace` lines to `go.mod`/`go.sum` and writes generated files (`otel.instrumentation.go`, `otelc.runtime.go`, `.otelc-build/`) needed to compile the weave. `scripts/otel-auto-build.sh` wraps this in a `setup` → build → `otelc cleanup` lifecycle, verified empirically to leave `go.mod`/`go.sum` byte-identical afterward (the Module Mutation Guard). So `otelc` stays an external tool whose bootstrap is ephemeral, not a permanent addition to this repo's dependency graph.
- **`OTEL_GO_SIMPLE_SPAN_PROCESSOR=true` is a documented footgun — do not set it outside short-lived manual debugging.** Spike B reproduced an app-wide deadlock with this env var: a single slow/stuck synchronous span export serialized every application goroutine (ConnectRPC handlers, background pollers) behind `simpleSpanProcessor`'s global mutex. The default `BatchSpanProcessor` (used throughout the Run Recipe above) is what's validated and should be used.

## Pre-runtime sanity check (heuristic)

Before running the full trace-verification loop, a quick symbol-count comparison against a normal build is a fast (if imprecise) signal that otelc actually wove something in:

```bash
go tool nm stapler-squad | grep -c otel
```

This is a **heuristic, not proof of correct span placement** — a higher count only means more otel-related symbols got linked in, not that spans are emitted correctly or attached to the right operations. Use `make otel-auto-smoke` for an actual pass/fail verification (it asserts a real `db.system` span arrived at a collector). Source: `project_plans/go-auto-instrumentation/research/ux.md` §4.
