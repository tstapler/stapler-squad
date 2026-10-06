# ADR-003: Contain the auto-instrumented build structurally (separate target + separate binary), not with a runtime flag

**Status**: Accepted
**Date**: 2026-08-21
**Deciders**: SDD Phase 3 planning

## Context

`requirements.md`'s Constraints are unambiguous: this iteration is **opt-in only** and *"must not change what `make build`, `make install-service`, or `make ci` produce by default."* The deployed systemd-managed instance at `:8543` runs backlog automation that other work depends on, and `CLAUDE.md` already forbids using `make install-service` for in-progress changes because a restart kills the tmux server and every live session with it.

There are two plausible ways to honour that.

**Runtime gating**: build one binary, weave it always, and add a flag inside `telemetry/telemetry.go` (or a new env var) that decides whether woven instrumentation does anything. This keeps a single build artifact and a single Makefile path.

**Structural gating**: build a *separate* binary, from a *separate* target, that nothing in the default/CI/deploy chain references.

Runtime gating fails the constraint on its own terms: to gate at runtime, the default build must weave — which changes what `make build` and `make install-service` produce. It also puts new code on a path the deployed service executes, to solve a problem only the opt-in build has. `research/pitfalls.md` §5.1 argues the same point from the other direction: given the documented `-race`/injected-code interaction and the fact that woven source is not what `go vet`/staticcheck are designed to analyse, the woven path must be *structurally* unable to reach `make ci`, not merely discouraged from it in docs.

This repo already has the structural precedent. `coverage-integration` (`Makefile:544`) builds a differently-instrumented binary to a distinct path (`stapler-squad-cov`) via a plain flag change — "same source, different build invocation, separate output binary" is an established shape here, not a new idea (`research/ux.md` §1).

## Decision

**Containment is structural.**

- A dedicated target, `build-otel-auto`, following this repo's `build-<variant>` naming convention (`build-embedded`, `build-mux`, `build-tmux`).
- A distinct output binary, `stapler-squad-otel`, so the woven build can never overwrite the binary `install-service` deploys — and so the binary's mere path is the first-order "which build is this" signal (`research/ux.md` §4 notes the tool provides no post-hoc way to confirm a binary was woven).
- No changes to `build`, `stapler-squad`, `build-embedded`, `ci`, `ready`, `quick-check`, `pre-commit`, or `install-service`.
- A machine-checked **Isolation Guard** (`scripts/otel-auto-isolation-guard.sh`) that runs `make -n` over the five default/CI/deploy targets and fails if any of them would invoke `otelc` or produce `stapler-squad-otel`. Documentation is not the enforcement mechanism; the guard is.
- **The guard is a prerequisite of `ci`** (`Makefile:785`), and therefore of `ready` (`ready: ci …`, `Makefile:794`). This is what makes it an enforcement mechanism rather than a checklist item: a guard that only runs when someone remembers to invoke it cannot catch the edit it exists to catch, because the person making that edit is by definition not thinking about it.
- No new runtime flag. Emission stays gated by the pre-existing `OTEL_ENABLED` / `DD_TRACE_ENABLED` toggles (`telemetry/telemetry.go:68`), giving two independent layers: don't build it, or build it and leave the toggles unset.

Manual interactive testing of the woven binary uses this repo's documented manual-instance pattern — `~/.stapler-squad/manual-builds/`, `PORT=62871`, `STAPLER_SQUAD_INSTANCE=<name>`, `--tmux-keep-server` — never `make install-service`.

## Consequences

**Positive**
- The rollback procedure is `rm -f stapler-squad-otel` plus a normal PR revert; nothing that touches the live service can regress, because nothing that touches the live service changed.
- The constraint survives future edits by someone who hasn't read this plan, because the Isolation Guard runs inside `ci` and turns that edit's own CI run red — rather than trusting them to remember to invoke a check they don't know exists.
- `.claude/rules/prefer-go-git-over-subshells.md`-style drift is avoided in the other direction too: the default build's recipe is not duplicated (see ADR-004).

**Negative**
- Two binaries exist in the working tree, and `stapler-squad-otel` must be added to `.gitignore` alongside `stapler-squad` / `stapler-squad.prev` or it will show up in `git status`.
- No CI coverage *of the woven build itself*. A `go.mod` bump or an `otelc` release could break `build-otel-auto` and nobody would notice until someone runs it. Accepted deliberately: adding a CI job that *builds* the woven binary would be the coupling this ADR exists to prevent, and `scripts/otel-auto-smoke.sh` gives the on-demand check instead. (The Isolation Guard's presence in `ci` is not such a job — see below.)
- `ci` gains one more prerequisite, and with it a small runtime cost: five `make -n` dry runs plus a grep.

**Neutral**
- **The Isolation Guard runs in `ci`, and this does not import the coupling the ADR forbids.** An earlier draft of this ADR excluded it on the grounds that "wiring an otel-auto-named target into the CI chain is the exact shape it forbids." That reasoning was wrong on two counts, and it defeated the guard's purpose:
  - *It dry-runs; it does not invoke.* The guard's entire behaviour is `make -n <target>` (which prints recipe lines without executing them) piped to `grep`. It never runs `build-otel-auto`, never executes `otelc`, and never produces `stapler-squad-otel`. `ci` therefore gains no dependency on the weaving toolchain — the guard passes identically on a machine where `otelc` is not installed. What the constraint forbids is CI *building or depending on* the woven artifact; a text check over dry-run output does neither.
  - *The guard does not trip over itself.* It matches only the literals `otelc` and `stapler-squad-otel`. Neither its target name (`otel-auto-isolation-guard`) nor its filename (`scripts/otel-auto-isolation-guard.sh`) contains either string, so `ci`'s own dry-run output naming the guard is not a match. This is the same false-positive class already handled for `lint-shell`, which shellchecks `scripts/otel-auto-*.sh` by filename.
  - *Not running it in CI was the actual failure mode.* A guard invoked only on demand cannot catch a future edit that wires `build-otel-auto` into CI, because whoever makes that edit is precisely the person who will not run it.
