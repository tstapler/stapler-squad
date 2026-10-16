# ADR-001: Reject Full Bazel/bzlmod Adoption for This Initiative

**Date**: 2026-08-27
**Status**: Accepted
**Context**: ci-speed — evaluating build-graph/caching tooling changes (requirements.md Scope; explicitly flagged as a "Rabbit Hole")

---

## Context

`requirements.md` asks Phase 2 research to give full Bazel/bzlmod adoption (for the whole Go + protobuf + ent + tmux + Next.js/pnpm build graph) a genuine, non-prejudiced look, despite this repo's prior narrow Bazel integration (an optional `rules_foreign_cc` path for the bundled tmux native build) having been removed in commit `b51b60eb1` (2026-07-15) because WORKSPACE-mode is permanently broken under Bazel 9. Three independent research passes (`research/stack.md` §4, `research/architecture.md` §2 Pattern C, `research/build-vs-buy.md` Option A, `research/pitfalls.md` §3) converged on the same verdict.

## Evidence

- **Migration surface is large relative to this initiative's appetite.** At 1,392 Go files / 423,927 LOC and 1,281 web-app TS/TSX files (`research/build-vs-buy.md`), hand-writing BUILD files is a non-starter; Gazelle auto-generates Go BUILD files well, but `session/ent/*.go` (gitignored, regenerated per-build) and `buf`-generated proto code both need custom Gazelle extensions or hand-written `genrule`s, and the pinned tmux native build (already tried once under `rules_foreign_cc` and pulled out) would need re-wiring under bzlmod. A full bzlmod adoption alone is plausibly a multi-week project — this initiative's whole appetite is 3–6 weeks (`requirements.md` Appetite).
- **The free-runner constraint removes Bazel's biggest lever.** This project is constrained to `ubuntu-latest`/free-tier hosted runners (`requirements.md` Constraints) — no self-hosted runners, no remote execution. Bazel's most decisive advantage over what this repo already has (Go's own `GOCACHE`/`GOMODCACHE`, warmed via `actions/setup-go`'s `cache: true`) is remote *execution*, which is off the table here; a GHA-cache-backed remote *cache* (not RBE) is a much weaker win, and even that would require a new external dependency (BuildBuddy free tier, or a bespoke GHA-cache-as-remote-cache shim) that this repo doesn't currently operate for any other part of its build (`research/stack.md` §4).
- **No mature equivalent of Gazelle exists for the pnpm/Next.js half.** `aspect_rules_js` is bzlmod-native and current (3.2.3, `rules_js` 3.0 shipped 2026-02-09), but its Next.js integration story is reported as rougher than a plain SPA, and `web-app/`'s pnpm-lock translation into Bazel's lockfile model has more sharp edges and fewer worked examples than the Go side (`research/stack.md` §4, `research/pitfalls.md` §3). Realistically this would land as a *split* build system (Bazel for Go, pnpm-native for the frontend) rather than the single unified graph the "full adoption" framing implies.
- **This repo has direct, dated scar tissue from a *narrower* Bazel adoption already breaking under a platform migration** (the `b51b60eb1` removal). Re-adopting Bazel — even correctly, bzlmod-first this time — carries a real risk of repeating that pattern at the next Bazel/rules_go/aspect_rules_js major-version boundary, for a repo whose code is written almost entirely by Claude Code sessions that would now need to reason in Bazel's model in addition to `go build`/`pnpm`/`make`.
- **The lower-risk alternatives capture most of the achievable win.** `research/build-vs-buy.md` Option C+D (fix cache-key correctness, `cache-dependency-path`, and build-once/fan-out via `actions/upload-artifact`/`download-artifact` — the mechanism this plan implements in Phase 3) directly targets the concrete, measured redundancy (9+ independent `setup-go` cache scopes, 4x redundant tmux builds, 7+ independent re-runs of `buf generate`/`ent generate`/`next build` across workflow files) without adopting a new build system at all.

## Decision

**Do not adopt Bazel/bzlmod for this initiative.** No Bazel implementation work is planned in any phase of `project_plans/ci-speed/implementation/plan.md`. The plan instead generalizes the build-once/fan-out pattern `build.yml`'s own `prepare` job already proves works (Phase 3), fixes cache-key/toolchain-pin correctness (Phase 2), and closes the tail-latency/duration-regression gap natively via `timeout-minutes:` plus a small advisory `gh api`-based trend script (Phase 4) — see ADR-002.

## Consequences

- This repo keeps exactly one build/dependency model (`go build`/`go test` + `make` targets + `pnpm`), with no new mental model for contributors or Claude Code sessions to learn.
- The Go+protobuf+ent+tmux+web-app build graph remains *approximated* by hand-maintained `paths:`/`detect-changes` regex lists (three of them today: `build.yml`, `mcp-integration.yml`, `tools/ci/detect-feature-changes.sh`) rather than a single affected-target graph — `research/architecture.md` §4c already flags this as a correctness risk worth its own consolidation story, independent of Bazel.
- If a future initiative has its own dedicated appetite and a stronger multi-team monorepo-governance driver (not just CI speed), Bazel/bzlmod can be revisited from a clean slate — bzlmod itself is not rejected as a technology, only as *this initiative's* mechanism.

## Alternatives Considered

- **Full Bazel/bzlmod adoption (Go + web-app + ent + proto + tmux)** — rejected; see Evidence above.
- **Go-only Bazel (Gazelle-driven), pnpm-native web-app** — considered as a narrower variant; still rejected for this initiative because it still requires wiring ent/buf codegen into Gazelle and doesn't remove the free-runner remote-execution gap; revisit only with dedicated appetite.
- **Turborepo/Nx for `web-app/` only** — evaluated separately (`research/build-vs-buy.md` Option B); not rejected outright, but deferred: `web-app/` is a single-package pnpm project today, not a multi-package workspace, so Turborepo/Nx's core value (cross-package affected-graph analysis) has little to bite on until/unless that changes.
