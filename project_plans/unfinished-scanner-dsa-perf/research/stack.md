# Research: Stack (go-git internals, fork patch points, Go libraries)

Fork = `~/code/github.com/tstapler/go-git` (F), branch `ssq/v5`. Scanner = `session/unfinished` (S).
VERIFIED = source opened or command run; INFERRED = reasoned, not run.

## 0. Corrections to the brief
- VERIFIED: the fork is not at zero patches in the consumed form. Root `go.mod:231` is `replace github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 v5.19.3-ssq.1`; tags `v5.19.2-ssq.0`, `v5.19.3-ssq.1` exist. The only commit on top of upstream is `98880159 ssq: add fork CI and README-SSQ (no source changes)` (`git log upstream/main..ssq/v5`). So zero *source* patches, but the next tag is `-ssq.2`.
- VERIFIED: `go.mod:3` is `go 1.26.6`; local toolchain `go1.26.6 darwin/arm64`.

## 1. Commit decode path

VERIFIED (F = fork paths):
- `plumbing/object/commit.go:135` `DecodeCommit(s, o)` allocates `&Commit{s: s}` then `c.Decode(o)`.
- `commit.go:253-283` `(*Commit).Decode`: `c.reset()`; `o.Reader()`; bufio reader from `utils/sync.GetBufioReader` (`utils/sync/bufio.go:20`, sync.Pool, so bufio itself is already pooled); then `commitScanner` state machine `scanTree -> scanParents -> scanAuthor -> scanCommitter -> ... scanMessage`; final `c.Message = s.msgbuf.String()` (`commit.go:281`) copies the whole message.
- `plumbing/object/commit_scanner.go:17-42` `commitScanner` embeds a `bytes.Buffer msgbuf` (allocated per decode, source of `bytes.growSlice` + `Buffer.String`). `readLine` (`:52`) uses `bufio.Reader.ReadBytes('\n')`, which allocates a fresh slice per line (the `bufio.ReadBytes` 10.3% allocs line).
- `scanTree` (`:75`) fills `TreeHash`, `scanParents` (`:105`) appends to `c.ParentHashes` and stops at the first non-`parent` line. Header-only parse can therefore stop right after `scanParents` - the parents block ends at the first non-parent line (`:129-130`), and the tree is line 1.
- `plumbing/memory.go:53-58` `MemoryObject.Write` does `o.cont = append(o.cont, p...)` - the `MemoryObject.Write` 10.8% allocs line. This happens in the **storage/packfile layer** (inflate into a MemoryObject), *before* Decode. A header-only parser cannot remove it.

Where a header-only reader goes additively (INFERRED design, anchored on VERIFIED code):
- New file in `plumbing/object/`, e.g. `commit_header.go`, exporting `func DecodeCommitHeader(o plumbing.EncodedObject) (tree plumbing.Hash, parents []plumbing.Hash, err error)` (or fill a caller-supplied `[]plumbing.Hash` to avoid the append alloc). Reuse `parseObjectIDHex`, `splitHeader`, `isBlankLine` (same package), and `GetBufioReader`. Read only the first 1 + N lines; use `r.ReadSlice('\n')` (no alloc) instead of `ReadBytes`. Skip `msgbuf`, `scanAuthor/Committer`, gpgsig/mergetag continuation (`continuationCont` 9.2%).
- Additive: no change to `Commit.Decode`, so upstream rebase cost stays near zero (matches AGENTS.md "new functions, not changed behaviour").
- Honest limit (INFERRED from the `MemoryObject.Write` path above): header-only saves the parse-side allocs (`ReadBytes`, `continuationCont`, `Buffer.String`, most `growSlice`), roughly half the 51.9% decode share by the profile split in requirements.md, but the inflate-side `MemoryObject.Write` copy remains. To also skip that, a storage-level primitive is needed: read only the first ~N bytes of the inflated stream (commit headers are < ~300 bytes for non-merge commits, but gpgsig can precede... no, gpgsig is after committer, parents come first). Pack path: `packfile.Packfile.GetByOffset` (`gogitstore/store.go:566,582`) always materializes the full object; a "peek first K bytes" variant would be a second, larger fork patch (not additive-trivial: delta resolution needs the base object in full).
- Cache angle (VERIFIED): the shared `ObjectLRU` stores `plumbing.EncodedObject` (inflated bytes), not decoded commits (`plumbing/cache/object_lru.go:35`, `common.go:12-20`). So a cache hit still re-parses via Decode every time. A scanner-side parents cache (`hash -> []Hash`) removes that; `parentMemo` (`S/gogit_vcs_reader.go:2190-2215`) is per-computation only.

## 2. Commit-graph

