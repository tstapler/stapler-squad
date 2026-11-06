# Research: Pitfalls of a go-git fork replacing the git CLI

Date: 2026-10-08. Label key: VERIFIED = source opened or command run this session; INFERRED = reasoned from verified facts. Issue states retrieved with `gh api repos/go-git/go-git/issues/N` on 2026-10-08.

## 0. Prior art already in this repo (read first)

`project_plans/go-git-worktree-and-merge/` (2026-09) already researched a closely related question (reimplementing worktree plumbing on go-git). Its `research/pitfalls.md` is directly reusable. VERIFIED (read in this session):

- go-git is documented "not thread-safe"; concurrent reads can corrupt results ([go-git#773](https://github.com/go-git/go-git/issues/773), open; `Worktree.Add()` unchecked on its checklist). Relevant to "dozens of concurrent sessions".
- go-git writes are not crash-safe or CLI-interoperable: `index` is written by a truncating `Create` (no `index.lock`, no temp+rename); loose refs use in-process advisory `flock` and in-place truncate+write, so a real `git` process does not see the lock; crash leaves a zero-byte ref. (`project_plans/go-git-worktree-and-merge/research/pitfalls.md` §1.4, verified there against v5.19.2 source; ADR-001 there chose a temp+rename protocol.) Because this repo mixes CLI and go-git on the same repo today, and the requirements keep a CLI fallback during rollout, mixed-writer corruption is a live risk, not a hypothetical.
- [go-git#2242](https://github.com/go-git/go-git/issues/2242) (open): `object not found` when a concurrent repack runs; go-git builds its pack index once and never retries, CLI does. [go-git#827](https://github.com/go-git/go-git/issues/827) (open): intermittent `object not found` for HEAD.
- JGit still has no worktree support after 15 years ([eclipse-jgit#76](https://github.com/eclipse-jgit/jgit/issues/76), [#264](https://github.com/eclipse-jgit/jgit/issues/264), both open per that doc): evidence that the "per-worktree HEAD/index + commondir" surface is deep.

Repo-local go-git failure lessons (VERIFIED, files opened):

- `session/git/util.go:326-345` (`getHeadCommitSHA`): go-git `repo.Head()` errors right after a CLI `git worktree add`, and in production returned a syntactically valid SHA with no matching object (consistent with an unlocked ref-file read racing a CLI atomic-rename update). Mitigation: 3 retries x 20 ms, then CLI fallback. This is the only documented "torn read" workaround; `.claude/skills/prefer-go-git-over-subshells/SKILL.md` instructs "fall back only for a named failure mode" and lists merge and credential-helper push/fetch as still-CLI.
- `docs/bugs/fixed/BUG-104-worktree-status-merkletrie-tree-diff-cpu.md`: `Worktree.Status()` always builds a full merkletrie diff; it was 16.58% of production CPU for a boolean dirty check. Fix was a hand-rolled index-vs-HEAD hash compare plus mtime/size check (`session/git/worktree_dirty_fast.go`). A fork that "replaces `git status`" must bring this kind of fast path with it; plain `Status()` is the slow path.
- `docs/tasks/squad-ux-polish.md:185,619`: stale `.git/index.lock` after killed `git worktree add` in batch creation; mitigated by per-repo mutex and `git worktree prune` at start. go-git's `Prune`/lock/move are not implemented (see 2.1), so the fork must own this recovery.
- `docs/bugs/fixed/BUG-077...`: CLI `git worktree list` is a pure read of `.git/worktrees/*` and takes tens of ms; the 5 s timeout failures were host CPU starvation, not git slowness. Evidence that the real cost driver is process spawn under load (matches requirements.md Baseline), not git work.
- `project_plans/stapler-squad-painpoints/research/findings-stack.md:837,948`: earlier research already flagged that go-git lacks sparse-checkout/submodule/shallow edge-case parity ("have D1 (shell) fallback").

## 1. Long-lived fork maintenance

### 1.1 The upstream target is v6, not v5 (largest rebase/API-churn fact)

- VERIFIED: linked-worktree creation exists only under `x/plumbing/worktree` in the **v6** line. `gh api contents/x/plumbing/worktree?ref=v5.19.2` returns 404; `?ref=v6.0.0-beta.1` lists `worktree.go`, `worktree_options.go`. Docs say it is "experimental ... may change without notice", `add` only; "Worktree lock, move, and prune operations are not yet supported", and only `storage/filesystem` satisfies `WorktreeStorer` ([go-git docs: Worktrees](https://go-git.github.io/docs/tutorials/worktrees/), [COMPATIBILITY.md](https://github.com/go-git/go-git/blob/main/COMPATIBILITY.md): `worktree add` = partial).
- VERIFIED: release state 2026-10-08: v5.19.3 (stable, 2026-10-04), v6.0.0-beta.1 (prerelease, 2026-10-04), alpha.5 (2026-07-29). The repo pins v5.19.2 (`go.mod:30`) with `go-billy/v5 v5.9.0`. v6 imports are `github.com/go-git/go-git/v6` and `go-billy/v6` (per the docs example). INFERRED: using the worktree package means a module-path migration of every `session/git` import plus billy v6, while v6 is still beta; a fork based on v5 would have to back-port the worktree package and keep it in sync with a moving v6 API.
- VERIFIED: upstream security policy: "only the latest minor release is actively supported" ([SECURITY.md](https://github.com/go-git/go-git/blob/main/SECURITY.md)). A fork pinned to an older minor gets no upstream fixes; every advisory must be re-applied or the fork must track the latest minor continuously.

### 1.2 Security advisory cadence (rebase pressure is high and recent)

From a WebSearch result list (INFERRED-grade: aggregator pages, not each GHSA opened; verify IDs against https://github.com/go-git/go-git/security/advisories before citing in an ADR). Roughly 12 advisories in 18 months:

- 2025: CVE-2025-21613 / GHSA-v725-9546-7q7m (argument injection via `file` transport URL, fixed 5.13.0, CVSS 9.8); CVE-2025-21614 (server-response resource exhaustion, 5.13).
- 2026: CVE-2026-25934 (pack/idx integrity not verified, 5.16.5); CVE-2026-33762 and CVE-2026-34165 (index v4 decoder panic, crafted `.idx` memory DoS; 5.17.1); CVE-2026-41506 / GHSA-3xc5-wrhm-f963 (credential leak on cross-host redirect, 5.18.0); CVE-2026-45022 (object parse differs from git, 5.19.0); CVE-2026-45570 (SSH transport quote escaping in repo path, 5.19.1) and CVE-2026-45571 / GHSA-crhj-59gh-8x96 (path validation can reach `.git`, 5.19.1); CVE-2026-71556 (worktree ops follow symlinks outside worktree) and CVE-2026-71557 (unsanitised ref names write outside ref dir), both fixed in 5.19.2 / 6.0.0-alpha.5; one more malformed-object DoS advisory (GHSA-w5pp-99ch-qj29) without a CVE number in the results.
- Implications (INFERRED): (a) the bug classes are exactly the code a worktree/checkout fork touches (symlinks, path validation, ref names, credential handling); a fork that rewrites checkout/worktree paths must re-verify each advisory still holds, because a textual cherry-pick may not apply. (b) Releases landing every ~2-3 months (5.19.1 on 2026-05-18, 5.19.2 on 2026-07-29, 5.19.3 on 2026-10-04) with security content each time means a single maintainer owes a rebase plus review per release. (c) A known-affected-version scanner will flag the fork unless its module path/version scheme is preserved; see fleetdm/fleet#45352 in the results for a case where scanners missed CVE-2026-45022 (not opened).
- This repo is itself the credential-bearing consumer: CVE-2026-41506 and #2136 ([BasicAuth cleared permanently after cross-origin redirect](https://github.com/go-git/go-git/issues/2136), open) are in the exact code path (smart HTTP with tokens, GitHub Enterprise redirects) that credential parity would exercise.

### 1.3 Rebase-burden shape

- INFERRED: cost is proportional to how much of the fork is in files upstream also edits. Worktree work in `storage/filesystem/dotgit`, `repository.go`, `worktree*.go` collides with upstream's active worktree work (open: [#1956 checkout perf for worktree creation](https://github.com/go-git/go-git/issues/1956), [#1896](https://github.com/go-git/go-git/issues/1896), [#2297](https://github.com/go-git/go-git/issues/2297); recently merged [#2336 relative worktrees](https://github.com/go-git/go-git/pull/2336), closing #2324, 2026-10-01). Each of these touched the same layer within the last 6 weeks.
- INFERRED mitigation: keep the fork as additive packages (new files, new `x/`-style package) plus a minimal patch series, rather than edits in hot files; contribute upstream where upstream already has the surface (their `x/plumbing/worktree` accepts PRs; maintainer said on #1420 "more than happy to review PRs" for credential helper support).
- Dependency skew: any library in this repo's graph that depends on upstream go-git will resolve a second copy unless `replace` is applied module-wide; types from the fork and upstream will not interoperate across package boundaries (INFERRED from Go module semantics).

## 2. Replacing the CLI in a worktree-heavy, concurrent tool: known go-git gaps

### 2.1 Feature support table (VERIFIED against COMPATIBILITY.md on main, fetched via `gh api`)

| Area | Status in go-git | Why it matters here |
|---|---|---|
| `worktree add` | partial (v6 `x/` only) | needed for session create |
| `worktree remove/prune/lock/move/list` | not supported (docs Limitations) | cleanup, crash recovery (index.lock story in section 0) |
| `rev-parse` | no | 19 product call sites; must be reimplemented as library helpers (revision resolver `ResolveRevision` exists in plumbing but is not the CLI's full grammar; INFERRED) |
| `update-ref`, `write-tree`, `update-index`, `read-tree`, `commit-tree`, `diff-index`, `check-ignore` | no | plumbing used by fixtures and some product flows |
| `merge` / `pull` | fast-forward only ([#942](https://github.com/go-git/go-git/issues/942) open since 2022) | `MergeMainIntoWorktree` stays CLI |
| `rebase`, `revert`, `stash`, `apply`, `describe`, `gc`, `fsck`, `reflog`, `prune`, `bundle`, `archive` | no | any use blocks "no git binary" |
| `merge-base` | partial (two commits only; no `--fork-point`/`--octopus`) | |
| `sparse-checkout` | listed as supported, but [#2460](https://github.com/go-git/go-git/issues/2460) (open): switching branches leaves skip-worktree entries | |
| index format | v2 only; v1 and **v3** unsupported | v3 is what skip-worktree / intent-to-add entries use; a repo touched by a newer CLI can contain v3/v4 indexes (v4 decoder was the subject of CVE-2026-33762; treat v4 as partial, INFERRED) |
| pack protocol | v1 only; v2 not supported | large-repo ref advertisement is bigger than CLI's v2 (INFERRED perf impact) |
| `lfs` | no ([#381](https://github.com/go-git/go-git/issues/381) open, [#89 smudge/clean filters](https://github.com/go-git/go-git/issues/89) open) | LFS repos get raw pointer files; [#643](https://github.com/go-git/go-git/issues/643) (closed) showed Status fooled by LFS |
| hooks | not run (library; [#2185](https://github.com/go-git/go-git/issues/2185) was server-side hooks, closed) | `commit`/`checkout`/`push` via go-git skip pre-commit, post-checkout, pre-push hooks that a developer repo has (INFERRED from absence; confirm by test) |
| `.gitattributes` | parsed ([#1438](https://github.com/go-git/go-git/issues/1438) closed) but filters/eol/`autocrlf` not applied: [#436](https://github.com/go-git/go-git/issues/436) open (Status differs from git on newlines); #691 and #594 closed | false "dirty" on CRLF/attribute repos, matters for dirty checks and review gate |
| submodules | add/update ok; `deinit` no; [#2297](https://github.com/go-git/go-git/issues/2297) (open): `Status()` mutates disk by initialising uncloned submodules | a "read" call writing in a worktree is a hazard under concurrency |
| shallow | clone `--depth` ok; [#2409](https://github.com/go-git/go-git/issues/2409) (open) `Log()`/`MergeBase()` fail on shallow repos; [#2367](https://github.com/go-git/go-git/issues/2367) (open) false non-FF rejection on push with multiple shallow boundaries | |
| `repack` | `RepackObjects` exists; [#2425](https://github.com/go-git/go-git/issues/2425) (open) intermittent pack checksum mismatch; no `gc` | long-lived worktree-heavy repos accumulate loose objects; auto-gc never runs from go-git; INFERRED |
| GPG | verify only; signing of commits via `SignKey` exists in the library (INFERRED, not re-verified) but does not honour `commit.gpgsign`, `gpg.format=ssh`, or `gpg.program` config | CLI users with signed-commit policy get unsigned commits unless the fork implements config-driven signing |
| file modes / symlinks | worktree ops now boundary-checked after [#2276](https://github.com/go-git/go-git/pull/2276) and CVE-2026-71556; `core.fileMode` / `core.symlinks` handling not verified here | UNVERIFIED; needs a conformance test (exec bit flip, symlink checkout) |

### 2.2 Performance and memory (INFERRED unless cited)

- `Worktree.Status()` always does a full tree diff (VERIFIED, BUG-104 above); no fsmonitor, no untracked cache. For monorepos the CLI's status benefits from the index's mtime/stat caching and optional untracked cache; go-git rehashes or re-walks more (INFERRED).
- [#1956](https://github.com/go-git/go-git/issues/1956) (open, VERIFIED title and state; summary from search): creating a linked worktree decompresses every file from the object store; the issue proposes caching/parallel checkout/copying from an existing worktree. CLI `git worktree add` is also a full checkout but in C with parallel checkout available; expect worktree create to be slower than CLI on large trees. This is the single most likely regression against the "no slower than CLI" SLO and against "dozens of concurrent sessions".
- Docs recommend native SHA1 over sha1cd for speed (go-git docs, Advanced Usage: sha1cd "results in slower performance"). Swapping disables collision detection; a security/perf tradeoff the fork would have to decide explicitly.
- Memory: go-git loads objects through an LRU cache per storer; one `*Repository` per session x dozens of sessions x large packs multiplies resident memory, and a shared `*Repository` is unsafe (#773). Whether per-session storers or a shared cache is acceptable needs a measurement (UNVERIFIED).
- No index lock means two goroutines running `Status()`/`Add` on the same linked worktree can interleave; the per-repo mutex used today (`squad-ux-polish.md`) must be preserved and extended to every go-git write path (INFERRED).

### 2.3 Fixture and test-parity traps (from this repo's own history)

- Commits #955/#956-era fixtures moved HEAD reads and commits to go-git in tests (recent commits `b5af54fb8`, `85bd89b5c`). Remaining `git worktree add`/`checkout`/`clone` fixtures stay CLI because go-git output differs; after replacement, tests can pass against a go-git-created worktree while real `git` rejects it (e.g. unsupported `relativeWorktrees` extension: [#2324](https://github.com/go-git/go-git/issues/2324) closed via #2336 on 2026-10-01 means newer CLI-created worktrees can set that extension and older go-git failed to open them at all, VERIFIED titles/states). Keep a CLI-based cross-check job (tests where the real git validates go-git's output, `git worktree list`, `git fsck`) even if product code has no CLI.
- go-git-created worktrees must be readable by the CLI a user runs inside the worktree afterwards (`.git` file, `gitdir` back-pointer, `commondir`, HEAD symbolic ref); [#1580](https://github.com/go-git/go-git/issues/1580) (closed) and [#1843](https://github.com/go-git/go-git/issues/1843) (closed 2026-09-10) show these were wrong for linked worktrees recently, so regressions are plausible (INFERRED).

## 3. Credential and SSH parity

### 3.1 go-git's auth model vs `git credential`

- VERIFIED: go-git has no `credential.helper` support. [#250](https://github.com/go-git/go-git/issues/250), [#490](https://github.com/go-git/go-git/issues/490), [#1420](https://github.com/go-git/go-git/issues/1420) all closed; on #1420 the maintainer wrote "this is currently a feature that go-git does not support ... happy to review PRs"; stale-bot closed on 2025-09-03. Auth is supplied by the caller as `transport.AuthMethod` (basic/token/ssh). So parity means the fork (or this repo) must implement the `git credential fill/approve/reject` protocol: spawn `git-credential-<helper>` (that is still a subprocess, and `osxkeychain`, `manager`, `gh auth git-credential`, `libsecret` are separate binaries) or call keychain APIs directly. Hitting "zero subprocess" for credentials is not possible if helpers are third-party binaries; INFERRED requirement: a Go-native keychain path plus `gh` token reuse, with the helper binary as an opt-in fallback.
- `.gitconfig` is not fully honoured: `url.<base>.insteadOf` is not applied in clone ([#844](https://github.com/go-git/go-git/issues/844), open; related #830/#862/#863 closed); conditional `includeIf` fixed ([#388](https://github.com/go-git/go-git/issues/388) closed) but config loading scope is narrower than git's (INFERRED). Many GHE/work setups rely on `insteadOf` to rewrite HTTPS to SSH; this is exactly the user's HTTPS->SSH fallback shape.
- Credential-safety bugs in redirect handling are recent (CVE-2026-41506, [#2136](https://github.com/go-git/go-git/issues/2136) open). Do not rely on go-git's redirect-following for token-bearing requests to GHE; set `CheckRedirect` policy explicitly (INFERRED).
- Existing repo rule: native GitHub REST/GraphQL calls already go through `NewConditionalRequest` (`.claude/rules/norawghrequest.md`); a go-git fetch/push over HTTPS is a second HTTP client path that bypasses ETag/rate-limit plumbing and the `GhBaseURL()`/host routing (INFERRED). GHE hosts need explicit per-host token selection in the fork; there is no `gh` host-config awareness in go-git.

### 3.2 SSH

- ssh-agent: supported (`NewSSHAgentAuth`; docs trace shows agent socket, key discovery, known_hosts). VERIFIED from the go-git Troubleshooting doc.
- `~/.ssh/config` is not interpreted like OpenSSH: [#509](https://github.com/go-git/go-git/issues/509) (open, 2024) "Hostname from ssh config is handled incompatible to how git+openssh does it". `ProxyCommand`/`ProxyJump`/`IdentityFile`/`Match` blocks are not a go-git feature (INFERRED from the transport's use of `golang.org/x/crypto/ssh` and absence of a ProxyCommand search hit; search for it returned nothing, so absence is not conclusively verified). Bastion setups (this dotfiles repo ships `ssh-bastion-client`) and SSH-CA users would break silently.
- known_hosts: key-mismatch and CA handling bugs ([#1551](https://github.com/go-git/go-git/issues/1551) open, #1234 and #1417 closed). `@cert-authority` support was dropped from v5 then re-added ([#1417](https://github.com/go-git/go-git/issues/1417) closed): version-skew trap if the fork lags.
- CVE-2026-45570: go-git's SSH transport mis-escaped quotes in the repo path (fixed 5.19.1); the SSH exec path is security-sensitive, avoid carrying local divergence in it.
- Hardware keys: the go-git trace shows an `sk-ssh-ed25519@openssh.com` key found via agent; direct (non-agent) FIDO key use is not supported by x/crypto/ssh in that mode (INFERRED).

### 3.3 The local "ssh-fallback" wrapper (resolves an Open Question)

- VERIFIED: `~/.local/bin/git` is `git-ssh-fallback`, a POSIX-sh wrapper deployed by cfgcaddy from the dotfiles repo (`stapler-scripts/git-ssh-fallback`), shadowing `/usr/bin/git` via PATH, with a recursion guard (`GIT_SSH_FALLBACK_ACTIVE`). It is a developer-machine convenience in the dotfiles repo, not stapler-squad product behaviour; design docs live in dotfiles `project_plans/git-ssh-fallback/`. INFERRED consequence: the product only inherits the HTTPS->SSH behaviour when it spawns `git` through PATH. Replacing the CLI with go-git drops that behaviour unless re-implemented; deciding whether it is a product requirement is a user decision. Also, `type -a git` shows both `~/.local/bin/git` and `/usr/bin/git`, confirming the wrapper is what product subprocesses run on this machine (the cause of the spawn-cost baseline in requirements.md).

## 4. Summary of top risks (ordered)

1. Worktree add exists only in v6 beta `x/` with add-only and no prune/lock/move; the fork is therefore a v6 migration plus a build-out of worktree remove/prune/list and crash recovery.
2. go-git write paths are not atomic or lock-compatible with the CLI; mixed CLI/go-git operation (required by the rollback plan) on the same repo can corrupt refs/index, and go-git itself is not thread-safe (#773).
3. No credential-helper support; ssh_config/insteadOf/ProxyCommand parity is missing; "zero git subprocess" is incompatible with third-party credential helper binaries.
4. Perf: full-tree `Status()`, per-file decompression on worktree creation (#1956), index v2 only, protocol v1 only.
5. Fork security burden: ~12 advisories in 18 months in code the fork would modify; upstream supports only the latest minor.
6. Silent semantic differences: hooks not run, autocrlf/attributes/LFS filters not applied (#436, #89, #381), gc never runs, GPG config ignored.

## 5. Suggested gates for the plan (INFERRED, for the planner)

- Differential tests: run each operation through go-git and the CLI against the same fixture and compare ref/index/worktree-admin bytes, then run `git fsck` and `git worktree list` over go-git output.
- Concurrency soak: N sessions x worktree create/status/commit/remove against one repo, with a concurrent `git gc --auto`/`git repack` to reproduce #2242.
- Keep a repo-capability preflight (LFS present, `.gitattributes` filters, index version >= 3, `commit.gpgsign`, hooks present, submodules, shallow, sparse) that routes such repos to the CLI path even after rollout; this is the honest form of "per-operation fallback".
- Track `go-git/go-git` GHSA feed and release tags as a monthly rebase trigger; budget one rebase-plus-regression run per upstream minor.
