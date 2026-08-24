# Implementation Plan: go-auto-instrumentation

**Feature**: An opt-in, compile-time auto-instrumented build of stapler-squad (`make build-otel-auto` → `stapler-squad-otel`) using `open-telemetry/opentelemetry-go-compile-instrumentation` (`otelc`), gated behind four go/no-go validation spikes, measured against this repo's existing hot-path benchmarks, and ending in a written recommendation on default-build adoption.
**Date**: 2026-08-21
**Status**: Ready for implementation
**ADRs**:
- [ADR-001: Target `otelc` instead of loongsuite-go](../decisions/ADR-001-target-otelc-over-loongsuite-go.md)
- [ADR-002: Gate all build-path work behind four go/no-go spikes](../decisions/ADR-002-spike-first-sequencing.md)
- [ADR-003: Structural opt-in via a separate binary, not a runtime flag](../decisions/ADR-003-structural-opt-in-separate-binary.md)
- [ADR-004: `GOFLAGS` toolexec injection instead of a duplicated build recipe](../decisions/ADR-004-goflags-toolexec-injection.md)
- [ADR-005: Adopt as default once named preconditions hold — not yet, not never](../decisions/ADR-005-default-build-adoption-verdict.md)

---

## Approach Selection (Step 0.5 — CREATIVE pass)

Three high-level approaches were considered before committing to a structure. The chosen one is **A**; **B** and **C** are recorded in the Pattern Decisions table.

**A — Spike-gated opt-in build variant (CHOSEN).** Four small, independently falsifiable validation spikes (build-flag passthrough, CGO/sqlite weave, coexistence with `otelhttp`/`otelconnect`, `OTEL_ENABLED=false` suppression) run first and each write a pass/fail verdict with a pre-declared fallback; only then are the Makefile target, benchmarking, docs, and custom-plugin stories executed.
*Strength*: every load-bearing unknown named in `research/pitfalls.md` and `research/architecture.md` is retired before any story depends on it, and requirements.md's Feasibility Risks each map to a named "what happens next."
*Weakness*: front-loads roughly a third of the appetite on work that produces no shippable artifact — if all four spikes pass cleanly the sequencing reads as ceremony in hindsight.

