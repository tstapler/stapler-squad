# Research: Stack (go-git-worktree-and-merge)

All claims below are VERIFIED against source read directly in this session — module cache path
`/home/tstapler/.local/lib/go/pkg/mod/github.com/go-git/go-git/v5@v5.19.2` (this repo's exact
pinned version, confirmed via `go.mod:29`), plus `git/git` and `go-git/go-git` (v6) fetched from
GitHub via `gh api`. No claim here is inferred from memory of go-git's API.

## 1. Versions to pin

| Module | Current (`go.mod`) | Recommendation |
|---|---|---|
| `github.com/go-git/go-git/v5` | `v5.19.2` | **Keep.** Confirmed latest v5 tag (published 2026-07-29, `gh api repos/go-git/go-git/releases/tags/v5.19.2`); next tag after it is `v6.0.0-alpha.1`. No newer v5 patch exists to pick up. |
| `github.com/go-git/go-billy/v5` | `v5.9.0` | Minor bump available: `v5.9.1` is the latest v5 tag (`gh api repos/go-git/go-billy/tags`). Not required for this project, but a trivial, low-risk bump worth doing alongside it. |
| `github.com/sergi/go-diff` | `v1.3.2-0.20230802210424-...` (transitive, already in `go.sum`) | **Reuse, don't add.** This is go-git's own line-diff engine (`go-git/v5@v5.19.2/utils/diff/diff.go` wraps `diffmatchpatch.DiffLinesToRunes`+`DiffMainRunes`). It's already a resolved transitive dependency — the merge implementation can import it directly with zero new `go.mod` entries. |
| `github.com/go-git/go-git/v6` (incl. `x/plumbing/worktree`) | n/a | **Do not depend on.** Per Constraints — alpha (`v6.0.0-alpha.5` is the latest tag), `x/` packages are explicitly experimental. Used below only as a design reference, fetched read-only via `gh api`. |
| `github.com/epiclabs-io/diff3` | n/a | **Do not depend on; reference only** (see §4). MIT-licensed, actively maintained (last push 2026-05-20 per `gh api repos/epiclabs-io/diff3`), but its conflict-marker format is not byte-for-byte git-compatible out of the box (9-char markers, no `|||||||` section) — see §4.3. Not worth a dependency for an incompatible format; worth 20 minutes reading `diff3.go`'s hunk-classification algorithm before writing our own. |

## 2. Worktree lifecycle: go-git v5.19.2 primitives + git's own on-disk format

### 2.1 What v5.19.2 already gives you (verified against source)

