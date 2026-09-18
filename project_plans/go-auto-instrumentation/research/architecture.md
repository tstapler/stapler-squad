# Architecture Research: go-auto-instrumentation

Research Agent 3 (Architecture), SDD Phase 2. Scope: how a `build-otel-auto`
target would hook into this repo's Makefile, how loongsuite-go's woven
instrumentation would interact with `telemetry/telemetry.go`'s existing OTel
SDK setup, and what's still unverified from docs alone.

## Event-Command-Policy table: skipped

This is single-pipeline build/tooling plumbing (a build step and a runtime
SDK-initialization ordering question), not a multi-actor business domain —
there's no meaningful set of domain events/commands/policies to model. Skipped
per the task instructions.

## 1. Build system integration

### Current build graph (Makefile)

```
stapler-squad: ensure-tools proto-gen ent-gen server/web/dist $(GO_FILES)
	go build -ldflags "$(LDFLAGS)" -o stapler-squad .          # Makefile:146-157

build-embedded: build-tmux-embed
	go build -tags embed_tmux -ldflags "$(LDFLAGS)" -o stapler-squad .   # Makefile:277-286
```

Both the plain and `embed_tmux` builds share the same prerequisite chain:
`ensure-tools` → `proto-gen` (writes `gen/proto/go/`, gitignored) → `ent-gen`
(writes `session/ent/*.go`, gitignored) → `server/web/dist` (built Next.js
UI, embedded via `go:embed`) → the actual `go build` invocation. `LDFLAGS :=
-X main.version=$(VERSION)` (Makefile:23) is injected into every build via
`-ldflags`.

### Where `build-otel-auto` hooks in

A new target should mirror `build-embedded`'s shape exactly, substituting the
build verb:

```makefile
build-otel-auto: ensure-tools proto-gen ent-gen server/web/dist ## Opt-in: build with loongsuite-go compile-time auto-instrumentation
	otel go build -ldflags "$(LDFLAGS)" -o stapler-squad-otel .
	@echo "✅ stapler-squad built with loongsuite-go auto-instrumentation (stapler-squad-otel)"
```

