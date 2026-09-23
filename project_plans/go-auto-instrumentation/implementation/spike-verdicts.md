# Spike Verdict Log — go-auto-instrumentation

Durable, append-only record of every Spike Verdict from Phase 1 (`plan.md`). Entry template:

```
## <Spike ID> — <name>
**Verdict**: PASS|FAIL|PARTIAL
**Command**:
**Output**:
**Next action**:
```

---

## Tool acquisition — otelc

**Verdict**: PASS
**Command**:
```
go install go.opentelemetry.io/otelc/tool/cmd/otelc@v1.0.1
ln -sf "$(go env GOPATH)/bin/otelc" /home/tstapler/.local/bin/otelc   # /usr/local/bin not writable (no sudo); ~/.local/bin is already on PATH
otelc version
```
**Output**:
```
otelc version v1.0.1
```
**Details**:
- **Version**: `v1.0.1` (latest GitHub release as of 2026-08-21; released 2026-07-14 — https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/releases/tag/v1.0.1)
- **Install method used**: `go install go.opentelemetry.io/otelc/tool/cmd/otelc@v1.0.1` (module path and `tool/cmd/otelc` package location confirmed via `gh api repos/open-telemetry/opentelemetry-go-compile-instrumentation/contents/go.mod` → `module go.opentelemetry.io/otelc`, and `.../contents/tool/cmd/otelc` listing `main.go`). This is a variant of the two paths `docs/getting-started.md` documents (Option 1: `git clone` + `make build` producing a local `./otelc`; Option 2: `go get -tool ...` + `go tool otelc`, which adds a tool dependency to the *consuming* module's `go.mod`). `go install <module>/tool/cmd/otelc@<version>` was used instead because it installs a real binary onto `$GOPATH/bin` without mutating `stapler-squad`'s own `go.mod` — this repo has an explicit Module Mutation Guard requirement (Story 2.1.4) and Epic 1.1's task should not pre-empt it by adding a tool dependency to the target repo before Phase 1 even starts.
- **Go-version support statement**: "no ceiling stated; 1.26.3/1.26.4 untested upstream" — README badge states a floor of **Go 1.25+** (https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/blob/main/README.md, badge row) and `go.mod` requires `go 1.25.0` (https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/blob/main/go.mod). No upper bound is declared anywhere in README, getting-started.md, or the CI workflow list (`test-versionmatrix.yaml` runs on a schedule using `go-version-file: go.mod`, i.e. whatever floor version is pinned there — not an explicit matrix of newer versions). This repo's `go.mod` declares `go 1.26.3` and the installed toolchain is `go1.26.4` — above otelc's stated floor but not explicitly validated upstream. **Spike B's verdict must note if any failure could be a Go-version artifact**, per Story 1.1.1's AC.
- **Debug/verbose flag**: `--debug` / `-d` global flag, or `OTELC_DEBUG` env var (confirmed via `otelc --help`: `--debug, -d  Enable debug mode [$OTELC_DEBUG]`). Applies to every subcommand (`setup`, `go`, `toolexec`, `pin`, `cleanup`, `version`). There is also a `--work-dir`/`-w` global flag (defaults to cwd) and a `--rules` flag for a custom rules file.
- **Wrapper Prefix Mode verbs confirmed**: `otelc go build`, `otelc go install`, `otelc go test` (from `otelc go --help` error text: "Only 'go build', 'go install' and 'go test' are supported").
**Next action**: Proceed to Epic 1.2 (Spike A).

---

## Spike A — build-flag passthrough (Toolexec Injection)

**Verdict**: PASS
**Command**:
```
make proto-gen ent-gen                    # prerequisite generators (architecture.md §1)
make server/web/dist                      # prerequisite: go:embed all:dist in server/web/embed.go
                                           # needs web-app/out to exist — a plain `go build .`
                                           # fails identically without otelc, confirmed by running
                                           # it unweaved first; not an otelc-specific issue.
otelc setup                               # one-time per-module instrumentation analysis; downloads
                                           # instrumentation packages, writes .otelc-build/

export GOFLAGS="${GOFLAGS} '-toolexec=otelc toolexec'"
go build -ldflags "-X main.version=spikeA" -o /tmp/ssq-spike-a .
/tmp/ssq-spike-a version

make build-tmux-embed                     # stages session/tmux/embed/tmux (init-submodules + tmux 3.4 build)
go build -tags embed_tmux -ldflags "-X main.version=spikeA2" -o /tmp/ssq-spike-a2 .
/tmp/ssq-spike-a2 version
ls -la /tmp/ssq-spike-a /tmp/ssq-spike-a2
```
**Output**:
```
$ /tmp/ssq-spike-a version
{"time":"...","level":"INFO","msg":"trace provider initialized with auto-export"}
{"time":"...","level":"INFO","msg":"meter provider initialized with auto-export"}
{"time":"...","level":"INFO","msg":"logger provider initialized with auto-export"}
{"time":"...","level":"INFO","msg":"OpenTelemetry initialized","instrumentation_name":"go.opentelemetry.io/otelc","instrumentation_version":"dev"}
{"time":"...","level":"INFO","msg":"runtime metrics enabled"}
stapler-squad version spikeA
https://github.com/TylerStaplerAtFanatics/stapler-squad/releases/tag/vspikeA

$ /tmp/ssq-spike-a2 version
[same 5 INFO lines]
stapler-squad version spikeA2
https://github.com/TylerStaplerAtFanatics/stapler-squad/releases/tag/vspikeA2

$ ls -la /tmp/ssq-spike-a /tmp/ssq-spike-a2
-rwxr-xr-x 1 tstapler tstapler 155334912 /tmp/ssq-spike-a
-rwxr-xr-x 1 tstapler tstapler 156513395 /tmp/ssq-spike-a2
# diff: 1,178,483 bytes (~1.12 MB) — well above the 500 KB embed_tmux threshold
```
**Story 1.2.1 (ldflags)**: PASS. `go build` under Toolexec Injection (`GOFLAGS="... -toolexec=otelc toolexec"`, no `otelc go build` wrapper needed) exited 0 in 34s wall (220s user / 40s sys — 766% CPU, consistent with parallel weaving across GOMAXPROCS). `version` subcommand printed the exact string `spikeA`, not `dev` — `-ldflags "-X main.version=..."` survives weaving intact. **Toolexec Injection worked on the first attempt; the Wrapper Prefix Mode fallback was not needed.**
**Story 1.2.2 (embed_tmux / go:embed)**: PASS. The `-tags embed_tmux` weave exited 0 in 14.2s wall, `version` printed `spikeA2`, and the embedded-tmux binary is 1,178,483 bytes larger than the non-embedded one (threshold: ≥500 KB) — confirming the real ~1.18 MB `session/tmux/embed/tmux` (tmux 3.4) blob was actually embedded via `go:embed`, not silently dropped by the toolexec rewrite.
**Observation carried forward to Spike D**: both spike binaries print 5 OTel `INFO` lines ("trace/meter/logger provider initialized with auto-export", "OpenTelemetry initialized", "runtime metrics enabled") on *every* invocation — including a bare `version` subcommand that never reaches `main.go`'s own `telemetry.Initialize` call. This is the Injected Bootstrap `research/architecture.md` §3 flagged: an `otelc`-injected `init()` sets up its own OTel SDK unconditionally, before this repo's `OTEL_ENABLED` gate is ever consulted. This is exactly the risk Epic 1.5 exists to settle — recorded here as first-hand evidence, not yet a verdict on suppression.
**Next action**: Proceed to Epic 1.3 (Spike B — the go/no-go gate). No fallback branch needed for Spike A.

---

## Spike B — CGO and SQLite weaving (THE GO/NO-GO GATE)

**Verdict**: PASS
**Command**:
```
# Story 1.3.1 — build + smoke run
export GOFLAGS="${GOFLAGS} '-toolexec=otelc toolexec'"
export CGO_ENABLED=1
go build -ldflags "-X main.version=spikeB" -o /tmp/ssq-spike-b . 2> /tmp/spike-b-stderr.log
grep -n "cgo1.go\|go-sqlite3\|mattn" /tmp/spike-b-stderr.log   # empty — no #624 signature

PORT=62871 STAPLER_SQUAD_INSTANCE=claude-otel-spike /tmp/ssq-spike-b --tmux-keep-server &
curl -sf http://localhost:62871/
kill <pid>   # SIGTERM; confirmed clean exit, no panic

# Story 1.3.2 — collector + driven ent query
docker run -d --name otelcol-spike -p 4317:4317 -p 4318:4318 \
  -v <scratchpad>/otelcol-config.yaml:/etc/otelcol-contrib/config.yaml:ro \
  otel/opentelemetry-collector-contrib:latest   # debug/detailed exporter, otlp receiver on :4317/:4318

OTEL_ENABLED=true \
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 OTEL_EXPORTER_OTLP_PROTOCOL=grpc \
PORT=62871 STAPLER_SQUAD_INSTANCE=claude-otel-spike /tmp/ssq-spike-b --tmux-keep-server &
curl -sf http://localhost:62871/
curl -s http://localhost:62871/api/session.v1.SessionService/ListSessions \
  -H "Content-Type: application/json" -H "Connect-Protocol-Version: 1" -d '{}'
sleep 8   # default BatchSpanProcessor export interval
docker logs otelcol-spike | grep -A20 "Span #"
```
**Output**:
```
$ go build ... -o /tmp/ssq-spike-b .   # exit 0, empty stderr, 4.1s wall

$ curl -sf http://localhost:62871/  →  HTTP 200; SIGTERM → clean exit, no panic in log

$ docker logs otelcol-spike | grep -A20 "Span #0"
InstrumentationScope go.opentelemetry.io/otelc/instrumentation/database/sql dev
Span #0
    Name           : PRAGMA
    Kind           : Client
Attributes:
     -> db.operation.name: Str(PRAGMA)
     -> db.namespace: Str(sessions.db)
     -> server.address: Str(sqlite3)
     -> network.transport: Str(tcp)
     -> db.query.text: Str(PRAGMA table_info(workflows))
     -> db.system.name: Str(other_sql)
# ... 278 spans total in the collector output for one driven request + startup
# ... includes ent SELECT spans directly attributable to the ListSessions RPC, e.g.:
#     db.query.text: Str(SELECT COUNT(*) FROM sessions WHERE status > 4)
#     db.query.text: Str(SELECT `sessions`.`id`, ... FROM `sessions` WHERE ...)
```
**Story 1.3.1**: PASS. `CGO_ENABLED=1` weave of the full repo (including `mattn/go-sqlite3`) exited 0 in 4.1s wall with empty stderr — no `*.cgo1.go` match, no `go-sqlite3`/`mattn` match in stderr. **The `pitfalls.md` #624 failure class did not reproduce.** Note: the toolchain's build cache from Spike A's implicit CGO build (CGO_ENABLED=1 is this environment's `go env` default) likely absorbed most of the compile cost, which is why this build was much faster than Spike A's cold 34s — not evidence of a smaller build, just a warm cache. `/tmp/ssq-spike-b` started on the manual port block (`PORT=62871`, `STAPLER_SQUAD_INSTANCE=claude-otel-spike`, `--tmux-keep-server`), served `curl -sf http://localhost:62871/` → HTTP 200, and exited cleanly on `SIGTERM` with no panic anywhere in its log.
**Story 1.3.2**: PASS. Stood up `otel/opentelemetry-collector-contrib:latest` via Docker (no `otelcol-contrib` binary pre-installed on this machine; Docker was available so no alternative was needed) listening on `localhost:4317` (OTLP gRPC) and `:4318` (OTLP HTTP) with a `debug`/`verbosity: detailed` exporter. Driving one `POST /api/session.v1.SessionService/ListSessions` request against `/tmp/ssq-spike-b` (`OTEL_ENABLED=true`) produced dozens of ent `database/sql` spans in the collector output carrying `db.system.name: Str(other_sql)` (e.g. the `PRAGMA table_info(workflows)` span above, and multiple `SELECT` spans matching the exact SQL text ent generated for this request, e.g. `SELECT COUNT(*) FROM sessions WHERE status > 4`). **Instrumentation Scope for these spans is `go.opentelemetry.io/otelc/instrumentation/database/sql`** — recorded here for Story 1.4.2's reuse.
- **Semantic-convention note (not a failure)**: the AC's wording ("sqlite-family value") assumed `db.system` would read `sqlite`; the actual value is the OTel semconv 1.29+ field `db.system.name` = `other_sql`. This is expected: `database/sql`-level instrumentation sees only the generic driver interface, not `mattn/go-sqlite3` specifically, so it reports the generic `other_sql` enum member rather than a driver-specific one. `db.namespace: sessions.db` and `server.address: sqlite3` (visible on every span) are the discriminators that make these queries identifiably SQLite in the absence of a sqlite-specific `db.system.name`. This is still unambiguously "a span with a `db.system` attribute" per REQ-SM2 — recorded as a documentation note for Story 4.1.1, not a partial-pass condition.
- **#736 signature check**: `invalid DSN` and `parsing DDL` are **absent** from both the binary's own log and the collector's output — no PARTIAL noise-severity call needed.
- One `Status code: Error` span was observed (`GET https://generativelanguage.googleapis.com/... → 403`) — confirmed unrelated to weaving or SQLite: it is this repo's own background Gemini-API health-check goroutine hitting a real 403 in this sandboxed environment, now correctly *visible as a span* (arguably evidence the weave is working, not a defect).
**Go-version-artifact note** (per Story 1.1.1's AC): no failure occurred, so there is nothing to attribute to the Go 1.26.3/1.26.4-vs-1.25-floor gap noted in the Tool acquisition entry above.
**Secondary finding (not part of Spike B's own pass/fail, carried to the Adoption Verdict as an operational caution)**: an earlier attempt at this same request, made with the non-default `OTEL_GO_SIMPLE_SPAN_PROCESSOR=true` env var (an `otelc` "export immediately" debugging knob documented in `docs/configuration.md`, used here only to avoid waiting out the default batch-export interval — **not** part of the plan's prescribed commands), **hung indefinitely**: a `kill -QUIT` goroutine dump showed the ConnectRPC handler goroutine and an unrelated background goroutine (this repo's own GitHub PR-cache fetcher) both blocked on `go.opentelemetry.io/otel/sdk/trace.(*simpleSpanProcessor).OnEnd`'s single global mutex, which was itself held by a goroutine stuck inside `otlptracegrpc.(*client).UploadTraces`'s retry loop — i.e. one slow/stuck synchronous span export serialized and blocked *all* application traffic, not just telemetry. This is a known, documented risk of `SimpleSpanProcessor` in general (the otelc docs call out the export-throughput cost), not an otelc-specific bug, and it did **not** reproduce with the default `BatchSpanProcessor` (used in the PASS run above) — but it is a real, reproduced footgun that the Run Recipe (Story 4.1.1) should explicitly warn against setting `OTEL_GO_SIMPLE_SPAN_PROCESSOR=true` outside of short-lived manual debugging.
**Next action**: Spike B PASSES outright — no fallback branch taken (neither the `modernc.org/sqlite` retry nor the Fallback Tool were needed). Proceed to Epic 1.4 (Spike C) and Epic 1.5 (Spike D), sequentially, against this same `/tmp/ssq-spike-b` binary.

---

## Spike C — coexistence with `otelhttp` / `otelconnect`

**Verdict**: PARTIAL (coexistence itself PASSES cleanly; the Baseline Build comparison leg is blocked by an unrelated, pre-existing bug — see below)

**Blocking discovery — this repo's own manual telemetry pipeline is currently broken, independent of otelc**: before capturing a Baseline Span Census, a standalone diagnostic (`go run` against a throwaway module in the scratchpad, `replace`-directived to import this repo's `telemetry` package directly — no repo files touched) calling `telemetry.Initialize(ctx, telemetry.DefaultConfig())` with `OTEL_ENABLED=true` returned:
```
Config: Enabled=true Endpoint=localhost:4317
2026/08/21 19:59:36 INFO initializing OpenTelemetry endpoint=localhost:4317 env=development version=dev
Initialize error: conflicting Schema URL: https://opentelemetry.io/schemas/1.41.0 and https://opentelemetry.io/schemas/1.24.0
```
**Root cause**: `telemetry/telemetry.go:116-124` calls `resource.Merge(resource.Default(), resource.NewWithAttributes(semconv.SchemaURL, ...))`, where `semconv` is the pinned `go.opentelemetry.io/otel/semconv/v1.24.0` package (`telemetry/telemetry.go:20`). The currently-resolved `go.opentelemetry.io/otel/sdk` version's `resource.Default()` now advertises Schema URL `1.41.0`. `resource.Merge` requires identical schema URLs on both inputs and errors otherwise — this is a **pre-existing bug, unrelated to otelc or weaving**, caused by a semconv/SDK version drift in this repo's own dependencies. It means: **on `main`, right now, starting `./stapler-squad` (or `/tmp/ssq-spike-b`, which contains the identical `telemetry.go` code) with `OTEL_ENABLED=true` silently fails to install this repo's own manual `TracerProvider`** — `main.go`'s `err != nil` branch only `log.Warn`s (to the file logger, not stdout — see logging note below) and continues running with telemetry effectively off. **Recommended as a follow-up bug independent of this project**, not fixed here per this task's scope (spike-verdicts.md only).
**Logging note (secondary, incidental discovery)**: this repo's `log` package's own `GetConfigDir()` (`log/log.go:312-320`) is hardcoded to `~/.stapler-squad` and does **not** consult `STAPLER_SQUAD_INSTANCE` — unlike `config.GetConfigDir()` (used for DB/session-state isolation, which does). This means every instance's file-logged lines (including "telemetry disabled"/"initializing OpenTelemetry"/"Failed to initialize telemetry") land in the **one shared** `~/.stapler-squad/logs/staplersquad.log`, tagged only by an in-line `[<instance>]` prefix — not a genuinely separate file. This made capturing this repo's own log output for the spike binaries impractical without cross-contaminating the live deployed instance's log stream; collector output was used as the source of truth instead wherever possible. Also worth a follow-up bug/doc note, not fixed here.

**Consequence for Story 1.4.1's methodology**: because the Baseline Build's manual instrumentation cannot actually turn on, a literal "baseline vs. woven Span Census" comparison is not meaningful — the baseline produces **zero** spans for any request, not because manual instrumentation is absent by design, but because it errors out before `otel.SetTracerProvider` is ever called. Recorded as data, not worked around:

**Command**:
```
# Baseline Span Census attempt (manual instrumentation)
git checkout -- go.mod go.sum   # otelc setup had bumped these; reverted for a genuine unweaved build
mv otel.instrumentation.go otelc.runtime.go .otelc-build /tmp/otelc-setup-backup/   # otelc.runtime.go
  # has NO build tag (unlike otel.instrumentation.go's `//go:build tools`) and unconditionally
  # blank-imports otelc instrumentation packages into `package main` — its mere presence broke
  # even a plain `go build .` (undefined db.Endpoint/db.DriverName/etc. on *sql.DB) until moved out.
go build -ldflags "-X main.version=baseline" -o /tmp/ssq-baseline .   # exit 0, clean unweaved build
OTEL_ENABLED=true PORT=62873 STAPLER_SQUAD_INSTANCE=claude-baseline-spike /tmp/ssq-baseline --tmux-keep-server &
curl -sf http://localhost:62873/api/session.v1.SessionService/ListSessions -d '{}' ...   # HTTP 200
# → zero collector activity, zero outbound connections to :4317 (confirmed via `ss -tnp`
#   across 12 driven requests and 2+ minutes) — matches the standalone diagnostic's error above.

# Woven Span Census (Story 1.4.1/1.4.2, against /tmp/ssq-spike-b, already running from Spike B)
curl -sf http://localhost:62871/   # a second, distinguishable request
curl -s http://localhost:62871/api/session.v1.SessionService/ListSessions -d '{}'   # the request under test
# wait for BatchSpanProcessor's 5s export interval, then inspect the collector's per-trace output
```
**Output** — the woven Span Census for the one `POST /api/session.v1.SessionService/ListSessions` request, isolated by Trace ID `2d7c324903b03e04218434a32822c569`:

| Span name | Instrumentation Scope | Kind | Parent ID |
|---|---|---|---|
| `POST` | `go.opentelemetry.io/otelc/instrumentation/net/http` **(Woven)** | Server | *(empty — root)* |
| `stapler-squad-http` | `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` **(Manual)** | Server | `7691b1f8c918035e` (the `POST` span) |
| `session.v1.SessionService/ListSessions` | `connectrpc.com/otelconnect` **(Manual)** | Server | `bcc850ffbf31c98f` (the `stapler-squad-http` span) |

**Story 1.4.1**: PASS. **Exactly one span has an empty parent span ID** for the one driven request — no Duplicate Span Hierarchy. The other request I drove in the same window (`GET /`) produced its own separate, single-root trace (`6cfcab288d293b848c1675487c0add68`) with no cross-contamination between the two. Every ent `database/sql` span captured during this window (the ~278 `PRAGMA`/`SELECT` spans referenced in Spike B) carried **its own unique Trace ID with an empty parent**, correctly identified as **ambient background-poller activity** (this app runs continuous session/workflow/review-queue pollers independent of any HTTP request; `db.query.text` values like `SELECT ... FROM workflows`/`approval_rules` match those pollers, not `ListSessions`'s own query) rather than orphaned children of the driven request — confirmed by checking that none of their Trace IDs matched `2d7c3249...`. **Baseline comparison**: not obtainable as designed (see blocking discovery above) — recorded as "0 spans, telemetry pipeline non-functional" rather than omitted.
**Story 1.4.2**: PASS (by a different discriminator than the plan assumed). Every span in the woven census carries a non-empty Instrumentation Scope, and there is a clean, mechanical way to tell Woven from Manual: **Woven Spans' scope always starts with `go.opentelemetry.io/otelc/instrumentation/`** (`.../database/sql`, `.../net/http`); **Manual Spans carry their originating library's own scope** (`connectrpc.com/otelconnect`, `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`) — never `stapler-squad`. **Documentation correction for Story 4.1.1/4.1.2**: the Domain Glossary's claim that "Manual Spans carry scope `stapler-squad`" does not hold — that scope name is only used for spans created directly via `telemetry.StartSpan` (`telemetry.go:220`, `otel.Tracer(ServiceName)`); `otelhttp`/`otelconnect` spans use their own library names as scope, per normal OTel convention (tracer name = instrumenting library, not service name). The correct, verified discriminator to document is the `go.opentelemetry.io/otelc/instrumentation/` scope prefix, not a `stapler-squad` vs. non-`stapler-squad` split.
**Interesting side-effect worth flagging in the Adoption Verdict**: because otelc's Injected Bootstrap sets the global `TracerProvider` at `init()` time — before `main.go` ever reaches the broken `telemetry.Initialize()` call — the woven binary's Manual Spans (`otelhttp`, `otelconnect`) are, in practice, currently being recorded through **otelc's provider**, not this repo's own (which never successfully installs itself due to the schema-conflict bug). This is why coexistence "just works" cleanly above: there is really only one active provider in play right now, not two coordinating ones. If the schema-conflict bug is fixed independently, this coexistence behavior should be **re-verified**, since two independently-configured providers race to call `otel.SetTracerProvider` and whichever runs last wins.
**Next action**: Spike C's coexistence property PASSES; no `net/http` Instrumentation Rule exclusion fallback needed (no Duplicate Span Hierarchy observed). File a follow-up bug for the `resource.Merge` schema-conflict in `telemetry/telemetry.go` (blocks this repo's manual OTel pipeline, independent of this project) and for the `log` package's non-instance-aware `GetConfigDir()`. Proceed to Epic 1.5 (Spike D).

---

## Spike D — `OTEL_ENABLED=false` suppression

**Verdict**: PASS

**Methodology note on the "auto-export" default**: none of Spike D's runs use otelc's bare default exporter config — the Injected Bootstrap's "auto-export" path does **not** reach a bare `localhost:4317` collector without `OTEL_EXPORTER_OTLP_ENDPOINT`/`OTEL_EXPORTER_OTLP_PROTOCOL` set explicitly (confirmed: a first liveness-check attempt with neither var set produced zero collector activity and zero outbound connections from the process after 20+ seconds and two driven requests). All "enabled" runs below therefore set `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 OTEL_EXPORTER_OTLP_PROTOCOL=grpc` explicitly, matching Story 1.3.2's working configuration. **This is a real, load-bearing finding for the Run Recipe (Story 4.1.1): document that these two vars must be set explicitly — the bare default does not talk to a local collector.**

**Command**:
```
# 1.5.1a — Collector Liveness Check
docker restart otelcol-spike   # fresh, collector process kept running for the whole spike
OTEL_ENABLED=true OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 OTEL_EXPORTER_OTLP_PROTOCOL=grpc \
  PORT=62871 STAPLER_SQUAD_INSTANCE=claude-otel-spike-d /tmp/ssq-spike-b --tmux-keep-server &
curl -sf http://localhost:62871/
# (checkpoint collector line count instead of literally truncating `docker logs` —
#  Docker's log driver has no truncate primitive short of a restart, which the AC forbids
#  between the liveness check and the suppression run; a saved line-count offset is the
#  equivalent "only the truncation is the only intervening action" for a containerized collector)

# 1.5.1b — suppression run, same collector process, no restart
kill <old pid>   # SIGTERM
env -u OTEL_ENABLED -u DD_TRACE_ENABLED PORT=62871 STAPLER_SQUAD_INSTANCE=claude-otel-spike-d \
  /tmp/ssq-spike-b --tmux-keep-server &
curl -sf http://localhost:62871/ ; curl -sf .../ListSessions -d '{}'
# checkpoint collector line count again → diff against pre-run checkpoint

# 1.5.2a — mandatory positive control, same still-running collector
kill <pid>
OTEL_ENABLED=true OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 OTEL_EXPORTER_OTLP_PROTOCOL=grpc \
  PORT=62871 STAPLER_SQUAD_INSTANCE=claude-otel-spike-d /tmp/ssq-spike-b --tmux-keep-server &
curl -sf http://localhost:62871/ ; curl -sf .../ListSessions -d '{}'
```
**Output**:
```
1.5.1a Collector Liveness Check: PASS (266 spans, first span "PRAGMA" — InstrumentationScope
  go.opentelemetry.io/otelc/instrumentation/database/sql). Process env confirmed via
  /proc/<pid>/environ to include OTEL_ENABLED=true and the explicit OTLP endpoint/protocol.

1.5.1b suppression run — first pass at the log-diff boundary showed ONE span
  (Trace ID 61e10084..., Name "SELECT", db.namespace sessions.db). Root-caused via
  `ps -o pid,lstart` on the new process (started 2026-08-21 20:05:40 PDT) vs. the leaked
  span's own Start time (2026-08-21 20:05:28.789 PDT, i.e. 11+ seconds *before* the
  disabled-telemetry process even existed): this was a late-arriving BatchSpanProcessor
  flush from the just-killed *enabled* liveness-check process (SIGTERM triggers
  Provider.Shutdown → ForceFlush, which can deliver a buffered background-poller span a
  few hundred ms to low-seconds after the kill signal — a timing artifact of overlapping
  process lifetimes, not a suppression failure). Re-checkpointed after this stray flush
  settled, then re-drove both requests: **zero new spans, zero collector-log growth**,
  confirmed again after one additional 8s batch-timeout wait. Env re-verified via
  /proc/<pid>/environ: no OTEL_ENABLED, no DD_TRACE_ENABLED.

1.5.2a Positive control: PASS (276 spans, including db.system.name spans, arrived on the
  same collector process immediately after re-enabling telemetry — proving the collector
  stayed live throughout and the 1.5.1b zero-span result was not a dead-collector artifact).
```
**Story 1.5.1**: PASS, after correcting one methodology artifact (documented above, not papered over) rather than a real leak. The Collector Liveness Check passed before the suppression run; the same collector process ran continuously start-to-finish (never restarted mid-spike); the corrected suppression window showed **zero** received spans across two driven requests and a full batch-timeout wait, with the disabled-telemetry environment independently re-confirmed via `/proc/<pid>/environ`.
**"telemetry disabled" log-line check**: **not verifiable** — a real, disclosed limitation, not a fabricated pass. This repo's `log` package writes to a single shared, non-instance-aware file (`~/.stapler-squad/logs/staplersquad.log`, the same gap flagged in Spike C) that the live, continuously-active production `stapler-squad` instance also writes to at high (DEBUG) volume with log rotation (`log_max_size: 10MB`, `log_max_files: 5`) — by the time this check was attempted, the target line had almost certainly already rotated out from underneath the running spike, and no instance-tagged occurrence of "telemetry disabled" was found in the current file at all (for any instance, live or spike). The suppression conclusion above rests entirely on collector evidence (spans received/not received) plus direct `/proc/<pid>/environ` inspection, both of which are unaffected by this log-routing gap.
**Story 1.5.2**: PASS. The positive control ran unconditionally, on the same collector, immediately after the (ultimately-passing) disabled run, per the AC's "PASS or FAIL, no exceptions" requirement. Since Story 1.5.1 passed with a **confirmed-live** collector (both the pre-check liveness observation and the post-check positive control bracket it), the Exporter Toggle remedy leg is:
> **Spike D.2 remedy leg — N/A (D.1 passed with a confirmed-live collector; no Injected Bootstrap leak observed).**
The positive-control evidence is recorded above regardless, per the AC.
**Run Recipe requirement carried to Story 4.1.1**: launch-time env for tracing ON is `OTEL_ENABLED=true OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 OTEL_EXPORTER_OTLP_PROTOCOL=grpc` (the bare default does **not** work, see methodology note above); tracing OFF is simply leaving `OTEL_ENABLED`/`DD_TRACE_ENABLED` unset (no Exporter Toggle vars needed, since no leak was observed here) — but note Spike C's finding that this repo's own manual pipeline (`telemetry.go`) is currently non-functional due to the unrelated `resource.Merge` schema bug, so today, "OTEL_ENABLED=true" for the woven binary only reliably lights up **Woven Spans plus otelc's own bootstrap-provider-routed Manual Spans** — not a fully-working, independently-configured manual pipeline.
**Next action**: All four Phase 1 spikes are now recorded. Spike B (the go/no-go gate) **PASSED**. Proceed to Phase 2 per the plan's Dependency Visualization, carrying forward: (1) the `resource.Merge` schema-conflict bug report, (2) the `log` package instance-isolation gap report, (3) the documentation corrections for Instrumentation Scope naming and the `OTEL_EXPORTER_OTLP_ENDPOINT`/`PROTOCOL` Run Recipe requirement, and (4) the `OTEL_GO_SIMPLE_SPAN_PROCESSOR` operational caution from Spike B.

---

## Task 2.1.3c — Isolation Guard proves it actually fails, then reverts clean

**Verdict**: PASS

**Command (deliberate-failure leg)** — temporarily added `build-otel-auto` to `ci`'s prerequisite list (`Makefile:811`), then ran:
```
make otel-auto-isolation-guard
```
**Output**:
```
✗ FAIL: 'ci' is reachable to the otelc auto-instrumentation path
which otelc >/dev/null 2>&1 || (echo "❌ otelc not found on PATH. Install it from https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation (see project_plans/go-auto-instrumentation/implementation/spike-verdicts.md for the exact install command used in this repo)." && exit 1)
./scripts/otel-auto-build.sh go build -ldflags "-X main.version=1.46.0-36-gb6df5daaa-dirty" -o stapler-squad-otel .
echo "✅ stapler-squad built with otelc auto-instrumentation → ./stapler-squad-otel"
✗ FAIL: 'ready' is reachable to the otelc auto-instrumentation path
which otelc >/dev/null 2>&1 || (echo "❌ otelc not found on PATH. Install it from https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation (see project_plans/go-auto-instrumentation/implementation/spike-verdicts.md for the exact install command used in this repo)." && exit 1)
./scripts/otel-auto-build.sh go build -ldflags "-X main.version=1.46.0-36-gb6df5daaa-dirty" -o stapler-squad-otel .
echo "✅ stapler-squad built with otelc auto-instrumentation → ./stapler-squad-otel"
✅ PASS: quick-check
✅ PASS: pre-commit
✅ PASS: install-service
✗ Isolation Guard failed for: ci ready
make: *** [Makefile:306: otel-auto-isolation-guard] Error 1
exit status: 2
```
The guard correctly named both `ci` (the directly-edited target) and `ready` (which inherits via `ready: ci ...`) as offending, and left `quick-check`/`pre-commit`/`install-service` passing — proving the guard's five-target coverage discriminates per-target rather than failing wholesale.

**Command (revert + re-verify leg)** — reverted `Makefile:811` to its original prerequisite list (confirmed via `git diff --stat Makefile` that the only surviving addition afterward was `otel-auto-isolation-guard`, not `build-otel-auto`), then ran the same command again:
```
make otel-auto-isolation-guard
```
**Output**:
```
✅ PASS: ci
✅ PASS: ready
✅ PASS: quick-check
✅ PASS: pre-commit
✅ PASS: install-service
exit status: 0
```
**Next action**: Task 2.1.3c's proof requirement is satisfied. The working tree was left in the passing state (no `build-otel-auto` in `ci`'s prerequisites) for the remainder of Epic 2.1/2.2/2.3 implementation.

---

## Spike E — otelc extension API

**Verdict**: supported — https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation/blob/v1.0.1/docs/rules.md, rule shape: a `inject_hooks` Function Hook Rule.

**Command** (doc reads, not builds — no weave/compile run):
```
otelc --help                 # confirms --rules/OTELC_RULES exist as global flags (already recorded
                              # in the Tool acquisition entry); --work-dir also global
find "$(go env GOPATH)/pkg/mod/go.opentelemetry.io/otelc@v1.0.1" -iname "*.md"
                              # README.md lists ./docs/instrument-guide.md and ./docs/rules.md, but
                              # neither ships inside the go.opentelemetry.io/otelc module zip
                              # (docs/ is excluded from the packaged module — confirmed absent from
                              # the local module cache) — fetched from GitHub instead:
# https://raw.githubusercontent.com/open-telemetry/opentelemetry-go-compile-instrumentation/v1.0.1/docs/rules.md
# https://raw.githubusercontent.com/open-telemetry/opentelemetry-go-compile-instrumentation/v1.0.1/docs/instrument-guide.md
```
**Output** (verbatim excerpts):
```
docs/rules.md — Function Hook Rule example:
  hook_helloworld:
    target: main
    where:
      func: Example
    do:
      - inject_hooks:
          before: MyHookBefore
          after: MyHookAfter
          path: "go.opentelemetry.io/otelc/demo/app/basic/instrumentation"

docs/rules.md — targeting a consumer's own code: "Custom instrumentation for traces and
metrics" is listed as a use case; `target` accepts an exact import path (e.g. `database/sql`)
or a glob (`example.com/svc/*`) — nothing restricts it to otelc's own bundled libraries.

docs/rules.md — rule source precedence (highest wins): OTELC_RULES env var > --rules flag >
otel.instrumentation.go (blank-import composition file in the user's own module) > embedded
defaults. "For development and debugging, rules may also be loaded directly using the
--rules (or OTELC_RULES environment variable) flag."

docs/rules.md — path constraint: "The package referenced by `path` must be available in the
user's module at build time." (With --rules/OTELC_RULES the package must already be a real
import in that module's go.mod; with the otel.instrumentation.go file, otelc adds the import
itself.) `executor/safeexec.CommandContext` already lives in stapler-squad's own module, so
this constraint is satisfied trivially — no vendoring or external package needed.

docs/instrument-guide.md — three-step process for adding a hook: (1) a rule YAML under
instrumentation/<import_path>/ (naming: otelc.yaml or *.otelc.yaml) with target/where/do;
(2) hook Go functions in the package named by `path` — first parameter is hook.HookContext,
"before" hooks match the target function's arguments, "after" hooks match its return values,
and only the target library + OpenTelemetry + stdlib may be imported from that hook package; (3) unit
tests beside the hooks plus integration tests under test/integration/. The guide frames this
as "the workflow for adding compile-time instrumentation for a third-party library" and does
not state or imply the workflow is restricted to otelc's own maintainers/built-in set.
```
**Details**:
- **Not found locally**: `docs/rules.md` and `docs/instrument-guide.md` are not present in the
  `go install`ed module cache (`$(go env GOPATH)/pkg/mod/go.opentelemetry.io/otelc@v1.0.1`) —
  only top-level `README.md`/`AGENTS.md`/etc. and `demo/` ship in the module zip; `docs/` is
  excluded. Read from GitHub's raw content instead (`gh api` was rate-limited at read time —
  "API rate limit exceeded for user ID 3860386" — so `WebFetch` against
  `raw.githubusercontent.com/.../v1.0.1/docs/{rules,instrument-guide}.md` was used, same
  effective source, pinned to the v1.0.1 tag actually installed).
- **Precise mechanism for Story 5.1.2**: an `inject_hooks` rule with `target:
  github.com/tstapler/stapler-squad/executor/safeexec`, `where: {func: CommandContext}`, `do:
  [inject_hooks: {before: <Fn>, after: <Fn>, path: "github.com/tstapler/stapler-squad/instrumentation/otelc/safeexec"}]`
  — loaded via a repo-local `otel.instrumentation.go` blank-import file (the doc's non-"debugging"
  path) or via `--rules`/`OTELC_RULES` pointed at a YAML file, both viable since
  `executor/safeexec` is already a real package in this module's own `go.mod`.
- **Gap not resolved by the docs read**: the exact `hook.HookContext`-based Go function
  signature for `before`/`after` hooks (parameter/return shape, how the hook obtains the
  wrapped function's arguments and — for `CommandContext`, which returns a single `*exec.Cmd`
  with no error — its return value) is not spelled out in `docs/rules.md`; `docs/instrument-guide.md`
  only states the first parameter is `hook.HookContext` and that before-hooks see arguments while
  after-hooks see return values, without a literal signature. The built-in rule YAML/hook sources
  for `database/sql`/`net/http` (which would show a concrete example) are not shipped in the
  module either — `otelc setup` downloads them separately into `.otelc-build/`, which no longer
  exists locally (cleaned up after Task 2.1.3c). Confirming the exact signature requires either
  inspecting a live `.otelc-build/` output or a small trial rule — that trial-and-error is
  properly Story 5.1.2's work, not this feasibility read's.
- **Time estimate for Story 5.1.2**: ~2–4 hours. Breakdown: ~30 min to write the rule YAML and a
  stub hook function and discover the real `hook.HookContext` signature by inspecting
  `.otelc-build/`'s generated code after a first `otelc setup`/weave attempt; ~30–60 min to wire
  attributes through `telemetry/attributes.go`'s `Attr<Concept>`/`<Concept>Attr(...)` convention
  per the plan's Pattern Decisions; ~30–60 min to get a clean weave + confirm a span appears via
  the same collector-based verification Spike B/C used; remainder as buffer for the
  single-return-value (`*exec.Cmd`, no `error`) hook shape being less-documented than the
  error-returning examples in the docs.
**Next action**: Proceed to Story 5.1.2 with the mechanism above. No Fallback Tool evaluation
needed — the AC's "if unavailable" branch does not apply.

---

## Spike E — Addendum: Story 5.1.2/5.1.3 execution findings (2026-08-22)

Story 5.1.2 (hook implementation) and 5.1.3 (collector verification) are **complete**. This
addendum records what the feasibility read above could not: the exact `hook.HookContext`
signature, three real `otelc`/build-script defects hit while wiring the hook in (none in the
hook's own logic), and the captured evidence for 5.1.3's AC.

### Discovered hook signature

Confirmed by inspecting `.otelc-build/instrumentation/{net/http/client,database/sql,go.../redis}`'s
fetched sources after a real `otelc setup` (Spike E's flagged gap): a "before" hook takes
`hook.HookContext` plus the target function's own parameters in order (variadic included,
e.g. `google.golang.org/grpc/client`'s `BeforeDialContext(ictx, ctx, target, opts...)` mirrors
`grpc.DialContext(ctx, target, opts...)`); an "after" hook takes `hook.HookContext` plus the
target's return values in order — for a single-return, no-error function this is confirmed by
`go.opentelemetry.io/otelc/instrumentation/github.com/redis/go-redis/v9`'s
`afterNewRedisClientV9(ictx hook.HookContext, client *redis.Client)`. Data passed from
before→after hook uses `ictx.SetData`/`ictx.GetData`/`ictx.GetKeyData` (see
`net/http/client/client_hook.go`'s `BeforeRoundTrip`/`AfterRoundTrip`), not a return value or
shared closure. Applied at `instrumentation/otelc/safeexec/hook.go`:
`BeforeCommandContext(ictx hook.HookContext, ctx context.Context, name string, arg ...string)` /
`AfterCommandContext(ictx hook.HookContext, cmd *exec.Cmd)`, matching `safeexec.CommandContext`'s
own signature exactly.

### Three otelc/build-script defects found while wiring the rule in

All three are documented in `scripts/otel-auto-build.sh`'s own comments at the point they're
worked around; summarized here for the adoption-verdict record (Phase 6):

1. **`--rules`/`OTELC_RULES` REPLACES, doesn't merge.** `tool/internal/setup/setup.go`'s
   `loadRules` returns early on either being set, skipping `AutoPin` and the embedded-defaults
   path entirely — using either to add a custom rule would silently drop every built-in
   instrumentation (net/http, database/sql, ...) for that build. Worked around with a two-pass
   `otelc setup`: pass 1 auto-discovers the built-in `otel.instrumentation.go` tool file with no
   custom rule present yet; the script then appends the custom package's blank import to that
   file; pass 2 re-runs `otelc setup`, which (per `pin.go`'s `pinLocked`) takes the
   validate-and-keep path (`updatePinnedProjects`) because a tool file now exists, instead of the
   auto-discovery-only path (`generatePinnedProjects`) — merging both rule sets into one
   `matched.json`. Confirmed via `matched.json` showing both `hook_command_context` (custom) and
   the built-in rules present together after pass 2.
2. **Bare `otelc setup` only wires runtime/hook linkage for `.`.** `getBuildPackages` falls back
   to loading just `"."` when given an empty args slice, so `generateRuntimePerPackage` never
   ran for any package besides the repo root. This is invisible for `go build .` (the main
   binary IS `.`), but silently starves every *other* target package of its
   `otelc.runtime.go`-equivalent linkname file: `go test ./executor/... ./session/git/...` under
   a bare `otelc setup` failed to **link** with "relocation target ... not defined" for every
   rule's trampoline — built-in ones (net/http, grpc, log, slog, otel/trace, otel/sdk/trace)
   included, not just the new custom one — proving this is a general otelc gap, not a defect in
   the safeexec hook. Fixed by forwarding the real build-target package args to `otelc setup`
   (filtered to bare positionals — `otelc setup`'s standalone CLI parses its own flags strictly
   and rejects `go build`-style flags like `-o`/`-ldflags`, unlike `otelc go build/test`, which
   sets `SkipFlagParsing`).
3. **Multi-target `otelc setup` produces duplicate global symbols across import-related
   packages.** Once (2) was fixed, `go test ./executor/... ./session/git/...` progressed past
   the relocation error but then failed to link with `duplicated definition of symbol
   go.opentelemetry.io/otelc/instrumentation/go.opentelemetry.io/otel.OtelGetStackImpl, from
   github.com/tstapler/stapler-squad/executor/safeexec ... and github.com/tstapler/stapler-squad/executor ...`
   — because `executor` (also a setup target, via the `./executor/...` glob) imports
   `executor/safeexec` (also a setup target), and `generateRuntimePerPackage` writes an
   independent copy of the same otel-required global stubs (`OtelGetStackImpl` etc.) into
   *each* target package's own generated runtime file. Linking a test binary for a package that
   both IS a setup target and IMPORTS another setup target pulls in both copies. **This is a
   general `otelc` v1.0.1 multi-target `go test` weaving limitation** (reproduces with zero
   custom rules — the colliding symbol here belongs to otelc's own bundled `go.opentelemetry.io/otel`
   instrumentation, not the safeexec hook) and extends `parity-report.md`'s Story 2.3.1 finding
   ("`go test` weaving unverified — coverage gap") with a second, distinct, reproducible failure
   mode beyond the load-spike one already recorded there: even where a load spike doesn't occur,
   multi-target `go test` weaving does not currently link when import-related packages are
   listed together. **Workaround used for this story's own AC**: run each target package as its
   own separate `otelc-auto-build.sh go test <single-package>` invocation rather than one
   combined `go test ./executor/... ./session/git/...` — see Verification below. No general fix
   attempted; out of scope for a subprocess-hook story, and squarely Phase 6's territory.

A fourth, non-blocking issue was also found and fixed defensively: `otelc cleanup`'s
state-manager revert of go.mod/go.sum is not reliable once `otelc setup` runs more than once
per script invocation (as the two-pass fix above requires) — confirmed empirically that a
second `otelc setup` call's `AutoPin`/`TrackAll` snapshots go.mod/go.sum as they stood *after*
the first call's dependency bump, not the true pre-setup original, so `otelc cleanup` "reverts"
to an already-bumped state. `scripts/otel-auto-build.sh` now snapshots go.mod/go.sum itself
(plain `cp`, independent of otelc's own tracking) before calling `otelc setup` at all, and
restores that snapshot in its cleanup trap — confirmed byte-identical (`git status` clean)
across every build/test run performed for this story.

A fifth issue — the one that actually broke the *default* build, not just the otelc-auto
path — was caught by running a plain `go build ./...` after writing the hook: since
`instrumentation/otelc/safeexec/hook.go` lives inside this module's own tree (unlike the
built-in rules, which live in `go.opentelemetry.io/otelc`'s separate module, added/removed from
go.mod entirely by `otelc setup`/`cleanup`), it is one of the packages `go build ./...` /
`make build` / `make lint` compiles by default — and it imports
`go.opentelemetry.io/otelc/pkg/hook`, which is *only* present in go.mod during the temporary
`otelc setup` window. Confirmed: `go build ./...` on this checkout failed with `no required
module provides package go.opentelemetry.io/otelc/pkg/hook` even though nothing in the default
build imports this package at all. Fixed with a `//go:build otelcauto` tag on `hook.go`, with
`scripts/otel-auto-build.sh` injecting `-tags=otelcauto` — merged with any caller-supplied
`-tags` (e.g. `build-otel-auto-embedded`'s `-tags embed_tmux`), not overwriting it — for both
`otelc setup` (via `GOFLAGS`, since `otelc setup`'s own CLI rejects `-tags` as an unrecognized
flag the same way it rejects `-o`) and the real build/test invocation (as an explicit CLI flag,
positioned before any package-pattern positional — a first attempt that appended it at the very
end broke with `malformed import path "-tags=...": leading dash`, since `go build` treats
anything after a package pattern as another package pattern, not a flag). Re-verified after the
fix: `go build ./...` (default, no otelc) passes clean; `go vet ./...` and `gofmt -l` are clean;
`make build-otel-auto` and both `otel-auto-smoke.sh` modes (with and without
`--with-subprocess`) still pass — see Verification sections below.

### Story 5.1.2 verification: `go test` parity vs. Baseline Build

Baseline (unwoven): `go test ./executor/... ./session/git/...` → all three packages `ok`.

Woven, run per-package (per defect 3 above — the combined multi-target invocation hits the
general otelc limitation, not a safeexec-hook defect):

```
$ ./scripts/otel-auto-build.sh go test ./executor/safeexec   → ok
$ ./scripts/otel-auto-build.sh go test ./executor            → ok
$ ./scripts/otel-auto-build.sh go test ./session/git         → ok
```

All three match the Baseline Build's pass/fail result. `git status --short` clean and no
`otelc.runtime.go`/`otel.instrumentation.go` left behind after every run (including the failed
ones during defect discovery, once the cleanup fix above was in place).

### Story 5.1.3 verification: real spans in the collector

Built `stapler-squad-otel` via `./scripts/otel-auto-build.sh go build -o /tmp/claude/stapler-squad-otel .`,
confirmed via `go tool nm` that `OtelBeforeTrampoline_CommandContext.../OtelAfterTrampoline_CommandContext...`
resolve to `instrumentation/otelc/safeexec.BeforeCommandContext`/`AfterCommandContext`. Ran it
on the manual port block (`PORT=62871`, `STAPLER_SQUAD_INSTANCE=claude-otel-subproc`,
`--tmux-keep-server`, `OTEL_ENABLED=true` + gRPC OTLP endpoint) against a local
`otel/opentelemetry-collector-contrib:latest` (same pattern as Spike B), then drove a
`CreateSession` (`sessionType: SESSION_TYPE_DIRECTORY`, pointed at this checkout) followed by
`GetVCSStatus` against the ConnectRPC API. Both produced real `git` subprocess spans via the
hook — `CreateSession`'s own git-detection step, and `GetVCSStatus`'s
`vc.NewGitProvider(...).GetStatus()`, which (like `session/git`'s `IsDirty`) runs `git` through
`safeexec.CommandContext`. Full captured span (the `arg_count: 4` one matches
`session/git.runGitCommand`'s exact `git -C <path> status --porcelain` shape — `-C`, path,
`status`, `--porcelain` = 4 args):

```
InstrumentationScope github.com/tstapler/stapler-squad/instrumentation/otelc/safeexec
Span #0
    Trace ID       : 52e36d3b6f90734d6c3e41d65e61e1bf
    Parent ID      : 755e97b3592a7285
    ID             : 20d7eb9adf9de7a0
    Name           : git
    Kind           : Internal
    Start time     : 2026-08-23 01:43:31.987173543 +0000 UTC
    End time       : 2026-08-23 01:43:31.987205483 +0000 UTC
    Status code    : Unset
    Status message :
    DroppedAttributesCount: 0
    DroppedEventsCount: 0
    DroppedLinksCount: 0
Attributes:
     -> subprocess.command: Str(git)
     -> subprocess.arg_count: Int(4)
```

Resource attributes on the same batch confirmed this came from the intended process:
`process.executable.path: Str(/tmp/claude/stapler-squad-otel)`, `process.pid: Int(465114)`.

`scripts/otel-auto-smoke.sh --with-subprocess` (Task 5.1.3b) converts this into a repeatable
assertion — `trigger_subprocess_check()` drives the same `CreateSession`→`GetVCSStatus`→
`DeleteSession` round trip, `count_subprocess_git_since`/`subprocess_census_since` mirror the
existing `db.system` census helpers. Verified both ways:
`./scripts/otel-auto-smoke.sh` (no flag) → unaffected, `db.system` assertion still passes exactly
as before; `./scripts/otel-auto-smoke.sh --with-subprocess` → both the `db.system` assertion and
the new `subprocess.command=git` assertion pass (5 `git` spans observed, arg counts 4–6,
matching CreateSession's git-detection plus GetVCSStatus's multiple git invocations). `make
lint-shell` passes (27 scripts, including the extended one). No stray tmux sessions or otelc
scaffolding left behind on either run (`git status --short` clean, `tmux ls` clean).

### Known limitation carried forward from the hook design itself

`safeexec.CommandContext` only *constructs* an `*exec.Cmd` (single return, no error) — it never
runs the subprocess. So the span this hook produces brackets construction, not execution: it
cannot observe or record a genuine subprocess exit-code failure (that happens later, in the
caller's own `cmd.Run()`/`cmd.Wait()`, outside this hook's reach). The only failure the
`AfterCommandContext` hook can observe and record via `span.RecordError` is the caller's `ctx`
already being canceled/past its deadline by the time `CommandContext` returned. This is
documented in the hook's own doc comment (`instrumentation/otelc/safeexec/hook.go`) and is a
deliberate, disclosed scope decision, not an oversight — extending it to cover actual exit-code
failures would require also hooking `os/exec`'s `*exec.Cmd.Run`/`Wait` methods and correlating
them back to the originating span (e.g. via a `context.WithValue` on the `ctx` passed through
`ictx.SetParam`), which is out of this story's scope and not attempted.

---
