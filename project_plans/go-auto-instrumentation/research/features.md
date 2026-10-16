# Research: Features — go-auto-instrumentation

Research Agent 2 (Features), SDD Phase 2. Scope: what OTel surface already exists in
stapler-squad, edge cases/failure modes an auto-instrumentation design must handle, users'
unstated needs, and industry comparables for structuring the opt-in build path.

## 1. Existing OTel surface in stapler-squad (what auto-instrumentation must coexist with)

### `telemetry/telemetry.go`
- Single `Config{Enabled, OTLPEndpoint, ServiceVersion, Environment, SampleRate}`, built by
  `DefaultConfig()`. Gating: `Enabled := os.Getenv("OTEL_ENABLED") == "true" ||
  os.Getenv("DD_TRACE_ENABLED") == "true"` — **disabled by default**, both env vars are
  independently sufficient (
  [telemetry/telemetry.go:68](telemetry/telemetry.go#L68)).
- When disabled, `Initialize` still returns a `*Provider` wrapping the process-wide *global*
  `otel.Tracer(ServiceName)` / `otel.Meter(ServiceName)` (no-op implementations from the SDK
  until a real provider is registered) — so `telemetry.GetTracer()`/`GetMeter()` are always
  safe to call from any package's `init()` or constructor, before `Initialize` runs
  ([telemetry/telemetry.go:92-101](telemetry/telemetry.go#L92-L101)).
  `executor/safeexec/safeexec_metrics.go` relies on exactly this ordering guarantee to
  register its counter at package-init time.
- When enabled: OTLP gRPC exporter (both traces and metrics) to a single endpoint (default
  `localhost:4317`, i.e. the local Datadog Agent's OTLP receiver), `WithInsecure()` (no TLS —
  correct only because the target is localhost), resource built from `resource.Default()` +
  merged service name/version/environment attrs, trace sampler
  `TraceIDRatioBased(cfg.SampleRate)` (default 1.0 = sample everything), composite
  `TraceContext{} + Baggage{}` propagator set globally, batch span processor (5s timeout),
  periodic metric reader (15s interval).
- `Provider.Shutdown` flushes both providers with a 5s timeout context, called via `defer` in
  `main.go`.
- Convenience wrappers `StartSpan`/`AddEvent`/`RecordError` exist but `SetAttributes` is a
  documented no-op stub (comment says "Caller should use span.SetAttributes directly" —
  [telemetry/telemetry.go:256-262](telemetry/telemetry.go#L256-L262)); worth noting because an
  auto-instrumentation pass that hooks *this* function would silently instrument nothing.

### `telemetry/attributes.go`
Flat `const` block of dotted attribute-key strings (`session.id`, `history.entry_count`,
`search.duration_ms`, `db.operation`, `review_queue.size`, etc.) plus typed constructor
functions (`SessionIDAttr(id string) attribute.KeyValue`, etc.). Convention: `Attr<Concept>`
constant, `<Concept>Attr(...)` constructor, dot-namespaced key
(`"<domain>.<field>[_<unit>]"`, e.g. `search.duration_ms`). This is the naming convention any
manually-added spans (e.g. a custom safeexec/tmux plugin) should follow, and it's a useful
reference point for whether an auto-instrumentation tool's own attribute keys would collide —
its keys are all effectively `stapler-squad`-domain-prefixed (`session.*`, `history.*`,
`search.*`, `storage.*`, `db.*`, `review_queue.*`), while the OTel semantic-convention keys a
weaving tool emits (`http.route`, `db.system`, `db.statement`, `net.peer.name`, etc.) live in a
different (numeric.io-registered) namespace — collision risk is low by construction, but no
existing code enforces this, it's just accidental convention alignment.

### Metric-naming precedent (`executor/safeexec/safeexec_metrics.go`)
`safeexec.sigkill_escalations` — `<package>.<snake_case_event>`, registered once at package
`init` via `telemetry.GetMeter()`, panics on a malformed instrument name (build-time-constant
programmer error, not a runtime condition —
[safeexec_metrics.go:20-30](executor/safeexec/safeexec_metrics.go#L20-L30)). This is the
pattern a custom loongsuite-go/otelc plugin for the safeexec/tmux/git subprocess layer should
match if it emits its own counters rather than only spans.

### Wiring: HTTP and ConnectRPC
- `server/server.go` wraps the entire HTTP handler chain in `otelhttp.NewHandler(...,
  "stapler-squad-http", otelhttp.WithMessageEvents(otelhttp.ReadEvents,
  otelhttp.WriteEvents))` at the outermost layer: `otelhttp -> logging -> CORS -> gzip ->
  [auth] -> mux` ([server/server.go:1072-1083](server/server.go#L1072-L1083)). There are
  **two** call sites doing this (`server.go:1078` and `server.go:1282`) — worth confirming in
  planning whether both are live/reachable or one is dead code, since a duplicate
  `otelhttp.NewHandler` wrap could itself double-span independent of any auto-instrumentation
  question.
- `ConnectOptions(registry)` builds `otelconnect.NewInterceptor(otelconnect.WithTrustRemote())`
  chained with a first-party `interceptors.NewErrorRecorderInterceptor(registry)` — order is
  error-recorder *then* otel interceptor in the `connect.WithInterceptors(...)` list, i.e. the
  OTel span is already active by the time the error recorder's `trace.SpanFromContext(ctx)`
  call runs, so it can safely add error events to it. If `otelconnect.NewInterceptor` fails to
  construct, `ConnectOptions` logs a warning and returns interceptors *without* it — RPCs still
  work but are entirely unspanned; no fallback/retry (
  [server/server.go:1405-1422](server/server.go#L1405-L1422)).
- `server/interceptors/error_recorder.go`: `span.IsRecording()` guards all OTel calls, so this
  interceptor is safe (a no-op for the OTel path) when telemetry is disabled or when
  `otelconnect` failed to attach. It also unconditionally persists errors to a SQLite
  `ErrorRecorder` regardless of OTel state — that's a *separate* observability channel from
  OTel entirely, worth noting so an auto-instrumentation design doesn't assume "OTel is the
  only signal path."

### Build system context relevant to any weaving tool
- `Makefile`: normal build is `go build -ldflags "$(LDFLAGS)" -o stapler-squad .`
  ([Makefile:150](Makefile#L150),
  [Makefile:155](Makefile#L155)); the tmux-embedded variant adds `-tags embed_tmux`
  ([Makefile:279-284](Makefile#L279-L284)); `LDFLAGS := -X main.version=$(VERSION)` — no
  stripping (`-s -w`) currently, which matters for eBPF options (see §2).
- `go.mod` declares `go 1.26.3` — very recent. Several CI workflows pin an *older* Go via
  `actions/setup-go`'s `go-version: '1.25.0'` (lint.yml, mcp-integration.yml,
  registry-validation.yml, release.yml, release-please.yml) or `'1.23'` (demo-publish.yml,
  ux-analysis.yml), while `benchmark.yml` uses `go-version-file: 'go.mod'` (i.e. actually
  1.26.3). This existing version skew across workflows is itself worth flagging to whoever
  plans the opt-in CI job: pick a workflow-version source deliberately rather than copying an
  already-stale pin.
- `telemetry.Initialize` is called from `main.go:262`, inside the web-server-mode command
  path, after acquiring the instance lock and before starting continuous profiling — i.e. it
  is already one of several "opt-in observability" subsystems started in sequence in
  `main.go`, alongside pprof/pyroscope profiling (`main.go:276`). Worth reusing that sequencing
  pattern (log a warning, don't fail startup) for any auto-instrumentation initialization hook
  that needs a companion runtime call.

## 2. Edge cases and failure modes for the design

1. **Partial/silent instrumentation failure.** loongsuite-go's own docs state plainly that a
   *compile* failure where plain `go build` succeeds is "likely a bug, file an issue" — but
   there is no documented guarantee about a library that's *silently* skipped (e.g. a
   `database/sql` driver method the tool doesn't recognize) producing no spans without any
   warning. Design implication: the verification step in Success Metrics ("surfaces spans for
   at least one previously-untraced path") needs to be a runtime smoke test (build, run, hit
   ent's DB, assert a span with `db.system=sqlite` appears in a local collector), not just "the
   build succeeded" — a clean build is not evidence of instrumentation coverage.
2. **Go-version compatibility is a real, currently-unresolved risk**, not a hypothetical.
   `alibaba/loongsuite-go`'s own `go.mod` requires `go 1.24.0`, and its CI matrix (verified
   live against the actual `basic.yml` workflow) tests only `go_version: [1.24, 1.25]` —
   **Go 1.26 is untested upstream**, while stapler-squad's `go.mod` is `1.26.3`. This is a
   go/no-go gate for the opt-in build path, not just a footnote: the plan should budget time to
   either (a) confirm loongsuite-go/otelc actually works against 1.26.3 empirically, or
   (b) pin a secondary, older Go toolchain specifically for the `otel`/`otelc` build job (the
   normal build stays on 1.26.3). The official OTel-org successor tool, `otelc` (see §4),
   requires **Go 1.25+** per its docs — still short of 1.26.3, same open question.
3. **`OTEL_ENABLED=false` with an auto-instrumented binary still baked in — overhead
   question.** Auto-instrumentation weaving happens at *compile* time; the woven code still
   executes at runtime (checking `span.IsRecording()`/whether a global TracerProvider is
   registered) even when `OTEL_ENABLED=false` leaves `telemetry.Initialize` on the no-op
   provider path. This mirrors exactly how the *existing* `otelhttp`/`otelconnect` middleware
   already behaves when disabled (`error_recorder.go`'s `span.IsRecording()` guard is the same
   idiom) — so the overhead when disabled should be small (no-op span creation + attribute
   calls into a discarded span), but it is not literally zero, and the Success Metrics'
   benchmarking requirement should explicitly include an `OTEL_ENABLED=false` auto-instrumented
   build in the comparison matrix, not just enabled-vs-normal-build.
4. **Double/nested spans on the same request path.** Both `net/http` and `database/sql` are in
   loongsuite-go/otelc's supported-library table, but nothing in either project's docs
   describes composition behavior with hand-written `otelhttp`/`otelconnect` middleware already
   wrapping the same handlers. This is explicitly named as a Rabbit Hole in requirements.md and
   is corroborated by research as an open question with no vendor guarantee either way —
   verify empirically (single request → count spans, check for either (a) two independent root
   HTTP spans, or (b) an auto-instrumented span nested oddly inside/around the otelhttp span)
   before claiming coexistence works.
5. **`GOFLAGS`/`-toolexec` single-slot conflict.** `-toolexec` is a single flag slot — stacking
   two different `-toolexec`-based tools (not a concern today since stapler-squad uses none,
   but worth documenting) would break. `otelc`'s own docs recommend exactly the
   `GOFLAGS="${GOFLAGS} '-toolexec=otelc toolexec'"` injection pattern for exactly this reason
   — it composes with an *unmodified* `go build` invocation, so the existing
   `-tags embed_tmux -ldflags "$(LDFLAGS)"` build line should not need to change at all, only
   the environment the `make build-otel-auto` target sets before calling it. This should be
   verified empirically, not assumed — no direct doc statement confirms build-tag/ldflags
   passthrough for either tool.
6. **ent-generated code must exist first (already a stated constraint).** Both weaving
   approaches operate on already-resolved Go source per compilation unit, so the existing
   `make ent-gen` dependency ordering (build/test/lint all depend on it) is unaffected in
   principle — but it's worth an explicit smoke-test step in validation since `-toolexec`
   tooling sometimes has stricter assumptions about a fully-buildable module graph than plain
   `go build`.
7. **eBPF options fail closed for this deployment model, not silently — but for structural
   reasons, not runtime ones.** Odigos requires a Kubernetes operator/CRDs — no standalone
   mode; ruled out outright for a systemd-managed single binary. Grafana Beyla/OBI (the
   official OTel org's actively-maintained eBPF instrumentation project, having absorbed the
   original `open-telemetry/opentelemetry-go-instrumentation` effort) has a standalone-process
   mode so it's not literally impossible, but it (a) needs root/elevated Linux capabilities and
   kernel ≥5.8 — no macOS parity for local dev, breaking this repo's stated cross-platform
   posture; (b) uses a Go-version-keyed **offset generator** for non-SDK library instrumentation
   that is documented to **break entirely on stripped binaries** (`-ldflags="-s -w"`) — a real
   trap if this repo's `LDFLAGS` is ever extended to strip symbols for release size, since the
   auto-instrumentation would silently stop working with no build-time signal; (c) is pre-1.0
   (OBI GA targeted late 2026). This is why compile-time weaving (loongsuite-go/otelc) is the
   only option compatible with the "single opt-in binary, same deployment model" constraint —
   eBPF options fundamentally want a second privileged process, which this project's Success
   Metrics and Constraints don't budget for.

## 3. Unstated needs beyond the explicit requirements

- **Distinguishing auto- vs manually-instrumented spans in the trace view.** Operators
  watching Datadog APM (the named user) will want to tell, at a glance, whether a given span
  came from hand-written code (`telemetry/telemetry.go`, `otelhttp`, `otelconnect`) or from
  woven instrumentation — otherwise a bug in generated instrumentation is indistinguishable
  from a bug in first-party code when triaging. OTel's standard mechanism for this is
  `InstrumentationScope` (`otel.library.name`/`otel.library.version`), which is how the SDK
  already tags spans by *which* tracer created them (`tp.Tracer(ServiceName)` sets this scope
  today for all of stapler-squad's own spans). Neither loongsuite-go nor `otelc`'s docs state
  explicitly what scope name their woven spans carry by default — this should be verified
  empirically during the build-and-verify phase, and if it's not sufficiently distinct,
  documenting/renaming it (or filtering by it, e.g. via Datadog's per-integration
  `DD_TRACE_<INTEGRATION>_ENABLED=false` knob, which implies scope-level distinguishability
  already exists for Datadog's own auto-instrumentation) is a real requirement, not a nice-to-have.
- **Fast local iteration.** No hard build-time-overhead numbers were found for either
  loongsuite-go or `otelc` in the research pass (the repo ships a `benchmark` example but its
  numbers weren't published inline) — but the `-toolexec` mechanism inherently reprocesses
  every compilation unit through an extra pass, so a developer switching between
  `make build` and `make build-otel-auto` during iterative work should expect *some* build-time
  tax. Given this is explicitly opt-in and not the default inner-loop build (per Constraints),
  this is lower-priority, but the Success Metrics section should still capture a build-time
  delta number alongside the runtime-overhead number, since "large appetite, 3-6 weeks" implies
  someone will iterate on this build path repeatedly during development.
- **A documented, falsifiable "it's actually different" check**, not just "the binary built."
  Given failure mode #1 above (silent skip is plausible, vendor docs don't rule it out), the
  project's own success metric ("surfaces spans for at least one previously-untraced path")
  should be implemented as an automated check (e.g. an e2e-style smoke test that starts the
  auto-instrumented binary, drives one ent query, and asserts a span appears in a local OTLP
  collector) rather than a one-time manual verification — this is exactly the kind of
  regression an auto-instrumentation vendor upgrade could silently reintroduce later.
- **A rollback-safe opt-in surface.** Because both `OTEL_ENABLED` and `DD_TRACE_ENABLED` already
  independently gate telemetry emission at runtime, and the auto-instrumented build is a
  *separate binary/artifact* under Constraints, operators get a natural two-layer safety net
  (don't build with the flag; or build with it but leave both env vars unset) — worth stating
  explicitly in the written recommendation so a future "flip the default" decision has a
  documented rollback path, not just a forward one.

## 4. Industry comparables for structuring the opt-in build path

- **The most load-bearing finding of this research pass: the candidate landscape has shifted
  since requirements.md was written.** In 2025, Alibaba (loongsuite-go's origin) and Datadog
  (owner of the competing Orchestrion compile-time weaver) formed the **OpenTelemetry Go
  Compile-Time Instrumentation SIG** and merged their approaches into
  `open-telemetry/opentelemetry-go-compile-instrumentation` (CLI: `otelc`), which reached
  **v1 (stable)** in July 2026
  ([SIG formation](https://opentelemetry.io/blog/2025/go-compile-time-instrumentation/),
  [v1 announcement](https://opentelemetry.io/blog/2026/go-compile-time-instrumentation-v1/),
  [repo](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation)).
  Alibaba's original project was itself renamed from
  `alibaba/opentelemetry-go-auto-instrumentation` to `alibaba/loongsuite-go` (older name
  `alibaba/loongsuite-go-agent` still resolves). Given `otelc` is now an OTel-org-governed,
  multi-vendor, stable-v1 project rather than a single-company tool, **the planning phase
  should treat `otelc` — not the standalone Alibaba repo — as the primary compile-time
  candidate**, with loongsuite-go as a fallback/comparison point if `otelc`'s Go-1.25+
  requirement or its own maturity turns out to be a blocker.
- **Build-path integration pattern (concrete, from `otelc`'s own docs):** `otelc` supports two
  invocation modes — a direct wrapper (`otelc go build ...`, same shape as loongsuite-go's
  `otel go build`) and, more relevant here, **`-toolexec` injection via `GOFLAGS`**:
  `export GOFLAGS="${GOFLAGS} '-toolexec=otelc toolexec'"` set *before* an otherwise-unmodified
  `go build` invocation. The docs explicitly recommend this form "when the build command is
  owned by a Makefile, CI pipeline, or another tool you don't want to change" — which maps
  directly onto this repo's situation: a `make build-otel-auto` target could set `GOFLAGS` and
  then literally re-invoke the existing `go build -tags embed_tmux -ldflags "$(LDFLAGS)" -o
  stapler-squad .` line unchanged, rather than needing a parallel, drifting build recipe.
- **No established real-world adoption precedent found for CI job separation or binary
  artifact naming.** Multiple search passes across both loongsuite-go and `otelc` (and their
  predecessor names) surfaced only the vendors' own docs/announcements — no third-party blog
  posts, case studies, or GitHub repos documenting how another team structured their opt-in
  build target, a separate/nightly CI job, or artifact naming conventions for a compile-time
  auto-instrumented binary. This is corroborated across all three research passes and should
  be reported as a genuine gap, not smoothed over: **compile-time Go OTel weaving has very
  little public adoption history to imitate as of 2026**, meaning the plan phase should design
  the opt-in build path (make target name, CI job cadence, artifact naming, smoke-test
  pattern) from first principles / this repo's own conventions rather than copying an
  established pattern.
- **Generic OTel guidance that does transfer:** `InstrumentationScope`
  (`otel.library.name`/`otel.library.version`) is the standard, backend-agnostic mechanism for
  attributing a span to the library/tool that created it — confirmed generically across
  multiple tracing-backend docs, though no source directly confirms what scope name `otelc`- or
  loongsuite-go-woven spans carry by default, or exactly how Datadog's UI surfaces
  `InstrumentationScope` as a distinct facet (Datadog's `DD_TRACE_<INTEGRATION>_ENABLED=false`
  per-integration disable knob implies per-library distinguishability exists in their auto-
  instrumentation product, but that's Datadog's own tracer, not OTel-generic evidence). Treat
  this as "verify empirically once a real auto-instrumented span exists," per §3 above.
- **eBPF ecosystem consolidation is itself a comparable data point.** Grafana donated Beyla to
  the OpenTelemetry org in 2025, rebranded as **OBI (OpenTelemetry eBPF Instrumentation)**, now
  driven by a dedicated SIG (Grafana, Splunk, Coralogix, Odigos, others), v0.8.0 as of
  2026-04-16, targeting 1.0 GA late 2026. The original `open-telemetry/opentelemetry-go-
  instrumentation` repo is effectively superseded/inactive as the primary eBPF effort. Odigos
  is itself now an OBI SIG contributor, suggesting its own future direction folds into OBI
  rather than remaining a distinct tool. Net effect for this project: eBPF isn't a live
  three-way comparison so much as "one consolidating OTel-org project (OBI) plus one
  k8s-only tool (Odigos) that's converging toward the same project" — worth stating plainly in
  the comparison table rather than treating all three named alternatives as equally distinct,
  live options.

## Sources

- [alibaba/loongsuite-go](https://github.com/alibaba/loongsuite-go) — README (supported
  libraries table, usage examples), `docs/dev/rule_def.md`, `docs/dev/register.md`, CI
  `basic.yml` matrix (fetched live via `gh api`, 2026-08-21).
- [OpenTelemetry blog: Go compile-time instrumentation SIG
  formation](https://opentelemetry.io/blog/2025/go-compile-time-instrumentation/)
- [OpenTelemetry blog: Go compile-time instrumentation v1
  announcement](https://opentelemetry.io/blog/2026/go-compile-time-instrumentation-v1/)
- [open-telemetry/opentelemetry-go-compile-instrumentation](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation) —
  `docs/getting-started.md` (`GOFLAGS`/`-toolexec` pattern).
- [open-telemetry/opentelemetry-go-instrumentation](https://github.com/open-telemetry/opentelemetry-go-instrumentation) —
  README ("work in progress" status).
- [Grafana blog: OpenTelemetry eBPF Instrumentation (Beyla)
  donation](https://grafana.com/blog/opentelemetry-ebpf-instrumentation-beyla-donation/)
- [OpenTelemetry blog: OBI first
  release](https://opentelemetry.io/blog/2025/obi-announcing-first-release/)
- [OpenTelemetry OBI docs](https://opentelemetry.io/docs/zero-code/obi/)
- [Grafana Beyla docs](https://grafana.com/docs/beyla/latest/), [deployment options via
  DeepWiki](https://deepwiki.com/grafana/beyla/2.3-deployment-options)
- [grafana/beyla issue #1331](https://github.com/grafana/beyla/issues/1331) — stripped-binary
  offset-generator failure mode.
- Odigos Kubernetes-operator deployment model — search synthesis, no single authoritative
  standalone-mode doc found (flagged as a gap in the sourcing itself).
- In-repo: `telemetry/telemetry.go`, `telemetry/attributes.go`,
  `server/interceptors/error_recorder.go`, `server/server.go`, `main.go`,
  `executor/safeexec/safeexec_metrics.go`, `Makefile`, `go.mod`, `.github/workflows/*.yml`,
  `.claude/docs/opentelemetry.md`.
