# ADR-001: Target `open-telemetry/opentelemetry-go-compile-instrumentation` (`otelc`) instead of `alibaba/loongsuite-go`

**Status**: Accepted
**Date**: 2026-08-21
**Deciders**: SDD Phase 3 planning
**Supersedes**: the tool named in `project_plans/go-auto-instrumentation/requirements.md`

## Context

`requirements.md` names `alibaba/loongsuite-go` as the primary candidate throughout — the problem statement, the scope items, the rabbit holes, and five of the six open questions are written against it. **This ADR reverses that explicit instruction.** It exists because reversing a user's named choice needs a durable, checkable record of why, not a footnote in a plan.

The requirements document did, however, ask research to do exactly this check: *"Phase 2 research must do a real comparison against other Go auto-instrumentation options before the plan commits to it exclusively — particularly projects under the official `open-telemetry` GitHub org."* Research answered that question, and the answer moved the recommendation.

### What research found

1. **The candidate landscape changed after `requirements.md` was written.** Alibaba (loongsuite-go's origin) and Datadog (owner of the competing Orchestrion compile-time weaver) formed the OpenTelemetry Go Compile-Time Instrumentation SIG and merged their approaches into `open-telemetry/opentelemetry-go-compile-instrumentation` (CLI: `otelc`), which reached **v1 stable in July 2026** ([research/features.md §4](../research/features.md), [research/build-vs-buy.md §6](../research/build-vs-buy.md)).
2. **Same mechanism, so nothing already-settled is reopened.** `otelc` weaves at compile time behind a `go build` wrapper, exactly as loongsuite-go does. Every deployment-model argument that ruled out the three eBPF candidates (root/`CAP_SYS_ADMIN`, kernel floors, Kubernetes control planes — all incompatible with this repo's unprivileged `systemd --user` unit) applies identically to both compile-time tools, so switching between them changes nothing that was already decided (`build-vs-buy.md` §§2–4).
3. **Governance and continuity both favour `otelc`.** It is OTel-org governed with multi-vendor project leads (Alibaba, Datadog, QuesmaOrg) rather than single-vendor. Its Alibaba lead is the same engineer behind loongsuite-go — this is consolidation, not competition (`build-vs-buy.md` §6).
4. **loongsuite-go's own maintainers plan to rebuild on top of `otelc`.** [Issue #708](https://github.com/alibaba/loongsuite-go/issues/708) is an open roadmap item to rebuild loongsuite-go as v2.0.0 on `opentelemetry-go-compile-instrumentation`, with module-path and CLI-name changes (`otel` → `otelc`) (`research/pitfalls.md` §1a). Adopting loongsuite-go now means adopting a tool with a scheduled breaking rewrite toward the tool this ADR selects directly.
5. **loongsuite-go has open bugs that reproduce on this repo's exact stack.** [#624](https://github.com/alibaba/loongsuite-go/issues/624) (cannot inject packages using CGO, because the toolchain compiles generated `*.cgo1.go` files the weaver can't match) hits this repo's mandatory `CGO_ENABLED=1` `mattn/go-sqlite3` dependency; [#736](https://github.com/alibaba/loongsuite-go/issues/736) (`database/sql` instrumentation's `parseDSN` rejects the `sqlite3` driver name; its DDL extractor runs SQLite DDL through a MySQL-only parser) hits the exact path this project most wants instrumented (`research/pitfalls.md` §1b–1c).
6. **The coverage gap against `otelc` doesn't bite here.** `otelc` v1 covers a much narrower library set (net/http, database/sql, gRPC, Redis, Go runtime metrics) than loongsuite-go's 80+. Checked against this repo's actual `go.mod`, the paths needing coverage are `net/http` and `database/sql` (via `entgo.io/ent`) — both covered. Kafka/RabbitMQ/GORM/Redis are unused here. ConnectRPC is covered by *neither* tool and already has manual `otelconnect` coverage, so it's a wash (`build-vs-buy.md` §6).

