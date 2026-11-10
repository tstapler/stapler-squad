# ADR-003: Web build is a Bun-owned install wrapped in a repo-owned repository rule

**Date**: 2026-11-01
**Status**: Accepted 2026-11-06 (finalised at Task 0.14d). Spike S4 (Epic 0, Story 0.6) is `Pass`, path `repo-rule+no-sandbox` (`implementation/spike-results.yaml`; evidence: `implementation/spike-results.md` S4 "Verdict"), so the repo-owned Bun repository rule is the adopted default and neither `rules_bun` (`FallbackWorkaround`) nor the `aspect_rules_js` fork (`FallbackFork`) was needed. The `Pass` is conditional: it holds against a pinned-environment `make web-build` (Bun 1.4.3, Node 24.13.1, `BUN_INSTALL_GLOBAL_STORE=0`); against the default developer environment 174 of 380 files differ. The owner kept `Pass` on this basis (`implementation/owner-attestations.yaml`, `s4_verdict`). The fork-versus-workaround costing lives in ADR-008.
**Context**: `aspect_rules_js` 3.5.2 consumes a pnpm lockfile only (`research/stack.md` section 5, VERIFIED absence of `bun.lock` support in docs). Repo rule: Bun only, one lockfile (`web-app/bun.lock`, `docs/how-to/use-bun-in-web-app.md`). Third-party `rules_bun` projects exist but are 0-star, months old, not in BCR.

## Decision (default)

- Fetch a pinned Bun per OS/arch via `http_archive` with sha256 (version from `web-app/package.json` `packageManager`).
- A repo-local repository rule runs `bun install --frozen-lockfile` keyed on `bun.lock` + `package.json` + `patches/`, producing a `node_modules` repository.
- One action runs `bun build/build.ts` and emits `dist/` (tree artifact); Jest runs as a test over the same tree. Proto TS output (`protoc-gen-es`) feeds in as an input, replacing `web-app/src/gen/`.
- No pnpm lockfile is introduced. Per-package TS caching granularity is given up; the web build is one leaf.

## S4 verdict vocabulary (one meaning, shared with plan.md's Fallback matrix)

| S4 result | `SpikeVerdict` | Path taken |
|---|---|---|
| Every check passes with the repository rule | `Pass` | Bun repository rule (this ADR's default) |
| Passes, but the sandboxed `dist` action takes more than 1.5x `make web-build` | `Pass`, `path` = `repo-rule+no-sandbox` | Same rule; consuming actions tagged `no-sandbox` (a recorded cost, not a downgrade) |
| The rule fails output parity (delta outside `allowed-deltas.txt`), hermeticity (reads outside `web-app/`, writes into the source tree) or determinism | `FallbackWorkaround` | Pinned third-party `rules_bun` via `git_override` (Task 0.6f; bus factor one, recorded as risk) |
| `rules_bun` fails the same checks, or per-package granularity is needed for Jest runtime | `FallbackFork` | Fork `aspect_rules_js` to read `bun.lock` (ADR-008 rules; costed in Task 0.6d) |
| The fork is infeasible within ADR-008's cap | `Fail` | plan.md "If S4 ends `Fail`" consequences and owner decision D-HERMETIC |

## Consequences

- The install step has network access inside a repository rule (integrity comes from `bun.lock`); this is a recorded hermeticity exception, tagged so it is never uploaded to a shared cache until the determinism check (Story 7.7) passes.
- Phantom-dependency problems that rules_js strictness would surface are not detected.
- The Next/Bun migration (`web-app-bun-build`) stays out of scope; this wraps whichever `build:bun` produces.

## Alternatives considered

- aspect_rules_js + derived `pnpm-lock.yaml`: second lockfile, can resolve versions differently from `bun.lock`, violates the Bun rule. Rejected.
- Leave web-app outside Bazel (host `bun`): contradicts the hermeticity driver. Rejected.

## Spike outcome (Task 0.14d, evidence in `implementation/spike-results.md`)

- S4 checks: parity 380 of 380 files identical against the pinned reference; Jest 231 of 231 equal; hidden-coupling escapes `0 0`; wall time 2.6x sandboxed and 1.5-2.1x unsandboxed against the 1.5x limit, so consuming actions carry `no-sandbox` (the "Passes, but ... 1.5x" row of the vocabulary table above, a recorded cost, not a downgrade).
- Condition that comes with the acceptance: Bun and Node stay pinned and `BUN_INSTALL_GLOBAL_STORE=0` in the parity oracle; the `make` path is not itself deterministic against unpinned host tools.
- S1-ts `Pass`: proto TypeScript output feeds the web build as an input (`gen = //tools/bzl/proto:gen_ts`).
