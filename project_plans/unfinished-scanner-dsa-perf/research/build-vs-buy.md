# Build vs. Buy: unfinished-scanner-dsa-perf

All options are go-git or pure Go (owner policy). Labels: VERIFIED = opened or ran it locally this session; INFERRED = reasoned from verified facts; UNVERIFIED = not checkable locally (no browsing done).

## Local facts used

- Root `go.mod`: `go-git/v5 v5.19.3` replaced by `tstapler/go-git/v5 v5.19.3-ssq.1` (line 231); `puzpuzpuz/xsync/v4 v4.5.0` (line 40); `sergi/go-diff v1.4.0` (line 42); `fsnotify v1.10.1` (line 28). VERIFIED.
- Fork `ssq/v5` HEAD is `98880159 ssq: add fork CI and README-SSQ (no source changes)` on top of upstream. VERIFIED (`git log`).
- `xsync` is used in `session/instance_status.go`, `session/review_queue_poller.go`, `server/services/*` but NOT in `session/unfinished`, which uses `sync.Map` for every cache (`gogit_vcs_reader.go` lines 495-601). VERIFIED (grep).
- No roaring-bitmap module and no `bitset` module sources in the module cache; only `bits-and-blooms/bitset` v1.24.4 `.info` in `cache/download` (no source, not in go.mod). `dgraph-io/ristretto/v2@v2.4.0` sources are cached but not in this go.mod. VERIFIED (ls/grep). Licenses of roaring/otter/theine: UNVERIFIED.
- The stapler-squad main checkout has `.git/objects/info/commit-graph` (535,292 bytes, mtime Oct 8; header `CGPH` version 1, hash version 1). VERIFIED (`ls`, `xxd`). It is stale by up to a week of commits, so commits newer than the file need a fallback to object decode. Other repos: UNVERIFIED (only this one checked).
- Fork commit-graph code: `plumbing/format/commitgraph` (v1 types), `plumbing/format/commitgraph/v2` (reader `OpenFileIndex`, `OpenChainOrFileIndex(fs)`, plus `Encoder` writer), and `plumbing/object/commitgraph` with `NewGraphCommitNodeIndex(index, storer)` and `NewObjectCommitNodeIndex(storer)` (both return `CommitNodeIndex` with `ParentHashes()`, `Generation()`, `GenerationV2()`). VERIFIED (grep/ls).
- Nothing in `storage/`, `repository.go`, `plumbing/object/merge_base.go`, or `commit_walker*.go` loads a commit-graph file; only `plumbing/object/commitgraph` references the package. So `Commit.MergeBase`, `Log` and `IsAncestor` decode full commit objects and do not use the graph. VERIFIED by grep (no match outside those packages); behaviour of `Log` options INFERRED from absence.
- Fork's `utils/diff` is a thin wrapper over `sergi/go-diff/diffmatchpatch` (line-mode Myers via `DiffMain`, default 1 h timeout). VERIFIED (`utils/diff/diff.go`, `plumbing/object/patch.go` imports it).
- Current shortstat uses a hand-written O(n·m) LCS DP (`lcsLength`, `lcsLengthBytes`, `gogit_vcs_reader.go:1828,1907`) capped by `maxLCSLines`, returning 0 / fully-replaced approximation above the cap. VERIFIED.
- `watcher.go` is only repo discovery (`WatchDirWatcher`); the actual per-repo `.git` fsnotify watch lives in `Scanner.watchRepo` in `scanner.go` (stated in watcher.go's doc comment). VERIFIED (read header).

## (1) Header-only commit-parents reader

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| A. Fork patch: add `object.DecodeParents(EncodedObject)` / `storer`-level helper that streams the object and stops at first blank line | Reuses go-git's object reader, pack/loose handling, delta resolution; one place; benefits any caller; small additive patch (INFERRED) | Adds fork rebase burden; needs tag + CI + `replace` bump per patch; still pays inflate + `MemoryObject` copy unless it reads from `Reader()` directly | **Recommended** if (2) cannot cover enough commits |
| B. In-package reader using existing public APIs: `storer.EncodedObject.Reader()` + `bufio` scan to first blank line, parse `parent ` lines | No fork change, fast to ship; parse is ~20 lines | Must go through `repo.Storer.EncodedObject(CommitObject, h)` which already buffers the whole object into `MemoryObject` (the 10.8% `MemoryObject.Write` and `bytes.growSlice` allocations) unless a `ObjectStorage` variant avoids it; so savings are only the message/gpgsig parse (`continuationCont` 9.2%, `Buffer.String` 8.0%, `bufio.ReadBytes` partially). INFERRED from the baseline alloc breakdown | **Viable**: cheap first step and the right fallback for commits absent from the graph |
| C. Use existing `object.GetCommit`/`DecodeCommit` | Zero code | This is the 51.9% allocation baseline | Not recommended |

Correctness risk: parsing commit headers is low-risk (format is `tree`, `parent`*, `author`, ... blank line) but the edge cases are real: `mergetag` and `gpgsig` multi-line continuation lines start with a space and may contain any text, so stop strictly at the first empty line, not on pattern matching; grafts/replace refs ignored (go-git ignores them too). Differential test against `DecodeCommit` over a corpus of commits is mandatory.

## (2) Commit-graph based merge-base / ahead-behind / reachability

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| A. Read git's own `commit-graph` with the fork's `commitgraph/v2` (`OpenChainOrFileIndex`) via `NewGraphCommitNodeIndex`, fall back to `NewObjectCommitNodeIndex` per commit | Already shipped in the fork, no patch; parents + generation numbers + commit dates with zero object decode; generation numbers let merge-base/reachability prune (stop when gen < min gen of targets); file exists in at least the main checkout (VERIFIED) | Format v1 files (`CGPH` version 1) are what v2 package reads (the "v2" name is about fixing incorrect types, not the file version) - INFERRED, must be confirmed by opening the real file in a test; file can be stale (Oct 8) so uncovered commits need object fallback; a `commit-graph-chain` split layout needs `OpenChainOrFileIndex`; file may be replaced by `git gc` while mmapped/open (reopen on mtime change); other repos unchecked; relies on someone else writing it (git maintenance) | **Recommended** as primary source for parents/generation, composed with (1) as fallback |
| B. Have the scanner write the graph with the fork's `v2.Encoder` | Pure Go; guarantees freshness | Writing is O(R) decodes once per repo per change (the cost we want to avoid), must match git's on-disk format exactly or git itself could reject it; touching the repo's `.git` from a read-only scanner is a side-effect risk | Not recommended |
| C. Build our own in-memory index (hash -> interned uint32, parents []uint32, gen) lazily as commits are first decoded, per repo | Needs no on-disk file; works for any repo; gives merge-base and ahead/behind with generation-style pruning, shared across worktrees (key: common git dir) | We own correctness of generation/merge-base algorithms; memory grows with R | **Recommended as the fallback layer** under (A); this is the (3) structure |
| D. Rely on go-git `Commit.MergeBase`/`Log` already using the graph | Zero work | Does not hold: no loader wired in storage or `MergeBase` (VERIFIED by grep) | Not an option |

Correctness risk of bespoke merge-base: the existing lockstep walk is already custom, and merge-base on a DAG with criss-cross merges has multiple best ancestors; go-git's `MergeBase` returns a set. Anything we write must be differentially tested against `Commit.MergeBase` (the oracle available in go-git) on randomized DAGs, and `generation` cutoffs are only sound when the number is trustworthy (gen 0 = not computed must disable pruning).

## (3) Compact reachable-set structure

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| A. Roaring bitmaps (`RoaringBitmap/roaring`) over interned uint32 commit ids | Compressed, fast set ops (and/andnot for ahead/behind) , widely used | New dependency; not in go.mod or module cache here (license/last release UNVERIFIED); at R <= 50,000 a dense bitset is 6.25 KB so compression buys nothing | Not recommended for R this size; revisit only if R grows by 100x |
| B. `bits-and-blooms/bitset` | Simple, tiny; only a `.info` for v1.24.4 is cached, not in go.mod (license UNVERIFIED) | New dependency for ~30 lines of functionality | Viable but unnecessary |
| C. Hand-rolled: intern hash -> uint32 once per repo (`map[Hash]uint32` + `[]Hash`), parents in a CSR-style `[]uint32` pair, reachable set as `[]uint64` bitset (R/64 words) | Zero deps; 50k commits = ~6 KB per set vs a `map[plumbing.Hash]bool` (~ MBs) ; cache hits stay allocation-free; trivial to reason about | We own the code; bitset ops are simple enough that the risk is low if fuzzed against the `map[Hash]bool` implementation | **Recommended** |

Custom code is reckless for: crypto/diff/graph algorithms with subtle invariants (merge-base, generation pruning) without an oracle. It is fine for a 64-bit-word bitset and an interning table, which are 30-50 lines each and can be fuzz-compared with the current `map[plumbing.Hash]bool`.

## (4) Diff algorithm for shortstat

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| A. `sergi/go-diff/diffmatchpatch` (already a direct dep; the fork's `utils/diff` wraps it) with `DiffLinesToRunes` + `DiffMainRunes` | Battle-tested (VERIFIED it backs go-git's patches); O(ND) Myers with bisect; zero new deps; insertion/deletion counts = sum of Insert/Delete lengths | Allocates rune/diff slices; the timeout (default 1 s in diffmatchpatch; fork sets 1 h) must be set explicitly; Myers is minimal-edit-script so counts equal LCS-derived counts (inserted = len(new) - LCS, deleted = len(old) - LCS) mathematically, but diffmatchpatch has a post-pass `DiffCleanupSemantic` only if called - do not call it, it changes counts; line-to-rune mapping caps at ~1.1M unique lines | **Recommended**: swap for `lcsLength` behind a parity test |
| B. Own Myers implementation (LLM-written) | No dependency; can be allocation-lean (reuse V arrays) | Classic off-by-one and forward/reverse overlap bugs; shortstat only needs the LCS *length* (D), so the simple greedy O(ND) forward-only variant is ~25 lines, but is exactly the kind of algorithm where plausible-looking code fails on rare inputs | **Not recommended** unless profiling shows A's allocations matter, then only with fuzz parity vs the current `lcsLength` DP (kept as test oracle) and vs A |
| C. Keep LCS DP | Known-correct | O(m·k), `maxLCSLines` cap returns an approximation for large files (VERIFIED line 1736) | Not recommended as the end state; keep as the parity oracle in tests |

Parity: for shortstat, additions = len(new) - LCS and deletions = len(old) - LCS (INFERRED from definition); Myers' D = additions + deletions, so counts match exactly whenever both algorithms are exact. Differences arise only (a) above `maxLCSLines` (old code approximates; new code would become exact - a deliberate behaviour change to call out), and (b) with diffmatchpatch's timeout truncation (set it large or check `time`), and (c) trailing-newline/`\r\n` line splitting which stays in our own splitter. Verify with a property test comparing both on random line slices.

## (5) Concurrent map / cache

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| A. Keep `sync.Map` + TTL (today) | No change; works | Unbounded (the requirement asks for a memory budget); no eviction counters; type-unsafe values; per-path keys | Not recommended for the new shared per-repo caches; fine for the small existing ones |
| B. `xsync/v4` `Map[K,V]` (already in go.mod, used in `session/`) | Generic, no new dep, lock-free reads, `Compute`/`LoadOrCompute` gives single-flight-style load; Apache-2 per its `LICENSE` file (VERIFIED, header of cached LICENSE) | No eviction or size budget of its own: you add budgeting yourself (LRU list or generation sweep) | **Recommended** for the key->entry table (per common git dir) |
| C. `ristretto/v2` (v2.4.0 sources in module cache, not in this go.mod) | Cost-based admission and eviction, bounded memory | Admission is probabilistic (TinyLFU): `Set` may silently drop, and `Wait` is needed for deterministic tests, which fights the `deterministic-fast-tests` rule; new direct dep | Viable only if byte-budgeted eviction is required |
| D. otter / theine | Deterministic-capable, W-TinyLFU, size-based eviction | Not in go.mod or cache; license/maintenance UNVERIFIED | Not recommended until verified |
| E. Small bespoke budget: `xsync.Map` + per-repo byte counter + oldest-first eviction of whole-repo entries | Whole-repo eviction matches how the data is used (a reachable set for a repo is useless in parts) and is deterministic to test | Own code, simple (counter + ordering), needs a lock around eviction | **Recommended** as the budgeting layer atop B |

## (6) Dirty-path tracking

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| A. Extend the existing fsnotify use in `Scanner.watchRepo` (`scanner.go`; `fsnotify v1.10.1`) to record changed paths per worktree and stat only those in `HasUncommitted` | Dep present; O(changed) not O(n); `.git` filter + debounce already landed (commits `c46c69faf`, `150168a4c`) | fsnotify is not recursive on Linux (needs a watch per dir, inotify limits) and can overflow or drop events; rename and editor temp-file patterns; the worktree working tree (not just `.git`) is not watched today as far as the doc comment says (INFERRED); correctness needs a safe fallback to the full walk on overflow, dir creation, or any `Error` event, plus a periodic full reconcile | **Recommended with strict fallback**: treat the dirty set as a hint that can only add work-skipping when the watcher has been continuously healthy since the last full walk |
| B. Index-stat-cache only (cheapest-exit ordering, already in place) | Safe | Still O(n) lstat | Keep as the fallback path |

## Overall recommended composition

1. (2A) read the commit-graph when present, (2C)+(3C) per-repo interned index and `[]uint64` reachable bitsets as the fallback and shared structure, (1B) in-package header reader first and (1A) fork patch only if profiling after that still shows decode cost.
2. (4A) go-diff behind a parity test, retaining LCS as oracle.
3. (5B)+(5E) xsync map plus whole-repo byte budget.
4. (6A) watcher-driven hint with mandatory full-walk fallback.

## Open items

- Confirm by test that `v2.OpenFileIndex` reads the real `CGPH` v1 file at `.git/objects/info/commit-graph` (not yet run; no builds this phase).
- Survey other repos for commit-graph presence (only stapler-squad checked) and staleness policy.
- Verify licenses/last-release of roaring, otter, theine from their repos if any is ever reconsidered (UNVERIFIED here).
