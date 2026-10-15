# Spike S1: fork bootstrap and `replace` viability (Story 0.2.1)

Date: 2026-10-08. Scope: ADR-002 and gate G1. No product code or real repo file was changed; nothing was pushed and no GitHub repository was created.

## Verdict

**G1: GO.** The repo builds, vets and runs `./session/git/...` identically against an unpatched fork of go-git `v5.19.2` (tag `v5.19.2-ssq.0`) wired with `replace`, with one go-git copy in the build graph. ADR-002 stands: the rename fallback is not needed. Two acceptance criteria could not be run here and stay open (see "Not verified").

G0-related note: none of S1 depends on G0 counts. The import-site count measured below (80 `.go` files, 18 of them non-test packages' imports; see F4) is a scratch-copy figure and should be reconciled with the Story 0.1.1 audit.

## Method

- Upstream clone: `git clone https://github.com/go-git/go-git.git /tmp/s1/go-git-up` (default branch is v6 `main`; tags `v5.19.2` = `3eeb238d`, `v5.19.3` = `bdae57cc`).
- Fork stand-in (no remote fork was created): `/tmp/s1/go-git`, branch `ssq/v5` from tag `v5.19.2`, tag `v5.19.2-ssq.0`. Its `go.mod` line 1 is untouched: `module github.com/go-git/go-git/v5`.
- Repo stand-in: `/tmp/s1/repo`, a copy of go.mod, go.sum, root `*.go` and the Go source directories (not `web-app/`, `docs/`, `tests/`, `.git`). The copy was needed because `rsync`/`du` over the live checkout (27 GB `.git`, 2.5 GB `web-app`) timed out. `server/web/embed.go` embeds `all:dist`, which exists in the copy.
- Tagged-version `replace` was simulated with a file GOPROXY holding a module zip built by `golang.org/x/mod/zip` (`/tmp/s1/mkproxy`) at path `github.com/tstapler/go-git/v5`. Env: `GOPROXY=file:///tmp/s1/proxy,https://proxy.golang.org GONOSUMDB=github.com/tstapler/go-git`.
- Toolchain: go 1.26.6 darwin/arm64 (the repo's go.mod needs >= 1.26.6; `GOTOOLCHAIN=local` here is 1.26.5 and refuses, so do not set it).

## Findings

### F1. Baseline: the copy builds with the stock dependency (VERIFIED)
`go build ./...` in `/tmp/s1/repo` with the original go.mod/go.sum: `BASELINE_BUILD_EXIT=0`, 2:37 cold. `diff` against the live repo's go.mod and go.sum: identical.
`go test ./session/git/...` baseline fails exactly three tests:
`TestSetupFromExistingBranch_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError`, `TestSetupFromExistingBranch_SelfHeals_When_WorktreeRegisteredByDelayedRaceWinner`, `TestScaffoldingExcludePatterns_MatchGitignore`.
Causes (VERIFIED from test output): the first two compare `/var/folders/...` with `/private/var/folders/...` (macOS symlink, pre-existing environment issue); the third reads `../../.gitignore`, which my scratch copy omitted. So "all exit 0" for tests was not achievable in this environment; the criterion that matters is an identical failure set before and after, which held (F3).

### F2. `replace` to a local directory and to a tagged fork both work (VERIFIED)
Local directory (`go mod edit -replace github.com/go-git/go-git/v5=/tmp/s1/go-git`):
```
go mod tidy      -> TIDY_EXIT=0
go mod verify    -> all modules verified
go build ./...   -> BUILD_EXIT=0 (1:08)
go vet ./...     -> VETALL_EXIT=0
go list -m -f '{{.Path}} {{.Version}} => {{.Replace}}' github.com/go-git/go-git/v5
  github.com/go-git/go-git/v5 v5.19.2 => /tmp/s1/go-git
```
Tagged (`go mod edit -replace github.com/go-git/go-git/v5=github.com/tstapler/go-git/v5@v5.19.2-ssq.0`, then tidy with `-mod=mod`):
```
TIDY=0   go mod verify -> all modules verified   BUILD=0   VET=0
go list -m ... -> github.com/go-git/go-git/v5 v5.19.2 => github.com/tstapler/go-git/v5 v5.19.2-ssq.0
diff go.mod go.mod.orig -> only the added blank line + replace line
```
go.sum change: the two `github.com/go-git/go-git/v5 v5.19.2` lines are replaced by `github.com/tstapler/go-git/v5 v5.19.2-ssq.0 h1:u7lSp9DO...` and `/go.mod h1:QqCBE1EF...`. The `/go.mod` hash `QqCBE1EF...` equals the upstream v5.19.2 `/go.mod` hash, as expected for an unpatched fork. Tidy also dropped four go.sum lines (go-runewidth, tablewriter, uniseg) that only upstream's own module graph needed; harmless but expect that diff in the real PR.

### F3. Tests: failure set identical with and without the replace (VERIFIED)
`go test ./session/git/...` with the tagged `v5.19.2-ssq.0` replace: same three failures as F1 (`diff <(sort baseline) <(sort tagged)` differed only in timings). Same three again with the local replace and with `v5.19.3-ssq.0` (F6). No new failure and no pass-to-fail change.

### F4. Type identity: the single copy holds, and no third-party package consumes go-git (VERIFIED)
- `go list -m all | grep go-git/go-git` after replace: one `github.com/go-git/go-git/v5 v5.19.2 => ...` line.
- `go mod graph | grep -c ' github.com/go-git/go-git/v5@v5.19.2$'` -> `1` after the replace; the graph also lists `v5.17.1` required by `tailscale.com@v1.102.4`, which MVS resolves up to the replaced version.
- Importers: `go list -deps -test -f '{{.ImportPath}}|{{with .Module}}{{.Path}}{{end}}|{{join .Imports " "}}' ./...` filtered to go-git importers outside `stapler-squad` and outside go-git itself printed nothing. Only stapler-squad packages import go-git v5 (18 packages by that listing; `grep -rl 'go-git/go-git/v5' . --include='*.go'` in the copy: 80 files, 50 of them `_test.go`). `go-billy/v5` appears in 10 import lines.
- Consequence: today there is no transitive library whose exported API would carry `*git.Repository`, `plumbing.*` or `transport.AuthMethod` across a boundary, so identity risk is empirically nil for the current graph. It would matter only if a future dependency imports go-git; `replace` (unlike a rename) would still unify it.
- `tools/lint/go.mod`: `grep -c go-git tools/lint/go.mod` -> `0`. No replace needed there (answers Task 0.2.1b's conditional).

### F5. ADR-002's open point: the replacement's `module` line (VERIFIED, with a correction)
ADR-002 predicted Go requires the replacement's `module` directive to match the replaced path. Measured: both forms work.
- Fork keeping `module github.com/go-git/go-git/v5` (F2): works.
- Fork with `module github.com/tstapler/go-git/v5` on line 1 (tag `v5.19.2-ssq.9`, proxy `/tmp/s1/proxy-renamed`), consumer requiring `github.com/go-git/go-git/v5` with `replace ... => github.com/tstapler/go-git/v5 v5.19.2-ssq.9`, importing `github.com/go-git/go-git/v5`: `go build ./...` -> `EXIT=0`, no error. Go does not enforce the module line for a replaced module.
- Recommendation: keep the upstream module line unchanged. It costs nothing, keeps `go.mod` byte-identical to upstream in every rebase (no conflict on line 1), and the fork's internal imports (`github.com/go-git/go-git/v5/...`) resolve to itself.
- The true rename alternative (consumers import `github.com/tstapler/go-git/v5`) fails the moment any package still imports the upstream path: `cannot find module providing package github.com/go-git/go-git/v5`. Rename remains rejected (ADR-002).
- Caveat on the fork's own `go.mod`: because the fork's requirements flow into MVS (F6), the fork's `go.mod` should be left to upstream's values; do not edit it in patches.

### F6. Rebase burden v5.19.2 -> v5.19.3 (VERIFIED)
Upstream delta: `git diff --shortstat v5.19.2 v5.19.3` -> 22 files, 1937 insertions, 90 deletions; excluding tests, go.sum, `cli/`, `.github/`: 10 files, 465 insertions, 36 deletions. Touched non-test files include `repository.go`, `plumbing/object/tree.go`, `plumbing/revlist/revlist.go`, `plumbing/transport/http/*`, `plumbing/format/packfile/delta_selector.go`, `config/config.go`, `go.mod`.
Probe patches on `ssq/v5` (tag `v5.19.2-ssq.1`): P1 new file `ssq_ext.go` (additive), P2 one line at the top of `worktree.go` (file upstream did not touch), P3 one line in `repository.go` directly beside upstream's v5.19.3 hunk (`@@ -435,0 +436 @@`). `git rebase v5.19.3` -> `Successfully rebased and updated refs/heads/scratch-rebase`, **0 conflicts** of 3 patches (feeds O-7). Result: 3 files changed, 6 insertions.
Then wired as `v5.19.3-ssq.0` (replace + tidy): `TIDY=0`, `go mod verify` all verified, `BUILD=0`, `VET=0`; failure set of `go test ./session/git/...` unchanged (F3).
Side effect to plan for: the v5.19.3 bump changes the root go.mod because the fork's requirements flow in: `go-billy/v5 v5.9.0 -> v5.9.2`, `sha1cd v0.6.0 -> v0.7.0`, `golang.org/x/crypto v0.55.0 -> v0.56.0`, and removes `klauspost/cpuid/v2` (indirect). v5.19.3's own `go.mod` also raises `go` to 1.26.0 (repo is already >= 1.26.6, fine). Review these in the bump PR.
Limit: only additive, non-overlapping patches were probed. A patch editing a line upstream also changed would conflict; none of the planned fork patches (per ADR-001/ADR-004) are known to be of that kind, but that is INFERRED, not tested.

## Not verified (open items for G1 sign-off)
1. **Task 0.2.1a (real fork)**: no repository was created (not permitted in this task, and Story 1.2.0 requires Tyler to confirm owner/name). Everything above used a local stand-in with the same tag names and module layout. Residual risk is limited to GitHub-side mechanics.
2. **Task 0.2.1c (public-fork CI access)**: not run, because the public repo does not exist yet. `go mod download` of a public module via `proxy.golang.org` needs no `GOPRIVATE`/token by design, but that is INFERRED until a real tag exists. Note that `proxy.golang.org` returned 404 for `github.com/tstapler/go-git/v5/@v/list` today (`curl` -> 404), consistent with no such module existing yet.
3. `go test` was run for `./session/git/...` only (the story's scope), not the whole repo.

## Cleanup debt (needs a human)
My simulated fork modules were extracted into the real Go module cache, because I used the real `GOMODCACHE` to avoid a multi-GB re-download. Directories left behind (a removal command was blocked by a safety check, so they were not deleted):
- `$(go env GOMODCACHE)/github.com/tstapler/go-git/`
- `$(go env GOMODCACHE)/cache/download/github.com/tstapler/go-git/`
They contain fake `v5.19.2-ssq.0` and `v5.19.3-ssq.0` zips. If the real fork later publishes the same tags, `go` would hit a go.sum/cache hash mismatch (`SECURITY ERROR`) until these are removed (`chmod -R u+w` then `rm -rf` on both paths, or `go clean -modcache` for a full reset). Name the real first tag differently or clear these before Story 1.2.0.

## Scratch artifacts
`/tmp/s1/go-git` (fork stand-in), `/tmp/s1/go-git-up` (upstream clone), `/tmp/s1/repo` (repo copy; `go.mod.orig`, `go.mod.replace` beside it), `/tmp/s1/mkproxy` + `/tmp/s1/tools/main.go` (zip builder), `/tmp/s1/proxy*`, `/tmp/s1/neg`. Safe to delete.
