# Requirements: go-auto-instrumentation

**Date**: 2026-08-21
**Type**: feature addition
**Complexity**: 3 — system design

## Problem Statement
stapler-squad's OpenTelemetry coverage (`.claude/docs/opentelemetry.md`) is entirely hand-written: `otelhttp` on the HTTP server, `otelconnect` on ConnectRPC, and a handful of manual spans/counters in `telemetry/telemetry.go` (history cache, search, `safeexec.sigkill_escalations`). Every other code path — ent's `database/sql` driver, git operations, tmux/subprocess orchestration, backlog service internals — is untraced, and adding coverage today means hand-writing another span at every call site, which does not scale and drifts out of date as the codebase grows.

[loongsuite-go](https://github.com/alibaba/loongsuite-go) is Alibaba's compile-time auto-instrumentation tool for Go: it wraps `go build` (`otel go build`) and weaves OTel spans/metrics into ~60 supported libraries (`net/http`, `database/sql`, `gorm`, `grpc`, `redis`, `kafka`, `logrus`/`zap`/`slog`, etc.) with no source changes, and exposes a plugin mechanism (`docs/dev/overview.md` in that repo) for injecting custom instrumentation into libraries it doesn't support out of the box.

## Baseline
Today, only the explicitly-instrumented paths above produce spans. Anything not manually wrapped — ent-generated SQL queries, git/tmux subprocess calls, most of `session/` and `server/services/` — is invisible in the Datadog APM trace view. Diagnosing a slow request currently requires either adding a one-off manual span and redeploying, or falling back to the pprof-based profiling workflow (`.claude/docs/profiling.md`).

## Users / Consumers
- Whoever operates the deployed stapler-squad instance and watches Datadog APM for latency/error investigation (the same audience `.claude/docs/opentelemetry.md` already serves).
- Future contributors extending observability, who currently must hand-write a span for every new code path they want visibility into.

## Success Metrics
*(Note: Phase 2/3 research and planning pivoted the target tool from loongsuite-go to `open-telemetry/opentelemetry-go-compile-instrumentation` — see [ADR-001](decisions/ADR-001-target-otelc-over-loongsuite-go.md). The literal `otel go build` command below is loongsuite-go's invocation form; the actual opt-in build path uses `otelc`'s Toolexec Injection or Wrapper Prefix Mode instead, per the plan's "equivalent opt-in make target" escape hatch.)*
- `otel go build` (or an equivalent opt-in make target) produces a working stapler-squad binary that is functionally identical to the normal build (same CLI flags, same behavior) and passes the existing test/e2e suite.
- Running that binary with `OTEL_ENABLED=true` and pointing at an OTLP collector surfaces spans for at least one previously-untraced path (target: ent's `database/sql` driver) that the manual instrumentation does not currently produce — verified by inspecting the collector output, not just "no build errors."
- Per-request overhead of the auto-instrumented binary is measured against baseline (`.claude/docs/benchmarks.md` methodology) and stays within a documented, acceptable bound — not necessarily zero, but known and stated rather than unknown.
- A written recommendation exists on whether/how to move this from opt-in to the default build path, including what would have to be true first (compatibility, overhead, support-burden) — a decision, not necessarily an executed default-flip.

*(Note on Success Metric #4: shipping the recommendation document is this project's contractual deliverable regardless of its verdict — it is not the same thing as closing the Problem Statement's underlying observability gap. If the verdict is "do not adopt as default," that gap remains open and becomes separate, future work; this project's job is to produce a trustworthy verdict backed by real spike/benchmark evidence, not to guarantee the gap closes.)*

## Appetite
Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Prioritization
No single triggering incident motivates this now — it is discretionary infrastructure investment, not an urgent fix, and competes for the same calendar time as other backlog work.
- **Reach**: every future debugging session that touches a currently-untraced path (ent/`database/sql`, git/tmux subprocess calls, most of `session/` and `server/services/` — see Baseline) — today each of those requires either a manual-span-plus-redeploy cycle or falling back to pprof; this removes that entire class of future work if it pans out, not just what this project happens to touch once.
- **Impact**: high leverage *if the tool proves viable* — which is exactly the open question. Confidence is medium: research (`research/build-vs-buy.md`, `research/pitfalls.md`) found near-zero real-world production-adoption precedent for this class of compile-time weaving tool, which is why the plan is spike-first (Phase 1's four validation spikes each carry a pre-declared fallback) rather than committing the full Large appetite up front — a failed Spike A or B stops the project having spent roughly 15–25% of the appetite, not all of it.
- **Effort**: Large (3–6 weeks), explicitly bounded; the Subprocess Hook (Phase 5) is marked first-to-cut if time runs short, so the appetite protects the higher-confidence deliverables (opt-in build path, verification, benchmarking, the recommendation) over the speculative one.

## Constraints
- Opt-in only for this iteration: must not change what `make build`, `make install-service`, or `make ci` produce by default. loongsuite-go is new (compile-time bytecode/AST weaving of a third-party tool from a different ecosystem's primary support community — docs and community channels are partly Chinese-language/DingTalk) and unproven in this codebase; the deployed systemd-managed instance at `:8543` must not be put at risk (per existing "never use `make install-service` for in-progress changes" guidance in CLAUDE.md).
- Must coexist with, not replace, the existing manual instrumentation (`telemetry/telemetry.go`, `otelhttp`, `otelconnect`) — both should be able to run in the same binary without producing broken/duplicate span hierarchies for the same request.
- Must work with this repo's actual build matrix: the `embed_tmux` build tag (`make build-tmux-embed`), `-ldflags "$(LDFLAGS)"`, and cross-platform builds (Linux primary/CI, macOS at work) — not just a bare `go build .`.
- ent-generated code (`session/ent/*.go`, gitignored, regenerated by `make ent-gen`) must exist before `otel go build` runs, same ordering constraint the normal build already has.

## Non-functional Requirements
- **Performance SLO**: not specified numerically; overhead must be measured and documented per the Success Metrics above, using the existing benchmark reference (`.claude/docs/benchmarks.md`).
- **Scalability**: not applicable — this changes how a binary is built and traced, not its runtime capacity.
- **Security classification**: internal (dev/ops tooling). No new PII exposure expected — new spans follow the same OTLP export pipeline as existing telemetry (Datadog agent, `.claude/docs/opentelemetry.md`).
- **Data residency**: no special requirements beyond what the existing OTel setup already has.

## Scope
### In Scope
- Comparative research across Go auto-instrumentation options — loongsuite-go (compile-time weaving), `open-telemetry/opentelemetry-go-instrumentation` (official OTel org, eBPF), Grafana Beyla (eBPF), and a lighter pass over other eBPF-based options (e.g. Odigos) — covering deployment model, privilege requirements, library coverage, and maturity, before committing further work to loongsuite-go specifically.
- Research loongsuite-go's compatibility with this repo's build setup: `embed_tmux` tag, `ldflags`, cross-platform (Linux + macOS) builds, and ent's generate-then-build ordering.
- A dedicated opt-in build path (e.g. `make build-otel-auto`) that produces an auto-instrumented binary alongside (not replacing) the normal build targets.
- Verification that auto-instrumented spans actually appear for at least one currently-untraced path (ent/`database/sql` is the target) without breaking or duplicating the existing manual `otelhttp`/`otelconnect` spans.
- Custom instrumentation, via loongsuite-go's plugin/injection mechanism, for this repo's internal subprocess layer (`executor/safeexec`, git/tmux orchestration in `session/`) if loongsuite-go's built-in library list doesn't cover them (it does not, per the supported-libraries list) and it's feasible within the appetite.
- Overhead/performance benchmarking of the auto-instrumented binary vs. baseline.
- A documented decision (not necessarily execution) on a path toward wiring this into the default build/CI.
- Documentation update to `.claude/docs/opentelemetry.md` (or a new doc it links to) covering the new opt-in build path.

### Out of Scope
- Replacing or removing any existing manual instrumentation.
- Actually flipping `make build`/`make install-service`/CI to use `otel go build` by default — that's a follow-on decision gated on this work's findings, not something this project executes.
- Instrumenting the deployed production instance with the auto-instrumented binary.
- Building or contributing upstream fixes to loongsuite-go itself (bugs found get filed upstream per its own contribution guidance, not patched in a fork).

## Rabbit Holes
- loongsuite-go weaves bytecode/AST at compile time — it may not tolerate `-tags embed_tmux`, custom `-ldflags`, or the way this repo already wraps `go build` in Makefile logic (e.g. embedding a built tmux binary via `go:embed`). This is the single biggest unknown and should be the first thing research validates, before any deeper work.
- The tool's primary support/community channels (DingTalk groups, partly Chinese docs) may mean English-language troubleshooting resources are thin if something breaks non-obviously — budget for reading source/plugin docs directly rather than expecting Stack Overflow-style answers.
- Auto-instrumentation of `net/http`/`database/sql` alongside the existing manual `otelhttp` middleware could double-instrument the same request (nested or duplicate spans) rather than cleanly composing — needs explicit verification, not an assumption that "more instrumentation" is strictly additive.
- Custom injection for the safeexec/tmux/git subprocess layer (in-scope per the Large appetite) depends on an internal plugin API (`docs/dev/overview.md` in the loongsuite-go repo) whose stability/maturity is unknown going in — this could easily balloon past the time budgeted for it; treat it as the first thing to cut if the appetite is tight.
- Binary size and cross-compilation: loongsuite-go ships prebuilt binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 — matches this repo's platforms, but "ships a binary for the platform" isn't the same as "produces a correct cross-compiled *target* binary" (e.g. building a Linux target binary from a macOS host, which this repo's release process may or may not need) — confirm this repo's actual cross-compilation needs before assuming it's covered.

## Alternatives Considered
loongsuite-go is the primary candidate, but Phase 2 research must do a real comparison against other Go auto-instrumentation options before the plan commits to it exclusively — particularly projects under the official `open-telemetry` GitHub org, which carry different support/stability guarantees than a single-vendor (Alibaba) tool. Candidates to evaluate:
- **`open-telemetry/opentelemetry-go-instrumentation`**: the official OTel project's Go auto-instrumentation, eBPF-based — instruments the *running* binary via kernel probes instead of at compile time, so no rebuild is needed. Requires elevated privileges (`CAP_SYS_ADMIN` or root) and kernel/eBPF support, a materially different and heavier deployment model than this project's current single-binary-plus-systemd-service approach. Being an official OTel project is a meaningful trust/longevity signal that loongsuite-go (community/vendor project) doesn't have — worth weighing even though it wasn't the tool the user named.
- **Grafana Beyla**: another eBPF-based, no-code Go (and multi-language) auto-instrumentation project, OTel-native output. Similar deployment model/tradeoffs to the official OTel eBPF instrumentation; worth a light comparison pass.
- **Odigos**: eBPF-based auto-instrumentation platform (broader than just Go), typically deployed as a Kubernetes operator — likely a poor fit for this project's non-k8s systemd-service deployment model, but worth noting why in the research writeup rather than assuming it.
- **Writing more manual spans**: the status quo approach — doesn't scale, is what this project is trying to move away from for broad coverage.

The research phase should produce an explicit compile-time-weaving-vs-eBPF tradeoff comparison (deployment privileges, rebuild requirements, official-OTel-org support vs. vendor project, library coverage, maturity/issue activity) — not just a loongsuite-go feasibility check in isolation.

## Feasibility Risks
- Build-tag/ldflags/embed_tmux incompatibility (see Rabbit Holes) could make the opt-in build path infeasible without upstream changes to loongsuite-go, in which case scope shrinks to "documented findings + no shipped build target."
- Overhead could turn out to be unacceptable for a terminal-multiplexing/session-management tool with tight latency expectations (this repo already tracks perf hotspots — see `perf-mutex-hotspots-2026-07`, `performance-hotfixes-2026-05` in `project_plans/`) — if so, the "path toward default adoption" recommendation would explicitly be "not yet, here's why."
- The plugin/custom-injection work for the safeexec/tmux/git layer may not be achievable within the Large appetite once the build-compatibility research is done; the Scope above already flags this as first-to-cut.

## Observability Requirements
New auto-instrumented spans/metrics flow through the same OTLP export pipeline the existing manual instrumentation uses (`OTEL_EXPORTER_OTLP_ENDPOINT`, Datadog agent OTLP receiver — see `.claude/docs/opentelemetry.md`). No new alerting is required; success is verified by observing new span types (e.g. `database/sql` query spans) in the trace view during manual testing of the opt-in build, not by a new oncall alert.

## Risk Control
Containment is structural, not a runtime flag: the auto-instrumented binary is a separate, opt-in build artifact (its own make target, its own manual-build instance per the "Manual/interactive testing" pattern in this repo's CLAUDE.md — a numbered port block, `STAPLER_SQUAD_INSTANCE`-scoped state dir). It is never the binary `make install-service` deploys, so there is no rollback procedure needed beyond "don't build/run it" — the default build and the live systemd-managed instance are untouched by this work.

## Open Questions
- How does loongsuite-go actually compare to `open-telemetry/opentelemetry-go-instrumentation` (official OTel org, eBPF-based) and Grafana Beyla on: deployment privileges required, whether a rebuild is needed, library coverage for what this repo actually uses, and project maturity/support? Does the official-OTel-org option change the recommendation despite the user naming loongsuite-go specifically?
- Does loongsuite-go's compile-time weaving tolerate this repo's `embed_tmux` build tag and custom `-ldflags` without modification? (First research question if loongsuite-go remains the lead candidate after the comparison above — blocks almost everything else.)
- Does ent's generated `database/sql` driver code get picked up by loongsuite-go's `database/sql` instrumentation, given it's generated immediately before build rather than checked in?
- Is there actual overlap/conflict between loongsuite-go's `net/http` auto-instrumentation and the existing manual `otelhttp` middleware on the same server?
- What does loongsuite-go's custom-injection plugin API (`docs/dev/overview.md`) actually require — is it realistic to target the safeexec/tmux/git subprocess layer with it inside the remaining appetite after build-compatibility research?
- Does this repo have an actual cross-compilation requirement (e.g. building Linux release binaries from a non-Linux CI runner), or do all current build paths run natively on their target platform?
