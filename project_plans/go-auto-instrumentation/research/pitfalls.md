# Research: Pitfalls of Go Auto-Instrumentation (compile-time weaving + eBPF)

**Research Agent 4** — SDD Phase 2, `go-auto-instrumentation`
**Date**: 2026-08-21

## Summary

loongsuite-go is real and actively maintained (26 open issues, last push 2026-08-20), but it
is **mid-rewrite**: its own maintainers have an open roadmap issue to rebuild the whole tool
on top of the official (pre-1.0, "not ready for production") OpenTelemetry compile-instrumentation
project. Independently of that churn risk, its issue tracker documents concrete, still-relevant
failure classes: CGO incompatibility, `-race` detector segfaults (fixed, but the class exists),
`-ldflags`/module-replace collisions, and a `database/sql` instrumentation bug that reproduces
almost exactly with this repo's own cgo sqlite driver. eBPF alternatives trade compile-time
brittleness for a privileged-daemon operational burden that doesn't fit this repo's
single-binary/systemd model. This repo has independently-documented, cycle-counted evidence
that it is sensitive to exactly the kind of opaque, hard-to-attribute overhead auto-instrumentation
would introduce.

---

## 1. loongsuite-go: reported problems (GitHub issues, `alibaba/loongsuite-go`)

Source: `gh api repos/alibaba/loongsuite-go/issues` (queried 2026-08-21; repo has 26 open
issues, most recent push 2026-08-20T03:12:24Z — actively maintained, not abandoned). Note:
`alibaba/loongsuite-go-agent` is the old name/redirect for the same repo.

### 1a. Existential/roadmap risk: the tool itself is being rebuilt