VERIFIED API (F):
- v1 package `plumbing/format/commitgraph/` is marked Deprecated (`commitgraph.go:13,29`: wrong types for Generation/Index). Use `plumbing/format/commitgraph/v2/`.
- v2: `OpenFileIndex(reader ReaderAtCloser)` (`v2/file.go:64`), `OpenFileIndexWithParent` (`:70`), `OpenChainIndex(fs)` (`v2/chain.go:69`), `OpenChainOrFileIndex(fs)` (`v2/chain.go:48`: tries single file, then chain; error if neither). `Index` interface at `v2/commitgraph.go:41`; `CommitData` at `:13` (TreeHash, ParentHashes, Generation, GenerationV2, When). `GetIndexByHash` at `v2/file.go:187`, `GetCommitDataByIndex` at `:226`, `GetHashByIndex` at `:334`. Reads via `ReadAt` (so mmap-able or file-backed; no full load).
- Header check (`v2/file.go:118-137`): accepts format version 1 and SHA1/SHA256 hash byte; any other version returns `ErrUnsupportedVersion`. Chunk table includes GDA2/GDO2 (`v2/chunk.go:14`), `hasGenerationV2` field (`file.go`), so both generation v1 (in CDAT) and corrected-date v2 are decoded when present.
- Node layer: `plumbing/object/commitgraph/commitnode.go:14-39` `CommitNode` (ID, ParentHashes, NumParents, Generation, GenerationV2, CommitTime, Commit) and `CommitNodeIndex` (`:41`). `NewGraphCommitNodeIndex(index, storer)` (`commitnode_graph.go:39`) serves from the graph and **falls back to full `object.GetCommit`** for hashes not in the graph (`:43-70`). `NewObjectCommitNodeIndex` (`commitnode_object.go:22`) is the no-graph variant. `graphCommitNode.ParentHashes()` is served from `CommitData` (no decode).
- **Not wired into storage**: grep for `commit-graph`/`CommitGraph` in `storage/`, `repository.go`, `options.go`, `plumbing/object/commit_walker*.go` found nothing (only `plumbing/format/commitgraph*` and `plumbing/object/commitgraph/`). So go-git never opens the file on its own; the scanner must open `objects/info/commit-graph` (or `commit-graphs/commit-graph-chain`) itself through the common-dir billy fs.
- `Commit.MergeBase` (`plumbing/object/merge_base.go:17`) uses commit-date sorting (`sortByCommitDateDesc :148`) and does not use generation numbers or the graph. Generation-based reachability pruning would be new code in S (or a fork helper).

Usability for the scanner's operations (INFERRED semantics from the documented invariant at `v2/doc.go`: commits with older generation are not reachable from newer):
- Merge-base: lockstep walk can order frontiers by (generation desc) and stop a side once its max generation < the other side's min; parents come from `CommitData` with zero object decode for commits in the graph.
- Ahead/behind: `countCommitsTo` becomes graph-only traversal while commits are covered; uncovered commits (new since last `git gc`/`fetch.writeCommitGraph`) fall back to decode (`commitnode_graph.go:70`).
- Reachable-set (`maxReachableSetCommits` 50k): parents-only graph walk, no decode, if covered.

