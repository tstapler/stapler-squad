# Research: Build vs. Buy — go-git-worktree-and-merge

Scope: evaluate whether an existing OSS library, a SaaS API, or an adapted
copy of go-git v6-alpha's experimental worktree code should replace some or
all of the from-scratch implementation requirements.md scopes, and assess the
correctness risk of an LLM-authored git-plumbing implementation specifically.
All source claims below were checked against the actual module source in
`~/.local/lib/go/pkg/mod/github.com/go-git/go-git/...` and the upstream
GitHub repo/releases, not docs or memory.

## 1. Existing OSS library

### go-git v5.19.2 — confirmed latest v5 release, no closer to production-ready
`go list -m -versions github.com/go-git/go-git/v5` (proxy.golang.org, checked
2026-09-06) shows `v5.19.2` as the newest v5 tag — requirements.md is current,
nothing shipped since. Verified directly against the vendored source
(`$GOMODCACHE/github.com/go-git/go-git/v5@v5.19.2`):
- **Worktree management**: `grep -n "func (r \*Repository)" repository.go | grep -i worktree` returns exactly one hit, [`Repository.Worktree()`](https://github.com/go-git/go-git/blob/v5.19.2/repository.go#L1528) — the single-working-copy handle every `go-git` repo already has, not a `git worktree add/list/remove`-equivalent. There is no multi-worktree API in v5 at all, and no `CHANGELOG` file in the module to suggest one is coming to this branch.
- **Merge**: [`repository.go:1800`](https://github.com/go-git/go-git/blob/v5.19.2/repository.go#L1800), `Repository.Merge()`, hard-rejects anything but fast-forward:
  ```go
  if opts.Strategy != FastForwardMerge {
      return ErrUnsupportedMergeStrategy
  }
  ```
  `options.go` declares exactly one `MergeStrategy` value (`FastForwardMerge`). Confirmed unchanged from requirements.md's claim.

### go-git v6.0.0-alpha.5 — confirmed latest v6 tag, merge still non-functional, worktree package more mature than requirements.md's phrasing suggests
`go list -m -versions github.com/go-git/go-git/v6` shows `alpha.1`–`alpha.5`,
no GA. Downloaded `v6.0.0-alpha.5` into a scratch module to inspect source
directly (not cached locally otherwise):
- **Merge**: `repository.go:2094`'s `Repository.Merge()` has the *identical*
  `if opts.Strategy != FastForwardMerge { return ErrUnsupportedMergeStrategy }`
  guard as v5. `options.go` does declare a new `OrtMergeStrategyOption` type
  ("Since Git v2.50.0, ORT is synonym of recursive") — but it only has two
  values, `TheirsMergeStrategy`/`OursMergeStrategy` (whole-file take-theirs or
  take-ours), and critically **`MergeStrategy` itself still only has one
  enum value, `FastForwardMerge`** — `OrtMergeStrategyOption` isn't wired into
  the strategy dispatch at all. This is scaffolding for a future whole-file
  auto-resolve option, not the content-level three-way merge (base/ours/theirs
  diff3, conflict markers) this project needs. Scanned all five alpha
  changelogs (`gh api repos/go-git/go-git/releases/tags/v6.0.0-alpha.{1..5}`)
  for any merge-related PR — none exists. **Confirms and sharpens**
  requirements.md's claim: not just "not yet functional," but not even on the
  visible roadmap as of alpha.5.
- **Worktree**: this is where the picture is better than requirements.md's
  one-line summary suggests. `x/plumbing/worktree` (`worktree.go`, 361 lines)
  implements `Add`/`Remove`/`List`/`Open`/`Init`, has a 1463-line test file
  including `TestWorktreeIsolation`, `TestWorktreeConfig`, and two fuzz
  targets (`FuzzAdd`, `FuzzOpen`), and — checked across all five alpha
  changelogs — has received steady hardening commits through alpha.5:
  `git: worktree, make the filesystem wrapper a symlink-safe boundary` (#2276),
  `git: worktree path hardening` (#2097), `git: worktree, fix Add silently
  failing for absolute paths` (#1949), `git: Worktree: Add stores index
  entires with backslashes on Windows` (#2253). This is not throwaway code;
  it is the same team's actively-maintained answer to exactly this project's
  Scope item #1. It still lives under `x/` (upstream's own experimental
  marker) and is pinned to `go-git/v6`'s alpha dependency graph (which pulls
  in `go-billy/v6-alpha.2`, `sha1cd`, `sergi/go-diff`, etc.) — see §4 for
  whether to adapt it directly.

### Third-party "go-git add-on" search — none found, narrower than the whole-library search requirements.md already did
Searched pkg.go.dev and GitHub for a module built specifically as a worktree-
or merge-add-on for `go-git` (not a whole new git implementation, not a CLI
wrapper):
- `github.com/ldez/go-git-cmd-wrapper/{worktree,merge}` — checked its GitHub
  description directly: "A simple wrapper around git command in Go." Shells
  out to the `git` binary, same category as `gogs/git-module` that
  requirements.md already rejected for the same reason.
- `gg-scm.io/pkg/git` — "A high-level interface for interacting with a Git
  subprocess in Go" (its own repo description) — also a subprocess wrapper,
  not `go-git`-native.
- `github.com/knwoop/giwo/pkg/worktree`, `github.com/overthinker1127/tui-worktree`
  — the former is an undocumented personal utility, the latter is explicitly
  "A terminal UI for reviewing AI-generated Git worktree changes," an
  application, not a library.
- No result in either search surfaced a library that extends `go-git`'s own
  object/tree/index primitives with worktree or merge logic. This closes the
  narrower version of the question requirements.md's "Other pure-Go git
  libraries" alternative already answered at the whole-library level.

**Verdict: Not recommended (as a full solution).** No OSS library — v5, v6-alpha,
or third-party — provides usable worktree management with real git on-disk
interop *and* real merge out of the box. v6-alpha's worktree package is the
one piece worth reusing rather than reimplementing; see §4.

## 2. SaaS / managed API

Not applicable. This project manipulates a local filesystem's `.git`
directory and worktree admin files on the same host as the caller — there is
no network boundary to place a managed service behind, and every constraint
in requirements.md (no cgo, on-disk interop with a locally-installed `git`,
50-90+ concurrent local worktrees) is a local-filesystem concern a remote API
cannot satisfy. No further analysis performed.

## 3. LLM-generated implementation vs. battle-tested library

This project is itself an LLM-agent-authored reimplementation of git
plumbing — three-way merge and worktree administrative-file handling are
exactly the domain (subtle, adversarially-tested-for-decades correctness
requirements) where "the AI wrote it and it compiled" is the weakest possible
signal. Assessed honestly, by risk category:

**Specific risks an LLM-authored implementation carries that a careful human
port from real git's C source would not:**
- **Silent scope-narrowing that looks complete.** An LLM implementing "three-way
  merge" from a description of the algorithm (rather than porting
  `merge-ort.c`/`xdiff` line-by-line) will very plausibly handle the common
  path (two-parent merge, textual conflicts) correctly while silently
  mishandling edge cases it wasn't prompted to consider: add/add conflicts,
  delete/modify conflicts, mode-only conflicts (100644 vs 100755 on the same
  blob), and binary files (real git refuses a content merge and reports a
  binary conflict; a naive diff3 implementation may instead try to
  byte-merge or hang). requirements.md's Rabbit Holes already flags rename
  detection as a similar case needing an explicit decision — the risk here is
  the same shape but broader: an LLM will not spontaneously enumerate every
  case real git's test suite (`t64*.sh` merge tests) exists to cover.
- **Format fidelity is verified by "it compiles and one test passes," not by
  adversarial review.** The on-disk interop constraint (Constraints section)
  is the highest-risk item in requirements.md for exactly this reason — a
  `.git/worktrees/<name>/HEAD` file with a trailing newline in the wrong
  place, or a `gitdir` file with a relative vs. absolute path git itself
  wouldn't produce, is invisible to `go build` and often invisible to a test
  that only checks "did my own code read it back correctly" (a self-
  consistent bug, not a caught one). This project's own prior investigation
  already found one instance of exactly this failure mode from go-git itself
  (see `session/git/util.go`'s `getHeadCommitSHA` doc comment: go-git's HEAD
  read returned a syntactically-valid SHA absent from `git cat-file`, `git
  rev-list --all`, `git reflog show --all`, and `git fsck --unreachable` —
  a self-consistent-but-wrong result a same-codebase test would not catch).
  An LLM-authored *writer* of the same format has the mirror-image risk.
- **Index conflict-stage encoding is a narrow, easy-to-get-subtly-wrong
  target.** Checked directly:
  [`plumbing/format/index/index.go`](https://github.com/go-git/go-git/blob/v5.19.2/plumbing/format/index/index.go#L28-L39)
  in go-git v5.19.2 defines `Stage` with `AncestorMode=1`, `OurMode=2`,
  `TheirMode=3`, matching real git's stage 1/2/3 index-conflict convention,
  and [`encoder.go:104`](https://github.com/go-git/go-git/blob/v5.19.2/plumbing/format/index/encoder.go#L104)
  packs `entry.Stage` into the flags field on write — so the **format-level**
  primitive this project's Open Question worried about does exist in v5. This
  answers requirements.md's first Open Question (index-writing API
  sufficiency) at the format level: **yes, stage 1/2/3 entries are
  representable and round-trip through the encoder/decoder.** What is *not*
  verified — and is exactly the kind of gap an LLM implementation could get
  wrong without noticing — is whether `git status`/`git diff` on a real
  checkout correctly renders a conflict written this way when the *content*
  written to the working-tree file doesn't also carry real git's exact
  conflict-marker byte format (`<<<<<<< `, `=======`, `>>>>>>> `, with the
  right label after each marker) — the index stage and the working-tree
  markers are two independent things that both have to be right, and
  getting one right doesn't imply the other is.

**What would meaningfully de-risk it — a two-layer verification strategy, not
just "add unit tests":**
1. **Differential testing against real git as the oracle** — for every
   representative merge scenario (fast-forward, clean 3-way, conflicting,
   binary-file conflict, add/add, delete/modify, mode-only), run the *same*
   base/ours/theirs commits through both real `git merge --no-edit` and this
   project's implementation in parallel, then diff: the resulting tree
   content, the index's stage entries (`git ls-files --stage` output format),
   and the conflict-marker byte content of any conflicted file. This is
   directly buildable on a pattern that already exists in this repo:
   `session/git/*_test.go` already shells out to real `git` inside tests to
   assert on-disk state (`worktree_ops_test.go`, `worktree_creation_test.go`,
   `diff_test.go` all use `safeexec.CommandContext(..., "git", ...)` for
   verification) — this project's interop tests (already required by
   requirements.md's Scope) are the natural home for a differential harness,
   not a new test category.
2. **Property-based / fuzz testing over merge inputs**, not just hand-picked
   scenarios — generate randomized base/ours/theirs trees (varying file
   count, overlapping vs. non-overlapping edits, deletions, mode changes) and
   assert the implementation either (a) matches real git's merge result
   exactly for cases git can auto-resolve, or (b) reports a conflict whenever
   and only when real git would. This repo already uses Go's native fuzzing
   (`func Fuzz...`) elsewhere — `server/services/rules_service_test.go`,
   `session/backlog_test.go`, `session/unfinished/gogitstore/mmap_adversarial_test.go`
   — so no new dependency (no `pgregory.net/rapid`/`gopter`) is needed; the
   stdlib `testing.F` corpus-based fuzzer is sufficient and matches existing
   repo convention. go-git v6-alpha's own `worktree_test.go` already ships
   `FuzzAdd`/`FuzzOpen` for the worktree side — the same style directly
   applies to worktree-admin-file generation (fuzz over worktree names, path
   depths, concurrent Add/Remove ordering) as a second target alongside the
   merge fuzzer.
3. **A staged conflict-marker byte-format golden test** — since
   requirements.md's Rabbit Holes explicitly calls out that byte-for-byte
   match matters (backlog automation shows real conflict markers to an LLM
   fix-agent), this needs its own fixed-fixture test independent of the
   differential harness: freeze a small set of real `git merge` conflict
   outputs as golden files, and assert this implementation's marker output is
   byte-identical, not just "looks like a conflict."

None of this changes the Build vs. Buy verdict (no adequate library exists to
buy), but it is the concrete answer to requirements.md's implicit ask under
Success Metrics ("behavioral parity with real git") — parity claims for this
specific domain are not credible without an oracle-based test, and this repo
already has both the pattern (subprocess-git-in-tests) and the tooling
(stdlib fuzzing) to build one without new dependencies.

## 4. Fork or adapt go-git v6-alpha's `x/plumbing/worktree`

### License compatibility — confirmed compatible, one direction
- go-git's `LICENSE` file (checked directly in the v5.19.2 module) is
  **Apache License, Version 2.0**.
- This repo's `LICENSE.md` is **GNU Affero General Public License v3
  (AGPLv3)**, not MIT as this research task's brief assumed — checked
  directly, not assumed.
- Per the FSF's license-compatibility list and the Apache Software
  Foundation's own statement, Apache-2.0 is one-way compatible with GPLv3
  (and by extension AGPLv3, which is GPLv3 plus an additional network-use
  section): Apache-2.0 code can be incorporated into a GPLv3/AGPLv3 work, and
  the combined work is then distributed under GPLv3/AGPLv3 terms — not the
  reverse. ([GNU license list](https://www.gnu.org/licenses/license-list.en.html), [Apache Software Foundation: Apache License v2.0 and GPL Compatibility](https://www.apache.org/licenses/GPL-compatibility.html))
- Practical requirement: Apache-2.0 §4 requires preserving the original
  copyright/license notice and stating what was changed. Vendoring a copy of
  `x/plumbing/worktree` into this repo needs a retained `LICENSE`/`NOTICE`
  attribution for the go-git project alongside the adapted file(s) (e.g. a
  `THIRD_PARTY_NOTICES` entry or a header comment), not a rewrite-from-scratch
  clean-room requirement — this is a low-cost condition, not a blocker.

**Verdict on license: no obstacle.**

### Is adapting it viable/preferable to writing from scratch?

**Pros of vendoring/adapting `x/plumbing/worktree`:**
- It already solves the two hardest structural problems: (1) the dual
  commondir/gitdir routing (`getDualFS`/`dotgit.NewRepositoryFilesystem`)
  that JGit's own worktree support needed a repository-model change to
  support (see `research/features.md`'s JGit findings) — this code already
  has that split modeled; and (2) admin-file generation
  (`addDotGitDirs`/`addDotGitFiles`/`addWorktreeDotGitFile`) that writes
  `commondir`, `gitdir`, `HEAD`, `ORIG_HEAD`, and a `refs/` dir per worktree —
  the exact file set requirements.md's Constraints section names.
- It has been hardened over 5 alpha releases against real bugs a from-scratch
  implementation would have to rediscover independently: absolute-path
  handling (#1949), symlink-boundary escapes (#2276), Windows path
  backslashes (#2253), and root-path `MkdirAll` edge cases (#2115). Starting
  from this code inherits those fixes; starting from zero does not.
- It ships its own test suite (`worktree_test.go`, 1463 lines) covering
  `TestAdd`/`TestRemove`/`TestList`/`TestOpen`/`TestInit`/
  `TestWorktreeIsolation`/`TestWorktreeConfig` plus `FuzzAdd`/`FuzzOpen` —
  adaptable as a starting differential-test corpus rather than needing to be
  authored from nothing.

**Cons / what adapting actually requires:**
- **Dependency surgery, not a copy-paste.** `x/plumbing/worktree` imports
  `github.com/go-git/go-git/v6` (the whole v6 package, not just `x/`),
  `go-billy/v6` (itself alpha: `v6.0.0-alpha.2`), and `x/storage`
  (`WorktreeStorer` interface, `dotgit.NewRepositoryFilesystem`). None of
  these types exist in this repo's pinned `go-git/v5` — the interfaces
  (`storage.Storer`, `billy.Filesystem`, `dotgit` layout helpers) changed
  between v5 and v6 (confirmed: `go get github.com/go-git/go-git/v6@v6.0.0-alpha.5`
  in a scratch module pulled in `go-billy/v6-alpha.2`, a different major
  version than this repo's `go-billy/v5@v5.9.0`). "Adapt" therefore means
  porting the logic onto this repo's v5 types (`storage.Storer`,
  `billy.Filesystem` v5, this repo's own `filesystem.Storage`/`dotgit`
  equivalents), not vendoring the file verbatim — closer to "use as a design
  reference and behavioral test oracle" than "drop-in replacement."
- **One correctness gap worth flagging even in the reference implementation
  itself**: `addDotGitFiles` (`worktree.go:324-341`) writes the per-worktree
  `HEAD` file as a raw commit hash (`opts.commit.String()`) unconditionally,
  before `Checkout()` runs. Real git's `HEAD` for a worktree checked out to a
  branch (the common case — `opt.Create/Branch` path, not `-d`/detached) is a
  symbolic ref (`ref: refs/heads/<branch>`), not a hash. The subsequent
  `work.Checkout(opt)` call likely corrects this for the branch case through
  go-git's normal checkout path, but this needs explicit differential-test
  coverage (§3's approach) rather than being assumed correct by inspection —
  it is exactly the kind of narrow, plausible-looking gap this section is
  warning about, present even in the more mature reference code.
- Still upstream-experimental (`x/` path) with no stability guarantee —
  consistent with requirements.md's Constraints already ruling out a direct
  dependency on it, which is why "adapt and own a local copy" (decoupling
  from upstream's ability to change/break it) rather than "depend on it" is
  the only option consistent with that constraint.

**Verdict: Recommended, scoped narrowly.** Use `x/plumbing/worktree`'s
source as the primary design reference and a behavioral/test oracle for the
worktree-management portion of this project (admin-file layout, dual-
filesystem routing, the specific bugs its alpha-cycle hardening already
fixed) — port the logic onto this repo's `go-git/v5` types rather than
vendoring verbatim, given the v5/v6 type incompatibility, and re-verify the
HEAD-write-ordering behavior explicitly rather than assuming it. This does
not extend to the merge portion of this project: v6-alpha has no merge
implementation to adapt (§1), so requirements.md's from-scratch three-way-
merge scope stands as planned, with the added verification strategy from §3.

## Summary Verdicts

| Option | Verdict |
|---|---|
| 1. Existing OSS library (go-git v5/v6, third-party add-ons) | Not recommended as a full solution — none exists; v6-alpha's worktree package is the one reusable piece (see §4) |
| 2. SaaS/managed API | Not applicable — local-filesystem operation, no network boundary to delegate to |
| 3. LLM-generated implementation, verification strategy | Proceed, but only with oracle-based differential testing + fuzzing against real git as a hard gate before ship — not optional hardening |
| 4. Fork/adapt go-git v6-alpha's `x/plumbing/worktree` | Recommended for the worktree-management portion only (as design reference + ported logic, not a vendored dependency); does not apply to the merge portion, which has nothing upstream to adapt |