Key points confirmed from the Makefile itself:
- `ent-gen` and `proto-gen` are **file-existence prerequisites**, not
  build-verb-specific — they write files to disk (`session/ent/*.go`,
  `gen/proto/go/`) that any subsequent `go build`/`otel go build` invocation
  reads via normal Go import resolution. There is nothing about `ent-gen`'s
  mechanism (`go run -mod=mod entgo.io/ent/cmd/ent generate --feature
  sql/upsert ./session/ent/schema`, Makefile:433) that depends on which
  build wrapper compiles the result afterward. So the ordering constraint
  the requirements doc calls out ("ent-gen must run before otel go build,
  same as it must before go build") is satisfied by literally reusing the
  same `ent-gen`/`proto-gen` prerequisites — no new ordering logic needed.
- Output binary name should differ (`stapler-squad-otel`, not `stapler-squad`)
  so this target can never collide with or overwrite the binary that
  `install-service`/`make preview`/CI produce — reinforces the "opt-in only,
  must not change default build output" constraint structurally, not just
  by target-naming convention.
- Should NOT be added to `.PHONY`'s existing `build`/`build-embedded` list
  interactions or to `ci`/`ready`/`quick-check` — those must stay untouched
  per the constraint.

### `otel go build` flag pass-through — confirmed vs. unverified

From loongsuite-go's own docs (`docs/user/config.md`, fetched
2026-08-21 from
[raw.githubusercontent.com/alibaba/loongsuite-go/main/docs/user/config.md](https://raw.githubusercontent.com/alibaba/loongsuite-go/main/docs/user/config.md)):

> `$ otel go build -o app cmd/app`
> `$ otel go build -gcflags="-m" cmd/app`
> "No matter how complex your project is, the otel tool simplifies the
> process by automatically instrumenting your code for effective
> observability, the only requirement being the addition of the `otel`
> prefix to your build commands."

**Confirmed (VERIFIED via docs):** `-o` and `-gcflags` pass through as
ordinary `go build` flags — the tool is a thin CLI prefix, not a full
reimplementation of `go build`'s flag parser.

**Not confirmed (UNVERIFIED):** the docs never show a `-tags` or `-ldflags`
example. A GitHub code search across the repo for `ldflags` and `BuildFlags`
returned no hits, which is inconclusive either way (could mean unhandled, or
just means those tokens don't appear literally in Go source under those
exact names). Given the tool wraps `go build` by shelling out to the real
`go` toolchain after weaving/rewriting source (see §2's "GLS" note — it
patches the Go runtime/stdlib, which means it must still invoke `go build`
itself under the hood to get a binary out), the most likely behavior is that
unrecognized flags are passed through verbatim to that inner `go build`
call — the same shape as `-o`/`-gcflags`. But this is inference, not a
documented guarantee. **This is the single largest open risk item carried
from the requirements' "Rabbit Holes" section** — it must be spiked
empirically (`otel go build -tags embed_tmux -ldflags "-X main.version=x" -o
/tmp/test .` against a minimal reproduction) before Phase 3 planning commits
to `build-otel-auto` depending on `build-tmux-embed` too.

### `embed_tmux` build tag — separate, orthogonal target

`build-embedded`/`build-tmux-embed` (Makefile:272-286) bundle a real tmux
binary into `session/tmux/embed/tmux` via `go:embed`, gated by `-tags
embed_tmux`. Nothing in loongsuite-go's docs addresses `go:embed` directives
specifically, but weaving happens at the AST/instruction level on *code*,
not on embedded binary blobs, so `go:embed` itself is not a likely
interaction point. The real unknown is purely the `-tags` pass-through
question above — if that's confirmed to work, a `build-otel-auto-tmux-embed`
variant (depending on `build-tmux-embed` + `otel go build -tags
embed_tmux ...`) is a mechanical follow-on, not a new risk.

## 2. Version compatibility — two concrete, verified blockers

### Go toolchain version

- This repo: `go.mod`'s `go` directive is `go 1.26.3` (confirmed via `Read`
  of `go.mod:3`); the environment's installed toolchain is `go1.26.4`
  (`go version`, run 2026-08-21).
- loongsuite-go: `docs/user/compatibility.md` lists its tested matrix as
  **Go 1.23 and 1.24 only**, across Ubuntu/macOS/Windows ×
  amd64/arm64/386. The doc explicitly hedges: "While this project should
  work for other systems, no compatibility guarantees are made for those
  systems currently."

**VERIFIED gap:** stapler-squad's toolchain (1.26.x) is two major Go
versions ahead of loongsuite-go's newest tested version (1.24). Given
loongsuite-go patches the Go runtime/stdlib to support goroutine-local
storage for context propagation (see below), a toolchain this far outside
its tested matrix is a real, not hypothetical, risk of build failure or
undefined behavior — not just an untested edge case. Any spike must pin a
Go 1.24 toolchain (e.g. via `asdf`/a second `.tool-versions` entry) to get a
fair first read on whether loongsuite-go works at all here, independent of
the `-tags`/`-ldflags` question.

### OTel SDK version pinning

`docs/user/compatibility.md`:

> "we need to instrument OpenTelemetry (OTel) itself with this `otel`. This
> means that if users explicitly add OTel dependencies, the version of
> those dependencies must match the `otel`'s requirements, otherwise, the
> tool will not function properly."

Their version table's newest entry: `v1.10.0` (tool) → OTel `v1.40.0` /
OTel-contrib `v0.65.0`.

This repo's `go.mod` (confirmed via `Grep`):
```
go.opentelemetry.io/otel                                            v1.44.0
go.opentelemetry.io/otel/sdk                                        v1.44.0
go.opentelemetry.io/otel/trace                                      v1.44.0
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp        v0.67.0
```

**VERIFIED gap:** stapler-squad pins OTel core to v1.44.0; loongsuite-go
v1.10.0 (its latest documented mapping) requires v1.40.0. Per their own
compatibility statement, this mismatch means "the tool will not function
properly" as-is. A `build-otel-auto` spike will need either (a) a
short-lived `go.mod` downgrade of the `go.opentelemetry.io/otel*` module set
to v1.40.0 for that build path only (not viable with a single shared
`go.mod` unless loongsuite-go tolerates a replace directive scoped
somehow), or (b) confirmation that a newer loongsuite-go release (past
v1.10.0, not yet reflected in the fetched docs) has moved its OTel pin
forward to v1.44.0. This needs a fresh check against loongsuite-go's
current release tags at spike time, not just the docs snapshot fetched for
this research pass.

## 3. Integration with existing telemetry (`telemetry/telemetry.go`) — the coexistence crux

### What loongsuite-go does at runtime

`docs/user/sdk-config.md` (fetched 2026-08-21):

> "In addition to automatic instrumentation, the `otel` tool injects
> configuration code to initialize the OpenTelemetry SDK when the
> application starts."

This is unambiguous: loongsuite-go does **not** passively look up whatever
global `TracerProvider` the host application happens to have configured — it
weaves in its **own** SDK bootstrap code that runs automatically, reading
standard OTel env vars directly:

| Env var | Effect |
|---|---|
| `OTEL_TRACES_EXPORTER` | `none`/`console`/`zipkin`/`otlp` (comma-separable); **default `otlp`** |
| `OTEL_METRICS_EXPORTER` | `none`/`console`/`prometheus`/`otlp`; **default `otlp`** |
| `OTEL_EXPORTER_OTLP_ENDPOINT` / `_TRACES_ENDPOINT` / `_METRICS_ENDPOINT` | exporter endpoint(s) |
| `OTEL_EXPORTER_OTLP_PROTOCOL` / `_TRACES_PROTOCOL` | `http/protobuf` (default) or `grpc` |
| `OTEL_TRACE_SAMPLER` | ratio 0.0–1.0, default parent-based-always-on |
| `OTEL_SERVICE_NAME` | service name |

None of these overlap with or are gated by this repo's own `OTEL_ENABLED` /
`DD_TRACE_ENABLED` toggle (`telemetry/telemetry.go:68`) — that variable is
private to `telemetry.DefaultConfig()` and has no meaning to loongsuite-go's
injected bootstrap.

### How the two initializations actually collide

`telemetry.Initialize()` (`telemetry/telemetry.go:92-177`) is called once,
explicitly, from `main.go:262` — well into `main()`'s body, after flag
parsing, config loading, and instance-lock acquisition. When
`cfg.Enabled` is true it calls `otel.SetTracerProvider(tp)` (line 139) and
`otel.SetMeterProvider(mp)` (line 163), i.e. it mutates the **process-global**
OTel SDK registration that `go.opentelemetry.io/otel`'s package-level
`otel.Tracer()`/`otel.Meter()` helpers read from.

loongsuite-go's injected bootstrap, per its own docs, runs "when the
application starts" — for compile-time-woven code that phrasing normally
means a synthetic `init()` function, which Go guarantees runs **before**
`main()` begins executing (all `init()`s across all imported packages run
first, in import-dependency order, then `main()`). That places loongsuite-go's
`otel.SetTracerProvider()` call chronologically **before** line 262 in
`main.go`.

Net effect, if `OTEL_ENABLED=true`:
1. Process starts → loongsuite-go's injected `init()` runs → sets a
   loongsuite-configured global `TracerProvider`/`MeterProvider` (reading
   `OTEL_EXPORTER_OTLP_ENDPOINT` etc. directly).
2. `main()` runs → reaches `telemetry.Initialize()` → **overwrites** the
   global provider with stapler-squad's own (same `otel.SetTracerProvider`
   call).