VERIFIED facts about real repos (checked `objects/info/commit-graph` and `commit-graphs/`):
| Repo | commit-graph | notes |
|---|---|---|
| `~/code/github.com/tstapler/stapler-squad` (this worktree's common git dir `.../stapler-squad/.git`, from `.git` file `gitdir: .../stapler-squad/.git/worktrees/...`) | **YES**, single file `objects/info/commit-graph`, 535,292 bytes, mtime Oct 8 10:02, no `commit-graphs/` chain | header bytes `CGPH 01 01 04 00` = file version 1, SHA1, 4 chunks, 0 base graphs; first chunk `OIDF`. 19 packfiles; `rev-list --all --count` = 9458 commits |
| consolette, docspan, gh-signal, go-git, kibitzer, magic-compact, pi-extensions, rtk, stapler-mcp (all under `~/code/github.com/tstapler/`) | **NO** (neither file nor chain) | |
- So: 1 of 10 repos checked has a graph; it is 7 days old (written Oct 8, today Oct 15) so commits since then are uncovered and must hit the fallback. No `gc.*`, `fetch.writeCommitGraph`, `core.commitGraph` keys in the stapler-squad repo config (`git config --get-regexp` printed nothing). Without them a graph appears only if some git process (IDE, `git gc`, `git maintenance`) wrote it - not reliable. The fork reader handles the version/hash in that header (v2 accepts version 1; `file.go:129`).
- Whether it has GDA2 (the 4 chunks are likely OIDF, OIDL, CDAT, plus one of EDGE/GDA2): UNVERIFIED, I did not decode the chunk table. File size 535,292 over ~9.5k commits ~ 56 B/commit (20 OIDL + 36 CDAT), which suggests no GDA2 (INFERRED). Generation v1 (CDAT) suffices for the pruning above.
- Consequence for planning: commit-graph as the *only* mechanism is unreliable (most repos have none). The fork would need to **write** a graph (`plumbing/format/commitgraph/v2/encoder.go`, `memory.go` exist, VERIFIED) or the scanner should build its own in-memory parent/generation index (hash -> parents, generation), which gives the same asymptotics without depending on external git state. Writing `objects/info/commit-graph` into user repos has side effects (INFERRED); an in-process or sidecar cache under `~/.stapler-squad/` is safer.

## 3. Object cache and object storage as used in S

VERIFIED:
- `plumbing/cache/common.go:12-20`: `Object` interface (`Put/Get/Clear`) caches `plumbing.EncodedObject`; `DefaultMaxSize` 96 MiB. `ObjectLRU` (`object_lru.go:23-35`) uses `container/list` + `map[interface{}]*list.Element` (key boxed in `interface{}` = alloc per Get/Put key; a global mutex). `BufferLRU` (`buffer_lru.go`) keys by pack offset, caches delta-base buffers.
- S wiring: `gogitstore/registry.go:185` builds `cache.NewObjectLRU(maxSize)` per `SharedObjectStore` (one per common git dir, shared by all worktrees, `store.go:50-66`); `gogit_vcs_reader.go:717-723` documents `perRepoObjectCacheSize = 12 MiB`. `gogitstore/open.go:41` opens a per-worktree `filesystem.NewStorageWithOptions(repositoryFs, throwawayObjectCache(), filesystem.Options{})` (empty Options, so no `KeepDescriptors`/`MaxOpenDescriptors`/`LargeObjectThreshold` there), wrapped in `WorktreeStorer`, which routes object reads to the shared store (`gogitstore/storer.go:105-125`).
- `SharedObjectStore.encodedObject` (`store.go:442-467`): checks ObjectLRU first, validated against the pack index, then `getFromUnpacked` (loose), then `getFromPackfile` (`store.go:543-583`) which constructs `packfile.NewPackfileWithCache(idx, fs, f, s.objectCache, threshold)` **per call** and `GetByOffset`. Every call allocates a Packfile wrapper (and a Scanner/ObjectReader inside, INFERRED from go-git's packfile.go; not opened).
- Implication: caching `EncodedObject` bytes (12 MiB) means a hit skips inflate but not Decode; commits are tiny (~250 B) so 12 MiB holds ~40k commits, but the LRU churns when trees/blobs from `diff`/index code share it (INFERRED). Option: a separate small commit-only cache or parents memo outside ObjectLRU (see section 4).
- Fork patch candidates on the storage side (INFERRED): (a) `ObjectStorage`-level `CommitParents(h)` that consults a graph/memo then falls back; (b) avoid per-call `NewPackfileWithCache` allocation - but S already reimplements the storage in `gogitstore`, so this is **S-side** work, not a fork patch (the fork's `filesystem.ObjectStorage` is not on the hot path).

## 4. Go 1.26 / library options (go.mod facts, VERIFIED by grep of `go.mod`)
Already direct/indirect deps: `github.com/puzpuzpuz/xsync/v4 v4.5.0` (line 40), `github.com/sergi/go-diff v1.4.0` (42), `golang.org/x/sync v0.22.0` (221), `golang.org/x/exp` (219, indirect only), go-billy/gcfg. **Not present**: roaring (`RoaringBitmap/roaring`), `bits-and-blooms/bitset`, lru libs (hashicorp/golang-lru, ristretto, otter), samber/hot (a skill exists but no go.mod entry).
- Swiss maps: Go 1.24+ built-in `map` is already the Swiss-table implementation (INFERRED from Go release notes; no import needed). The `map[plumbing.Hash]struct{}`/`map[Hash][]Hash` in `parentMemo`/`bfsSide` already benefit. `plumbing.Hash` is `[20]byte` (INFERRED; go-git v5 hash type) - a 20-byte key hashes fine but a **dense index** (hash -> uint32 id once per repo, then `[]uint32` parents and bitsets over ids) is the compact form.
- Compact sets: roaring (new dep, `go get`; mature, serializable) or `bitset` (new dep) apply only after commits get dense uint32 ids; otherwise a sorted `[]uint32` / `slices.BinarySearch` or stdlib-only `[]uint64` bitmap over generation-ordered ids is enough for 50k commits (6.25 KB per set). INFERRED: for R <= 50k a hand-rolled `[]uint64` beats adding roaring; roaring only wins for sparse or very large sets. Needs a socket.dev depscore check before adding either (repo has the `mcp__socket-dev__depscore` tool).
- `xsync/v4` (already a dep): `xsync.Map[K,V]` lock-free reads - fits the per-repo shared caches keyed by common git dir (and Hash); `xsync.MapOf` renamed to `Map` in v4 (INFERRED from v4 API; verify at use). Also gives `Compute`, `LoadOrStore`.
- `unique` (Go 1.23+): `unique.Handle[T]` interns comparable values, handle = pointer, equality is pointer compare; useful for interning path strings / `plumbing.Hash` in many maps (INFERRED). `weak` (Go 1.24+): `weak.Pointer[T]` for caches that must not pin memory (per-repo caches that drop when the repo entry is evicted); `runtime.AddCleanup` replaces finalizers (INFERRED). Neither exists in S today (grep of `S/*.go` for `unique.|weak.|xsync` showed no non-test hits beyond go-git/billy imports - xsync is used elsewhere in the repo).
- `golang.org/x/exp` is indirect only; `slices`, `maps`, `cmp` are stdlib in 1.26, no need.

## 5. Diff: go-git utils/diff vs `lcsLength`

VERIFIED:
- S `lcsLength` (`gogit_vcs_reader.go:1828-1859`): O(n*m) time with two rolling rows (`make([]int, len(b)+1)` x2 per call, plus an explicit O(m) zeroing pass per row, so ~2x the DP's inner work), returns **0** when either side exceeds `maxLCSLines` (20,000; `:1824`); `lcsLengthBytes` (`:1907`) is the `[][]byte` twin; the cap path at `:1736-1737` reports a "fully-replaced approximation".
- F `utils/diff/diff.go:20-45`: `Do(src, dst string)` = `DoWithTimeout(src, dst, time.Hour)`; implementation `dmp.DiffLinesToRunes` -> `DiffMainRunes(..., false)` -> `DiffCharsToLines`. It is the sergi/go-diff Myers bisect (O(ND)) on line-runes, but operates on **whole strings** (needs file contents joined, then re-splits) and returns `[]diffmatchpatch.Diff`, so it allocates the rune arrays and Diff strings - heavy for shortstat which only needs counts. It is already a dependency (go.mod:42, same lib the fork uses).
- Parity risk (VERIFIED + INFERRED): `diffHalfMatch` is skipped only if `DiffTimeout <= 0` (`diffmatchpatch/diff.go:515`); `Do` sets 1h, so the half-match speedup is **active** (`diff.go:172` calls it) and can yield a non-minimal edit script. LCS-based counts (insertions = m - lcs, deletions = n - lcs) are the minimum; Myers-with-halfmatch can report more. To keep parity set `DiffTimeout = 0` (then `DiffBisect` deadline is zero/no limit: INFERRED) or write a dedicated counting Myers.
- Recommendation (INFERRED): implement a small in-package Myers/greedy-LCS counter `myersEditDistance(a, b []uint32 /* interned line ids */) (ins, del int)` with common prefix/suffix trimming first (most edits are local, so n,m collapse), O((n+m)·D) time and O(D) or O(n+m) space, interning lines to `uint32` via one map per diff. This avoids go-diff's rune/Diff allocations, removes the `maxLCSLines` cliff, and needs no fork patch. A fork patch (`utils/diff.Counts(src, dst []byte) (ins, del int)`) is the second choice (order of preference in AGENTS.md: better algorithm in package first).
- Cheap, safe pre-win independent of algorithm: trim equal prefix/suffix lines before the DP (the current code does not, INFERRED: no trim in the lines shown at `:1828-1859`).

## Fork patch inventory (proposed, all additive)
1. `plumbing/object`: `DecodeCommitHeader` / parents-only reader (section 1). Cost: small. Saves parse-side allocs only.
2. Optional later: `Packfile` "peek first N bytes of non-delta commit" primitive; larger, riskier; defer until measured.
3. Optional: commit-graph *writer helper* using `v2/encoder.go`; prefer an in-process parents/generation index in S unless the owner accepts writing into `.git/objects/info`.
4. Optional: `utils/diff` counts-only helper (prefer S-side Myers first).

## Gaps / UNVERIFIED
- Chunk table of the stapler-squad commit-graph (GDA2/EDGE) not decoded.
- go-git `packfile.NewPackfileWithCache` allocation cost not opened.
- Whether the live binary includes branch commits (from requirements.md) not checked here.
- `unique`/`weak`/Swiss-map claims are from Go release knowledge, not verified in this checkout; roaring/bitset not in go.mod (VERIFIED absent).
