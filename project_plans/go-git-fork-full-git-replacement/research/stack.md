# Stack research: go-git versions, linked worktrees, network parity

Date: 2026-10-08. Labels: VERIFIED (primary source opened/run) or INFERRED.

## 1. Versions and branches
- VERIFIED: repo uses `github.com/go-git/go-git/v5 v5.19.2` (go.mod).
- VERIFIED (`gh api repos/go-git/go-git/releases`): v5.19.3 (2026-10-04, stable), v5.19.2 (2026-07-29); v6.0.0-beta.1 (2026-10-04, prerelease), v6.0.0-alpha.5 (2026-07-29), alpha.4 (2026-05-18). Default branch `main` is the v6 line (module path `github.com/go-git/go-git/v6`, billy v6). v6 is still pre-release, so there is no stable v6 yet.
- VERIFIED: v5.19.2 module cache has NO `x/` directory and no worktree Add/Remove/List API (`ls ~/go/pkg/mod/github.com/go-git/go-git/v5@v5.19.2`; the `Worktree` methods in `worktree*.go` are Pull/Checkout/Reset/Status/Commit/Clean/etc.).
- INFERRED: linked-worktree creation exists only on v6 (main). A v5 fork would need to backport it or stay on v6 pre-releases, and a v6 move changes import paths everywhere.

## 2. Linked worktrees
### What v5.19.2 has (VERIFIED, file:line in `~/go/pkg/mod/github.com/go-git/go-git/v5@v5.19.2`)
- `PlainOpenOptions.EnableDotGitCommonDir` (`options.go:815-817`) -> `dotGitCommonDirectory` reads `commondir` (`repository.go:326-327, 428-457`). Read/open side only.
- `dotgit.NewWithOptions` (`storage/filesystem/dotgit/dotgit.go:170`); `repository_filesystem.go:12-46` routes objects/refs/packed-refs/config/logs/worktrees... to commondir. So a linked worktree can be OPENED and used (HEAD/index remain per-worktree-dir).
- `worktreeConfig` extension allowed (`repository_extensions.go:52`) but only as a v0-era name; v5.19.1 reportedly rejected it for case mismatch (INFERRED from search summary, unverified).
- `ErrRepositoryIncomplete` for missing commondir (`repository.go:57`).
- Nothing in v5 writes `.git/worktrees/<name>/{HEAD,gitdir,commondir}` or the `.git` file in a new dir (no code writes "worktrees" beyond path routing: grep of `storage plumbing`).