### What is honestly worse about `otelc`

- Younger and smaller (397 stars vs. loongsuite-go's 895) — less field usage to learn from.
- Its Go-version support ceiling is unconfirmed against this repo's `go 1.26.3`; research found a documented Go 1.25+ floor and no stated ceiling (`research/features.md` §2). This is Unresolved Question 1 in the plan.
- Its distribution method (prebuilt binaries vs. `go install` vs. source build) was not confirmable from the fetched docs (`build-vs-buy.md` §6). Unresolved Question 2.
- Its custom hook/plugin API was not researched in depth — only loongsuite-go's was. Phase 5 opens with a feasibility read for exactly this reason.

## Decision

**Target `otelc` as the primary tool.** Build the opt-in `make build-otel-auto` path, the collector smoke test, the benchmarking matrix, and the documentation against it.

**Keep `alibaba/loongsuite-go` as a documented fallback only**, adopted for one specific path only when *all* of the following hold:

1. A concrete Coverage Gap has been reproduced — a real instrumentation need `otelc` cannot satisfy, recorded in `implementation/spike-verdicts.md` with the exact failing command and its output.
2. The gap is checked against loongsuite-go's own open issues first, since #624 and #736 mean the fallback may fail the same way (CGO weaving, SQLite `database/sql`) — a fallback that shares the failure is not a fallback.
3. The consequence of mixing tools (one path built by a different weaver than the rest) is stated in `implementation/adoption-recommendation.md`.

No pluggable tool-selection abstraction is built up front. A `Strategy`-shaped `OTEL_AUTO_TOOL` switch with one real implementation is a speculative interface (`.claude/rules/interface-pollution-checklist.md`, smell #1); it gets added when a second implementation actually exists, not before.

## Consequences

**Positive**
- The project builds on an OTel-org-governed, multi-vendor, v1-stable tool rather than a single-vendor one with a scheduled breaking rewrite ahead of it.
- Two documented, still-open bug classes (#624 CGO, #736 SQLite `database/sql`) are avoided as *known* defects of the rejected tool — though whether `otelc` shares them is precisely what Spike B exists to find out, and no claim is made here that it doesn't.
- No rework later when loongsuite-go v2 migrates to the same foundation.

**Negative**
- Trades a broad, battle-tested library list for a narrow, young one. If this repo later adds Kafka, RabbitMQ, GORM, or wants structured-logging auto-instrumentation, the coverage question reopens.
- Reverses an explicit user instruction. If the user disagrees with the reasoning, the whole plan's tool choice flips — which is why this ADR is written before implementation rather than after.
- Some of `requirements.md`'s open questions were written specifically about loongsuite-go's docs (its `docs/dev/overview.md` plugin API, its compatibility table) and are now moot; their `otelc` equivalents are carried into the plan's Unresolved Questions instead.

**Neutral**
- The eBPF options (`opentelemetry-go-instrumentation`, Beyla/OBI, Odigos) remain rejected for an unrelated and unchanged reason: all require host root/`CAP_SYS_ADMIN`-class privileges or a Kubernetes control plane, incompatible with this repo's unprivileged `systemd --user` deployment (`build-vs-buy.md` §§2–4). This ADR does not revisit them.
- Manual instrumentation in `telemetry/telemetry.go` is unaffected and stays — the two approaches are complementary, per `requirements.md`'s coexistence constraint.

## References

- [research/build-vs-buy.md](../research/build-vs-buy.md) — §6 and the comparison table; the primary evidence for this decision.
- [research/features.md](../research/features.md) — §4, the SIG formation and v1 timeline.
- [research/pitfalls.md](../research/pitfalls.md) — §1a (#708 rewrite roadmap), §1b (#736 SQLite), §1c (#624 CGO).
- [requirements.md](../requirements.md) — "Alternatives Considered" and the open question this ADR answers.
