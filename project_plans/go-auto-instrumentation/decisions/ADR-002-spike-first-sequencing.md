# ADR-002: Gate all build-path work behind four go/no-go validation spikes

**Status**: Accepted
**Date**: 2026-08-21
**Deciders**: SDD Phase 3 planning

## Context

Four load-bearing unknowns sit under this project, and each one, if it fails, invalidates a different subset of the work:

1. **Build-flag passthrough.** `research/architecture.md` §1 marks `-tags embed_tmux` and `-ldflags "$(LDFLAGS)"` passthrough as UNVERIFIED — the tool's docs demonstrate `-o` and `-gcflags` and nothing else. `requirements.md`'s Rabbit Holes calls this "the single biggest unknown."
2. **CGO/SQLite weaving.** `research/pitfalls.md` §1c documents loongsuite-go [#624](https://github.com/alibaba/loongsuite-go/issues/624): CGO packages compile from toolchain-generated `*.cgo1.go` files a source-level weaver can't match. This repo requires `CGO_ENABLED=1` and depends on `github.com/mattn/go-sqlite3`. §1b documents [#736](https://github.com/alibaba/loongsuite-go/issues/736): `database/sql` instrumentation rejects the `sqlite3` driver name. Both hit the exact path this project most wants instrumented.
3. **Coexistence.** `requirements.md`'s Rabbit Holes and `research/features.md` §2 both flag that woven `net/http` instrumentation alongside the existing `otelhttp` middleware may produce duplicate root spans rather than composing. No vendor doc guarantees either outcome.
4. **`OTEL_ENABLED=false` suppression.** `research/architecture.md` §3 works through the initialization ordering: an injected `init()` bootstrap runs before `main()` reaches `telemetry.Initialize` (`main.go:262`), so woven code may export spans regardless of this repo's own toggle — reading the standard OTel env vars directly, which default to an `otlp` exporter at `localhost:4317`, the same endpoint this repo's `telemetry.DefaultConfig()` uses.

None of the four is resolvable from documentation. All four were flagged by research as requiring empirical verification.

The obvious alternative shape — write `make build-otel-auto` first and discover the answers by running it — is appealing because it produces a shippable artifact immediately and exercises the real invocation rather than a reduction. It was rejected: a CGO weave failure would invalidate the target's design, the benchmarking matrix, the smoke test, the documentation, and the custom-plugin work simultaneously, with no pre-declared recovery path. `requirements.md`'s Feasibility Risks section asks the plan to sequence around exactly this.

## Decision

**Phase 1 consists of four Validation Spikes (A–D), each producing only a Spike Verdict — no shipped code. No Phase 2+ story begins until all four verdicts are recorded in `implementation/spike-verdicts.md`.**

Spike B (CGO + SQLite weaving) is the project's single go/no-go gate: on FAIL, the plan's pre-declared next action is to retry with the pure-Go `modernc.org/sqlite` driver, then to reproduce the failure against the Fallback Tool before considering it, and finally — if none succeed — to collapse scope to `requirements.md`'s own stated floor, *"documented findings + no shipped build target."*

Every spike has a pre-declared failure branch, tabulated in the plan under "If a spike fails." A spike that fails does not stall the project; it selects an already-written next action.

Each Spike Verdict must record: the exact command run, its trimmed output, a `PASS`/`FAIL`/`PARTIAL` verdict, and the next action taken. A verdict without a command is not a verdict.

## Consequences

**Positive**
- Every downstream story starts from a settled premise, and the plan already states what happens on each failure — the failure modes are decisions, not surprises.
- The Spike Verdict Log becomes the evidence base the Adoption Verdict (Phase 6) cites, so the final recommendation rests on recorded commands rather than recollection.
- The riskiest work is also the cheapest to abandon: Phase 1 produces no code to unwind.

**Negative**
- Roughly a third of the appetite is spent before any shippable artifact exists. If all four spikes pass cleanly the sequencing will look like ceremony in hindsight.
- Spike work against a reduction can diverge from the real build. Mitigated by running Spikes A and B against the *full* repo (not a minimal reproduction) wherever the build gets far enough to allow it.

**Neutral**
- Adds one durable artifact (`spike-verdicts.md`) to the project's output. It is a deliverable in its own right — the Adoption Verdict is not writable without it.
