# ADR-028: Search Index Persistence — mmap-Backed CSR Format Instead of Whole-File gob

**Status**: Proposed
**Date**: 2026-09-04
**Project**: perf profiling session (`stapler-squad-perf` branch) — PerfFix-3 follow-up

## Context

Live pprof heap profiling (`go tool pprof -inuse_space`, see the perf session's Phase 2
ranking) showed `encoding/gob`'s `decString` and `reflect.unsafe_NewArray` accounting for 170MB
+ 149MB of live heap — 55% of the process's total in-use heap at snapshot time. Both trace back
to `session/search/index_store.go`'s `loadGob`/`saveGob`
([index_store.go:238-284](../../session/search/index_store.go)), which persists
`InvertedIndex` and `DocumentStore` as whole-file `encoding/gob` blobs.

The current shape:

- **`Save`/`Load` are all-or-nothing.** Every call decodes or encodes the *entire* index —
  there's no partial read, no incremental update. A single new indexed session requires a full
  re-serialize of every term's postings.
- **`PostingsList` is already CSR-flat** (`session/search/inverted_index.go:15-27`): `DocIDs`,
  `Positions`, `PosOffsets`, `Frequency` are all `[]int32`, no nested pointers. This layout was
  chosen specifically to avoid per-position allocations (see the file's own "PerfFix-2" comment
  on `CurrentIndexVersion`) — but gob still has to walk and copy every element into fresh
  heap-allocated slices on load, so the flat layout's cache-friendliness is thrown away the
  moment it round-trips through gob.
- **No invalidation/update story.** `IndexSyncMetadata` tracks which sessions are stale, but
  the actual index mutation path is: decode the whole thing, mutate the in-memory maps, encode
  the whole thing back (`engine.go` calls into `Save`/`Load` wholesale — see
  `session/search/engine.go:142,270-306,488-491,651-654`).

`Docs`/`SessionIndex` (`session/search/document_store.go:26-35`) are less regular — `Docs` is a
`map[int32]*Document` with variable-length string fields, `SessionIndex` is
`map[string][]int32` — so they don't get the same clean win from this change; see
Consequences.

## Decision

Replace the two gob blobs backing `InvertedIndex.Index` (the term → `PostingsList` map) with a
single memory-mapped binary file, read via `golang.org/x/exp/mmap` (already permitted per this
repo's CDN/dependency norms — no new third-party runtime dependency beyond the Go toolchain's
own experimental module) or an unsafe.Slice cast into a raw mmap.ReaderAt. Layout:

```
┌─────────────────────────────────────────────────────────────┐
│ Header (fixed 32B): magic, format version, term count,      │
│ directory offset, data segment offset, total docs,          │
│ avg doc length (float64 bits)                                │
├─────────────────────────────────────────────────────────────┤
│ Term directory: sorted (term string, PostingsEntry) pairs.  │
│ PostingsEntry = {docIDsOff, docIDsLen, positionsOff,         │
│ positionsLen, posOffsetsOff, posOffsetsLen, freqOff} —       │
│ all uint32 byte offsets/lengths into the data segment below. │
│ Loaded eagerly into a small in-memory map[string]PostingsEntry│
│ (term strings and offsets only — this is the ONLY thing      │
│ decoded into the Go heap on Load).                            │
├─────────────────────────────────────────────────────────────┤
│ Data segment: every PostingsList's DocIDs/Positions/         │
│ PosOffsets/Frequency arrays, concatenated back-to-back,      │
│ each 4-byte-aligned (int32-per-element, so natural alignment  │
│ requires no padding beyond the segment start). Never copied   │
│ into the Go heap — PostingsList.DocIDs etc. become            │
│ unsafe.Slice([]int32) views directly over the mapped bytes.   │
└─────────────────────────────────────────────────────────────┘
```

`Load()` becomes: mmap the file, decode the small header + term directory (bounded by term
count, not posting-list size), done. A query for term *t* looks up its `PostingsEntry` in the
directory map and slices the mmap directly — the OS faults in only the 4KB pages that term's
arrays actually span, not the whole index. `DocLengths` (small: one `int32` per doc) stays a
plain in-memory map, decoded eagerly like the directory.

`Save()` writes a fresh file (header, then directory, then data segment, each section's byte
length computed from Go slice lengths before writing) to a temp path and atomically renames it
into place — reusing `saveGob`'s existing temp-file-then-rename pattern
([index_store.go:238-271](../../session/search/index_store.go)), not a new mechanism.

`DocumentStore` (`Docs`/`SessionIndex`) stays gob-encoded as-is: it's the smaller of the two
files in practice (metadata per document, not per-position postings), variable-length string
fields don't fit the fixed-width-array layout above cleanly, and the profiling evidence pointed
at postings-list decode, not document metadata.

## Consequences

**Wins**
- `Load()` cost drops from O(index size) to O(term count) — no more materializing every
  posting's positions into fresh `[]int32` slices on every process restart / index reopen.
- Query-time page faults replace an eager full decode; a process that only ever searches a
  handful of hot terms never pages in the cold majority of the index.
- Multiple readers (e.g. concurrent search requests) share one mmap'd region — no per-reader
  copy, unlike gob decode which allocates fresh slices per `Load()` call.

**Costs / risks — why this is a separate project, not folded into this perf session's diff**
- **Binary format correctness is unforgiving.** A miscomputed offset doesn't error like a gob
  type mismatch does — it silently reads garbage or panics on an out-of-range slice. This needs
  its own fuzz/round-trip test suite (write index → mmap-read → compare against the same index
  built and read via the current gob path) before it can replace the existing format, mirroring
  how `validateCSRInvariants` (`index_store.go:200-236`) already guards the in-memory CSR
  invariants — the mmap format needs an equivalent invariant check run after every `Load()`.
- **No true in-place update.** Appending a new document to an *existing* term's postings list
  still requires rewriting that term's arrays (they grow), so this format speeds up *load*, not
  incremental *mutation* — `engine.go`'s decode-mutate-reencode cycle for updates is unchanged
  unless a follow-up adds a write-ahead log or over-allocates slack per postings list. State
  that goal explicitly if pursued — this ADR only decides the read-path format.
- **Platform/alignment care.** `unsafe.Slice` casts require the data segment's arrays to start
  4-byte-aligned; the header must guarantee this by construction (pad section boundaries), and
  the format should pick an explicit endianness (little-endian, matching every platform this
  project ships on) rather than relying on native byte order, so the file is portable across
  architectures if that ever matters.
- **New format version.** `CurrentIndexVersion` (currently 2) bumps to 3; old gob-format
  indexes are rejected (`IndexVersionMismatchError`, already the existing pattern for version
  skew) and rebuilt from scratch on first load — no in-place migration needed since the index
  is a derived cache, not a source of truth.

## Recommendation

Scope this as its own planned effort (`/sdd:full` or at minimum `/sdd:quick` — it needs a real
test plan for the binary format, not just a perf-session patch) rather than implementing it
inside the `stapler-squad-perf` branch. The live-heap evidence motivating it (170MB gob +
149MB reflect, 55% of live heap) is real but not urgent on its own — live heap was 577MB total
at snapshot time, not a memory-pressure emergency — so it's reasonable to land PerfFix-1
(git subprocess coalescing) and PerfFix-2 (repo cache budget) first and treat this ADR as the
accepted direction for a follow-up implementation session.
