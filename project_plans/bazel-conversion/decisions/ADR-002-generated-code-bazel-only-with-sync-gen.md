# ADR-002: Generated code is produced by Bazel actions; `sync-gen` materialises it for `go`/gopls

**Date**: 2026-11-01
**Status**: Accepted 2026-11-06 (finalised at Task 0.14d). Mechanism confirmed with amendments by the spike verdicts in `implementation/spike-results.yaml`: S1 `FallbackWorkaround` (proto Go is produced by a `buf generate` genrule with pinned local plugins, not `go_proto_library`), S2 `FallbackWorkaround` (ent is produced by a declared-output `ent_gen` rule plus an `entc` program, no fork), S11 `Pass` (Gazelle directives and `map_kind`, no extension; 6 resolve lines against the threshold of 8). S1-ts `Pass` covers the TypeScript half. The Bazel-ignores-synced-tree rule is not spike-dependent. See "Spike outcome" below.
**Context**: `gen/`, `web-app/src/gen/`, `session/ent/*` are gitignored by repo rule and must never be committed. Plain `go build`, gopls, golangci-lint and every agent habit assume `go build ./...` works.

## Decision

Design "C" from `research/architecture.md` section 2.2:

1. **Bazel actions are the only producer** (proto Go + connect-go, proto TS, ent). `bazel build //...` needs no precondition on a fresh clone.
2. `bazel run //:sync-gen` copies the Bazel outputs into the existing gitignored paths so `go build`, gopls, golangci-lint and nilaway keep working. `make sync` wraps it; `make build/lint/test` wrappers call it first during the strangler period.
3. **Bazel never reads the synced copy**: `gen/` and `web-app/src/gen/` are in `.bazelignore` and `# gazelle:exclude`; synced ent files and entity directories under `session/ent/` are excluded from Gazelle (mechanism picked by spike S11). A stale or absent synced tree cannot change Bazel results.
4. A freshness check (hash of synced tree vs `bazel-bin` output) runs in `make ready`; it only protects humans/IDEs.
5. Gazelle learns about the generated packages via `# gazelle:resolve go ...` lines (kept to the count where a custom Gazelle extension would be cheaper, threshold: more than 8 lines or any per-entity line, see S11).

6. **Feature-registry inputs stay outside Bazel** (Task 2.4c): `docs/registry/features/*.json` is a committed, scan-derived artifact produced by `make registry-generate`, not gitignored generated code. CI keeps running `make registry-generate` and fails on a non-empty `git diff -- docs/registry`; it is listed as a non-Bazel input in `tools/bzl/non-bazel-inputs.txt`.

## Consequences

- One codegen definition feeds both Bazel and sync-gen; no duplicated logic.
- Cost per extra worktree is a copy of generated files, not a regeneration (disk-cache hit).
- Byte-equality of Bazel output vs `make proto-gen ent-gen` is parity item P4.

## Alternatives considered

- **Bazel-only, no sync**: hermetic but breaks `go`/gopls/lint and agent muscle memory. Rejected.
- **Sync only (Bazel reads synced tree)**: reintroduces the worktree-drift failure the project exists to remove. Rejected.
- **Commit generated code**: forbidden by repo rule (`session/ent/*` incident).

## Post-cutover status of native go tooling (amendment, triad iteration 1)

After the flip, Bazel is the build of record and the only path that produces shipped artifacts and the authoritative test verdict. Native `go build`, `go vet`, `go test` and gopls remain working in every worktree after `make sync` and are supported for editor tooling, golangci-lint, quick local iteration and bisecting only; a native `go test` result is never authoritative and a disagreement is settled by the Bazel result. They are not removed in Epic 10. The precondition is inverted and must be announced: Bazel needs no generated code, plain go/gopls needs `make sync` (printed by `make help` and `make bazel-doctor`; staleness caught by the cheap input-hash check, Tasks 2.4d and 8.2d).

## Spike outcome (Task 0.14d, evidence in `implementation/spike-results.md`)

- **Decision 1 holds, with a different action per half.** Proto Go: S1 section "Verdict" - `go_proto_library` + connect compiler did not reproduce `make proto-gen` (20 of 20 `*.pb.go` differ, connect half does not build), while the `buf generate` genrule (`//proto/session/v1:gen_go`) is byte-identical (37 files, `compare-proto.sh buf`). Proto TS: S1-ts `Pass` (`buf generate` plus `protoc-gen-es` from the Bun install). Ent: S2 section "Mechanism" - output byte-identical to the `ent generate --feature sql/upsert` reference (333 files, 52 generated libraries under `//tools/bzl/ent`).
- **Decision 3 and 5 (Gazelle) hold.** S11 table: second Gazelle run has zero diff with and without a synced tree; 6 `resolve` lines (limit 8), no per-entity line, so no custom extension. The ent exclusion is seven prefix-negation globs ("all but `schema`"), replacing the brittle two-line form.
- **Importer check closed.** S2 was run on a subset only; "Importer completion check (S2/S11)" then built `//session/... //server/...` 253 of 253 and passed 5 of 5 importer tests, so S2 is not downgraded to `Fail`.
- **Costs recorded:** each generated-code list (`packages.bzl`, `schemas.go`) needs `bazel run //tools/bzl/ent:update_packages` or an edit on a schema change; the first is enforced by the action.
