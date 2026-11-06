# S2: go-git v5 versus v6 confirmation (Story 0.2.2, gate G2)

Date run: 2026-10-08 (`date` -> `Thu Oct  8 17:13:55 PDT 2026`). Repo at branch `chore/sdd-go-git-fork-planning`, `go.mod:30` pins `go-git/v5 v5.19.2`, `go.mod:149` pins `go-billy/v5 v5.9.0`. All migration work ran in a scratch copy at `/tmp/s2-v6` (rsync, excluding `.git`, `worktrees`, `node_modules`, `web-app`, `docs`, `tests`, etc.); the real repo was not edited.

## G2 verdict: ADR-001 CONFIRMED (stay on v5)

The gate says switch to v6 only if all three hold. None holds:

| Criterion | Result | Evidence |
|---|---|---|
| v6.0.0 stable is out | NO. Latest v6 is `v6.0.0-beta.1` (prerelease) | releases and proxy output below |
| Measured migration under 1 day | NOT demonstrated: a mechanical rewrite leaves 19+ compile errors in 7 files that block every downstream package, and API removals (see below) | build output below |
| `x/plumbing/worktree` would delete native worktree code | NO. It covers Add/Remove(metadata only)/List/Open/Init, not prune, lock/unlock, or move, and is `x/` (experimental) | API listing below |

ADR-001's re-evaluation trigger ("v6.0.0 stable shipped and v5 has had no release for 90 days, or v5 EOL announced") is also not met: v5.19.3 shipped 2026-10-04, four days ago.

## 1. Release status (VERIFIED)

`gh api repos/go-git/go-git/releases --jq '.[:12][]|[.tag_name,.prerelease,.published_at]|@tsv'`:

```
v6.0.0-beta.1   true   2026-10-04T13:27:12Z
v5.19.3         false  2026-10-04T14:01:52Z
v6.0.0-alpha.5  true   2026-07-29T11:36:23Z
v5.19.2         false  2026-07-29T11:34:17Z
v6.0.0-alpha.4  true   2026-05-18T20:50:08Z
v5.19.1         false  2026-05-18T20:48:48Z
v6.0.0-alpha.3  true   2026-05-06T16:11:33Z
v5.19.0         false  2026-05-06T14:57:28Z
v6.0.0-alpha.2  true   2026-04-16T21:43:52Z
v5.18.0         false  2026-04-16T21:50:09Z
v6.0.0-alpha.1  true   2026-04-01T22:37:12Z
v5.17.2         false  2026-03-31T14:04:49Z
```

`gh api repos/go-git/go-git/tags --jq '.[].name' | head -5` -> newest tag `v6.0.0-beta.1`, then alpha.5..alpha.2 (no `v6.0.0` tag).

`go list -m -versions github.com/go-git/go-git/v6` -> `v6.0.0-alpha.1 .. v6.0.0-alpha.5 v6.0.0-beta.1` (proxy has no v6.0.0). `go list -m -versions github.com/go-git/go-git/v5` ends `... v5.19.0 v5.19.1 v5.19.2 v5.19.3`. `go-billy/v6` versions: `v6.0.0-alpha.1, alpha.2, beta.1` (also no stable).

v6 cadence so far: five pre-releases in six months, mirrored by v5 minors on the same days; v5 is actively released alongside v6.

## 2. v5.19.3 changelog (VERIFIED)

`gh api repos/go-git/go-git/releases/tags/v5.19.3 --jq .body` (full changelog `v5.19.2...v5.19.3`). 13 PRs, all fixes/maintenance, no feature or API change:

- Security/hardening: `transport/http` contain credentials across redirects (#2358); `x/crypto` to v0.55.0 then v0.56.0, both tagged SECURITY upstream (#2348, #2357); `go-git/v5` self-bump to v5.19.2, tagged SECURITY upstream (#2304); `packfile` break cyclic delta chains instead of recursing (#2340); `revlist` skip path validation in the object walk (#2305).
- Correctness: `repository` ensure commondir is closed (#2285); `config` read `repositoryformatversion` in `unmarshalCore` (#2167).
- Build/deps: go-billy/v5 to v5.9.2 (#2467), `sha1cd` v0.7.0 (#2468), drop Go 1.25 (#2451), Go 1.27 workflow fixes (#2364), test flake fix (#2421).

Planning implication: the v5.19.3 bump is worth taking for the credential-redirect and cyclic-delta fixes; note the Go 1.25 drop (this repo builds with go1.26.x, so no conflict) and billy v5.9.2.

## 3. Migration cost in a scratch copy (VERIFIED)

Baseline: `cd /tmp/s2-v6 && go build ./...` exits 0 (4m16s, cold cache), so the copy is a valid starting point.

Import rewrite:

```
grep -rlE 'go-git/go-(git|billy)/v5' --include='*.go' .      -> 80 files (30 non-test, 50 test)
sed -i '' -E 's#github.com/go-git/go-git/v5#.../v6#g; s#github.com/go-git/go-billy/v5#.../v6#g'
go mod edit -droprequire ...v5 (both); go get go-git/v6@v6.0.0-beta.1 go-billy/v6@v6.0.0-beta.1
```

(80 includes 2 `tools/lint/norawgitopen` testdata files; ADR-001's 30 non-test count matches.) Files by directory: `session/git` 30, `session/unfinished/gogitstore` 13, `session` 12, `server/services` 10, `session/unfinished` 7, and 1 each in `github`, `server`, `server/mcp`, `session/vc`, `testutil/gitfixture`, `tools/lint/norawgitopen`.

`go get` also bumped unrelated modules (`x/sync`, `x/sys`, `x/term`, `x/text`, `x/tools`) and pulled `gcfg/v2`, so v6 forces a transitive dependency refresh.

Side finding: the `norawgitopen` analyzer hardcodes the import path (`tools/lint/norawgitopen/analyzer.go:97`, `fn.Pkg().Path() != "github.com/go-git/go-git/v5"`), so a v6 move also needs the lint rule and its testdata path (`testdata/src/github.com/go-git/go-git/v5`) migrated.

Build: `go build -gcflags=-e ./...` -> exit 1, 19 error lines in 2 packages, 7 files (`/tmp/s2-build.txt`):

| File | Errors |
|---|---|
| `session/git/gitignore_fs_cache.go` | 5 |
| `session/git/worktree_git.go` | 2 |
| `session/git/native_merge_index.go` | 1 |
| `session/git/util.go` | 1 |
| `session/unfinished/gogitstore/store.go` | 5 |
| `session/unfinished/gogitstore/mmapindex.go` | 4 |
| `session/unfinished/gogitstore/index.go` | 1 |

Nature of the errors (deduplicated):

- `billy.Filesystem` interface change: `ReadDir` now returns `[]fs.DirEntry` not `[]os.FileInfo`; `cachedFile` lacks `Stat`; `cachedFilesystem` lacks `Chroot`. `Worktree.Filesystem` is now `func() billy.Filesystem`, not an assignable field (3 errors in `gitignore_fs_cache.go`/`worktree_git.go`).
- `git.PlainOpenOptions.EnableDotGitCommonDir` no longer exists (`session/git/util.go:36`; field absent from v6 source, `grep -rn EnableDotGitCommonDir` over the v6 module dir returns nothing). The repo relies on it for linked-worktree `OpenRepo` semantics (see plan Story 0.2.4 note on `CommonDir`), so behaviour, not just a rename, must be re-verified.
- Hash width API: `hash.Size` undefined; `plumbing.Hash` is now a struct, not sliceable (`idx.PackfileChecksum[:]`, `idx.IdxChecksum[:]`); `idxfile.NewMemoryIndex`, `idxfile.NewDecoder`, `objfile.NewReader`, `index.NewEncoder` all take an extra argument (object-format/hasher).
- `idxfile.Index` interface now requires `Close` (`lockedIndex` in `gogitstore/index.go:43`).
- `packfile.NewPackfileWithCache` removed.

The `gogitstore` package (13 files, mmap pack/index store under `session/unfinished/`) is the heaviest: it implements go-git storage internals that v6 reshaped around multi-hash (SHA-256) support.

Lower bound caveat: Go type-checks per package and skips dependents of a failing package. `go build ./github ./session ./session/vc ./server/...` all stop at `session/git` or `gogitstore`, and only `./testutil/gitfixture` built clean. The 19 errors are therefore a floor; errors in the 8 packages that import `session/git`, in the 12 `session` files, and in the 10 `server/services` files are not yet visible. Test files were not compiled (`go vet ./...` is blocked the same way). Estimated effort is not verified; I did not fix the errors, so I can not claim a number of hours. What can be said: 7 files with API-semantic (not rename-only) changes in just the first two layers, plus an unmeasured remainder, plus lint-rule and dependency-refresh work, is not credibly "under 1 day" and cannot be shown to be.

## 4. v6 `x/plumbing/worktree` surface versus the repo's native worktree code (VERIFIED by reading v6.0.0-beta.1 source)

Module dir: `~/.asdf/installs/golang/1.26.4/packages/pkg/mod/github.com/go-git/go-git/v6@v6.0.0-beta.1/x/plumbing/worktree/worktree.go`.

```
New(storer) (*Worktree, error)
(*Worktree).Add(wt billy.Filesystem, name string, opts ...Option) error   // WithCommit, WithDetachedHead
(*Worktree).Remove(name string) error      // removes .git/worktrees/<name> metadata only
(*Worktree).List() ([]string, error)       // names only
(*Worktree).Open(wt billy.Filesystem) (*git.Repository, error)
(*Worktree).Init(wt billy.Filesystem, name string) error
```

`Remove`'s doc comment: "this only removes the metadata; it does not delete the actual worktree filesystem or its files." It is in `x/`, the package path upstream marks as may-change-without-notice (ADR-001 context).

Repo side: `session/git/native_worktree_{add,common,list,prune,remove}.go` is 530 lines, plus `_test` files bringing the group to 1491 lines. Functions: `AllocateAdminDirName`, `nativeSetupNewWorktree`, `writeNativeWorktreeAdminFiles`, `checkoutNativeWorktree`, `nativeUnlockWorktree`, `ListWorktrees` (returns path and HEAD ref, not just names), `nativeWorktreePrune`, `nativeRemoveWorktree`.

Gaps in v6 versus repo needs: no prune; no lock/unlock; no move; `List` returns names only (the repo needs path and HEAD ref and on-disk existence checks); `Remove` leaves the worktree directory; `Add` takes a billy filesystem and a name, with no admin-dir allocation (the repo's `AllocateAdminDirName` avoids a Lstat-then-Mkdir race that ADR-001 attributes to v6 `Add`; not re-verified here). Conclusion: adopting v6 would not let the repo delete `native_worktree_*.go`; at best `Add`'s metadata writing could replace about 2 of the 8 functions, which the plan's criterion ("would delete native worktree code") does not reach.

## Not verified / gaps

- v6 migration effort in hours (downstream errors and test compilation not measured).
- Whether the v6 `Add` race ADR-001 mentions exists (cited from research, not re-run).
- v5 EOL announcement: not found in releases, but not searched for in upstream discussions or `SECURITY.md`.

## Recommendation for `gates.md`

G2: GO on v5 / ADR-001 confirmed, status Accepted. Re-check at each rebase and at the 90-day mark (2027-01-02) from v5.19.3 if v6.0.0 stable has shipped by then. Add a plan note that v6 would also require migrating `tools/lint/norawgitopen` and replacing `EnableDotGitCommonDir` reliance. Take v5.19.3 in the S1 rebase step as planned.
