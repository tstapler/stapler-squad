# Pitfalls
- Two separate defects: (1) V0 map keys camelCase vs lowercased lookup (also silently breaks partialclone/preciousobjects); (2) worktreeconfig missing from builtinExtensions -> ErrUnknownExtension for V1 and after (1) is fixed for V0. Fixing only one leaves the failure (AC1 vs AC2).
- Semantics: go-git does not read `config.worktree`. Acceptable for branch/ref/remote resolution (reads shared config); per-worktree overrides (e.g. core.bare, sparse settings) are ignored. Document in the fork test/comment.
- In-process gogit backend has its own V0 guard (`enrichment_test.go` TestReadEnrichment_RejectsIncludesAndUnsupported); must stay green and unchanged (AC8).
- Tag/pin ordering: bump `go.mod` only after the tag is pushed and fetchable via GOPROXY/GOFLAGS; module cache may need `GOFLAGS=-mod=mod`.
- Fork is public; pushing is outward-facing but the backlog item scopes it.
