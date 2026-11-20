---
globs: ["session/**", "server/services/**", "tools/bzl/scoping/**", ".golangci.yml"]
---

# Leaf Packages: Extract Stores and Variants as Open-Closed Leaves

Intent: `session` and `server/services` shrink to stable cores; each concern becomes a leaf (a store, a registry entry, a variant) that the core embeds or the composition root wires, so a new variant is a new file or package plus one registration. Plans and status: `project_plans/package-split-ocp/` (`roadmap.md`, `ent-split-plan.md`); recipe: `docs/explanation/package-layout.md` ("How to add a leaf").

## Standards

1. **A leaf is its own store type, built by `New(client, ...)`.** `session.EntRepository` embeds it (`session/ent_repository.go`) and `initStores()` builds it. `initStores()` runs on every constructor path (`NewEntRepository`, `NewEntRepositoryFromClient`, the seed path behind `NewTestEntRepository`); a new path that sets `Client` must call it. `TestEntRepository_should_BuildEveryEmbeddedStore_When_ConstructedByAnyPath` (`session/ent_repository_webhook_registration_test.go`) reflects over the embedded fields and fails on a nil one. Test helpers that build a second repository go through `NewEntRepositoryFromClient` (`newCountingRepo`).
2. **A leaf cannot hold `func (r *EntRepository)`.** Methods become methods of the store type and reach callers by promotion through the embedded field, so call sites do not change. `scoping move` does not rewrite receivers, and it is blind to build-tagged and GOOS-specific files: `grep -rn` for the moved symbols in `*_windows.go`, `*_darwin.go` and `//go:build` files (#1638).
3. **Ports are consumer-side and passed as `Ports{...}`** (`session/backlogstore/guidance_port.go`). Use a getter (`func() T`) for a value installed after construction: `AfterLinkHook` and `DbPath` (#1701, #1702). A nil required port panics on first use, so `initStores()` wires all of them.
4. **Imports.** A leaf never imports `session` or `server`. `github` and `session/instance` only where its `isolate_<leaf>` row lists them (`sessionpr` today). Each leaf has an `isolate_<leaf>` depguard row (`list-mode: strict`, `.golangci.yml`) whose allow-list equals its real non-test imports exactly, and a `boundaries.txt` row (`!server`, plus `!github` where it holds). DTOs live in `session/domain` or `session/domain/backlogdto`, with no lasting type aliases. To keep an error sentinel's identity, define it in `domain` and re-point callers.
5. **Budget.** `go run ./tools/bzl/scoping budget --base origin/main` must print nothing. `budget --init <pkgs>` prints caps for the tree (rounded up 3%); take a new row from that output, not from hand-picked numbers. A raised row needs a `scoping-budget-raise: <pkg> files A -> B ...` line in a commit message that says whose growth it is: #1708 repaired the drift #1706 left and said so.
6. **Bazel.** Each leaf has a hand-maintained `go_sources` filegroup, listed in `data` of `//session:session_test` and `//server/services:services_test`; other packages are covered by `//tools/bzl/scandata:module_go_sources`. Run `go run ./tools/bzl/scandata .` and `make gazelle` (no diff). `bazel build --nobuild //...` is a blocking step in `.github/workflows/bazel.yml` (#1690).
7. **BacklogItem status writes (ADR-007 D1).** `SetStatus` on a BacklogItem builder may appear only in the allow-listed file and function pairs in `server/services/workflow_bypass_strict_test.go` (`clsSetStatusAllowed`): all in `session/backlogstore/status_writers.go` (`Backlog.CreateBacklogItem`, `RawArchiveBacklogItem`, `RawUnarchiveBacklogItem`, `RawTransitionBacklogItemStatusWithNote`, `RawTransitionBacklogItemStatusWithPRFields`). The `Raw*` writers are exported and promoted through `EntRepository`, so the compiler does not stop a caller; the lexical ratchets do (`TestBacklogItemSetStatus_OnlyInsideAllowListedWriters`, `TestWorkflowBypassStrict_NoLegacyWriterCallOutsideTheAllowList`, `TestWorkflowBypassInventory_EveryStatusWriteCallIsClassified`). Production callers use `Storage`'s gated path or `workflowengine`. Blind spot: SQL held in a package-level const or var. A new file that names `PrNumber` or `PrURL` needs a row in `session/legacy_item_pr_readers_test.go`.

## Verify before you push

- `golangci-lint run --new-from-rev` hides depguard failures on new files and `unused` on newly dead code (#1696, #1700). Also run `golangci-lint run --enable-only=depguard ./session/... ./server/...` and, whole-tree, `--enable-only=unused,ineffassign,staticcheck ./session/... ./server/...` (stub `server/web/dist/index.html` if the embed fails).
- gocognit and gocyclo gate modified existing functions too. revive `file-length-limit` (1000) flags any line you add to a file already over it.
- A `go test -run` pattern that matches nothing passes. Count RUN and PASS lines with `-v`.
- Add a mutation check for every moved or new branch: break it, see a test fail, restore.
- CI tests against the live base. Merge `origin/main` before blaming a lint failure on your diff. A push restarts CI (make-only runs take about 2 hours), so batch fixes.
- A test that blocks on a channel opens the gate in a `t.Cleanup` registered after the `Close` cleanup (LIFO runs it first) and bounds every wait (#1703).

## Why

Each rule above was learned from a red CI run or a silent guarantee loss. Receiver and constructor-path mistakes compile and fail only at first call; a missing depguard row or `go_sources` entry fails far from the cause; and moving the `Raw*` writers out of the compiler's reach turned ADR-007's choke point into a lexical test, which only works if the allow-list stays exact.