- **`git.PlainOpenWithOptions(path, &git.PlainOpenOptions{EnableDotGitCommonDir: true})`
  already correctly opens a linked worktree.** `dotGitToOSFilesystems` (`repository.go:341-399`)
  detects a `.git` *file* (vs. directory) and follows its `gitdir: <path>` redirect
  (`dotGitFileToOSFilesystem`, `repository.go:401-426`); when `EnableDotGitCommonDir` is set, it
  also reads `.git/worktrees/<name>/commondir` (`dotGitCommonDirectory`, `repository.go:428+`) and
  wraps both filesystems in `storage/filesystem/dotgit.RepositoryFilesystem`
  (`storage/filesystem/dotgit/repository_filesystem.go:19-51`), which routes `objects/`, `refs/`,
  `config`, `hooks/`, `info/`, `logs/` (except `logs/HEAD`), `packed-refs`, and `worktrees/` itself
  to the **common** dir, and everything else (notably `HEAD` and `index`) to the **per-worktree**
  dir — this is the exact split `git-scm.com`'s `gitrepository-layout` documents (see §2.2).
  **`EnableDotGitCommonDir` is opt-in, not automatic, in v5.19.2** — every call site that opens a
  worktree path must set it, or object/ref resolution silently falls back to per-worktree-only
  paths. This repo already hit this exact bug independently: `session/git/util.go`'s
  `getHeadCommitSHA` doc comment (as of the 2026-09-02 fix referenced in commit
  [8496f8573](https://github.com/tstapler/stapler-squad/blob/8496f85733645e7f7bdace119d45b40f0b9b38de/session/git/util.go#L354-L361))
  records that *without* the flag, go-git returned "a real-but-wrong commit object for a linked
  worktree... a stale SHA from before the worktree was created" — not an error, a silently wrong
  answer. New code in this project must set this flag on every `PlainOpenWithOptions` call against
  a worktree path; there is no safe default.
- **`Repository.Worktree()` + `Worktree.Checkout(&git.CheckoutOptions{Hash, Branch, Create: true})`
  already exists and works** (`options.go:377-396`) — this is the same API go-git v6's reference
  worktree package uses (see §2.3) to populate a brand-new worktree's files *and* write a correct
  stage-0 index, entry by entry, from a tree. **This means the "clean add" path does not need a
  hand-rolled index writer at all** — only the merge-conflict path (§3) does, because `Checkout`
  has no concept of multi-stage conflict entries.
- **`Commit.MergeBase(other *Commit) ([]*Commit, error)`** (`plumbing/object/merge_base.go:17`) is
  a complete, real best-common-ancestor implementation ("mimics `git merge-base`", handles multiple
  candidate bases via `Independents`) — usable as-is for finding the 3-way merge base.

### 2.2 Real git's on-disk worktree format (from `git/git` source and docs directly, not go-git)

Fetched `Documentation/git-worktree.adoc` and `Documentation/gitrepository-layout.adoc`, plus the
library implementation `worktree.c` and `builtin/worktree.c`, from `git/git` at
`master`@[`3cb9185`](https://github.com/git/git/blob/3cb9185f65410273787f74333cc027d2ea5daada/worktree.c).

**Per linked worktree, git creates `$GIT_DIR/worktrees/<id>/`** containing:

| File | Content | Notes |
|---|---|---|
| `gitdir` | Absolute path to the linked worktree's `.git` **file** (e.g. `/path/other/test-next/.git`) | ["the mtime of this file should be updated every time the linked repository is accessed"](https://github.com/git/git/blob/3cb9185f65410273787f74333cc027d2ea5daada/Documentation/gitrepository-layout.adoc#L285-L290) — used by `git worktree prune`/`repair` to detect a manually-deleted worktree. |
| `commondir` | `../..` (relative, from `<id>/` back to the main `$GIT_DIR`) | Written verbatim in [`add_worktree`](https://github.com/git/git/blob/3cb9185f65410273787f74333cc027d2ea5daada/builtin/worktree.c#L544-L546): `write_file(sb.buf, "../..")`. |
| `HEAD` | Detached SHA, or `ref: refs/heads/<branch>` | Written through the **ref-store API** (`refs_update_ref`/`refs_update_symref`, [`builtin/worktree.c:563-569`](https://github.com/git/git/blob/3cb9185f65410273787f74333cc027d2ea5daada/builtin/worktree.c#L563-L569)), i.e. via git's normal atomic lock-file-then-rename ref update — not a raw `fwrite`. This is the exact mechanism whose *absence* is what `session/git/util.go`'s `getHeadCommitSHA` doc comment calls the "torn-read race" — real git never has this bug because it never writes `HEAD` non-atomically. New Go code writing `HEAD` directly (rather than shelling to git) must replicate this: write to a temp file in the same directory, `fsync`, then atomic `rename(2)` over `HEAD`. |
| `index` | Normal git index format (see §3) | Per-worktree, not shared — confirmed by the commondir-exception table (§2.1) and independently by the "prune" staleness check using this file's mtime (`should_prune_worktree`, [`worktree.c:1017-1025`](https://github.com/git/git/blob/3cb9185f65410273787f74333cc027d2ea5daada/worktree.c#L1017-L1025)). |
| `locked` | Free-text reason string, absent = unlocked | See below — also used transiently as a crash-safety marker during `add`, independent of the user-facing `git worktree lock` feature. |
| `ORIG_HEAD` (not always present) | Same format as `HEAD` | Written by v6's reference impl (§2.3) on `Add`; not written by real git's `add_worktree` itself (that's a checkout-time convention, not a worktree-admin-format requirement) — **do not assume this file is required for git/gh CLI interop**, it's optional. |

**The top of the linked worktree** gets a `.git` **file** (not directory) containing
`gitdir: <absolute path to $GIT_DIR/worktrees/<id>>` — written by
[`write_worktree_linking_files`](https://github.com/git/git/blob/3cb9185f65410273787f74333cc027d2ea5daada/worktree.c#L1109-L1143).
**By default this is an absolute path in both directions** (worktree → gitdir, and gitdir →
worktree); a relative-path mode exists but requires opting in to the
`extensions.relativeWorktrees` repository-format extension first — not worth adopting here (no
portability requirement in scope, and it complicates every consumer that has to resolve relative
paths against two different roots).

**Concurrency/atomicity, the actual mechanism** (`builtin/worktree.c:507-514`, `add_worktree`):
git protects against two concurrent `git worktree add` calls picking the same admin-dir name by
looping `mkdir(sb_repo.buf, 0777)` (a single atomic POSIX syscall) and, on `EEXIST`, appending an
incrementing numeric suffix to the name and retrying — it does **not** use a separate lock file for
this part. **This is the concrete, directly-portable pattern for this project's own concurrency
requirement** (Feasibility Risks: "real git worktree add has its own internal locking assumptions
this custom implementation must replicate or it risks racing itself"): use `os.Mkdir` (not
`MkdirAll`, which is not atomic and silently succeeds on an existing directory) on the
`.git/worktrees/<name>/` directory itself as the mutual-exclusion primitive, retry-with-suffix on
`EEXIST`, exactly as git does.

**Crash-safety during add** (`builtin/worktree.c:524-533,601-606`): git writes `<id>/locked` with
the content `"initializing"` (or the user's `--lock` reason) **before** creating any other admin
file, so a `git worktree prune` running concurrently, or a later `git worktree list`, sees the
half-built worktree as locked/non-prunable rather than corrupt-looking; the `locked` file is
removed at the very end on success (`unlink_or_warn`) unless the caller asked to keep it locked.
This project's `Add` implementation should adopt the identical staging pattern: write `locked`
first, do all other admin-file + checkout work, remove `locked` last — this directly closes the
"real git worktree add has its own internal locking assumptions" feasibility risk for the *crash-mid-add* case
(distinct from the *concurrent-add* case above, which `mkdir` handles).

**Pruning** (`worktree.c:941-1033`, `should_prune_worktree`): a worktree is prunable if (a) its
admin dir isn't a directory, unless (b) a `locked` file is present (never prunable then), or (c)
the `gitdir` file is missing/unreadable, or (d) the `.git` file it points at no longer exists **and**
either there's no `index` file or the `index` file's mtime is older than the expiry window
(`gc.worktreePruneExpire`, default 3 months). Per this project's own Rabbit Holes section, full
parity with this staleness heuristic is explicitly out of scope — this repo's session lifecycle
already tracks worktree existence in its own DB (`session/storage.go`) and doesn't need a
device-unplugged grace period. Recommend implementing only the simple case: prunable ⟺ `.git`
target path doesn't exist and no `locked` file — defer the mtime/expiry nuance to a follow-up if a
real need shows up.

### 2.3 go-git v6's `x/plumbing/worktree` (design reference only — NOT a dependency)

Fetched from `go-git/go-git` at tag `v6.0.0-alpha.5`
([`fc18716`](https://github.com/go-git/go-git/tree/fc18716c90bcd8e8c935742431e26e260ab7ef60/x/plumbing/worktree)):
`worktree.go` (361 lines), `worktree_options.go`, `x/storage/worktree_storer.go`.

What it does well, worth copying the *approach* (not the code — different major version, alpha,
different go-billy major):
- `Add` composes existing go-git primitives rather than hand-writing an index: it writes the four
  admin files (`gitdir`, `commondir`, `HEAD`, `ORIG_HEAD`) directly with `billy.Filesystem.Create`,
  writes the linked-worktree's `.git` redirect file, then opens the new repo via its own `Open`
  (which builds the same `dotgit.RepositoryFilesystem` commondir-wrapper as v5's
  `EnableDotGitCommonDir`, unconditionally — see §2.4) and calls
  `r.Worktree().Checkout(&git.CheckoutOptions{Hash: commit, Branch: ..., Create: !detached})` to do
  the actual file/index population — confirming the "reuse `Checkout`, don't hand-roll the index"
  approach recommended in §2.1 is sound and has prior art.
- `Remove` is metadata-only (`util.RemoveAll` on `.git/worktrees/<id>/`) and explicitly documents
  that it does not touch the actual worktree directory — matches real git's `--force`-gated
  behavior in spirit (this project can decide in planning whether to also remove the working
  directory; real git's default `remove` does, but only if the tree is clean).
- `List` just `ReadDir`s `worktrees/` and returns directory names — no cross-checking against
  `gitdir`/staleness at all (even simpler than the "just check `.git` target exists" recommendation
  in §2.2 — this project should do at least that much, since `worktree_ops.go`'s current callers
  likely depend on `list` not returning entries for worktrees git itself would consider gone).

What it does that **this project should deliberately not copy**:
- **Not concurrency-safe.** `Add` does `commonDir.Lstat(path)` to check `ErrWorktreeAlreadyExists`,
  then later `wt.MkdirAll(...)` — a check-then-act race (TOCTOU) between two concurrent `Add` calls
  for the same name, and `MkdirAll` doesn't fail on a pre-existing directory the way a bare `Mkdir`
  does. Real git's `mkdir`-with-EEXIST-retry loop (§2.2) is the correct pattern; this project's own
  Feasibility Risks section already flags exactly this gap, so diverge from v6 here, not from git.
- **No crash-safety staging.** No `locked`-file-first pattern — if the process dies mid-`Add`, the
  admin dir is left in a state indistinguishable from a valid worktree to `List`. Use the
  `locked`-first/`locked`-last pattern from real git instead (§2.2).
- No `lock`/`unlock`/prune support at all (consistent with this project's own explicit
  descope of that surface).

### 2.4 v6's unconditional commondir handling — a forward-compat note, not actionable now

`gh api search/commits` on `go-git/go-git` surfaced the commit that made this behavior change:
[`27d23a5`](https://github.com/go-git/go-git/commit/27d23a51f8e53e5e630c93687b39026615df6153)
("git: remove `PlainOpenOptions.EnableDotGitCommonDir`" — commondir detection becomes
unconditional) is an ancestor of `v6.0.0-alpha.5` (confirmed via
`gh api repos/go-git/go-git/compare/v6.0.0-alpha.5...27d23a5` → `"status": "behind"`) but
**diverged from, not reachable from, `v5.19.2`** (`compare/v5.19.2...27d23a5` → `"status":
"diverged"`) — i.e. it's a v6-only, not-yet-released-on-v5 change. No action needed now (v5.19.2
still requires the explicit flag, and this repo's code already sets it correctly per §2.1), but
worth knowing: if/when this project later migrates to a GA go-git v6 (per the requirements'
explicit "should not preclude migrating later" design goal), the `EnableDotGitCommonDir` call sites
this project adds become unnecessary (harmless no-op flag removal), not broken.

## 3. Writing a real conflicted index (stage 1/2/3) — go-git v5.19.2 API sufficiency

**Answered definitively: yes, the public API supports it**, read directly from
`plumbing/format/index/index.go`, `encoder.go`, `decoder.go` in the pinned `v5.19.2` module:

- `index.Entry` has a first-class `Stage index.Stage` field
  (`index.go:145-147`), with named constants `index.Merged`/`AncestorMode = 1`,
  `index.OurMode = 2`, `index.TheirMode = 3` (`index.go:31-39`) — these are literally git's stage
  numbers (base=1, ours=2, theirs=3), not an abstraction over them.
- **The encoder round-trips it for real**: `encodeEntry` packs `entry.Stage` into bits 12-13 of the
  16-bit flags field (`encoder.go:104`: `flags := uint16(entry.Stage&0x3) << 12`) — the exact bit
  layout the real git index format specifies for the stage field. The decoder reads it back
  identically (`decoder.go:132`: `e.Stage = Stage(flags>>12) & 0x3`). This is a genuine,
  functioning read/write path, not a stub — confirmed by reading both directions, not assumed from
  one.
- A conflicted path is represented as **multiple `Entry` values sharing the same `Name`**, one per
  stage present (a pure add/delete conflict may have only 2 of the 3 stages; a normal
  content-conflict has all 3) — `index.go:125-127`'s doc comment says this explicitly: "An entry
  represents exactly one stage of a file. If a file path is unmerged then multiple Entry instances
  may appear for the same path name."

**One real gap, found by reading the encoder, not assumed**: `encodeEntries`
(`encoder.go:72-73`) sorts entries with `sort.Sort(byName(idx.Entries))`, and `byName.Less`
(`encoder.go:246-250`) compares **only `Name`**, not `(Name, Stage)`. Two problems: (1)
`sort.Sort` is not a stable sort, so among entries sharing a `Name` (exactly the conflicted-file
case), output order is not guaranteed to match insertion order; (2) even a stable sort wouldn't
help, since nothing sorts by `Stage` as a tiebreaker at all. The real git index format requires
entries sorted by `(name, stage)` ascending — **this project's index-writing code must pre-sort
`idx.Entries` by `(Name, Stage)` itself before calling `index.NewEncoder(...).Encode(idx)`**, since
go-git's encoder will not do it correctly for a conflicted index. This is a concrete, testable
requirement for the merge implementation, not a design nicety — get it wrong and `git status`/`git
diff` on the resulting worktree will very likely misbehave or bail with a corrupt-index complaint,
which is exactly the interop failure mode this project's Success Metrics are guarding against.

**A second gap, found by grepping, not assumed**: go-git's own `Worktree.Status()` and
`Worktree.Checkout()` (`worktree_status.go`, `worktree.go` in v5.19.2) never reference `.Stage` at
all (`grep -rn "Stage\b" *.go` in the repo root returns nothing outside test files) — they iterate
`index.Entries` assuming one entry per path. **Do not call go-git's own `Worktree.Status()` or any
other go-git worktree API against an index this project has written with multi-stage entries** —
it will very likely double/triple-count the conflicted path or otherwise misbehave, since that code
path was never written with stage-aware entries in mind. This is fine for this project's actual
requirement (real `git status`/`git diff` — the *CLI* — must recognize the conflict; go-git's own
`Worktree` type consuming its own output was never a requirement), but it's a landmine worth
flagging explicitly so nobody reaches for `Worktree.Status()` to sanity-check the merge
implementation's own output and gets confused by a mismatch.

**Storage write path** (confirms Entry.Add for a fresh 0644, but for a conflicted write you
construct `index.Entry` values directly and append to `idx.Entries`, then call
`storage/filesystem.Storage.SetIndex`, which is a two-line wrapper —
`storage/filesystem/index.go:14-31` — around `dir.IndexWriter()` (writes to the worktree's private
`index` file via the same `dotgit`/commondir-aware filesystem from §2.1) and
`index.NewEncoder(bw).Encode(idx)`. No lower-level index-file API is needed; the gap is purely the
sort-order bug above, which the caller must work around, not a missing capability.

## 4. Three-way merge: what to build on

### 4.1 Base-finding and tree diffing (go-git primitives, verified)

- `(*object.Commit).MergeBase(other *Commit) ([]*Commit, error)` — real best-common-ancestor
  algorithm (`plumbing/object/merge_base.go:17-41`), handles the possibility of multiple candidate
  merge bases via `Independents` (mirrors `git merge-base --independent`). For this project's
  actual call pattern (two parents, `origin/<main>` into a session branch — Out of Scope
  explicitly excludes octopus merges), there will be exactly one merge base in the overwhelming
  majority of cases; the multi-base return type still needs a documented decision in planning (pick
  the first / error / fall back to subprocess) for the rare criss-cross-merge case.
- `(*object.Tree).Diff(to *Tree) (Changes, error)` (`plumbing/object/tree.go:421`) — per-path
  `Change` records (insert/delete/modify, both `filemode.FileMode`s, both blob hashes) between two
  trees. Calling this twice — `base.Diff(ours)` and `base.Diff(theirs)` — gives the two change-sets
  a diff3-style merge algorithm needs as input, without hand-rolling tree-walking.
- File content access: `object.Blob` (returned via `Change.Files()` →
  `plumbing/object/change.go:44`) exposes a `Reader()`; combine with
  `plumbing/filemode.FileMode` (`plumbing/filemode/filemode.go`) for mode comparison — a
  mode-only conflict (e.g. executable-bit changed on both sides to different values) is a distinct
  case from a content conflict and must be classified separately per this project's own Rabbit
  Holes note on scoping merge behavior deliberately rather than discovering gaps mid-implementation.

### 4.2 Line-level diff engine (already a transitive dependency, verified)

`go-git/v5@v5.19.2/utils/diff/diff.go` wraps `github.com/sergi/go-diff/diffmatchpatch`
(`DiffLinesToRunes` + `DiffMainRunes`, i.e. line-oriented Myers diff) — **already resolved in this
repo's `go.sum`** as a transitive dependency of `go-git` itself, so importing
`github.com/sergi/go-diff/diffmatchpatch` (or go-git's own `utils/diff` convenience wrapper)
directly adds no new `go.mod` line. This is the correct building block for the per-file line diff
step of a 3-way merge, but **it is a 2-way diff engine, not a merge algorithm** — it does not by
itself decide how to reconcile two independent diffs against a shared base into merged output +
conflict regions. That reconciliation (the actual "diff3" algorithm: classify each base line range
as unchanged/changed-by-ours-only/changed-by-theirs-only/changed-by-both, and only the last is a
true conflict) is the part this project must implement.

### 4.3 Conflict-marker format — exact, byte-for-byte, from git's own source

Fetched `xdiff/xmerge.c` from `git/git`@`master`
([`3cb9185`](https://github.com/git/git/blob/3cb9185f65410273787f74333cc027d2ea5daada/xdiff/xmerge.c)):

- `DEFAULT_CONFLICT_MARKER_SIZE` is **7** (`xdiff/xdiff.h:144`) — markers are exactly
  `<<<<<<<`, `=======`, `>>>>>>>`, and (diff3/zdiff3 style only) `|||||||`, each optionally followed
  by a space and a label (branch name / ref), confirmed by `fill_conflict_hunk`
  (`xmerge.c:196-279`): `memset(dest + size, '<', marker_size)` then, if a label string is
  non-empty, `' '` + the label.
- **Default `merge.conflictStyle` is `"merge"`** (`Documentation/config/merge.adoc`, fetched
  directly) — i.e. **no `|||||||` ancestor section by default**; that only appears with
  `diff3`/`zdiff3` styles. Since this project's stated need is byte-for-byte match with what
  backlog automation's fix-agent prompts show an LLM today (Rabbit Holes: "needed for interop...
  not just 'some' conflict representation"), confirm in planning which style the current subprocess
  `git merge` invocation actually produces (i.e. whatever this developer machine's/CI's global
  `merge.conflictStyle` is set to, which defaults to `"merge"` if unset) and match *that*, not
  necessarily the diff3 style.
- Order per conflict hunk: `<<<<<<< <ours-label>` → ours content → (if diff3 style) `||||||| <base-label>` → base content → `=======` → theirs content → `>>>>>>> <theirs-label>`.

### 4.4 `epiclabs-io/diff3` — read for algorithm shape, not for its output format

Fetched `diff3.go` (662 lines) from `epiclabs-io/diff3`@`master`
([`3b16698`](https://github.com/epiclabs-io/diff3/blob/3b1669897fb1aa7c1fb2699a3c6a45bbb46e9ec1/diff3.go)),
MIT-licensed, pure Go (own Myers-diff implementation in `myersdiff.go`, not `sergi/go-diff`), last
pushed 2026-05-20.

- Its core (`Diff3Merge[T comparable]`, line 523) does the real diff3 hunk-classification algorithm
  generically over `[]T` (works for `[]string` lines) — this is the part worth reading closely as a
  design reference for this project's own merge core, since it's a working, tested implementation
  of exactly the algorithm this project needs to build (compute two diffs against a shared base,
  walk them together, and mark each region agree/ours-only/theirs-only/conflict).
- **Its conflict-marker output does not match real git**: `addConflictMarkers` (line 598) emits
  `<<<<<<<<<` (9 characters) and `=========` (9 characters), and its default `Merge()` (line 608)
  does not include a `|||||||` base section at all even though it computes one internally
  (`item.Conflict.A`/`.B` are diffed further at line 642, not rendered with a base marker). **Do
  not vendor this file verbatim and expect git-compatible output** — either fork the marker-writing
  function to use 7-character markers matching §4.3, or (given it's ~140 lines once you strip the
  marker/rendering layer) treat it purely as an algorithm reference and write this project's own
  hunk-to-marker rendering against the tree/blob primitives from §4.1, which is what "built from
  go-git's tree/object/diff primitives" in the requirements' Scope section calls for anyway.

## 5. Summary of concrete, actionable findings for the planning phase

1. **Worktree add (clean path)**: no custom index writer needed — write the four admin files per
   §2.2's table (in the `locked`-first, `locked`-last crash-safe order), redirect-`.git` file, then
   `PlainOpenWithOptions{EnableDotGitCommonDir:true}` + `Worktree.Checkout(&CheckoutOptions{Hash,
   Branch, Create:true})` does the rest via existing go-git machinery.
2. **Worktree add concurrency**: use `os.Mkdir` (atomic, not `MkdirAll`) on
   `.git/worktrees/<name>/` with EEXIST-triggered numeric-suffix retry, matching real git's
   `add_worktree` exactly — diverge from go-git v6's reference implementation here, which has a
   TOCTOU race.
3. **HEAD writes**: always via write-temp-then-atomic-rename in the same directory, never a raw
   `fwrite` — matches real git's ref-store-API guarantee and directly extends this repo's own
   already-documented `getHeadCommitSHA` torn-read hardening rather than reintroducing that class
   of bug from the other direction (writing instead of reading).
4. **Every `PlainOpenWithOptions` call this project adds must set `EnableDotGitCommonDir: true`** —
   there is no safe default in the pinned v5.19.2, and this repo has already been burned by this
   exact gap once (`session/git/util.go`'s 2026-09-02 fix).
5. **Conflicted-index writing**: `index.Entry.Stage` + `index.OurMode`/`TheirMode`/`AncestorMode`
   round-trip correctly through go-git's encoder/decoder — but the caller must sort
   `idx.Entries` by `(Name, Stage)` before encoding, since go-git's own `byName` sort ignores
   `Stage` and isn't even a stable sort. Do not sanity-check the written index via go-git's own
   `Worktree.Status()` — that code path ignores `Stage` entirely and isn't safe to call against a
   multi-stage index; verify via a real `git status`/`git diff` subprocess call in tests instead
   (which the requirements already call for: "interop tests that shell out to real `git`").
6. **Three-way merge**: `Commit.MergeBase` + two `Tree.Diff` calls (base→ours, base→theirs) give
   the raw change-sets; `sergi/go-diff/diffmatchpatch` (already resolved, zero new dependency)
   gives line-level diffing; the actual diff3 hunk-reconciliation algorithm must be hand-written
   (`epiclabs-io/diff3`'s `Diff3Merge` is a good, MIT-licensed algorithm reference — do not reuse
   its marker-rendering code, which is not git-compatible). Conflict markers: 7-character
   `<<<<<<<`/`|||||||`/`=======`/`>>>>>>>`, `|||||||` section only if replicating
   `merge.conflictStyle=diff3`/`zdiff3` — confirm which style this repo's environment actually
   produces today before deciding whether to include it.