3. From that point on, both the auto-instrumented woven code (which — per
   the OTel Go convention — resolves `otel.Tracer(name)` against the
   *current* global provider, not a cached reference, unless loongsuite-go
   caches it earlier) **and** the existing manual spans
   (`otelhttp`/`otelconnect`/`telemetry.StartSpan`) end up flowing into the
   **same** `TracerProvider`, `same` batcher, `same` OTLP exporter instance
   — which is exactly the "coexist, one exporter, no duplication" outcome
   the requirements ask for.

This is a plausible, favorable outcome, but it rests on an assumption the
docs don't confirm: **whether loongsuite-go's woven span-creation call
sites re-resolve `otel.GetTracerProvider()` per-span, or cache a `Tracer`
handle once at their own `init()` time.** If they cache once (before
`telemetry.Initialize()` overwrites the provider), loongsuite-go's spans
would keep flowing to *its own* now-orphaned provider/exporter — a second,
independent OTLP export stream, running unconditionally regardless of this
repo's `OTEL_ENABLED` flag (defaulting to `otlp` exporter reading
`OTEL_EXPORTER_OTLP_ENDPOINT`, or the OTel SDK spec default of
`localhost:4317` if unset — the same default this repo's
`telemetry.DefaultConfig()` uses, `telemetry/telemetry.go:29`). This can't
be resolved from documentation; **it requires an empirical test**: build a
trivial `otel go build`'d binary with an OTLP collector listening, toggle
`OTEL_ENABLED=false` in the *host* app, and check whether spans still
arrive. If they do, the fix is straightforward — set `OTEL_TRACES_EXPORTER=none`
/`OTEL_METRICS_EXPORTER=none` as the default for the auto-instrumented
binary and let stapler-squad's own `OTEL_ENABLED=true` flip both that *and*
loongsuite-go's exporter toggle together (e.g. via a small wrapper script
invoked by `make build-otel-auto` that documents "these two toggles must be
set together" rather than trying to unify them in code).