**B — Build the target first, validate through it (REJECTED).** Write `make build-otel-auto` on day one, then discover incompatibilities empirically by running it.
*Strength*: a shippable artifact exists immediately and every spike exercises the real invocation rather than a reduction, so there's no "it worked in the spike" gap.
*Weakness*: a failed CGO/sqlite weave (`pitfalls.md` §1c, issue #624, which matches this repo's `CGO_ENABLED=1` `mattn/go-sqlite3` dependency exactly) invalidates the target's whole design plus every downstream story at once — precisely the failure requirements.md's Feasibility Risks section asks the plan to sequence around.

**C — Dual-tool matrix from the start (REJECTED).** Implement both an `otelc` path and a loongsuite-go (`otel`) path, and settle the tool choice with a head-to-head empirical comparison.
*Strength*: replaces `build-vs-buy.md`'s documentation-based recommendation with direct evidence, and delivers the fallback tool already working rather than as a paper contingency.
*Weakness*: doubles every downstream story (two build targets, two benchmark matrices, two docs, two plugin mechanisms) inside an appetite that already flags the custom-plugin work as first-to-cut — and `build-vs-buy.md` established the two tools share a mechanism, so the comparison would mostly re-derive the governance/maturity difference the research already settled.

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, target, script, env var, or file name. Exact names here must be used consistently in code, tests, comments, and commit messages.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `otelc` | The CLI binary from `open-telemetry/opentelemetry-go-compile-instrumentation`, the official OTel-org compile-time weaving tool; the primary target of this project. | Not `otel` — that is loongsuite-go's CLI. See ADR-001. |
| Compile-Time Weaving | Injecting OTel span/metric code into a program during compilation, so no source change is required and the instrumentation is baked into the binary. | The mechanism shared by `otelc` and loongsuite-go; contrasted with eBPF runtime attachment. |
| Toolexec Injection | Invoking `otelc` by exporting `GOFLAGS="${GOFLAGS} '-toolexec=otelc toolexec'"` before an otherwise-unmodified `go build`/`go test`, rather than prefixing the command with `otelc go build`. | The integration form `otelc`'s docs recommend for Makefile-owned builds. See ADR-004. |
| Wrapper Prefix Mode | The alternative invocation form, `otelc go build ...`, which replaces the build verb. | Fallback if Toolexec Injection fails Spike A. |
| `build-otel-auto` | The Makefile target that produces the auto-instrumented binary. | Matches the repo's `build-<variant>` convention (`build-embedded`, `build-mux`). |
| `build-otel-auto-embedded` | The `-tags embed_tmux` variant of `build-otel-auto`, mirroring `build-embedded`. | Only built if Spike A passes for `-tags`. |
| `stapler-squad-otel` | The output binary of `build-otel-auto`. | Distinct path from `stapler-squad`; precedent is `stapler-squad-cov` (`coverage-integration`). |
| Baseline Build | A binary produced by the unmodified `make build` / `make build-embedded` path, with no weaving. | The comparison point for every overhead measurement. |
| Auto-Instrumented Build | A binary produced by `build-otel-auto`; i.e. `stapler-squad-otel`. | |
| Woven Span | A span emitted by `otelc`-injected code (e.g. an ent `database/sql` query span). | |
| Manual Span | A span emitted by this repo's hand-written instrumentation: `telemetry.StartSpan`, `otelhttp`, `otelconnect`. | Defined in `telemetry/telemetry.go`. |
| Instrumentation Scope | The OTel `otel.library.name`/`otel.library.version` pair identifying which tracer created a span; the mechanism for telling Woven Spans from Manual Spans in Datadog APM. | Manual Spans carry scope `stapler-squad` (`telemetry.ServiceName`, `telemetry/telemetry.go:26`). |
| Span Census | The counted, per-scope inventory of spans produced by exactly one driven request, used to detect duplication. | The unit of evidence for Spike C. |
| Duplicate Span Hierarchy | Two independent root spans (or a nonsensical parent/child nesting) for a single HTTP/ConnectRPC request, caused by `otelhttp` and a Woven Span both wrapping the same handler. | The failure condition Spike C must rule out. |
| Span Suppression | The property that no Woven Span is exported when this repo's own `OTEL_ENABLED` toggle is false. | The pass condition for Spike D. |
| Injected Bootstrap | Any `init()`-time OTel SDK setup code the weaving tool adds, which runs before `main()` and therefore before `telemetry.Initialize`. | The mechanism `research/architecture.md` §3 identifies as able to bypass `OTEL_ENABLED`. |
| Exporter Toggle | The standard OTel env vars `OTEL_TRACES_EXPORTER` / `OTEL_METRICS_EXPORTER`, set to `none` to silence an Injected Bootstrap. | The remedy if Spike D fails. **Runtime-only**: they are read by the woven binary's own process at startup, so they belong in the *launch* recipe, never in `scripts/otel-auto-build.sh` (which exits before the binary ever runs). See ADR-004. |
| Run Recipe | The documented command line for *running* `stapler-squad-otel` — the manual-port-block invocation plus the `OTEL_ENABLED` / Exporter Toggle environment for each of the two modes (tracing on, tracing off). | Lives in `.claude/docs/opentelemetry-auto-instrumentation.md` (Story 4.1.1) and is exercised by the Suppression Smoke Test. |
| Collector Liveness Check | Proof, taken *immediately before* any zero-span assertion, that the collector under test is up and would have recorded a span if one had been sent — a known span is emitted and observed arriving. | Without it, "zero spans received" is indistinguishable from "nothing was listening." Required by Spike D and by the Suppression Smoke Test. |
| Suppression Smoke Test | The `--suppression` mode of `scripts/otel-auto-smoke.sh`: runs `stapler-squad-otel` with `OTEL_ENABLED=false` against a *liveness-checked* collector and asserts zero spans arrive. | The runtime counterpart of the Collector Smoke Test; Story 2.2.2. |
| Parity Suite Run | Executing this repo's existing test suite under Toolexec Injection, plus driving the existing `tests/e2e/` server-startup path against `stapler-squad-otel`, to substantiate requirements.md's "functionally identical to the normal build … passes the existing test/e2e suite" metric. | Epic 2.3. Recorded in `parity-report.md`. |
| Parity Report | `project_plans/go-auto-instrumentation/implementation/parity-report.md` — the written record of the Parity Suite Run: which suites ran woven, their pass/fail, and any behaviour difference found. | An input to the Adoption Verdict alongside the Overhead Report. |
| Validation Spike | A small, time-boxed experiment whose only deliverable is a Spike Verdict; produces no shipped code. | Spikes A–D. |
| Spike Verdict | A `PASS` / `FAIL` / `PARTIAL` result plus the exact command run, its output, and the resulting next action, appended to the Spike Verdict Log. | |
| Spike Verdict Log | `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md` — the durable, append-only record of all Spike Verdicts. | |
| Collector Smoke Test | An automated check that starts an Auto-Instrumented Build against a local OTLP collector, drives one ent query, and asserts a matching Woven Span arrived. | `scripts/otel-auto-smoke.sh`. Distinguishes "binary built" from "instrumentation actually wove in." |
| Hot Path Baseline | The previously-measured performance of the specific call sites named in `project_plans/perf-mutex-hotspots-2026-07/` and `project_plans/performance-hotfixes-2026-05/` (`GetStatus`, `ReviewQueuePoller.checkSession`, `CircularBuffer`, `GitWorktree.IsDirty`). | Overhead is measured against these, not a synthetic workload. |
| Overhead Delta | The measured runtime difference between Baseline Build and Auto-Instrumented Build across a three-way matrix: baseline, woven+`OTEL_ENABLED=false`, woven+`OTEL_ENABLED=true`. | |
| Build-Time Delta | Wall-clock difference between `make build` and `make build-otel-auto` on a cold and a warm build cache. | |
| Overhead Report | `project_plans/go-auto-instrumentation/implementation/overhead-report.md` — the written Overhead Delta + Build-Time Delta findings. | |
| Module Mutation Guard | A check that `go.mod`/`go.sum` are byte-identical before and after a `build-otel-auto` run. | Guards against `pitfalls.md` §1c's documented silent dependency upgrades. |
| Isolation Guard | A check that `build-otel-auto` is unreachable from `ci`, `ready`, `quick-check`, `pre-commit`, and `install-service`. | Makes the opt-in constraint structural, not documentary — and is itself a prerequisite of `ci` so that a future edit wiring the woven build into CI fails CI. It only reads `make -n` dry-run output; it never invokes `build-otel-auto` or `otelc`. See ADR-003. |
| Subprocess Hook | A custom `otelc` instrumentation rule that wraps `executor/safeexec.CommandContext` / `CommandContextPG` to emit spans for tmux/git subprocess calls. | Phase 5; first thing to cut. |
| Instrumentation Rule | `otelc`'s unit of configuration declaring which package/function to weave and which hook function to call. | Exact shape confirmed in Story 5.1.1. |
| Fallback Tool | `alibaba/loongsuite-go` (CLI: `otel`), adopted only for a specific, named Coverage Gap that `otelc` cannot close. | See ADR-001. |
| Coverage Gap | A concrete, reproduced instrumentation need that `otelc` v1 does not satisfy, documented with the failing command before any Fallback Tool work begins. | |
| Adoption Verdict | The final recommendation on moving auto-instrumentation from opt-in to the default build path, with the preconditions that would have to be true first. | `adoption-recommendation.md` + ADR-005. |

---

## Pattern Decisions

Most PoEAA/DDD patterns are genuinely **N/A** here: this project ships build tooling, shell scripts, Makefile targets, and documentation — there is no aggregate, no persistence layer, no transaction boundary, and (except in Phase 5) no new Go type. Where a pattern family does not apply it is listed as N/A with a reason rather than force-fit.

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall project sequencing | Risk-first spike gating: falsifiable Validation Spikes with pre-declared fallbacks before dependent work | Shape Up / tracer-bullet de-risking (Approach A) | Approach B — build `build-otel-auto` first, validate through it | A failed CGO weave (`pitfalls.md` #624, matching this repo's `CGO_ENABLED=1` `mattn/go-sqlite3`) would invalidate the target's design and every downstream story simultaneously. |
| Tool selection | Single concrete tool (`otelc`), no selector indirection | `.claude/rules/interface-pollution-checklist.md` smell #1 (speculative abstraction) | Approach C — a dual-tool `OTEL_AUTO_TOOL=otelc\|loongsuite` Strategy switch built up front | Second implementation is hypothetical; a Strategy with one real arm is speculative, and doubling every downstream story does not fit the appetite. Add the switch only when a Coverage Gap is actually reproduced. |
| Build invocation | Decorator — `build-otel-auto` decorates the existing `go build` line by setting `GOFLAGS`, re-using rather than restating the recipe | GoF Decorator; `otelc` docs' Makefile-owned-build guidance | Duplicated parallel build recipe (copy the `stapler-squad` target body and swap the verb) | `ux.md` §2: a drifted copy that forgets `-ldflags "$(LDFLAGS)"` silently produces a binary reporting version `dev`, quietly undermining the verification story. See ADR-004. |
| Opt-in containment | Structural separation — distinct target, distinct output binary (`stapler-squad-otel`), plus a machine-checked Isolation Guard | requirements.md Risk Control; `coverage-integration`'s `stapler-squad-cov` precedent | A runtime feature flag inside `telemetry/telemetry.go` gating woven behavior | A runtime flag requires changing shared code on the default build path — exactly what the "must not change what `make build` produces" constraint forbids. See ADR-003. |
| Span suppression when disabled | **Runtime** environment composition in the documented Run Recipe, verified by the Suppression Smoke Test: `OTEL_ENABLED` and the Exporter Toggle are set together *when launching the binary* | `research/architecture.md` §3 remedy | (a) Patching `telemetry.Initialize` to reset the global provider to a no-op when disabled; (b) exporting the Exporter Toggle from `scripts/otel-auto-build.sh` | (a) mutates a code path the default build also executes, to fix a problem only the woven build has. (b) is a category error: the build script's process exits once the binary is written, so env vars it exports cannot reach a later, separate invocation of `stapler-squad-otel`. See ADR-004. |
| Missing-tool handling in the Makefile | Hard-fail `@which otelc >/dev/null 2>&1 \|\| (echo ...; exit 1)` guard | `ux.md` §3; existing `lint-shell`/`sg` guards (`Makefile:653`, `Makefile:659`) | Self-install pattern (as `checklocks` does with `go install`) | Auto-running an external installer from a Make target is a materially riskier action than `go install`; if `otelc` turns out to be `go install`-able (Unresolved Question 2) the self-install pattern becomes admissible and Story 2.1.2 revisits it. |
| Spike record-keeping | Append-only decision log (Spike Verdict Log) + ADRs for choices that outlive the project | This repo's `docs/adr/` + `project_plans/*/decisions/` convention | Editing plan.md in place as spikes resolve | Loses the audit trail; a reader six months out cannot tell what was tried and rejected. |
| Domain modelling (Transaction Script / Domain Model / Repository / Unit of Work / Service Layer) | **N/A** | PoEAA | — | No domain entities, no persistence, no transactions. This project's artifacts are a Makefile target, two shell scripts, three markdown reports, and (Phase 5 only) one Go hook file. |
| Creational / Observer / Factory patterns | **N/A** | GoF | — | No object graph is constructed at runtime by this project; the weaving tool owns construction of its own SDK objects. |
| Type-driven design | Applies **only in Phase 5**: the Subprocess Hook reuses `telemetry/attributes.go`'s `Attr<Concept>` constant + `<Concept>Attr(...)` constructor convention rather than inlining raw `attribute.String("cmd", ...)` keys | `.claude/rules/primitive-obsession-checklist.md`; `type-driven-design` skill | Raw string attribute keys inline at the hook call site | Two same-typed `string` params (command name, argv) at a hook boundary are exactly the swap-silently-wrong shape the checklist exists to catch; the typed-constructor convention already exists in this repo. |
| Sum types / sealed interfaces for state | **N/A in code** — the only state machine here is the Spike Verdict (`PASS`/`FAIL`/`PARTIAL`), which lives in markdown, not Go | type-driven-design | Encoding verdicts as a Go enum | There is no program that reads them; a type would exist only to be read by humans. |

---

## Migration Plan

**Omitted — no schema or data changes.** `session/ent/schema/` is untouched; `make ent-gen` runs unchanged as an existing prerequisite of the new target.

---

## Observability Plan

This project *is* observability work, so this section covers observability **of the build path itself**, not of new application features.

- **Logs**:
  - `build-otel-auto` emits the repo's standard `@echo "✅ ..."` completion line naming the output path (`ux.md` §1 convention), and echoes the exact `GOFLAGS` value it exported so a failed weave is reproducible by hand from the build log.
  - Weaving failures surface through `otelc`'s own verbose/debug mode, enabled by `OTEL_AUTO_DEBUG=1` on the `build-otel-auto` invocation (flag name confirmed against `otelc` docs in Story 1.1.1).
  - `scripts/otel-auto-smoke.sh` prints the per-scope Span Census it asserted on, so a failure says *which* span was missing, not just "assertion failed." Its `--suppression` mode prints both span counts (liveness run, then tracing-off run), so a zero can never be reported without the non-zero that proves the collector was listening.
- **Metrics**: No new application metrics in Phases 1–4. Phase 5's Subprocess Hook, if built, emits spans only; if it also emits a counter it must follow `executor/safeexec/safeexec_metrics.go`'s `<package>.<snake_case_event>` naming and register via `telemetry.GetMeter()` at package `init`.
- **Alerts**: **No new alerts required.** Per requirements.md's Observability Requirements, success is verified by observing new span types during manual/smoke testing of the opt-in build, not by an oncall signal. The Auto-Instrumented Build never runs as the deployed service.

---

## Risk Control

- **Feature flag**: **Not gated by a runtime flag.** Containment is structural (ADR-003): a separate make target producing a separate binary at a separate path (`stapler-squad-otel`), never built by `make build`, `make ci`, `make quick-check`, `make pre-commit`, or `make install-service`, and enforced by the Isolation Guard (Story 2.1.3) — which runs as a prerequisite of `ci`, so an edit that wires the woven build into CI turns CI red instead of going unnoticed. Runtime emission remains gated by the pre-existing `OTEL_ENABLED` / `DD_TRACE_ENABLED` toggles, giving a two-layer net: don't build it, or build it and leave the toggles unset.
- **Rollback procedure**: `rm -f stapler-squad-otel` plus a standard PR revert. Because no default build path, no CI job, and no deployed unit references the new target, reverting cannot affect the running service at `:8543`. If a spike leaves a mutated `go.mod`/`go.sum`, `git checkout -- go.mod go.sum` restores it — the Module Mutation Guard (Story 2.1.4) detects this case automatically.
- **Staged rollout**: **N/A** — the artifact is opt-in and never deployed. Any manual interactive testing uses the repo's manual-instance pattern: `~/.stapler-squad/manual-builds/manual-1/`, `PORT=62871`, `STAPLER_SQUAD_INSTANCE=claude-otel-manual`, `--tmux-keep-server` (CLAUDE.md, "Manual/interactive testing"). `make install-service` is never used for this work.

---

## Unresolved Questions

- [ ] Does `otelc` v1 support Go 1.26.3? `research/features.md` §2 records its documented floor as Go 1.25+, with no stated ceiling; this repo's `go.mod:3` is `go 1.26.3`. — blocks Story 1.1.1 — owner: implementer, resolved by running `otelc version` / reading its compatibility doc as the first task.
- [ ] How is `otelc` distributed — prebuilt binaries, `go install`, or source build? `build-vs-buy.md` §6 explicitly flags this as unconfirmed. Determines the Makefile guard's install message and whether the self-install pattern is admissible. — blocks Story 1.1.1 and Story 2.1.2 — owner: implementer.
- [ ] What Instrumentation Scope name do Woven Spans carry by default? Neither tool's docs state it (`research/features.md` §3). Determines whether Datadog APM can distinguish Woven from Manual Spans without extra work. — blocks Story 1.4.2 — owner: implementer, resolved from real collector output.
- [ ] Does `GOFLAGS` Toolexec Injection apply to `go test` (and therefore to `go test -bench`) the same way it applies to `go build`? If not, the woven benchmark comparison must be driven through a running server + pprof instead of `make benchmark-tier1`, and the Parity Suite Run's Go-test leg is not meaningful (only the e2e/binary leg is). — blocks Story 2.3.1 (which settles it once) and, through it, Story 3.1.2 — owner: implementer.
- [ ] Is an additive `TEST_SERVER_BINARY` env override in `tests/e2e/helpers/test-server.ts` acceptable for pointing the existing e2e suite at `stapler-squad-otel`? Today `buildPath` is settable only via the `TestServer` constructor, and `getGlobalTestServer()` constructs it with no config, so there is no way to redirect the spawned binary without touching that file. The override is a no-op when the env var is unset, so it changes nothing about a normal `npm test` run — but it is a first-party edit outside the woven build's own files. — blocks Story 2.3.2 — owner: implementer; if the answer is no, Story 2.3.2 falls back to its second acceptance criterion (direct `--test-mode` startup parity without Playwright) and the Parity Report records that the Playwright suite was not run against the woven binary, and why.
- [ ] Does `otelc` v1 expose a custom hook/plugin mechanism at all, and in what shape? `build-vs-buy.md` documents its library list but not its extension API; only loongsuite-go's hook mechanism was researched in depth (`research/architecture.md` §5). — blocks Story 5.1.2 — owner: implementer, resolved by Story 5.1.1's feasibility read.
- [ ] Does the macOS `CGO_LDFLAGS="-sectcreate __TEXT __info_plist ..."` Info.plist embedding (`Makefile:149`, `Makefile:279`) survive weaving? Only relevant if the auto-instrumented build is ever needed on macOS. — blocks nothing in the critical path (Linux is primary); deferred to Story 2.1.2's platform note — owner: implementer.

---

## Dependency Visualization

Phase 1's spikes are **strictly sequential**, not parallel siblings. Spike A settles *which invocation form works* (Toolexec Injection vs Wrapper Prefix Mode), and every later woven build — B's included — uses the form A recorded. Spike B then produces `/tmp/ssq-spike-b`, which Stories 1.4.1 and 1.5.1 name literally in their `Given` clauses, so C and D are **children of Spike B, not siblings of it**. Epic 1.3's "no other story may begin until this verdict is recorded" therefore covers the sibling spikes C and D as well as all of Phases 2–6; only Spike A precedes it.

```
Phase 1 — SPIKES (strictly sequential; each gates the next)

 1.1.1 install otelc + create spike verdict log
   │
   ▼
 SPIKE A (tags/ldflags passthrough)          1.2.1 ──► 1.2.2
   │  records the working invocation form (Toolexec vs Wrapper Prefix)
   │  ── soft prerequisite: B/C/D all build with the form A recorded
   ▼
 SPIKE B (CGO + sqlite weave)  ◄── THE GO/NO-GO GATE   1.3.1 ──► 1.3.2
   │  produces /tmp/ssq-spike-b
   │  FAIL ⇒ STOP (see "If a spike fails" below)
   │  ── hard prerequisite: C and D name /tmp/ssq-spike-b in their Given clauses
   ├─────────────────────────────┬──────────────────────────────┐
   ▼                             ▼                              │
 SPIKE C (coexistence)         SPIKE D (OTEL_ENABLED=false)      │
  1.4.1 ──► 1.4.2               1.5.1 (incl. collector-liveness) │
                                  └► 1.5.2 (positive control —   │
                                     ALWAYS runs; remedy leg     │
                                     conditional)                │
   └─────────────────────────────┴──────────────────────────────┘
                            │ all four verdicts recorded
                            ▼
Phase 2 — BUILD PATH
 2.1.1 wrapper script (BUILD-TIME concerns only)
   └► 2.1.2 make target
        ├► 2.1.3 isolation guard ──► wired as a prerequisite of `ci`
        └► 2.1.4 module mutation guard
             ├► 2.2.1 collector smoke test   (tracing ON  ⇒ span present)
             └► 2.2.2 suppression smoke test (tracing OFF ⇒ zero spans,
                       after a Collector Liveness Check)
                        │
                        ▼
             Epic 2.3 — PARITY (requirements.md's "passes the existing
                       test/e2e suite" metric)
              2.3.1 woven Go test suite   ──┐
              2.3.2 CLI + e2e startup parity ┤► 2.3.3 parity-report.md
                        │
        ┌───────────────┼────────────────┐
        ▼               ▼                ▼
Phase 3 — BENCH   Phase 4 — DOCS   Phase 5 — SUBPROC HOOK
 3.1.1 baseline    4.1.1 doc +      5.1.1 feasibility read
 3.1.2 woven 3-way       index      5.1.2 implement hook
   (reuses 2.3.1's                  5.1.3 verify spans
    go-test weaving                 (FIRST TO CUT)
    finding)
 3.1.3 pprof hot
 3.1.4 build-time
 3.1.5 overhead report
        │               │                │
        └───────────────┴────────────────┘
                        ▼
Phase 6 — RECOMMENDATION
 6.1.1 adoption-recommendation.md (synthesises 3.1.5 + 2.3.3 + all verdicts)
   └► 6.1.2 ADR-005 adoption verdict
```

### If a spike fails — pre-declared next actions

| Spike | On FAIL | Consequence for scope |
|---|---|---|
| **A** (tags/ldflags passthrough) | Retry in Wrapper Prefix Mode (`otelc go build -tags embed_tmux -ldflags ...`). If that also fails, ship `build-otel-auto` **without** the `embed_tmux` variant and drop Story 2.1.2's `build-otel-auto-embedded`. | Scope shrinks by one target; core deliverable survives. |
| **B** (CGO + sqlite weave) | Retry with `modernc.org/sqlite` (pure-Go, no CGO) only. If weaving still fails, or ent spans never appear: try the Fallback Tool (loongsuite-go) for this one path *only after* reproducing the failure with an exact command in the Spike Verdict Log — noting `pitfalls.md` #736 documents the same SQLite `database/sql` bug there. If both fail, scope collapses to requirements.md's stated floor: **"documented findings + no shipped build target"**, and Phases 2–5 are cut, leaving Phase 6's recommendation as the sole deliverable. | This is the project's single go/no-go gate. |
| **C** (coexistence) | If a Duplicate Span Hierarchy appears: disable the `net/http` Instrumentation Rule for the woven build (keeping `database/sql`), re-run the Span Census, and document the exclusion in the doc and the Adoption Verdict as a precondition. | Coverage narrows; deliverable survives. |
| **D** (`OTEL_ENABLED=false` suppression) | Apply the Exporter Toggle remedy **at launch time**: the documented Run Recipe (Story 4.1.1) and the Suppression Smoke Test (Story 2.2.2) set `OTEL_TRACES_EXPORTER=none OTEL_METRICS_EXPORTER=none` alongside `OTEL_ENABLED=false`, and Story 1.5.2 re-verifies suppression. The remedy is **not** applied by `scripts/otel-auto-build.sh` — that process exits when the binary is written and cannot influence a later run of it (ADR-004). If suppression still can't be achieved, the Adoption Verdict must record "not suitable for default adoption" as a hard blocker. | Adds a documented Run Recipe requirement and one smoke-test assertion; deliverable survives. |

---

## Phase 1: Validation Spikes (go/no-go)

### Epic 1.1: Tool acquisition and verdict-log scaffolding
**Goal**: `otelc` is installed and its actual (not documented-from-research) version, Go support range, and distribution method are recorded, and a durable place exists to record every subsequent Spike Verdict.

#### Story 1.1.1: Install `otelc` and record its real compatibility facts
**As a** developer starting this project, **I want** `otelc` installed locally with its version and Go-support range written down, **so that** every later spike failure can be attributed to a known tool version rather than an unknown one.
**Acceptance Criteria**:
- `otelc` is on `PATH` and reports a version, and that version is recorded in the Spike Verdict Log alongside the install method used.
  - *Given* a machine with Go 1.26.4 installed and no `otelc` binary on `PATH`, *When* the implementer follows `open-telemetry/opentelemetry-go-compile-instrumentation`'s documented install path and runs `otelc version`, *Then* a version string of at least `v1.0.0` is printed and that exact string plus the install command appears in `spike-verdicts.md` under a "Tool acquisition" heading.
- The three Unresolved Questions about `otelc` distribution and Go-version support are answered with a citation, or explicitly recorded as still-unknown.
  - *Given* `otelc`'s published compatibility documentation, *When* the implementer reads it for a supported-Go-version statement, *Then* `spike-verdicts.md` records either "supports Go 1.26.x — <URL>" or "no ceiling stated; 1.26.3 untested upstream — <URL>", and in the latter case Spike B's verdict must explicitly note whether any failure could be a Go-version artifact.
**Files**: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.1.1a: Install the tool (~4 min)
- Fetch the project's install instructions from its repo (README / `docs/getting-started.md`); prefer a prebuilt binary or `go install` over a source build.
- Install to a `PATH` location; run `otelc version` and `command -v otelc`.
- Files: none (environment only)

##### Task 1.1.1b: Create the Spike Verdict Log with the tool-acquisition entry (~4 min)
- Create `spike-verdicts.md` with a fixed per-entry template: `## <Spike ID> — <name>`, `**Verdict**: PASS|FAIL|PARTIAL`, `**Command**:` (fenced), `**Output**:` (fenced, trimmed), `**Next action**:`.
- Write the tool-acquisition entry: version string, install command, distribution method, Go-support statement + URL.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.1.1c: Record the debug-mode invocation (~3 min)
- Find `otelc`'s verbose/debug flag or env var in its docs; record the exact form in `spike-verdicts.md` so every later spike can be re-run verbosely without re-reading docs.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

---

### Epic 1.2: Spike A — build-flag passthrough
**Goal**: Prove that Toolexec Injection leaves `-tags embed_tmux` and `-ldflags "$(LDFLAGS)"` semantically intact, since `research/architecture.md` §1 marks this UNVERIFIED and requirements.md names it the single biggest unknown.

#### Story 1.2.1: Verify `-ldflags` version stamping survives weaving
**As a** developer, **I want** proof that a woven binary still reports the real version, **so that** a silently-`dev`-stamped binary can't invalidate later verification work.
**Acceptance Criteria**:
- A woven build with `-ldflags "-X main.version=spikeA"` produces a binary whose `version` subcommand prints `spikeA`.
  - *Given* the repo at HEAD with `make ent-gen` and `make proto-gen` already run, *When* the implementer exports `GOFLAGS="${GOFLAGS} '-toolexec=otelc toolexec'"` and runs `go build -ldflags "-X main.version=spikeA" -o /tmp/ssq-spike-a .`, *Then* `/tmp/ssq-spike-a version` prints `spikeA` and not `dev`.
- The result is recorded as a Spike Verdict with the exact command and output.
  - *Given* the command above has been run, *When* it succeeds or fails, *Then* `spike-verdicts.md` gains a `## Spike A.1 — ldflags passthrough` entry with verdict, command, trimmed output, and next action.
**Files**: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.2.1a: Run the prerequisite generators (~3 min)
- Run `make proto-gen ent-gen` so `gen/proto/go/` and `session/ent/*.go` exist (`architecture.md` §1: these are file-existence prerequisites, independent of the build verb).
- Files: none (generated, gitignored output)

##### Task 1.2.1b: Run the ldflags weave and check the version string (~5 min)
- Export `GOFLAGS` per the Toolexec Injection form; run the `go build` above; run `/tmp/ssq-spike-a version`.
- If Toolexec Injection errors out, re-run in Wrapper Prefix Mode (`otelc go build -ldflags ... -o /tmp/ssq-spike-a .`) and record which mode worked.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

#### Story 1.2.2: Verify the `embed_tmux` build tag and `go:embed` survive weaving
**As a** developer, **I want** proof that the tag-gated embedded-tmux build still works when woven, **so that** `build-otel-auto-embedded` is known to be viable before it is written.
**Acceptance Criteria**:
- A woven `-tags embed_tmux` build produces a binary that resolves its embedded tmux.
  - *Given* `make build-tmux-embed` has placed a real tmux binary at `session/tmux/embed/tmux`, *When* the implementer runs the woven build with `-tags embed_tmux -ldflags "-X main.version=spikeA2" -o /tmp/ssq-spike-a2 .`, *Then* the build exits 0 and `/tmp/ssq-spike-a2 version` prints `spikeA2`.
- The woven embedded binary is materially larger than a woven non-embedded one, confirming the `go:embed` blob actually made it in.
  - *Given* both `/tmp/ssq-spike-a` and `/tmp/ssq-spike-a2` exist, *When* their sizes are compared, *Then* `/tmp/ssq-spike-a2` is at least 500 KB larger (a bundled tmux 3.4 binary), and both sizes are recorded in the Spike Verdict.
**Files**: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.2.2a: Build and stage the pinned tmux (~5 min)
- Run `make build-tmux-embed` (initialises the `third_party/tmux` submodule if needed via `make init-submodules`).
- Files: none (build artifact into `session/tmux/embed/`, gitignored)

##### Task 1.2.2b: Run the tagged weave and compare binary sizes (~4 min)
- Run the woven `-tags embed_tmux` build; run `version`; compare sizes of the two spike binaries.
- Record the Spike A verdict (combining A.1 and A.2), and on FAIL record which of the two pre-declared fallbacks applies.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

---

### Epic 1.3: Spike B — CGO and SQLite weaving (the go/no-go gate)
**Goal**: Prove that `otelc` weaves a `CGO_ENABLED=1` build containing `mattn/go-sqlite3` without the failure class `pitfalls.md` §1c documents (issue #624: cgo packages compile from toolchain-generated `*.cgo1.go` files the weaver can't match), and that ent's `database/sql` calls actually produce Woven Spans. **No other story may begin until this verdict is recorded** — including the sibling spikes C and D, whose acceptance criteria name `/tmp/ssq-spike-b` (this epic's output binary) as a `Given`. Only Spike A precedes this epic, because it settles the invocation form this epic's build command uses.

#### Story 1.3.1: Verify the CGO build completes and the binary runs
**As a** developer, **I want** to know whether weaving survives this repo's mandatory CGO SQLite dependency, **so that** the project's single biggest technical risk is settled before any target, benchmark, or doc work starts.
**Acceptance Criteria**:
- A woven `CGO_ENABLED=1` build of the full repo exits 0.
  - *Given* the repo at HEAD with generators run and `CGO_ENABLED=1` in the environment, *When* the implementer runs the woven build to `/tmp/ssq-spike-b`, *Then* the command exits 0; if it exits non-zero, the full stderr is captured verbatim into the Spike Verdict and checked against the `*.cgo1.go` signature from `pitfalls.md` #624.
- The resulting binary starts, serves, and shuts down cleanly on an isolated manual instance.
  - *Given* `/tmp/ssq-spike-b` exists, *When* it is started with `PORT=62871 STAPLER_SQUAD_INSTANCE=claude-otel-spike /tmp/ssq-spike-b --tmux-keep-server &` and `curl -sf http://localhost:62871/` is issued, *Then* the HTTP request returns 200 and the process exits cleanly on `kill %1`, with no panic in its log.
**Files**: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.3.1a: Run the CGO weave and capture full output (~5 min)
- Run the woven build with `CGO_ENABLED=1`, teeing stderr to a file; on failure grep it for `cgo1.go` and for the sqlite driver package path.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.3.1b: Smoke-run the woven binary on the manual port block (~5 min)
- Start on `PORT=62871` with `STAPLER_SQUAD_INSTANCE=claude-otel-spike` and `--tmux-keep-server`; curl the root; kill it. Never `make install-service`.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.3.1c: Record the Spike B.1 verdict and the fallback branch taken (~3 min)
- Write the verdict; on FAIL, record which pre-declared next action from the "If a spike fails" table is being taken.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

#### Story 1.3.2: Verify ent `database/sql` Woven Spans reach a collector
**As an** operator, **I want** a real `db.*` span from an ent query in collector output, **so that** requirements.md's core success metric ("spans for at least one previously-untraced path") is proven rather than assumed from a clean build.
**Acceptance Criteria**:
- Driving one ent-backed request against the woven binary produces at least one span with a `db.system` attribute in the collector output.
  - *Given* an OTLP collector (or `otelcol-contrib` with a `debug`/`logging` exporter) listening on `localhost:4317` and `/tmp/ssq-spike-b` running with `OTEL_ENABLED=true`, *When* the implementer drives one session-list request (`curl -sf http://localhost:62871/` plus a ConnectRPC `ListSessions` call), *Then* the collector log contains at least one span whose attributes include `db.system` with a sqlite-family value, and that span's name and attributes are pasted into the Spike Verdict.
- The `pitfalls.md` #736 failure signature is explicitly checked for and its presence or absence recorded.
  - *Given* the same collector run, *When* the implementer greps the binary's own stderr and the collector log for `invalid DSN` and `parsing DDL`, *Then* the Spike Verdict states either "signature absent" or quotes the matching lines and marks the verdict PARTIAL with a noise-severity assessment.
**Files**: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.3.2a: Stand up a local OTLP collector (~5 min)
- Run an OTLP collector on `localhost:4317` with a debug/logging exporter (matching `telemetry.DefaultConfig()`'s default endpoint, `telemetry/telemetry.go:29`); confirm it logs a received request.
- Files: none (throwaway collector config under the scratchpad)

##### Task 1.3.2b: Drive an ent query and capture the span (~5 min)
- Start `/tmp/ssq-spike-b` with `OTEL_ENABLED=true` on `PORT=62871`; issue a session-list request; capture the collector's span dump.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.3.2c: Check for the #736 SQLite signature and write the verdict (~4 min)
- Grep for `invalid DSN` / `parsing DDL`; record Spike B's combined verdict and the next action.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

---

### Epic 1.4: Spike C — coexistence with `otelhttp` / `otelconnect`
**Goal**: Prove that Woven Spans and Manual Spans compose into one sane trace rather than a Duplicate Span Hierarchy, and that the two are distinguishable in the trace view.
**Prerequisite**: Epic 1.3 (Spike B) complete — every story here runs against `/tmp/ssq-spike-b`. Do not substitute an ad hoc binary built outside Spike B's exact CGO/sqlite invocation; the whole point of this spike is that it runs on the artifact whose weave Spike B validated.

#### Story 1.4.1: Span Census for a single request
**As an** operator triaging a slow request, **I want** one request to produce one coherent trace, **so that** enabling auto-instrumentation doesn't make the existing Datadog APM view worse than it is today.
**Acceptance Criteria**:
- Exactly one root span exists for one driven HTTP request against the woven binary.
  - *Given* the collector from Story 1.3.2 and `/tmp/ssq-spike-b` running with `OTEL_ENABLED=true`, *When* exactly one `curl -sf http://localhost:62871/api/...` request is issued and the collector output for that trace ID is inspected, *Then* exactly one span has an empty parent span ID, and its Instrumentation Scope is recorded.
- A baseline Span Census from the unwoven binary is captured for comparison.
  - *Given* the same collector, *When* the same single request is driven against a Baseline Build (`./stapler-squad`, also `OTEL_ENABLED=true`), *Then* both censuses (span name, scope, parent) are tabulated side by side in the Spike Verdict, and any span present twice in the woven run but once in the baseline run is called out by name.
**Files**: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.4.1a: Capture the baseline Span Census (~5 min)
- Build/reuse `./stapler-squad`; run it on `PORT=62873` (manual instance #2) with `OTEL_ENABLED=true`; drive one request; tabulate spans by name/scope/parent.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.4.1b: Capture the woven Span Census and diff (~5 min)
- Repeat against `/tmp/ssq-spike-b`; diff the two tables; count root spans in each.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

#### Story 1.4.2: Confirm Woven Spans are distinguishable from Manual Spans
**As an** operator, **I want** to tell at a glance whether a span came from woven or hand-written code, **so that** a bug in generated instrumentation isn't mistaken for a bug in first-party code (`research/features.md` §3).
**Acceptance Criteria**:
- Every span in the woven census carries an Instrumentation Scope, and Woven Spans' scope differs from `stapler-squad`.
  - *Given* the woven Span Census from Story 1.4.1, *When* each span's `otel.library.name` is read, *Then* Manual Spans show `stapler-squad` (per `telemetry.ServiceName`, `telemetry/telemetry.go:26`) and Woven Spans show a different, non-empty scope, whose exact value is recorded verbatim in the Spike Verdict for later use in the doc.
- If Woven Spans carry the same scope as Manual Spans, that is recorded as a documented limitation with a filtering workaround.
  - *Given* the scope values collected above, *When* they are identical, *Then* the Spike Verdict is marked PARTIAL and records the fallback discriminator to be documented instead (e.g. span-name prefix or `db.system` presence).
**Files**: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.4.2a: Extract and tabulate Instrumentation Scopes (~4 min)
- From the captured collector output, extract `otel.library.name`/`version` per span; record the woven scope string verbatim.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.4.2b: Write the Spike C verdict (~3 min)
- Combine 1.4.1 and 1.4.2 into one Spike C verdict; on a Duplicate Span Hierarchy, record the `net/http`-rule-exclusion next action.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

---

### Epic 1.5: Spike D — `OTEL_ENABLED=false` suppression
**Goal**: Settle `research/architecture.md` §3's flagged risk that an Injected Bootstrap sets its own global provider at `init()` time, before `main()` reaches `telemetry.Initialize` (`main.go:262`), and therefore exports spans regardless of this repo's own toggle.
**Prerequisite**: Epic 1.3 (Spike B) complete — every story here runs against `/tmp/ssq-spike-b`, same as Epic 1.4.
**Methodology note (why this spike needs a control)**: this spike's pass condition is a *negative* — "zero spans received" — and a dead or unreachable collector produces exactly that observation for the wrong reason. Every zero-span assertion below is therefore bracketed by a Collector Liveness Check before it (Story 1.5.1) and a positive control after it (Story 1.5.2), neither of which is skippable.

#### Story 1.5.1: Verify no spans are exported when telemetry is disabled
**As a** developer running the Auto-Instrumented Build locally, **I want** `OTEL_ENABLED` unset to mean no telemetry leaves the process, **so that** the opt-in build honours the same off-switch every other build in this repo honours.
**Acceptance Criteria**:
- A Collector Liveness Check passes immediately before the suppression run, so a zero-span result cannot be a dead collector.
  - *Given* the local collector on `localhost:4317` freshly (re)started and its log truncated, *When* `/tmp/ssq-spike-b` is run *first* with `OTEL_ENABLED=true` and one HTTP request is driven against it, *Then* at least one span is observed in the collector log and the observation is pasted into the Spike Verdict as `Collector Liveness Check: PASS (<N> spans, <first span name>)`. If zero spans arrive here, Spike D is **blocked, not passed** — the collector or the exporter endpoint is misconfigured and must be fixed before the suppression run is meaningful.
  - *Given* the liveness check passed, *When* the collector log is truncated again for the suppression run, *Then* the truncation is the only intervening action — the collector process is **not** restarted between the liveness check and the suppression run, so the same proven-live collector observes both.
- With `OTEL_ENABLED` and `DD_TRACE_ENABLED` unset, that same proven-live collector receives zero spans from the woven binary.
  - *Given* the liveness-checked collector with its log truncated, *When* `/tmp/ssq-spike-b` is started with neither `OTEL_ENABLED` nor `DD_TRACE_ENABLED` set and one HTTP request is driven against it, *Then* the collector log contains zero received spans, and the request's success, the empty collector log, and a back-reference to the liveness check are all recorded in the Spike Verdict.
- The binary logs the existing "telemetry disabled" line, proving `telemetry.Initialize` took its disabled path.
  - *Given* the same run, *When* the binary's log is inspected, *Then* it contains `telemetry disabled (set OTEL_ENABLED=true or DD_TRACE_ENABLED=true to enable)` (`telemetry/telemetry.go:94`) — confirming the observation is about woven code, not about a misconfigured toggle.
**Files**: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.5.1a: Collector Liveness Check (~4 min)
- Start/restart the collector; truncate its log; run `/tmp/ssq-spike-b` with `OTEL_ENABLED=true`; drive one request; confirm ≥1 span arrives and record the count and first span name.
- If zero spans arrive, stop: fix the collector/endpoint and re-run this task. Do **not** proceed to 1.5.1b — the suppression result would be uninterpretable.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.5.1b: Truncate the log (collector left running) and run with telemetry off (~4 min)
- Truncate the collector log **without restarting the collector process**; `env -u OTEL_ENABLED -u DD_TRACE_ENABLED` run the woven binary; drive one request; inspect the collector log.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.5.1c: Confirm the disabled-path log line and record the verdict (~3 min)
- Grep the binary's log for the `telemetry disabled` line; write the Spike D.1 verdict, including the liveness-check evidence from Task 1.5.1a.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

#### Story 1.5.2: Positive control (always) and, if spans leaked, the Exporter Toggle remedy
**As a** developer, **I want** the disabled-run result confirmed by a positive control on the same collector, and a proven one-command way to silence an Injected Bootstrap if one leaks, **so that** "zero spans" is evidence of suppression rather than evidence of a dead collector, and a leaking woven exporter becomes a documented operational note rather than a blocker.
**Acceptance Criteria**:
- **The positive control runs unconditionally**, on the same collector process and immediately after Story 1.5.1's disabled run, whatever verdict 1.5.1 reached.
  - *Given* Story 1.5.1's disabled run has completed — **PASS or FAIL, no exceptions** — *When* the woven binary is re-run with `OTEL_ENABLED=true` against the still-running collector and one request is driven, *Then* the `db.system` span from Story 1.3.2 arrives, and the Spike D verdict records it as `Positive control: PASS (<N> spans)`.
  - *Given* the positive control instead observes zero spans, *When* the verdict is written, *Then* Story 1.5.1's result is **retracted, not kept** — recorded as `Spike D.1 — INVALID (collector not confirmed live; zero-span result is uninterpretable)` — and 1.5.1 is re-run after the collector is fixed. A suppression PASS against an unconfirmed-live collector is not evidence of suppression.
  - Rationale: this control and Story 1.5.1a's liveness check bracket the negative assertion on both sides. Making it conditional on a 1.5.1 PASS would skip it in exactly the case it exists to catch.
- **The Exporter Toggle remedy leg is the only conditional part**, and its skip is recorded rather than silent.
  - *Given* Story 1.5.1 recorded a non-zero span count with telemetry disabled, *When* the woven binary is re-run with `OTEL_TRACES_EXPORTER=none OTEL_METRICS_EXPORTER=none` and neither `OTEL_ENABLED` nor `DD_TRACE_ENABLED` set, *Then* the collector receives zero spans, and the pairing requirement is recorded as a **runtime** requirement for the Run Recipe (Story 4.1.1) and the Suppression Smoke Test (Story 2.2.2) — *not* for `scripts/otel-auto-build.sh`, which cannot affect a later run of the binary (ADR-004).
  - *Given* Story 1.5.1's verdict is PASS **and** the positive control passed, *When* this story is reached, *Then* `spike-verdicts.md` records `Spike D.2 remedy leg — N/A (D.1 passed with a confirmed-live collector; no Injected Bootstrap leak observed)`, the positive-control evidence is recorded anyway, and Story 2.2.2's suppression assertion still ships (it asserts the *property*, not the remedy).
**Files**: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.5.2a: Run the unconditional positive control (~4 min)
- On the same still-running collector, re-run the woven binary with `OTEL_ENABLED=true`; drive one request; confirm the `db.system` span arrives; record the count.
- If zero spans arrive, mark Spike D.1 INVALID and return to Task 1.5.1a.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.5.2b: If and only if D.1 leaked, run the Exporter Toggle remedy (~4 min)
- Run with `OTEL_TRACES_EXPORTER=none OTEL_METRICS_EXPORTER=none` and telemetry off; count received spans.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 1.5.2c: Write Spike D's combined verdict (~4 min)
- Record the liveness check, the disabled run, the positive control, and (if run) the remedy, plus the exact env pairing the **Run Recipe** must apply — naming it as a launch-time, not build-time, requirement.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

---

## Phase 2: The opt-in build path

### Epic 2.1: `make build-otel-auto`
**Goal**: A repo-idiomatic, structurally isolated build target producing `stapler-squad-otel`, that cannot leak into the default build, CI, or the deployed service.

#### Story 2.1.1: Build wrapper script
**As a** developer, **I want** the weaving environment composed in one auditable script, **so that** the Makefile target stays a thin decorator over the existing `go build` line rather than a drifting copy of it.

**Scope — build-time concerns only.** This script's process exists only for the duration of the build and exits once the binary is written. It therefore owns exactly: the Toolexec Injection `GOFLAGS` (or Wrapper Prefix Mode, per Spike A), the `command -v otelc` guard, the `-tags`/`-ldflags` passthrough, the `GOFLAGS` echo, and the Module Mutation Guard's pre/post `go.mod`/`go.sum` checksums. It **must not** set `OTEL_ENABLED`, `OTEL_TRACES_EXPORTER`, `OTEL_METRICS_EXPORTER`, or any other runtime telemetry variable: a later, separate invocation of `stapler-squad-otel` inherits nothing from this long-dead process, so such code would be inert and would falsely imply suppression is handled. Runtime suppression lives in the Run Recipe (Story 4.1.1) and is verified by Story 2.2.2. See ADR-004.

**Acceptance Criteria**:
- `scripts/otel-auto-build.sh` exports the Toolexec Injection `GOFLAGS`, then runs the caller-supplied `go build` argv as a child process — not `exec`, because Story 2.1.4's Module Mutation Guard must run checksum comparisons after the build finishes, which nothing can do once `exec` has replaced the shell's process image — and exits with the child's exit code (or a checksum-mismatch failure, per Story 2.1.4).
- The script sets no runtime telemetry environment variable.
  - *Given* the finished script, *When* it is grepped for `OTEL_ENABLED`, `OTEL_TRACES_EXPORTER`, and `OTEL_METRICS_EXPORTER`, *Then* the only matches permitted are inside a comment pointing at the Run Recipe; any `export` of one of them is a defect.
  - *Given* `otelc` on `PATH` and generators already run, *When* a developer runs `./scripts/otel-auto-build.sh go build -ldflags "-X main.version=1.2.3" -o stapler-squad-otel .`, *Then* `stapler-squad-otel` is created and `./stapler-squad-otel version` prints `1.2.3`.
- The script fails loudly and non-zero when `otelc` is missing.
  - *Given* a `PATH` with no `otelc`, *When* the script is run, *Then* it exits non-zero and prints an install hint naming `open-telemetry/opentelemetry-go-compile-instrumentation`, matching the tone of `Makefile:653`'s `shellcheck` guard.
- The script passes `shellcheck`, which `make ready` already runs over first-party shell scripts.
  - *Given* the finished script, *When* `make lint-shell` is run, *Then* it exits 0 with no findings for `scripts/otel-auto-build.sh`.
**Files**: `scripts/otel-auto-build.sh`

##### Task 2.1.1a: Write the script (~5 min)
- `set -euo pipefail`; `command -v otelc` guard with the install hint; export `GOFLAGS` in the Toolexec Injection form recorded by Spike A; run `"$@"` as a child process (`"$@"`, deliberately not `exec "$@"` — see Story 2.1.1's scope note) and capture its exit code into `$build_status`. Task 2.1.4a adds the checksum capture/compare around this same child invocation; the script's final line is `exit "$build_status"` unless Task 2.1.4a's mismatch check overrides it.
- Echo the exported `GOFLAGS` value before running the build (Observability Plan).
- Add a one-line comment stating that runtime telemetry env vars deliberately do **not** belong here (the process exits before the binary runs) and pointing at `.claude/docs/opentelemetry-auto-instrumentation.md`'s Run Recipe.
- Files: `scripts/otel-auto-build.sh`

##### Task 2.1.1b: Make it executable and shellcheck-clean (~3 min)
- `chmod +x`; run `make lint-shell`; fix findings.
- Files: `scripts/otel-auto-build.sh`

#### Story 2.1.2: The Makefile target(s)
**As a** developer, **I want** `make build-otel-auto` to behave like every other `build-<variant>` target here, **so that** it's discoverable via `make help` and needs no special knowledge to run.
**Acceptance Criteria**:
- `build-otel-auto` reuses the existing prerequisite chain and produces a distinctly-named binary.
  - *Given* a clean checkout with submodules initialised, *When* `make build-otel-auto` is run, *Then* `ensure-tools proto-gen ent-gen server/web/dist` run first, `stapler-squad-otel` is created, `./stapler-squad` is **not** modified (verified by comparing its mtime before and after), and the target prints `✅ stapler-squad built with otelc auto-instrumentation → ./stapler-squad-otel`.
- The target appears in `make help` and in `.PHONY`.
  - *Given* the edited Makefile, *When* `make help` is run, *Then* a line for `build-otel-auto` with its `##` help text is printed, and `build-otel-auto` appears in the `.PHONY` list at `Makefile:60`.
- A missing `otelc` produces the hard-fail message, not a confusing `go build` error.
  - *Given* a `PATH` without `otelc`, *When* `make build-otel-auto` is run, *Then* it exits non-zero with the install hint and no `go build` is attempted.
- The `embed_tmux` variant exists if and only if Spike A passed for `-tags`.
  - *Given* Spike A's verdict is PASS, *When* `make build-otel-auto-embedded` is run after `make build-tmux-embed`, *Then* `stapler-squad-otel` is produced with the embedded tmux blob; *Given* Spike A's verdict is FAIL for `-tags`, *Then* the target is not added and `spike-verdicts.md` is cited in the Makefile comment explaining its absence.
**Files**: `Makefile`, `.gitignore`

##### Task 2.1.2a: Add the target bodies (~5 min)
- Add `build-otel-auto` (and conditionally `build-otel-auto-embedded`) mirroring `build-embedded`'s shape (`Makefile:277-286`): same prerequisites, `which otelc` hard-fail guard, `./scripts/otel-auto-build.sh go build ... -o stapler-squad-otel .`, `@echo "✅ ..."`.
- Do **not** add a macOS `CGO_LDFLAGS` Info.plist branch yet; leave a comment referencing Unresolved Question 6.
- Files: `Makefile`

##### Task 2.1.2b: Register in `.PHONY` and `.gitignore` (~3 min)
- Append the new target name(s) to the `.PHONY` list at `Makefile:60`.
- Add `stapler-squad-otel` to `.gitignore` next to the existing `stapler-squad` / `stapler-squad.prev` entries (lines 4–5).
- Files: `Makefile`, `.gitignore`

##### Task 2.1.2c: Verify help output and default-build non-interference (~4 min)
- Run `make help | grep build-otel-auto`; stat `./stapler-squad` before and after `make build-otel-auto` to confirm it is untouched.
- Files: none (verification only)

#### Story 2.1.3: Isolation Guard
**As a** maintainer, **I want** it to be mechanically impossible to wire the woven build into CI or the deployed service, **so that** the opt-in constraint survives future edits by someone who hasn't read this plan.
**Acceptance Criteria**:
- A check fails if `build-otel-auto` becomes reachable from any default/CI/deploy target.
  - *Given* the Makefile with the new target, *When* the guard runs `make -n ci`, `make -n ready`, `make -n quick-check`, `make -n pre-commit`, and `make -n install-service` and greps each for `otelc` and `stapler-squad-otel`, *Then* all five produce zero matches and the guard exits 0.
  - *Given* a hypothetical edit adding `build-otel-auto` to `ci`'s prerequisite list, *When* the guard runs, *Then* it exits non-zero naming `ci` as the offending target.
- The guard is runnable as its own make target **and is a prerequisite of `ci`**, so a future edit that wires the woven build into CI turns CI red instead of going unnoticed.
  - *Given* the finished guard, *When* `make otel-auto-isolation-guard` is run, *Then* it prints a per-target PASS line for each of the five targets checked.
  - *Given* `otel-auto-isolation-guard` appended to `ci`'s prerequisite list (`Makefile:785`) and to `.PHONY`, *When* `make ci` runs, *Then* the guard executes as part of it and `make ci` fails if any of the five targets would reach `otelc` or `stapler-squad-otel`. `ready` (`Makefile:794`) inherits this automatically, since `ready: ci ...`.
  - *Given* the guard now runs inside `ci`, *When* it executes `make -n ci` on itself, *Then* it terminates and does not recurse: `make -n` prints recipe lines without executing them, and the guard's own recipe line is a plain `./scripts/otel-auto-isolation-guard.sh` invocation, not a `$(MAKE)` sub-make.
  - *Given* the guard is now named in `ci`'s output, *When* its own regex is applied to that output, *Then* it still passes: the guard matches only `otelc` and `stapler-squad-otel`, and neither the target name `otel-auto-isolation-guard` nor the filename `scripts/otel-auto-isolation-guard.sh` contains either string.
**Justification for including it in `ci` (correcting the earlier reasoning).** Excluding it was justified as "wiring an otel-auto-named target into CI is the coupling it forbids." That is a category error, and it defeated the guard's stated purpose — a guard that runs only when manually invoked cannot catch the future edit it exists to catch. The guard **dry-runs**: it executes `make -n <target>` and greps the printed output. It never invokes `build-otel-auto`, never runs `otelc`, never produces `stapler-squad-otel`, and adds no dependency on the weaving toolchain — a machine with no `otelc` installed runs it fine. What the constraint forbids is CI *building or depending on* the woven artifact; a text check over dry-run output does neither.
**Files**: `scripts/otel-auto-isolation-guard.sh`, `Makefile`

##### Task 2.1.3a: Write the guard script (~5 min)
- Loop over the five target names; `make -n <t> 2>/dev/null | grep -q -e otelc -e stapler-squad-otel` → fail with the target name; print PASS per target.
- Guard against a known false-positive source: `ci` → `lint` → `lint-shell` (`Makefile:627`, `Makefile:652`) shellchecks **every** first-party `*.sh` found by `SHELL_SCRIPTS` (`Makefile:650`), so `make -n ci` will legitimately mention `./scripts/otel-auto-build.sh` by filename. Match on `otelc` and `stapler-squad-otel` only — never on `otel-auto`, which appears in those script filenames and would make the guard fail on a correct Makefile.
- Files: `scripts/otel-auto-isolation-guard.sh`

##### Task 2.1.3b: Wire it as a make target and shellcheck it (~4 min)
- Add `otel-auto-isolation-guard` with `##` help text, add to `.PHONY`, run `make lint-shell`.
- Files: `Makefile`, `scripts/otel-auto-isolation-guard.sh`

##### Task 2.1.3c: Add the guard to `ci` and prove it fires there (~5 min)
- Append `otel-auto-isolation-guard` to `ci`'s prerequisite list (`Makefile:785`), keeping it last so it costs nothing on an already-failing run.
- Verify the self-reference is benign: run `make -n ci | grep -c -e otelc -e stapler-squad-otel` and confirm `0`; run `make otel-auto-isolation-guard` and confirm five PASS lines and exit 0.
- Prove it actually fails: temporarily add `build-otel-auto` to `ci`'s prerequisites, confirm `make otel-auto-isolation-guard` exits non-zero naming `ci`, then revert. Paste both outputs into `spike-verdicts.md`.
- Files: `Makefile`, `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

#### Story 2.1.4: Module Mutation Guard
**As a** maintainer, **I want** to know immediately if weaving rewrites `go.mod`/`go.sum`, **so that** the silent dependency upgrades `pitfalls.md` §1c documents can't reach a commit unnoticed.
**Acceptance Criteria**:
- A `build-otel-auto` run leaves `go.mod` and `go.sum` byte-identical, or fails loudly.
  - *Given* a clean working tree, *When* `make build-otel-auto` completes and `git diff --exit-code -- go.mod go.sum` is run, *Then* it exits 0; if it exits non-zero, the diff is captured into `spike-verdicts.md` and the target's exit status is non-zero.
- The check is part of the target itself, not a separate step someone can forget.
  - *Given* a deliberately dirtied `go.mod` (an added blank line) before the build, *When* `make build-otel-auto` runs, *Then* the guard distinguishes pre-existing dirt from build-introduced changes by comparing checksums captured immediately before and after the `go build` invocation, not against `HEAD`.
**Files**: `scripts/otel-auto-build.sh`

##### Task 2.1.4a: Add pre/post checksums around the child build (~5 min)
- Capture `sha256sum go.mod go.sum` before Task 2.1.1a's child invocation runs; capture them again after it exits (Task 2.1.1a already runs the build as a child rather than `exec`, specifically so this comparison is possible — no restructuring needed here, just adding the checksum calls around the existing child invocation and `$build_status` capture). On mismatch: print the diff and exit non-zero regardless of `$build_status`. On match: `exit "$build_status"`.
- Files: `scripts/otel-auto-build.sh`

##### Task 2.1.4b: Test both branches (~4 min)
- Run once normally (expect exit 0); run once with a deliberately touched `go.mod` mid-flight simulated by a stub to confirm the mismatch path fires.
- Files: none (verification only)

---

### Epic 2.2: Automated instrumentation verification
**Goal**: Convert Spike B.2's and Spike D's one-time manual checks into repeatable scripts, so a future `otelc` upgrade can neither silently stop weaving (Story 2.2.1, tracing **on**) nor silently start exporting when telemetry is off (Story 2.2.2, tracing **off**). The two are complements: neither result is trustworthy without the other, since one asserts a span is present and the other asserts none are.

#### Story 2.2.1: Collector Smoke Test
**As a** maintainer upgrading `otelc` later, **I want** one command that proves the build is still actually instrumented, **so that** "the binary built" is never mistaken for "instrumentation wove in" (`research/features.md` §2, failure mode 1).
**Acceptance Criteria**:
- The script exits 0 only when a `db.system` span is observed for a driven ent query.
  - *Given* `stapler-squad-otel` built and a local OTLP collector reachable on `localhost:4317`, *When* `./scripts/otel-auto-smoke.sh` is run, *Then* it starts the binary on `PORT=62871` with `STAPLER_SQUAD_INSTANCE=claude-otel-smoke` and `OTEL_ENABLED=true`, drives one session-list request, asserts at least one span carrying `db.system` arrived, prints the Span Census it saw, and exits 0.
- The script exits non-zero with a specific message when the span is absent.
  - *Given* the same setup but with `stapler-squad` (Baseline Build) substituted via an argument, *When* the script is run, *Then* it exits non-zero with `no db.system span observed — binary is not auto-instrumented`, proving the assertion actually discriminates.
- The script always cleans up its instance and never touches `~/.stapler-squad/`'s default state.
  - *Given* any exit path including failure, *When* the script terminates, *Then* the spawned process is killed via a `trap` and only `~/.stapler-squad/instances/claude-otel-smoke/` was written.
**Files**: `scripts/otel-auto-smoke.sh`, `Makefile`

##### Task 2.2.1a: Write the smoke script (~5 min)
- `set -euo pipefail`, `trap` cleanup, binary path as `${1:-./stapler-squad-otel}`, start on the manual port block, drive the request, grep the collector output for `db.system`, print the census.
- Files: `scripts/otel-auto-smoke.sh`

##### Task 2.2.1b: Verify both the positive and negative case (~5 min)
- Run against `stapler-squad-otel` (expect 0) and against `stapler-squad` (expect non-zero with the specific message).
- Files: none (verification only)

##### Task 2.2.1c: Add the make target and shellcheck (~3 min)
- Add `otel-auto-smoke` with `##` help text, add to `.PHONY`, run `make lint-shell`.
- Files: `Makefile`, `scripts/otel-auto-smoke.sh`

#### Story 2.2.2: Suppression Smoke Test — `OTEL_ENABLED=false` against a live collector
**As a** developer running the Auto-Instrumented Build locally, **I want** an automated check that the woven binary exports nothing when telemetry is off, **so that** the property Spike D established once keeps holding across `otelc` upgrades — and so that the *documented* off-switch is the thing under test, not a build-script env var that never reaches the running process.
**Acceptance Criteria**:
- The check follows Spike D's methodology exactly: liveness first, then suppression, then a positive control.
  - *Given* `stapler-squad-otel` built and a local OTLP collector on `localhost:4317`, *When* `./scripts/otel-auto-smoke.sh --suppression` is run, *Then* it (1) starts the binary with `OTEL_ENABLED=true`, drives one session-list request, and requires ≥1 span to arrive — exiting non-zero with `collector not live — suppression result would be meaningless` if none does; (2) truncates the collector's captured output **without restarting the collector**; (3) restarts the binary with the documented tracing-off Run Recipe and drives the same request; (4) asserts **zero** spans arrived; (5) prints both span counts.
- It runs the binary exactly as the doc tells a human to run it with tracing off.
  - *Given* `.claude/docs/opentelemetry-auto-instrumentation.md`'s Run Recipe (Story 4.1.1), *When* the script composes its tracing-off environment, *Then* it uses the identical variable set the doc prescribes — `OTEL_ENABLED=false` plus, if and only if Spike D.2's remedy leg applied, `OTEL_TRACES_EXPORTER=none OTEL_METRICS_EXPORTER=none` — so the script and the doc cannot drift into testing different things.
- The assertion discriminates: it fails when spans *are* exported.
  - *Given* the same setup, *When* the script is re-run with `OTEL_ENABLED=true` forced into the suppression leg (an internal `--self-test` switch or a documented one-off env override), *Then* it exits non-zero with `spans exported while telemetry disabled — suppression is broken`, proving the zero-span assertion is not vacuously true.
- Cleanup and isolation match Story 2.2.1.
  - *Given* any exit path including failure, *When* the script terminates, *Then* every spawned process is killed via a `trap`, and only `~/.stapler-squad/instances/claude-otel-smoke/` was written.
**Files**: `scripts/otel-auto-smoke.sh`, `Makefile`

##### Task 2.2.2a: Add the `--suppression` mode (~5 min)
- Factor the existing start/drive/collect logic into a helper; add the liveness → truncate → suppress → count sequence; keep the default (no-flag) behaviour of Story 2.2.1 unchanged.
- Files: `scripts/otel-auto-smoke.sh`

##### Task 2.2.2b: Verify both branches and the self-test (~5 min)
- Run `--suppression` (expect 0); run `--suppression --self-test` (expect non-zero with the specific message); kill the collector and run `--suppression` (expect the `collector not live` message, **not** a false PASS).
- Files: none (verification only)

##### Task 2.2.2c: Add the make target and shellcheck (~3 min)
- Add `otel-auto-smoke-suppression` with `##` help text, add to `.PHONY`, run `make lint-shell`.
- Files: `Makefile`, `scripts/otel-auto-smoke.sh`

---

### Epic 2.3: Functional parity with the Baseline Build
**Goal**: Substantiate requirements.md's first Success Metric — *"produces a working stapler-squad binary that is functionally identical to the normal build (same CLI flags, same behavior) and passes the existing test/e2e suite."* Phases 1–2 so far prove the binary *builds*, *starts*, and *emits spans*; none of them prove it *behaves the same*. This epic closes that gap and writes the Parity Report.

**Which mechanism, and why.** The two suites answer different halves of the metric and need different mechanisms:

- **The Go suite tests source behaviour, and weaving changes the compiled form of that source** — `otelc` injects code into the same packages `go test` compiles. Re-running the suite *under Toolexec Injection* (`./scripts/otel-auto-build.sh go test ./...`) therefore is meaningful and is the right mechanism: it exercises the woven code paths, not a stale unwoven artifact. Re-running plain `make test` against an unmodified source tree while merely *having* `stapler-squad-otel` sitting on disk would prove nothing, because the test binaries `go test` builds are unrelated to that file. This leg is contingent on Unresolved Question 4 (does `GOFLAGS` toolexec apply to `go test`?), which Story 2.3.1 settles once, for itself and for Story 3.1.2.
- **The e2e suite tests a running binary**, so for it the artifact *is* the unit under test — and `tests/e2e/helpers/test-server.ts` spawns whatever `buildPath` points at. Pointing it at `stapler-squad-otel` exercises the real CLI flags (`--test-mode`, `--test-dir`, `--tmux-keep-server`), the real startup path, and the real HTTP/RPC surface. That is exactly the "same CLI flags, same behavior" half of the metric.

#### Story 2.3.1: Run the existing Go test suite woven
**As a** maintainer, **I want** this repo's own test suite executed against woven code, **so that** "functionally identical" is a measured claim rather than an inference from one `curl` returning 200.
**Acceptance Criteria**:
- Whether `go test` is woven at all is settled first, with evidence, and recorded once for the whole plan.
  - *Given* the wrapper script from Story 2.1.1, *When* `go test -c` is run for one package with and without the exported `GOFLAGS` and the two output sizes and `go tool nm` otel-symbol counts are compared, *Then* `parity-report.md` records `go test weaving: CONFIRMED|NOT APPLIED` with both numbers and the exact commands, and Story 3.1.2 consumes this finding instead of re-deriving it.
- If weaving applies to `go test`, the existing suite passes woven.
  - *Given* `go test weaving: CONFIRMED`, *When* `./scripts/otel-auto-build.sh go test ./... -timeout=20m` is run (the same package set and timeout `make test` uses, after `make proto-gen ent-gen`), *Then* every package that passes on the Baseline Build also passes woven; any package that does not is listed by name in `parity-report.md` with its failure output, and is carried into the Adoption Verdict as a precondition.
  - *Given* a woven failure that also fails unwoven, *When* it is triaged, *Then* it is recorded as pre-existing — with the baseline run that proves it — and, per `.claude/rules/fix-flaky-tests-dont-defer.md`, either fixed or filed as its own bug rather than re-excused.
- If weaving does **not** apply to `go test`, that is recorded as a coverage gap rather than silently reported as a pass.
  - *Given* `go test weaving: NOT APPLIED`, *When* the report is written, *Then* it states plainly that the Go-suite leg cannot substantiate the metric, that Story 2.3.2's binary-level parity is the only evidence available, and that the Adoption Verdict must treat "unit-test behaviour under weaving is unverified" as an open precondition.
**Files**: `project_plans/go-auto-instrumentation/implementation/parity-report.md`

##### Task 2.3.1a: Settle whether `go test` is woven (~5 min)
- `go test -c` one package with and without `GOFLAGS`; compare sizes and otel symbol counts; record the verdict and commands.
- Files: `project_plans/go-auto-instrumentation/implementation/parity-report.md`

##### Task 2.3.1b: Run the suite woven and triage (~5 min to launch; runs long — background it)
- `./scripts/otel-auto-build.sh go test ./... -timeout=20m`, backgrounded, output teed to a file; diff the failing-package set against a baseline `make test` run.
- Files: `project_plans/go-auto-instrumentation/implementation/parity-report.md`

#### Story 2.3.2: CLI-flag and e2e startup parity for `stapler-squad-otel`
**As a** maintainer, **I want** the woven binary driven through the same startup path and interface the e2e suite already exercises, **so that** "same CLI flags, same behavior" is demonstrated on the artifact itself.
**Acceptance Criteria**:
- The woven binary accepts the same CLI surface as the Baseline Build.
  - *Given* both binaries built, *When* `--help` output and `version` output are captured from each, *Then* the flag sets are byte-identical apart from the version string, and any difference is quoted in `parity-report.md`.
- The woven binary satisfies the e2e suite's server-startup contract.
  - *Given* `stapler-squad-otel` freshly built, *When* it is started exactly as `tests/e2e/helpers/test-server.ts` starts a server — `--test-mode --test-dir <tmpdir> --tmux-keep-server` with `PORT=<free port>` and `STAPLER_SQUAD_INSTANCE=e2e-local` — *Then* it becomes healthy within the helper's existing readiness window, serves the seeded demo data (`go run ./tests/demo/seed`), and shuts down cleanly, with the observed startup time recorded next to the Baseline Build's.
- The full Playwright suite runs against the woven binary, or its absence is recorded with the reason.
  - *Given* Unresolved Question 5 answered yes, *When* `tests/e2e/helpers/test-server.ts`'s `buildPath` default is extended to `config.buildPath || process.env.TEST_SERVER_BINARY || <existing default>` and `cd tests/e2e && TEST_SERVER_BINARY=$(pwd)/../../stapler-squad-otel npm test` is run, *Then* the pass/fail counts are recorded beside a baseline run's, and any spec failing only against the woven binary is named. The override must be a no-op when unset: `cd tests/e2e && npm test` behaves exactly as before, proving the change does not wire the woven build into the default e2e path (and the Isolation Guard is unaffected — it checks make targets, and no make target gained a reference).
  - *Given* Unresolved Question 5 answered no, or the run is not attempted, *When* the report is written, *Then* it says so explicitly and states which parity evidence stands in its place, rather than leaving the metric silently unaddressed.
- A known trap is avoided: `ensureBinary()` (`tests/e2e/helpers/test-server.ts`) rebuilds via a hardcoded `go build -o stapler-squad .` whenever `buildPath` is missing or older than an hour — which would silently substitute an **unwoven** binary and produce a meaningless pass.
  - *Given* the override in use, *When* the run starts, *Then* `stapler-squad-otel` was rebuilt immediately beforehand (so its mtime is fresh), and the run log is checked for the helper's `Building Go binary...` line — whose presence invalidates the run and is recorded as such.
**Files**: `project_plans/go-auto-instrumentation/implementation/parity-report.md`, `tests/e2e/helpers/test-server.ts`

##### Task 2.3.2a: Diff the CLI surface (~4 min)
- Capture `--help` and `version` from both binaries; diff; record.
- Files: `project_plans/go-auto-instrumentation/implementation/parity-report.md`

##### Task 2.3.2b: Drive the e2e startup contract by hand (~5 min)
- Start the woven binary with the helper's exact flag set on a free port in a temp `--test-dir`; seed; poll for readiness; shut down; record timings.
- Files: `project_plans/go-auto-instrumentation/implementation/parity-report.md`

##### Task 2.3.2c: Add the `TEST_SERVER_BINARY` override and run the suite (~5 min to launch)
- Add the env fallback to `buildPath` only (default unchanged); confirm an unset-env run is unaffected; run the suite against `stapler-squad-otel`; check the log for the `Building Go binary...` line.
- Files: `tests/e2e/helpers/test-server.ts`, `project_plans/go-auto-instrumentation/implementation/parity-report.md`

#### Story 2.3.3: Write the Parity Report
**As a** decision-maker, **I want** one document stating what "functionally identical" was actually verified to mean, **so that** the Adoption Verdict can cite parity evidence the same way it cites overhead numbers.
**Acceptance Criteria**:
- `parity-report.md` states, per suite, what ran, against which artifact, and the result.
  - *Given* Stories 2.3.1 and 2.3.2 complete, *When* the report is written, *Then* it contains a table with one row per suite (Go unit suite, CLI surface, e2e startup contract, Playwright e2e) whose columns are: mechanism used, woven or not, result, and — for anything not run — the reason and what evidence substitutes.
- No claim of parity is stated more strongly than the evidence supports.
  - *Given* any leg that did not run woven, *When* the summary line is written, *Then* it is scoped to the legs that did (e.g. "binary-level parity verified; unit-test behaviour under weaving unverified — `go test` is not woven"), never generalised to "passes the existing test/e2e suite."
**Files**: `project_plans/go-auto-instrumentation/implementation/parity-report.md`

##### Task 2.3.3a: Assemble the table and scope the summary (~4 min)
- Fill the per-suite table; write the scoped headline; re-read once adversarially for over-claimed parity.
- Files: `project_plans/go-auto-instrumentation/implementation/parity-report.md`

---

## Phase 3: Overhead benchmarking

### Epic 3.1: Overhead Delta and Build-Time Delta against real hot paths
**Goal**: Produce the measured, documented overhead bound requirements.md's Success Metrics demand — measured against this repo's *already-profiled* hot paths (`pitfalls.md` §5.4), not a synthetic workload.

#### Story 3.1.1: Capture the Baseline Build benchmark baseline
**As a** maintainer, **I want** a reproducible pre-weaving benchmark snapshot, **so that** the Overhead Delta is a comparison against a captured artifact rather than a remembered number.
**Acceptance Criteria**:
- A `benchstat`-comparable baseline file exists covering the Hot Path Baseline benchmarks.
  - *Given* a quiesced machine and `make build` completed, *When* `go test -bench='BenchmarkCircularBuffer|BenchmarkSessionService_List|BenchmarkSessionService_Get|BenchmarkSessionService_Stream|BenchmarkEventBus|BenchmarkReactiveQueueManagerThroughput' -benchmem -count=8 -timeout=30m ./... > bench-otel-baseline.txt 2>&1 &` completes, *Then* `bench-otel-baseline.txt` contains at least 8 samples for each named benchmark and is referenced by path in `overhead-report.md`.
- The run follows this repo's benchmark methodology.
  - *Given* `.claude/docs/benchmarks.md`'s standing rule, *When* the benchmark command is issued, *Then* it is backgrounded with `&` and its `-count=8` matches `benchmark-tier1`'s existing convention (`Makefile:910-918`).
**Files**: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

##### Task 3.1.1a: Run and store the baseline (~5 min to launch; runs in background)
- Launch the backgrounded benchmark; record the exact command and machine state (load average, whether the deployed `:8543` instance was running) in `overhead-report.md`.
- Files: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

##### Task 3.1.1b: Sanity-check sample counts (~3 min)
- Confirm each named benchmark has 8 samples; re-run any that were truncated.
- Files: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

#### Story 3.1.2: Capture the three-way woven matrix
**As a** maintainer, **I want** overhead measured for both woven-and-disabled and woven-and-enabled, **so that** the cost of merely *having* the instrumentation is separated from the cost of *using* it (`research/features.md` §2, failure mode 3).
**Acceptance Criteria**:
- Three benchmark result files exist: baseline, woven+disabled, woven+enabled.
  - *Given* Toolexec Injection confirmed to apply to `go test` — the `go test weaving: CONFIRMED` finding Story 2.3.1 already recorded in `parity-report.md`, which this story cites rather than re-deriving (Unresolved Question 4) — *When* the same benchmark command is re-run twice under `./scripts/otel-auto-build.sh` — once with `OTEL_ENABLED` unset, once with `OTEL_ENABLED=true` and a collector listening — *Then* `bench-otel-woven-off.txt` and `bench-otel-woven-on.txt` exist with 8 samples per benchmark, and `benchstat bench-otel-baseline.txt bench-otel-woven-off.txt bench-otel-woven-on.txt` produces a three-column comparison pasted into `overhead-report.md`.
- If Toolexec Injection does not apply to `go test`, the fallback measurement path is used and stated.
  - *Given* a woven `go test -bench` run whose output shows no weaving occurred (verified by the same `db.system`-span discriminator as Story 2.2.1, or by binary-size comparison of `go test -c` output), *When* that is discovered, *Then* `overhead-report.md` records the limitation and the measurement switches to driving the running `stapler-squad-otel` on `PORT=62871` under load while capturing pprof CPU profiles, per Story 3.1.3's method.
**Files**: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

##### Task 3.1.2a: Carry over the `go test` weaving verdict (~2 min)
- Read the `go test weaving:` line Story 2.3.1a recorded in `parity-report.md` and cite it in `overhead-report.md`. Re-run its two commands only if the wrapper script or `otelc` version changed since; do not re-derive it otherwise.
- Files: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

##### Task 3.1.2b: Run the woven+disabled benchmark (~5 min to launch)
- Backgrounded, `-count=8`, `OTEL_ENABLED` unset; store to `bench-otel-woven-off.txt`.
- Files: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

##### Task 3.1.2c: Run the woven+enabled benchmark and benchstat all three (~5 min to launch)
- Backgrounded with `OTEL_ENABLED=true` and the collector up; then `benchstat` all three files; paste the table.
- Files: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

#### Story 3.1.3: pprof re-test against the previously-identified hot paths
**As a** maintainer, **I want** the specific call sites this project already root-caused re-profiled under weaving, **so that** the overhead number can be cross-referenced against this repo's own prior findings instead of being an uncheckable aggregate (`pitfalls.md` §4).
**Acceptance Criteria**:
- CPU and mutex profiles are captured from a running `stapler-squad-otel` under a session-poll workload and compared to the same profiles from a Baseline Build.
  - *Given* both binaries runnable with `--profile` on the manual port block, *When* each is run for an identical, scripted workload (N sessions listed and polled for a fixed duration) and `make profile-mutex` / `profile-block` / a CPU profile are captured via `PROFILE_SERVER=http://localhost:6060`, *Then* `overhead-report.md` contains a side-by-side table of cumulative samples for `session.GetStatus`, `ReviewQueuePoller.checkSession`, `session.CircularBuffer` read/write, and `session/git.GitWorktree.IsDirty`.
- Any hot path whose cost grows by more than a stated threshold is called out by name.
  - *Given* the side-by-side table, *When* a listed call site's cumulative CPU samples increase by more than 10% relative to baseline, *Then* it is listed under a "Regressed hot paths" heading in `overhead-report.md` with the absolute numbers, and is carried into the Adoption Verdict as a precondition; if none exceed the threshold, the report states that explicitly rather than omitting the section.
**Files**: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

##### Task 3.1.3a: Script the identical workload (~5 min)
- A small driver that creates/lists/polls a fixed number of sessions against a given port for a fixed duration, so both runs are comparable.
- Files: `scripts/otel-auto-loadgen.sh`

##### Task 3.1.3b: Capture baseline profiles (~5 min)
- Run `./stapler-squad --profile` on manual instance #2; run the load generator; capture CPU + mutex + block profiles.
- Files: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

##### Task 3.1.3c: Capture woven profiles and tabulate the four hot paths (~5 min)
- Repeat against `stapler-squad-otel`; extract cumulative samples for the four named call sites; write the side-by-side table and the regression callouts.
- Files: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

#### Story 3.1.4: Build-Time Delta
**As a** developer iterating on this build path, **I want** the build-time tax quantified, **so that** the DX cost of the opt-in path is stated rather than discovered (`research/features.md` §3).
**Acceptance Criteria**:
- Cold-cache and warm-cache build times are recorded for both paths.
  - *Given* `go clean -cache` run immediately before each cold measurement, *When* `make build` and `make build-otel-auto` are each timed cold and then again warm (no source change), *Then* `overhead-report.md` records four wall-clock numbers and the ratio of woven to baseline for each cache state.
**Files**: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

##### Task 3.1.4a: Time both builds cold and warm (~5 min plus build wall time)
- `go clean -cache` then time each; repeat warm; record numbers.
- Files: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

#### Story 3.1.5: Write the Overhead Report
**As a** decision-maker, **I want** one document stating the measured overhead bound, **so that** the Adoption Verdict rests on numbers with reproducible commands attached.
**Acceptance Criteria**:
- `overhead-report.md` states a single headline Overhead Delta with its measurement basis, plus every supporting table.
  - *Given* the benchstat output, the pprof hot-path table, and the build-time numbers, *When* the report is written, *Then* it opens with a one-line statement of the form "Woven+enabled costs +X% on <named benchmark set>, +Y% on <named hot path>; woven+disabled costs +Z%", each number traceable to a table below it and to the exact command that produced it.
- Every claim in the report carries a command or a file reference.
  - *Given* the finished report, *When* it is re-read adversarially, *Then* no percentage or conclusion appears without an adjacent command, file path, or table it was derived from.
**Files**: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

##### Task 3.1.5a: Assemble the report and self-review (~5 min)
- Write the headline; verify each number against its table; recount any "N of M" claims with a command.
- Files: `project_plans/go-auto-instrumentation/implementation/overhead-report.md`

---

## Phase 4: Documentation

### Epic 4.1: Document the opt-in path
**Goal**: A developer who has never seen this project can build, verify, and run the Auto-Instrumented Build from the docs alone.

#### Story 4.1.1: New auto-instrumentation doc, linked from the existing OTel doc and the repo index
**As a** future contributor, **I want** the opt-in build path documented where I already look for OTel setup, **so that** I don't rediscover the `otelc` install, the env pairing, and the smoke test from scratch.
**Acceptance Criteria**:
- A new doc exists in the repo's established short, command-block-first style.
  - *Given* `.claude/docs/bundling-tmux.md`'s format as the model, *When* `.claude/docs/opentelemetry-auto-instrumentation.md` is written, *Then* it opens with a fenced command block covering `otelc` install → `make build-otel-auto` → `make otel-auto-smoke` → running on the manual port block, with one line of prose per command, and states up front that `otelc` is an **external binary install**, not a Go module dependency.
- It contains the Run Recipe: both launch modes, spelled out as runnable lines.
  - *Given* Spike D's verdict, *When* the doc's "Running it" section is written, *Then* it gives two copy-pasteable invocations — tracing **on** (`OTEL_ENABLED=true` plus the collector endpoint) and tracing **off** (`OTEL_ENABLED=false`, plus `OTEL_TRACES_EXPORTER=none OTEL_METRICS_EXPORTER=none` if and only if Spike D.2's remedy leg applied) — and states explicitly that these are **runtime** variables set when launching `stapler-squad-otel`: `make build-otel-auto` and `scripts/otel-auto-build.sh` neither set nor can set them, because that process has exited by the time the binary runs (ADR-004). The off-mode line must be the identical variable set `scripts/otel-auto-smoke.sh --suppression` uses, so doc and test cannot drift.
- It documents the four Spike Verdicts' operational consequences.
  - *Given* the recorded verdicts, *When* the doc is written, *Then* it states: whether `-tags embed_tmux` is supported, the Instrumentation Scope string that identifies Woven Spans, whether the Exporter Toggle must be paired with `OTEL_ENABLED`, and the known `pitfalls.md` #736-class SQLite log noise if it was observed — each with a link to `spike-verdicts.md`.
- It is reachable from both existing entry points.
  - *Given* the new doc, *When* `.claude/docs/opentelemetry.md` and `CLAUDE.md`'s "Reference Documents Index" table are read, *Then* each contains a link to it — the former as a new "Compile-time auto-instrumentation (opt-in)" section, the latter as a new index row following the existing per-doc convention.
**Files**: `.claude/docs/opentelemetry-auto-instrumentation.md`, `.claude/docs/opentelemetry.md`, `CLAUDE.md`

##### Task 4.1.1a: Write the new doc (~5 min)
- Command block first; the Run Recipe's two launch modes (tracing on / tracing off) with the build-time-vs-runtime distinction stated once; the four verdict consequences; the `go tool nm stapler-squad-otel | grep -c otel` heuristic from `ux.md` §4 as a fast pre-runtime sanity check, labelled a heuristic; a "never `make install-service` this binary" warning.
- Files: `.claude/docs/opentelemetry-auto-instrumentation.md`

##### Task 4.1.1b: Link from `.claude/docs/opentelemetry.md` (~3 min)
- Add a short section pointing at the new doc; leave the existing content unchanged.
- Files: `.claude/docs/opentelemetry.md`

##### Task 4.1.1c: Add the CLAUDE.md index row (~2 min)
- Add a row to the "Reference Documents Index" table matching existing formatting.
- Files: `CLAUDE.md`

---

## Phase 5: Custom subprocess instrumentation — FIRST TO CUT

**Cut policy**: Per requirements.md's Rabbit Holes and Scope, this entire phase is the first thing to drop if the appetite runs out. It may not begin until Phases 1–4 are complete. If cut, Story 6.1.1 must record it as a scoped-out item with the reason, not omit it.

### Epic 5.1: Subprocess Hook for `executor/safeexec`
**Goal**: Spans for the tmux/git subprocess layer — the largest untraced surface no compile-time tool covers out of the box.

#### Story 5.1.1: Feasibility read of `otelc`'s extension mechanism
**As a** developer, **I want** to know what `otelc` v1's custom-rule API actually requires before writing any hook code, **so that** the cut decision is made on evidence rather than after sinking time into it.
**Acceptance Criteria**:
- A written go/no-go on the hook mechanism exists with a cited source.
  - *Given* `otelc`'s repository docs, *When* the implementer looks for a custom/local instrumentation-rule mechanism, *Then* `spike-verdicts.md` gains a `## Spike E — otelc extension API` entry stating either "supported — <doc URL>, rule shape: <summary>" or "not available at v1 — <evidence>", plus a time estimate for Story 5.1.2.
- If unavailable in `otelc`, the Fallback Tool option is evaluated but not started.
  - *Given* a "not available at v1" verdict, *When* the next action is recorded, *Then* it states that loongsuite-go's documented hook mechanism (`research/architecture.md` §5) could cover it, that using it would mean building this one path with a *different* tool than the rest, and defers the decision to Story 6.1.1 rather than starting it.
**Files**: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 5.1.1a: Read the extension docs and write the verdict (~5 min)
- Locate and read `otelc`'s rule/hook documentation; write the Spike E entry with URL and rule shape.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

#### Story 5.1.2: Implement the Subprocess Hook
**As an** operator, **I want** git and tmux subprocess calls to appear as spans, **so that** a slow `git status` fork shows up in the trace view instead of only in a pprof profile.
**Acceptance Criteria**:
- A hook wraps `safeexec.CommandContext` and emits one span per invocation.
  - *Given* Spike E's verdict is "supported", *When* the hook is registered for `github.com/tstapler/stapler-squad/executor/safeexec.CommandContext` and a woven binary runs a `git status`-triggering operation, *Then* a span named for the subprocess appears carrying the command name as an attribute.
- Attribute keys follow this repo's existing convention rather than raw inline strings.
  - *Given* `telemetry/attributes.go`'s `Attr<Concept>` constant + `<Concept>Attr(...)` constructor pattern, *When* the hook sets attributes, *Then* it uses constants/constructors added to `telemetry/attributes.go` (e.g. `AttrSubprocessCommand = "subprocess.command"`), not `attribute.String("cmd", ...)` inline — per `.claude/rules/primitive-obsession-checklist.md`.
- The hook does not change subprocess behaviour.
  - *Given* the hook installed, *When* `go test ./executor/... ./session/git/...` is run against a woven test build, *Then* all tests pass with the same results as the Baseline Build.
**Files**: `instrumentation/otelc/safeexec/hook.go` (exact path per Spike E's rule-shape finding), `telemetry/attributes.go`

##### Task 5.1.2a: Add the attribute constants and constructors (~4 min)
- Add `AttrSubprocessCommand` / `AttrSubprocessArgCount` constants plus their `...Attr(...)` constructors, matching the existing block's style.
- Files: `telemetry/attributes.go`

##### Task 5.1.2b: Write the hook (~5 min)
- Entry hook starting a span with the typed attributes; exit hook ending it and recording an error if the command failed.
- Files: `instrumentation/otelc/safeexec/hook.go`

##### Task 5.1.2c: Register the rule (~4 min)
- Add the rule declaration in the form Spike E documented; wire it into `scripts/otel-auto-build.sh`'s environment or config flag.
- Files: `instrumentation/otelc/safeexec/`, `scripts/otel-auto-build.sh`

#### Story 5.1.3: Verify subprocess spans in the collector
**As an** operator, **I want** proof the hook actually fires, **so that** the same "built ≠ instrumented" trap is avoided for custom rules too.
**Acceptance Criteria**:
- A driven git operation produces a subprocess span in collector output.
  - *Given* the collector listening and the hooked `stapler-squad-otel` running on `PORT=62871` with `OTEL_ENABLED=true`, *When* a session-dirty-check is triggered (exercising `session/git`'s `IsDirty` path), *Then* the collector receives a span carrying `subprocess.command` with value `git`, and that span's full attribute set is pasted into `spike-verdicts.md`.
- `scripts/otel-auto-smoke.sh` gains an optional assertion for it.
  - *Given* the hook is shipped, *When* `./scripts/otel-auto-smoke.sh --with-subprocess` is run, *Then* it additionally asserts a `subprocess.command` span and exits non-zero if absent.
**Files**: `scripts/otel-auto-smoke.sh`, `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 5.1.3a: Drive a git operation and capture the span (~5 min)
- Trigger the dirty-check path; capture and paste the span.
- Files: `project_plans/go-auto-instrumentation/implementation/spike-verdicts.md`

##### Task 5.1.3b: Extend the smoke test with the optional flag (~4 min)
- Add the `--with-subprocess` assertion; verify both with and without the flag; shellcheck.
- Files: `scripts/otel-auto-smoke.sh`

---

## Phase 6: The Adoption Verdict

### Epic 6.1: Written recommendation on default-build adoption
**Goal**: Deliver requirements.md's final Success Metric — a decision document synthesising the Spike Verdicts, the Overhead Report, and the Parity Report. **It may not be drafted before those exist.**

#### Story 6.1.1: Write the adoption recommendation
**As a** decision-maker, **I want** a recommendation on making auto-instrumentation the default build, with the preconditions spelled out, **so that** a future re-evaluation starts from evidence rather than re-running this whole project.
**Acceptance Criteria**:
- The recommendation states a clear verdict and names every precondition.
  - *Given* all four Spike Verdicts, `overhead-report.md`, and `parity-report.md` exist, *When* `adoption-recommendation.md` is written, *Then* it opens with one of "adopt as default now" / "adopt as default once <named conditions> hold" / "do not adopt as default", each named condition being independently checkable (e.g. "`otelc` states Go 1.26 support in its compatibility doc", "the `GetStatus` hot-path CPU regression is under 5%").
  - *Given* Unresolved Question 6 (does the macOS `CGO_LDFLAGS` Info.plist embedding survive weaving?) was never resolved because this project validated Linux only, *When* the recommendation lists preconditions, *Then* macOS support is named as an explicit open precondition ("not yet validated on macOS — Linux-only verdict"), not silently omitted, per `research/stack.md`'s cross-platform scope item and `cross-artifact-consistency` review's concern.
- The functional-parity evidence is stated at the strength the Parity Report supports, not stronger.
  - *Given* `parity-report.md`'s per-suite table, *When* the recommendation addresses requirements.md's "functionally identical … passes the existing test/e2e suite" metric, *Then* it names which suites ran woven and which did not, and any unrun leg becomes a named precondition rather than being folded into a general parity claim.
- Every claim cites the artifact it came from.
  - *Given* the finished document, *When* each factual sentence is checked, *Then* it links to `spike-verdicts.md`, `overhead-report.md`, `parity-report.md`, or a specific file path — no claim rests on memory.
- Anything cut is recorded as cut, with the reason.
  - *Given* Phase 5 was skipped or partially completed, *When* the document is written, *Then* it contains a "Scoped out" section naming the Subprocess Hook, the reason (appetite / Spike E verdict), and what it would take to pick it up later.
- The rollback and two-layer safety net are stated for the default-flip scenario.
  - *Given* a hypothetical future default flip, *When* the document's rollback section is read, *Then* it states both layers: revert the build target change, or ship it built but leave `OTEL_ENABLED`/`DD_TRACE_ENABLED` unset (`research/features.md` §3).
- A staleness-detection mechanism exists, because the Isolation Guard that protects the default build from `build-otel-auto` also guarantees CI never exercises it, so nothing else will notice it silently breaking (`pre-mortem.md` #2, P1).
  - *Given* the finished document, *When* its "Staying Current" section is read, *Then* it names a concrete revisit trigger — a dated backlog item (e.g. "re-run `scripts/otel-auto-smoke.sh` and re-check `otelc version` against Story 1.1.1's recorded value by <date ~3-6 months out>") or an equivalent periodic, non-CI-wired check — not just a point-in-time verdict with no mechanism for staying current.
**Files**: `project_plans/go-auto-instrumentation/implementation/adoption-recommendation.md`

##### Task 6.1.1a: Draft the verdict and preconditions (~5 min)
- Pick the verdict from the three forms; write independently-checkable preconditions; link each to its source artifact.
- Files: `project_plans/go-auto-instrumentation/implementation/adoption-recommendation.md`

##### Task 6.1.1b: Write the scoped-out, rollback, and staying-current sections (~5 min)
- Record cuts with reasons; state the two-layer rollback; write the "Staying Current" section naming a concrete revisit trigger (dated backlog item or periodic non-CI smoke-test re-run) per `pre-mortem.md` #2.
- Files: `project_plans/go-auto-instrumentation/implementation/adoption-recommendation.md`

##### Task 6.1.1c: Adversarial self-review (~4 min)
- Re-read once for uncited claims, stale numbers, and cross-references that don't resolve; fix before handing off.
- Files: `project_plans/go-auto-instrumentation/implementation/adoption-recommendation.md`

#### Story 6.1.2: Record the Adoption Verdict as an ADR
**As a** future maintainer, **I want** the adoption decision in the durable ADR record, **so that** it's discoverable without reading a project plan directory.
**Acceptance Criteria**:
- An ADR captures the verdict, its context, and its consequences.
  - *Given* `adoption-recommendation.md` is complete, *When* `ADR-005-default-build-adoption-verdict.md` is written, *Then* it follows the same Status/Context/Decision/Consequences shape as ADR-001 through ADR-004 in this project's `decisions/` directory, links to the recommendation and the overhead report, and is added to this plan's ADR list at the top.
**Files**: `project_plans/go-auto-instrumentation/decisions/ADR-005-default-build-adoption-verdict.md`, `project_plans/go-auto-instrumentation/implementation/plan.md`

##### Task 6.1.2a: Write ADR-005 (~5 min)
- Status/Context/Decision/Consequences; link the two source artifacts.
- Files: `project_plans/go-auto-instrumentation/decisions/ADR-005-default-build-adoption-verdict.md`

##### Task 6.1.2b: Add it to this plan's ADR list (~2 min)
- Append the link to the header block.
- Files: `project_plans/go-auto-instrumentation/implementation/plan.md`