**[Issue #708](https://github.com/alibaba/loongsuite-go/issues/708)** — "Rebuild Loongsuite
Go Agent based on opentelemetry-go-compile-instrumentation, starting from v2.0.0" (open,
`feat`/`effort:high`). The maintainers' own stated plan: once the official OTel org's
`opentelemetry-go-compile-instrumentation` project (currently v0.5.0, README says "not ready
for production use", requires **Go 1.25.0+** vs. loongsuite-go's current **Go 1.24.0**) hits
1.0.0, loongsuite-go will be forked/rebuilt on top of it as v2.0.0, with module path and CLI
tool (`otel` → possibly `otelc`) changes. **Implication**: adopting loongsuite-go now means
adopting a tool that plans a breaking rewrite on an unpinned future date, on top of a
dependency that is explicitly pre-1.0 and not production-ready by its own README.

### 1b. Semantic correctness bugs from woven code (not just build failures)

- **[Issue #742](https://github.com/alibaba/loongsuite-go/issues/742)** — net/http client
  instrumentation hooks `Transport.RoundTrip` and injects a `traceparent` header *after*
  aws-sdk-go has already SigV4-signed the request. On retry, aws-sdk-go reuses the same
  `*http.Request` (traceparent already present → included in `SignedHeaders`), then the
  instrumentation overwrites `traceparent` with a new span ID before resend → signature
  mismatch → intermittent `403 AccessDenied`. Root cause: the instrumentation mutates an
  already-signed request object without any awareness of signing middleware. Fixed via
  [PR #743](https://github.com/alibaba/loongsuite-go/pull/743) (dedicated aws-sdk-go rule),
  but it's evidence of the general failure mode: **woven code that mutates request/response
  objects can silently break correctness in libraries the instrumentation autho... don't know
  about**, and the bug only manifests intermittently (on retries), making it hard to catch in
  a quick smoke test.
- **[Issue #736](https://github.com/alibaba/loongsuite-go/issues/736)** — `database/sql`
  instrumentation's `parseDSN` only recognizes MySQL/Postgres driver names, so SQLite
  (`sqlite3` driver name) produces `failed to parse dsn: invalid DSN`; separately its DDL
  metadata extractor runs SQLite DDL through a MySQL-only SQL parser, which chokes on
  `AUTOINCREMENT` and logs `ignoring error parsing DDL` (spuriously, since the DDL actually
  succeeds). **Directly relevant to this repo**: this repo's `go.mod` depends on both
  `github.com/mattn/go-sqlite3 v1.14.40` (cgo sqlite driver) and `modernc.org/sqlite v1.56.0`
  (pure-Go), and ent's generated code drives `database/sql` against them. This is the closest
  possible match to an untraced path this project explicitly wants instrumented
  (`database/sql` via ent) — and it's a documented, still-open-in-main-as-of-v1.13.0 bug for
  exactly that combination.
- **[Issue #738](https://github.com/alibaba/loongsuite-go/issues/738)** — goredis
  instrumentation writes raw (non-UTF-8) binary Redis command args into the `db.query.text`
  span attribute, which breaks OTLP export because OTLP attribute values must be valid UTF-8.
  Same class as #736: instrumentation assumes text-shaped data and silently produces invalid
  telemetry (or, worse, an export pipeline failure) for binary-shaped real-world data.

### 1c. Build-tag / CGO / ldflags / toolchain incompatibilities

- **[Issue #624](https://github.com/alibaba/loongsuite-go/issues/624)** — "Unable to inject
  package with CGO code". Root cause: when a package uses cgo, the Go toolchain's compile
  step is invoked against toolchain-generated `*.cgo1.go` files (e.g. `consumer.cgo1.go`),
  not the original `consumer.go` — so loongsuite-go's AST-rewriting-by-source-file approach
  cannot find/patch the file it expects, and injection silently fails or errors. **Directly
  relevant**: this repo's `CGO_ENABLED=1` cgo-only `go-sqlite3` driver (see the `// CGO_ENABLED=1`
  comment block at the top of this repo's `go.mod`) is exactly the shape of dependency this
  bug describes.
- **[Issue #631](https://github.com/alibaba/loongsuite-go/issues/631)** — build fails with
  `go: finding module for package go.opentelemetry.io/otel/semconv/v1.37.0/rpcconv` when the
  target app has its *own* `go.mod` `replace` directive pinning `go.opentelemetry.io/otel` to
  an older version than what loongsuite-go's injected instrumentation code imports. The
  instrumentation's injected imports and the app's own OTel SDK version/replace directives
  can collide and produce a broken build — reproduced on a real app built with a normal
  `-ldflags "-X ..."` invocation (i.e., not an exotic build; ordinary version-stamping
  ldflags, the same pattern this repo's own `LDFLAGS` uses for `stapler-squad version`).
- **[Issue #677](https://github.com/alibaba/loongsuite-go/issues/677)** — combining `gorm`
  with `pg`/mysql drivers under `otel go build` v1.18 produced a `cannot use Column ... missing
  method AutoIncrement` compile error not present under plain `go build`; the reporter worked
  around it by manually pre-importing a driver package in `main.go`, which then silently
  auto-upgraded their `gorm` dependency version as a side effect of `otel`'s own dependency
  injection. Evidence of **opaque, build-time dependency graph mutation** — the instrumentation
  tool's `go build` wrapper doesn't just weave AST, it can pull in/upgrade dependencies in ways
  a plain `go build` never would, with confusing failure modes when it goes wrong.
- **[Issue #511](https://github.com/alibaba/loongsuite-go/issues/511)** (closed) and
  **[#456](https://github.com/alibaba/loongsuite-go/issues/456)** (closed) — further
  dependency-resolution build failures (`could not import go.opentelemetry.io/otel/semconv/...`)
  from version skew between the target app's OTel SDK pin and the instrumentation's.
- **[Issue #648](https://github.com/alibaba/loongsuite-go/issues/648)** (closed) — "Cannot
  match any rules on Windows". Not directly relevant (this repo builds Linux/macOS), but shows
  the tool's platform-matching logic itself has had OS-specific failure modes — worth
  spot-checking macOS behavior specifically before relying on it there.

### 1d. `-race` detector interaction

**[Issue #252](https://github.com/alibaba/loongsuite-go/issues/252)** (closed, fixed) —
segfault under `go build -race`: the injected `contextPropagate` snapshot call runs on the
`m0.g0` scheduling goroutine (via `newproc1`), which has no valid `racectx`, but the `-race`
instrumentation's `racefuncenter()` call assumes one exists → crash. Fixed with a
`//go:norace` directive on the offending function. This is now fixed for that specific call
site, but it is direct evidence that **compile-time weaving and Go's own compile-time `-race`
instrumentation can interact** at the level of runtime invariants the race detector assumes
(valid `racectx` per goroutine) — a class of bug, not a one-off, since any newly-injected
low-level call added in a future release could reintroduce the same failure shape at a
different call site.

### 1e. AST weaving directly touches user source

**[Issue #303](https://github.com/alibaba/loongsuite-go/issues/303)** (closed as
acknowledged-but-unfixed design tradeoff) — instrumentation injects `import _ "..."` blank
imports directly into `main.go` rather than a separate generated file (contrast: Datadog's
`orchestrion` outputs a separate `orchestrion.tool.go`). The maintainers agreed this is
confusing but the issue is closed without the suggested fix having shipped. Practical
consequence for this repo: **`otel go build` mutates checked-out source files in place**
(reportedly reverted after build, but the mutation-in-place model means anything that watches
the working tree during a build — a file watcher, an IDE, a `git status` in a concurrent
terminal — will see churn on tracked files during the build, not just in a build output
directory).

---

## 2. General pitfalls of compile-time AST/bytecode weaving (pattern-level, not tool-specific)

This class of tool (loongsuite-go, Datadog's Go `orchestrion`, the OpenTelemetry Java agent's
bytecode weaving, older experiments like `sqreen`'s Go agent) shares structural risks
independent of the specific implementation:

1. **Toolchain-version brittleness.** These tools parse/patch Go's internal compile invocation
   shape (as seen directly in #624's captured `compile` command line) or AST, which is not a
   stable public API — every Go minor release is a potential break. loongsuite-go's own
   `go.mod` currently targets Go 1.24 while this repo is on **Go 1.26.3** (`go.mod` line 3) —
   already two minor versions ahead of loongsuite-go's stated baseline, which is exactly the
   kind of gap issue #631/#511/#456 (module/semconv version mismatches) and #708 (upstream's
   1.25.0+ requirement) come from.
2. **Static-analysis/tooling interaction.** `go vet`, staticcheck, and similar tools operate on
   source; if they're run against the *woven* output rather than original source (or against a
   build that partially failed injection), findings become noisy or misleading. The safer
   design is keeping `go vet`/lint entirely off the auto-instrumented build path — see §5.
3. **`-race` detector interaction.** Demonstrated concretely by #252 above — compile-time
   injection at points the race detector's runtime assumes are goroutine-safe entry points can
   crash, not just produce false positives.
4. **Reproducible-build / supply-chain surface.** An `otel go build` wrapper that mutates
   source in place and can silently pull in or upgrade dependencies (per #677) breaks the
   assumption that `go build` output is a deterministic function of the committed `go.mod`/
   `go.sum` and source tree — relevant if this repo ever wants reproducible or supply-chain-
   attested builds for the auto-instrumented path.
5. **Debugging/stack-trace legibility.** Not documented as a specific loongsuite-go issue found
   in this pass, but it's the generic, widely-observed cost of this technique (same complaint
   historically made about the OTel Java agent and Datadog's Go tracer): injected wrapper
   frames and generated helper functions show up in panics/stack traces and pprof symbolization,
   making crash reports and profiles harder to read than the hand-instrumented baseline. Given
   this repo already leans on `.claude/docs/profiling.md`-driven pprof work as its primary
   performance-debugging tool (see §4), this is a real cost to weigh, not a hypothetical one.

---

## 3. eBPF-based alternatives: pitfalls

### `open-telemetry/opentelemetry-go-instrumentation` and Grafana Beyla (and by extension Odigos, which is built on the same eBPF uprobe technique)

- **Kernel version floor.** General eBPF Go auto-instrumentation needs roughly Linux ≥4.4 at
  the low end; the newer OpenTelemetry eBPF Instrumentation (OBI, the eBPF-based
  zero-code project under the OTel umbrella) states a Linux ≥5.8 requirement (with a
  documented 4.18+ exception for RHEL-family kernels) — a materially higher floor than the
  compile-time approach, which has no kernel dependency at all.
  Sources: [opentelemetry.io/docs/zero-code/obi](https://opentelemetry.io/docs/zero-code/obi/),
  [OpenTelemetry Go Instrumentation repo](https://github.com/open-telemetry/opentelemetry-go-instrumentation).
- **Privileged capabilities required.** Grafana's own docs list the minimum capability set for
  Beyla as `CAP_BPF`, `CAP_PERFMON` (or `CAP_SYS_ADMIN` on older kernels), `CAP_SYS_PTRACE` (to
  read target-process memory for symbol resolution), plus `CAP_DAC_READ_SEARCH` and
  `CAP_CHECKPOINT_RESTORE` for some probe types
  ([Beyla security docs](https://grafana.com/docs/beyla/latest/security/)). None of these are
  available to an ordinary non-root process; running eBPF instrumentation means either a
  privileged/`CAP_*`-elevated sidecar process or launching the instrumented binary itself with
  elevated capabilities.
- **Container/sandbox restrictions.** The instrumenting process (Beyla, the OTel Go
  instrumentation agent) is architecturally a *separate* process/daemon attaching via uprobes
  to the target binary's PID — a fundamentally different deployment shape from this repo's
  single Go binary run under a user-level systemd unit (`make install-service`). Standard
  Docker/Kubernetes default seccomp/AppArmor profiles block raw BPF syscalls unless the
  container is explicitly granted the capabilities above or run `--privileged`.
- **WSL2 relevance (per CLAUDE.md's own mention of WSL2 users).** WSL2 runs a
  Microsoft-maintained custom-built Linux kernel bundled with the WSL2 runtime, not the host
  Windows kernel — eBPF program-type support tracks that bundled kernel's version, which
  historically lags mainline and is not something an individual developer machine can easily
  bump independently of a Windows/WSL update. This is a plausible, not confirmed-by-search,
  risk (search did not surface a specific WSL2+Beyla/OTel-eBPF compatibility report) — flag it
  as **inferred, to be verified hands-on** rather than citing a source for it, since none was
  found.
- **Operational burden vs. this repo's deployment model.** This repo's entire deployment story
  (`.claude/docs/systemd-user-service.md`, `make install-service`) is a single binary managed
  by a user-level systemd unit with no root/privileged component anywhere in the stack. Adding
  an eBPF-based auto-instrumentation path means introducing the *first* privileged/root-adjacent
  component into that story — a materially larger operational and security-review footprint
  than the compile-time approach, which stays inside `go build` and produces an ordinary
  unprivileged binary. For an **opt-in dev/research build path** (this project's actual scope),
  that operational cost is hard to justify relative to loongsuite-go's compile-time approach,
  which at least preserves the single-unprivileged-binary deployment shape even where it has
  its own toolchain-compatibility risk.

---

## 4. This repo's own track record with opaque/latency-sensitive regressions

Checked per the task's instructions (`.claude/rules/fix-flaky-tests-dont-defer.md`,
`project_plans/perf-mutex-hotspots-2026-07/requirements.md`,
`project_plans/performance-hotfixes-2026-05/plan.md`):

- `project_plans/perf-mutex-hotspots-2026-07/requirements.md`: three real, **pprof-measured**
  mutex hotspots from a single 2026-07-01 profiling session, quantified in raw CPU cycles
  (~1.07 trillion cycles combined) and contention-event counts (7,000+ events on one hotspot
  alone) — e.g. a `sync.Mutex` in `session/circular_buffer.go` blocking writers during reads,
  and a per-poll `git status` subprocess fork with no caching in
  `session/git/worktree_git.go`.
- `project_plans/performance-hotfixes-2026-05/plan.md`: five more hotspots from a live pprof
  session, root-caused down to specific lines (e.g. `session/instance_status.go:78`'s
  unconditional `log.Printf` inside `GetStatus`, called on every poll cycle for every session,
  costing 2.2B cycles / 5,094 mutex acquisitions) — each fix paired with a **named
  golangci-lint enforcement rule** (e.g. `no-debug-log-in-hot-poll`) specifically so the
  regression can't silently reappear.
- `.claude/rules/fix-flaky-tests-dont-defer.md` documents this project's explicit policy
  against accepting "known pre-existing, unrelated" as a reason to route around a failure
  signal, precisely because that pattern let two real flakes go unfixed across multiple PRs
  before being root-caused.

**Conclusion for this project**: "opaque compiler-injected overhead in a latency-sensitive
terminal-multiplexing tool" is not a hypothetical risk category for stapler-squad — it is a
close match to hotspots this project has *already* found, root-caused to specific poll-loop
call sites, and guarded with lint rules, using pprof as the diagnostic tool of record. An
auto-instrumentation layer that adds wrapper-function call overhead, extra allocations
(spans/attributes) on hot loops (`ReviewQueuePoller.checkSession`, `GetStatus`, tmux output
streaming), and stack frames that make pprof/panic symbolization harder to read is working
directly against the diagnostic workflow this project already relies on. Any adoption plan
needs pprof-based before/after benchmarking on exactly those previously-identified hot paths,
not just an aggregate "% overhead" number.

---

## 5. Concrete design decisions the plan phase should make

Ranked against the top risks found above (not generic "be careful" advice):

1. **Never let the `otel go build` path touch `go vet`, `-race`, or `make ci`.** Given the
   documented `-race`/injected-code interaction (loongsuite-go #252) and the fact that woven
   source is not what `go vet`/staticcheck are designed to analyze, the opt-in build target
   (e.g. `make build-otel-auto`) must be its own isolated `go build` invocation, entirely
   outside `make ci`/`make quick-check`/`make pre-commit`, and must never be the input to
   `go vet ./...`, `go test -race`, or any static-analysis Make target. This is already implied
   by the requirement's "opt-in only" constraint — make it structurally impossible to
   accidentally wire in, not just docs-discouraged.
2. **Explicitly exclude/guard the `database/sql` rule for both sqlite drivers, or don't ship
   database/sql auto-instrumentation in v1 of this plan.** Given #736 reproduces almost exactly
   with this repo's `mattn/go-sqlite3`/`modernc.org/sqlite` combination (spurious "invalid DSN"
   and "ignoring error parsing DDL" log noise on every DDL statement — i.e. on every
   `make ent-gen`-driven schema operation), the plan should either (a) pick a different
   currently-untraced path to prove out first (git operations, tmux/subprocess layer via the
   custom plugin mechanism — both explicitly in scope already) and defer `database/sql`, or (b)
   budget explicit verification time against both sqlite drivers before claiming
   `database/sql` auto-instrumentation as a deliverable.
3. **Pin the loongsuite-go version and CGO-affected packages explicitly, and test the CGO
   build first, standalone.** Given #624 (CGO packages aren't matched because the compiler
   sees toolchain-generated `*.cgo1.go` files, not original source) and this repo's
   `CGO_ENABLED=1` `mattn/go-sqlite3` dependency, the very first validation step in the plan
   (before any custom plugin work) should be: does `otel go build` even complete successfully
   against this repo's existing `CGO_ENABLED=1` build, with no instrumentation rules touching
   the sqlite driver package, and does the resulting binary run correctly? If that alone fails,
   the appetite risk in the requirements doc ("Build-tag/ldflags/embed_tmux incompatibility
   could make the opt-in build path infeasible") has already materialized before `embed_tmux`
   is even tested — sequence the research/validation phase to hit this first, not last.
4. **Benchmark against the specific hot paths this project has already profiled, not a
   synthetic workload.** Given §4's evidence, the overhead benchmarking task in scope should
   explicitly re-run pprof against `ReviewQueuePoller.checkSession`/`GetStatus`-style poll
   loops and the WebSocket/tmux streaming path (the two hotspot clusters already documented in
   `project_plans/perf-mutex-hotspots-2026-07/` and `project_plans/performance-hotfixes-2026-05/`)
   under the auto-instrumented build, and compare cycle counts/contention events directly
   against those existing baselines — not just report an aggregate "+X% latency" number that
   can't be cross-referenced against this project's own prior findings.
5. **Treat loongsuite-go's #708 rewrite roadmap as a go/no-go gate, not a footnote.** Since the
   tool's own maintainers plan a breaking v2.0.0 rebuild once the pre-1.0 upstream
   `opentelemetry-go-compile-instrumentation` project reaches 1.0.0 (no committed date), the
   plan phase should decide up front whether this project is comfortable being on a tool with
   an open-ended breaking-rewrite risk for an *opt-in, non-default* build path (probably
   acceptable, given the low blast radius) — and should record that decision explicitly rather
   than silently accepting the risk, so a future re-evaluation isn't surprised by it.