### Manual instrumentation still composes on top

`docs/user/manual-instrument.md` confirms the two approaches are designed
to layer, not conflict, at the API level:

> "Automatic instrumentation creates traces for HTTP services and database
> operations automatically. When manual instrumentation is added via the
> tracer API, it layers additional spans on top of this foundation."

Their example is exactly this repo's existing pattern —
`var tracer = otel.Tracer("otel-manual-instr")` then `tracer.Start(ctx,
"...")` — which is precisely what `telemetry.StartSpan`
(`telemetry/telemetry.go:235-237`) and `telemetry.GetTracer()`
(`telemetry/telemetry.go:216-221`) already do. No API-level rework is
needed in `telemetry/telemetry.go` or `telemetry/attributes.go` for manual
spans to nest correctly under auto-instrumented parent spans — nesting is
handled by ordinary `context.Context` propagation (or loongsuite-go's
goroutine-local-storage fallback, see below), not by anything specific to
this repo's code.

### Context propagation mechanism (relevant to `session/`'s heavy goroutine use)

`docs/user/context-propagation.md`:

> "when `loongsuite-go` create a span, `loongsuite-go` save it to Golang's
> coroutine structure (i.e. GLS), and when `loongsuite-go` create a new
> coroutine, `loongsuite-go` also copy the corresponding data structure
> from the current coroutine."

This confirms loongsuite-go patches the Go runtime to add
goroutine-local-storage (modeled on Apache SkyWalking Go's approach) so
that spans survive `go func(){...}()` boundaries even when a developer
forgot to thread `context.Context` through explicitly. This is a deeper
runtime modification than a typical `-toolexec` wrapper and is the
mechanism behind the "full recompilation, not incremental" compile-time
cost noted in `docs/user/compilation-time.md`. Relevant to this repo
specifically because `session/`, `server/services/`, and the tmux/git
subprocess layers are goroutine-heavy — GLS-based propagation is a genuine
plus for tracing correctness there, but it is also the single most invasive
part of the tool and the most likely source of a subtle interaction with
this repo's own concurrency primitives (actor pattern, `deadlock.Mutex`
wrapping mentioned in `Makefile`'s `checklocks` target comments). No
evidence either way from docs alone that this causes a problem, but it's
worth calling out as a heightened-risk area for the empirical spike, not
just a footnote.

## 4. Data flow summary

```
                     ┌─────────────────────────────────────────┐
                     │  Process start (otel go build binary)    │
                     └───────────────┬───────────────────────────┘
                                     │  Go init() order (imports first)
                                     ▼
                     loongsuite-go injected init()
                     reads OTEL_TRACES_EXPORTER / OTEL_EXPORTER_OTLP_ENDPOINT
                     → builds its own TracerProvider/MeterProvider
                     → otel.SetTracerProvider(loongsuiteTP)     [UNVERIFIED
                       whether woven call sites cache this reference
                       or re-resolve per span — see §3]
                                     │
                                     ▼
                     main() runs → flag/config parsing → instance lock
                                     │
                                     ▼
                     telemetry.Initialize(ctx, telemetry.DefaultConfig())
                     reads OTEL_ENABLED / OTEL_EXPORTER_OTLP_ENDPOINT
                     if cfg.Enabled:
                       → otel.SetTracerProvider(staplerTP)   [OVERWRITES
                         whatever loongsuite-go set as the global provider]
                       → otel.SetMeterProvider(staplerMP)
                     else:
                       → provider left as loongsuite-go set it (no-op path
                         does NOT reset the global provider back to a
                         real no-op — see the OTEL_ENABLED=false risk in §3)
                                     │
                                     ▼
              ┌──────────────────────┴───────────────────────────┐
              │                                                    │
   Manual spans (otelhttp, otelconnect,           Auto-woven spans (database/sql via ent,
   telemetry.StartSpan/GetTracer)                  net/http, and ~60 other libraries)
              │                                                    │
              └──────────────────────┬───────────────────────────┘
                                     ▼
                     Single OTLP gRPC/HTTP exporter → same collector/
                     Datadog Agent — IF AND ONLY IF both initializations
                     ultimately resolve against the same global provider
                     at span-creation time (the open question this section
                     exists to flag).
```

## 5. Custom plugin/hook mechanism (for safeexec/tmux/git, if pursued)

`docs/dev/overview.md`, `docs/dev/hook.md`, `docs/dev/register.md` describe
a three-part mechanism for instrumenting a library loongsuite-go doesn't
support out of the box:

1. **Hook function** (`docs/dev/hook.md`): written with a `go:linkname`
   directive; first parameter is `api.CallContext`, remaining parameters
   mirror the target function's signature for entry hooks, or its return
   values for exit hooks. Example given: target `func foo(a int, b string,
   c float) (d string, e error)` → entry hook `func hook(call
   api.CallContext, a int, b string, c float)`. `CallContext.SetParam()` /
   `CallContext.SetReturnVal()` let the hook mutate arguments/return values,
   not just observe them.
2. **Rule registration** (`docs/dev/register.md`): a JSON file under
   `tool/data/rules/<name>.json` declaring a version range, `ImportPath`,
   target `Function`, the `OnEnter` handler name, and the `Path` to the hook
   code — pointing loongsuite-go at where/when to weave the hook in.
3. Can be developed purely locally (custom rule file passed via `otel set
   -rule=custom.json`, per `docs/user/config.md`) without contributing
   upstream, which matches the requirement's "if feasible within appetite"
   framing — a local-only rule for `safeexec.CommandContext` wrapping
   `os/exec` (which `database/sql`-style libraries in their supported list
   suggest is a reasonably common pattern to hook) is architecturally
   possible without an upstream PR.

This mechanism's stability/maturity is, per the requirements' own "Rabbit
Holes" framing, unknown — docs exist but are thin (`docs/dev/hook.md` is
1,456 bytes; `docs/dev/register.md` is 818 bytes), and the WebFetch pass
over `docs/dev/hook.md` explicitly notes: "The documentation acknowledges
limited guidance is currently available and recommends examining existing
implementations like `pkg/rules/mux`." Treat this as appetite-permitting
follow-on work, not a Phase 2 blocker — the ent/`database/sql` path (fully
supported out of the box, per §"Data flow" above) is sufficient to satisfy
the Success Metrics' "at least one previously-untraced path" bar without
touching the plugin mechanism at all.