### What v6 main has (VERIFIED via `gh api` of repo main, `x/plumbing/worktree/`)
- Experimental package `x/plumbing/worktree` (files worktree.go, worktree_options.go). API: `New(storer)`, `Add(billyFS, name, WithCommit/WithDetachedHead)`, `Remove(name)`, `List()`, `Open(fs)`, `Init(fs, name)`.
- `Remove` deletes ONLY `.git/worktrees/<name>`; it does not delete the worktree directory (doc comment, worktree.go ~234-260). No lock/move/prune/repair (docs: https://go-git.github.io/docs/tutorials/worktrees/ , COMPATIBILITY.md lines 124-127, 225-227: `git worktree add/remove/list` = partial; `prune` row 108 = not supported).
- Names must match `[a-zA-Z0-9-]+` with a max length (docs). Git allows more characters; branch-derived names would need sanitizing.
- COMPATIBILITY.md: `extensions.worktreeConfig` supported (per-worktree `config.worktree`, filesystem storage only); `extensions.relativeWorktrees` supported.
- Only `storage/filesystem` satisfies `WorktreeStorer`.

### Upstream issues/PRs
- https://github.com/go-git/go-git/issues/41 "Checkout multiple branches": CLOSED 2025-12-31 (original request).
- https://github.com/go-git/go-git/issues/1812 "Review API for opening linked-worktrees": CLOSED 2026-03-03.
- https://github.com/go-git/go-git/pull/2336 relative worktrees (fixes #2324): MERGED 2026-10-01.
- https://github.com/go-git/go-git/issues/1956 checkout perf for worktree creation (decompresses every file; proposes copying from existing worktree): OPEN.
- https://github.com/go-git/go-git/issues/1896 `Worktree.Status()` descends into nested worktree dirs and reports false untracked files: OPEN. Relevant: stapler-squad keeps worktrees under `~/.stapler-squad/worktrees/`, so only matters if a worktree is nested in another repo tree.
- Old fork https://github.com/cooper/go-git ("go-git + linked repository support"), last push 2020-02-22: dead, not a basis.
- Answer to requirements Open Question 1: upstream already has add/list/remove-metadata/open on v6 (experimental, beta). Missing: prune, lock/move/repair, directory removal, `git worktree add -b` semantics, branch checkout wiring (INFERRED: `WithCommit` takes a hash; creating/pointing a branch is the caller's job), and anything on v5.

### Gap list for a fork (INFERRED unless marked)
1. v5 backport of x/plumbing/worktree, or adopt v6 (import-path migration; v6 beta.1 not stable).
2. Real `remove` (rm worktree dir + metadata, refuse if dirty/locked), `prune` (stale `gitdir` pointers), `list` details (HEAD, branch, locked).
3. Branch creation in the same operation (`-b`), name sanitizing.
4. #1956 perf (initial checkout reads all blobs) and #1896 status behaviour.
5. Verify per-worktree index/HEAD against real git with conformance tests (README/docs say partial).

## 3. Network parity (fetch/push/clone)
- Credential helpers: VERIFIED not supported in go-git core. #1420 closed `not_planned` 2025-09-03 (maintainer: "currently a feature that go-git does not support ... happy to review PRs"; https://github.com/go-git/go-git/issues/1420); #490 closed but comments confirm unimplemented, suggests an out-of-tree implementation (https://github.com/go-git/go-git/issues/490). v6 main `plumbing/transport/http/credentials.go` adds `CredentialRequest`/`Authorizer`/credentials-func hooks (VERIFIED file exists; hook API only, no `git credential` helper-protocol client seen). Implication: the fork needs its own `git credential fill` protocol client (exec of `git-credential-*` helper binaries) or direct keychain access. Note spawning `git-credential-osxkeychain` is still a subprocess, though not `git`.
- SSH: v5.19.2 has `NewSSHAgentAuth` (`plumbing/transport/ssh/auth_method.go:185`), `PublicKeys`, known_hosts (`:232-300`), and `DefaultSSHConfig = ssh_config.DefaultUserSettings` (`common.go:25`). v6 `ssh.go` has `UserSettings` hook and agent default (ssh.go:44-45,169). So SSH-agent works in-process (VERIFIED). Gaps: #509 OPEN, ssh config Hostname handling differs from openssh (https://github.com/go-git/go-git/issues/509); ProxyCommand/IdentityFile/Include coverage unverified (INFERRED incomplete: grep for ProxyCommand/IdentityFile in v5 transport found nothing). #1680 (pass ssh config) closed not_planned 2026-05-12.
- insteadOf: #844 OPEN (https://github.com/go-git/go-git/issues/844) with a PR #1888 referenced; relevant to a HTTPS->SSH rewrite done via git config.
- HTTPS->SSH "ssh-fallback" in this environment is a wrapper on PATH (requirements Baseline); whether it is product behaviour is Open Question 3, not answered here. If implemented in-process it is an app-level retry: on HTTPS auth failure, rewrite the URL to `git@host:owner/repo` and use agent auth (INFERRED design, not verified against existing code).
- Basic fetch/push/clone, token and SSH auth are marked supported in COMPATIBILITY.md lines 13-14, 44-46.

## 4. Fork-vs-alternatives implication (INFERRED)
- Worktree add is largely solved upstream on v6; the fork's real work is remove/prune and v5/v6 choice. Credential-helper parity is the larger greenfield piece and can live OUT of tree (plug into `transport.AuthMethod` / v6 Authorizer) without forking go-git at all. Recommend: no fork for network auth; carry only a thin patch set for worktree remove/prune, or contribute upstream.
