# ADR-005: Adopt `otelc` auto-instrumentation as default once named preconditions hold — not yet, not never

**Status**: Accepted
**Date**: 2026-08-22
**Deciders**: SDD Phase 6 planning

## Context

This project's fourth Success Metric (`requirements.md`) required "a written recommendation on whether/how to move this from opt-in to the default build path, including what would have to be true first (compatibility, overhead, support-burden) — a decision, not necessarily an executed default-flip." By Phase 6, three evidence artifacts existed to base that decision on:

- `implementation/spike-verdicts.md` — all four Phase 1 go/no-go spikes (build-flag passthrough, CGO/SQLite weaving, `otelhttp`/`otelconnect` coexistence, `OTEL_ENABLED=false` suppression), plus Spike E (custom-hook feasibility) and its addendum (the Subprocess Hook actually implemented and verified, Story 5.1.2/5.1.3).
- `implementation/overhead-report.md` — hot-path pprof comparisons (no path showed a measurable regression above the sampling floor) and Build-Time Delta measurements (1.79x wall-clock / 4.81x CPU-seconds warm-cache cost of `make build-otel-auto` vs. `make build`), plus two reproduced operational risks (slow SIGTERM shutdown with orphaned tmux sessions; a persistent background export-retry when telemetry is disabled).
- `implementation/parity-report.md` — CLI-surface parity confirmed byte-identical (modulo a disclosed bootstrap-log preamble); Go unit-test behavior under weaving recorded as **NOT SAFELY DETERMINABLE** after two reproduced extreme-load incidents; the Playwright e2e suite not run against the woven binary.

The go/no-go gate (Spike B) passed outright, and nothing in three phases of evidence falsifies the tool. But requirements.md's own first Success Metric — "functionally identical … passes the existing test/e2e suite" — is only partially substantiated: two of its four legs (Go unit suite, Playwright e2e suite) are open coverage gaps, not passes. A default-build flip changes what every contributor's `go build`/`make ci` invocation produces; `ADR-002`'s entire premise was that load-bearing unknowns get resolved *before* code depends on them, and the unit-test-suite-under-weaving question remains exactly the kind of unresolved unknown that ADR-002 exists to gate on.

## Decision

**Adopt `otelc` auto-instrumentation as the default build once the preconditions named in `implementation/adoption-recommendation.md` hold. Do not adopt it as the default now, and do not reject it outright.**

The full precondition list, each independently checkable against a source artifact, lives in `implementation/adoption-recommendation.md` ("Named preconditions," P1–P9). Summarized:

1. `go test` weaving verified safe in a resource-isolated environment (P1).
2. The Playwright e2e suite actually run against the woven binary (P2).
3. macOS build survival validated — every spike in this project ran Linux-only (P3).
4. The build-time cost (1.79x wall / 4.81x CPU-seconds, warm cache) is either explicitly accepted or reduced (P4).
5. The slow-SIGTERM / orphaned-tmux-session risk is fixed or mitigated (P5).
6. The persistent background export-retry cost when disabled is addressed (P6).
7. Two pre-existing, unrelated bugs discovered during this project — the `telemetry/telemetry.go` `resource.Merge` schema-URL conflict, and the `log` package's non-instance-aware `GetConfigDir()` — are filed as their own tracked bugs; the schema-conflict bug in particular must be re-verified against Spike C's coexistence finding if fixed, since fixing it changes which `TracerProvider` wins the race (P7).
8. `otelc` v1.0.1's own multi-target `go test` duplicate-symbol linking bug is resolved upstream or has a general per-repo workaround (P8).
9. `otelc version` is re-confirmed against the `v1.0.1` this project's findings were produced against, since it is an external binary with no `go.mod`/`go.sum` pin (P9).

Until then, the opt-in build stays exactly as ADR-003 structured it: a separate `build-otel-auto` target producing a separate `stapler-squad-otel` binary, protected by the CI-wired Isolation Guard, never touching `build`/`ci`/`ready`/`quick-check`/`pre-commit`/`install-service`.

## Consequences

**Positive**
- The decision rests on recorded evidence (spike commands, pprof tables, a parity summary table) rather than a subjective "seems fine" — every precondition traces to a specific artifact and section, per `implementation/adoption-recommendation.md`'s self-review.
- Nothing about today's default build, CI, or the deployed systemd-managed instance changes as a result of this ADR. The opt-in surface built by Phases 1–5 remains available for anyone who wants ent/`database/sql`, git/tmux subprocess, and other currently-untraced-path coverage today, on the documented Run Recipe (`.claude/docs/opentelemetry-auto-instrumentation.md`).
- The precondition list gives a future re-evaluation a starting point that doesn't require re-running this whole project — exactly the durability goal `plan.md`'s Story 6.1.1 acceptance criteria named.

**Negative**
- The observability gap `requirements.md`'s Problem Statement opened with (ent, git/tmux, and most of `session`/`server/services` remaining untraced by default) stays open. This is an explicit, disclosed non-outcome — requirements.md's own footnote on Success Metric #4 says shipping this recommendation is the contractual deliverable regardless of verdict, not a guarantee the gap closes.
- A "some day, once conditions hold" verdict carries real risk of becoming permanent by default, since — per `pre-mortem.md` #2 — the same Isolation Guard that protects the default build from `build-otel-auto` also guarantees CI will never notice the opt-in path silently breaking. This is why `implementation/adoption-recommendation.md`'s "Staying Current" section names a concrete, dated revisit trigger (2027-02-22, ~6 months out) rather than leaving this as a point-in-time verdict with no re-check mechanism.
- Three of the nine preconditions (P1, P2, P8) require follow-up engineering work, not just re-verification, before they can be checked off — the "once conditions hold" branch is not a formality.

**Neutral**
- This ADR does not reopen ADR-001 (tool choice), ADR-002 (spike-first sequencing), ADR-003 (structural opt-in), or ADR-004 (Toolexec Injection). All four hold as-is; this decision is scoped entirely to the separate question of flipping the *default*.

## References

- [implementation/adoption-recommendation.md](../implementation/adoption-recommendation.md) — the full precondition list, rollback plan, and staying-current mechanism this ADR summarizes.
- [implementation/overhead-report.md](../implementation/overhead-report.md) — the build-time and hot-path evidence backing preconditions P4–P6.
- [implementation/parity-report.md](../implementation/parity-report.md) — the functional-parity evidence backing preconditions P1–P2.
- [implementation/spike-verdicts.md](../implementation/spike-verdicts.md) — the go/no-go gate result and the evidence backing preconditions P3, P7–P9.
- [ADR-003: Structural opt-in via a separate binary](ADR-003-structural-opt-in-separate-binary.md) — the containment this ADR leaves unchanged until the flip.