## 6. Recommendations carried into Phase 3 (plan)

1. **Spike before planning the full target.** The `-tags`/`-ldflags`
   pass-through question (§1) and the OTEL_ENABLED-gating question (§3) are
   both empirical, not resolvable from docs. Budget spike time for: (a) a
   minimal `otel go build -tags embed_tmux -ldflags "-X main.version=x" -o
   /tmp/test .` against this repo (or a reduced reproduction if the full
   repo doesn't build under loongsuite-go at all) pinned to Go 1.24 per
   §2, and (b) toggling `OTEL_ENABLED=false` on that binary with a
   collector listening, to see whether loongsuite-go's own exporter still
   fires.
2. **Pin the Go toolchain and OTel SDK version for the spike**, not the
   main build — this repo's `go 1.26.3` and OTel `v1.44.0` are both ahead
   of loongsuite-go's documented compatibility matrix (Go 1.23/1.24, OTel
   v1.40.0 as of tool v1.10.0). Confirm against loongsuite-go's current
   release tags at spike time (not just this doc snapshot) whether a newer
   release has moved these pins forward before assuming a downgrade is
   required.
3. **`build-otel-auto` target shape is otherwise low-risk**: reuse
   `ent-gen`/`proto-gen`/`server/web/dist` prerequisites unchanged, output
   to a distinct binary name (`stapler-squad-otel`), keep it out of
   `ci`/`ready`/`quick-check`/`install-service`. This part of the design
   needs no further research.
4. **Coexistence with `telemetry/telemetry.go` needs no source changes**
   for the manual-instrumentation half (StartSpan/GetTracer already use the
   `otel.Tracer()` global-lookup pattern loongsuite-go's own docs recommend
   pairing with). The open risk is entirely on whether loongsuite-go's
   *own* SDK bootstrap can be told to defer to, or be overridden cleanly
   by, this repo's `OTEL_ENABLED`-gated `telemetry.Initialize()` — resolve
   via the spike in (1), and be prepared to document a "set both
   `OTEL_ENABLED=true` and `OTEL_TRACES_EXPORTER=otlp`/`OTEL_METRICS_EXPORTER=otlp`
   together" operational note if the two toggles can't be unified in code.
