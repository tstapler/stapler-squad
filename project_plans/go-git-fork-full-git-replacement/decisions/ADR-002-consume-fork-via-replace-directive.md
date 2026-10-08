# ADR-002: Consume the fork with a `replace` directive, not a module-path rename

**Status**: Proposed (validated by spike S1 / gate G1)
**Date**: 2026-10-08

## Context
- go.mod has no `replace` directives and no `vendor/` today. `tools/lint` is a separate module (`tools/lint/go.mod`).
- Libraries in the graph that import upstream go-git (check `go mod graph | grep go-git`) pass `*git.Repository`, `plumbing.*`, `transport.AuthMethod` across package boundaries. A renamed fork makes those distinct types.
- `replace` applies only to the main module; stapler-squad is an application, so that is acceptable.

## Decision
`replace github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 v5.19.3-ssq.N` in the root `go.mod` (and in `tools/lint/go.mod` only if S1 shows it imports go-git). Import paths in source stay `github.com/go-git/go-git/v5`. Fork tags use the form `vX.Y.Z-ssq.N`, never branch pseudo-versions, so `make tidy`/`verify` is reproducible.

The fork repo is **private** per the user's direction. That requires `GOPRIVATE=github.com/tstapler/go-git` and a read token in `.github/workflows/{build,lint,release,goreleaser-check,mcp-integration}.yml` and goreleaser. Making it public removes all of that and is the cheaper option; flagged to the user as an open decision (O-4 in plan.md), default private.

Rollback is deleting the `replace` line.

## Consequences
- Zero churn in 30 importing files; type identity preserved for transitive users.
- Dependabot will not bump upstream through `replace`; rebase cadence is manual (runbook story 6.3).
- S1 must verify that `go mod tidy`, `go mod verify` and `go build` accept a replacement whose own `go.mod` declares `module github.com/go-git/go-git/v5` (the fork keeps the upstream module line). The upstream-named `module` line is the form Go's `replace` is documented to expect, so the rename fallback is likely moot (adversarial minor; not empirically run in this repair pass, S1 settles it). Run S1 first at the repo's pinned v5.19.2 (tag `v5.19.2-ssq.0`) and only then bump to v5.19.3, so the bump does not confound the result. If it fails, fall back to the rename and accept the identity break, re-run the S1 identity check, and if cross-boundary types break the build, G1 is no-go.

## Alternatives rejected
- **Module-path rename**: explicit and `go get`-able, but breaks type identity with transitive upstream users and conflicts on every import line at each rebase.
- **Vendoring**: no vendor dir today, CI does not expect it, does not solve identity.
