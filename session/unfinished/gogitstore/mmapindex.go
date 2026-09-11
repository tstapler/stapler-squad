// mmapindex.go implements the mmap-backed .idx loader described in
// session/unfinished/design/pluggable-gitstore.md §5 ("mmap for the index —
// designed, not built"). This is stage 2 of the staged rollout plan (§6).
//
// The key finding the design doc reaches (§5.1), reproduced here rather than
// re-derived: idxfile.MemoryIndex itself does not need to change. On disk,
// each of the Names/CRC32/Offset32 blocks is one contiguous byte run,
// ordered by fanout bucket; decoder.go only slices it bucket-by-bucket in
// memory because MemoryIndex.Names/Offset32/CRC32 want per-bucket []byte
// values, not because the underlying bytes are non-contiguous. So a
// from-scratch loader can compute each bucket's [start:end] byte range from
// the (1024-byte) fanout table and set idx.Names[k] = mapped[start:end] as a
// zero-copy subslice of an mmap'd region, instead of decoder.go's
// io.ReadFull-based make+copy per bucket.
//
// loadMemoryIndexMmap below is verified byte-for-byte against decoder.go's
// Decode (see mmapindex_test.go's TestMmapIndex_MatchesCopyBasedLoader) by
// reading the actual vendored source at
// $(go env GOMODCACHE)/github.com/go-git/go-git/v5@v5.14.0/plumbing/format/
// idxfile/{decoder,idxfile}.go — not re-derived from the design doc's prose
// alone.
package gogitstore

import (
	"bytes"
	encbin "encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	mmap "github.com/edsrzf/mmap-go"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/idxfile"
	"github.com/go-git/go-git/v5/plumbing/hash"
	"github.com/tstapler/stapler-squad/log"
)

const (
	idxMagicLen   = 4
	idxVersionLen = 4
	idxFanoutLen  = 256 * 4
	// idxHeaderLen is the byte offset where the Names block begins: 4-byte
	// magic + 4-byte version + 256*4-byte fanout table, with no padding
	// between them or before the Names block — verified against
	// decoder.go's validateHeader+readVersion+readFanout, which consume
	// exactly these byte counts in this order via a single io.Reader.
	idxHeaderLen = idxMagicLen + idxVersionLen + idxFanoutLen // 1032

	// idxNoMapping mirrors idxfile package's unexported `noMapping = -1`
	// constant (plumbing/format/idxfile/idxfile.go) — FanoutMapping entries
	// for fanout buckets with zero objects must be set to exactly this
	// value; idxfile's own findHashIndex treats it as a sentinel meaning
	// "this bucket has no entries."
	idxNoMapping = -1
)

// idxMagic is idxfile's on-disk header magic (idxHeader in idxfile.go),
// reproduced here since it is unexported upstream.
var idxMagic = [4]byte{0xff, 't', 'O', 'c'}

// loadMemoryIndexMmap decodes an idxfile.MemoryIndex from mapped — the full
// contents of an mmap'd .idx file — without copying the Names/CRC32/
// Offset32/Offset64 blocks: each is set as a zero-copy subslice of mapped
// itself (three-index sliced, mapped[a:b:b], so an accidental append by any
// future caller cannot silently write past the bucket's logical end into a
// neighboring bucket's bytes). Only the small, fixed-size Fanout/
// FanoutMapping table (1024 bytes) and the two trailing checksums are
// copied, matching the design doc §5.1's judgement that they aren't worth
// mmap'ing.
//
// mapped must remain validly mapped (not Unmap'd) for as long as the
// returned *idxfile.MemoryIndex — or anything derived from it, including
// EntryIter values obtained from its Entries() method — may still be read.
// This function does not manage that lifetime; see mmapIndexHandle, which
// is the only intended caller-facing owner of an mmap'd index.
func loadMemoryIndexMmap(mapped []byte) (*idxfile.MemoryIndex, error) {
	if len(mapped) < idxHeaderLen {
		return nil, fmt.Errorf("gogitstore: idx file too small to contain a header (%d bytes)", len(mapped))
	}
	if !bytes.Equal(mapped[:idxMagicLen], idxMagic[:]) {
		return nil, idxfile.ErrMalformedIdxFile
	}
	version := encbin.BigEndian.Uint32(mapped[idxMagicLen : idxMagicLen+idxVersionLen])
	if version > idxfile.VersionSupported {
		return nil, idxfile.ErrUnsupportedVersion
	}

	objectIDLength := hash.Size

	idx := idxfile.NewMemoryIndex()
	idx.Version = version

	fanoutOff := idxMagicLen + idxVersionLen
	var prevFanout uint32
	for k := 0; k < 256; k++ {
		off := fanoutOff + k*4
		v := encbin.BigEndian.Uint32(mapped[off : off+4])
		// A well-formed fanout table is cumulative and therefore
		// non-decreasing (idx.Fanout[k] is "count of objects with a first
		// hash byte <= k"). A corrupted/adversarial table with a
		// decreasing entry would make the per-bucket `buckets := cum -
		// prevCum` subtraction below underflow (uint32 wraparound to a
		// value near 2^32), which — unlike decoder.go's io.ReadFull-based
		// copy loader, which merely fails an oversized make()/ReadFull —
		// turns directly into an out-of-bounds `mapped[n0:n1:n1]` slice
		// expression here and PANICS the whole process instead of
		// returning a clean decode error. Caught empirically (this task's
		// adversarial-input hardening pass) via
		// TestMmapIndex_MalformedInput_NeverPanics/non-monotonic-fanout —
		// reject it here, before any bucket arithmetic runs.
		if v < prevFanout {
			return nil, fmt.Errorf("%w: fanout table is not non-decreasing at bucket %d (%d < %d)", idxfile.ErrMalformedIdxFile, k, v, prevFanout)
		}
		prevFanout = v
		idx.Fanout[k] = v
		idx.FanoutMapping[k] = idxNoMapping
	}

	totalObjects := int(idx.Fanout[255])
	namesBase := idxHeaderLen
	namesLen := totalObjects * objectIDLength
	namesEnd := namesBase + namesLen
	if namesEnd < namesBase || namesEnd > len(mapped) {
		return nil, fmt.Errorf("gogitstore: idx file truncated in names block (need %d bytes, have %d)", namesEnd, len(mapped))
	}

	crcBase := namesEnd
	crcLen := totalObjects * 4
	crcEnd := crcBase + crcLen

	off32Base := crcEnd
	off32Len := totalObjects * 4
	off32End := off32Base + off32Len
	if off32End > len(mapped) {
		return nil, fmt.Errorf("gogitstore: idx file truncated in crc/offset32 blocks (need %d bytes, have %d)", off32End, len(mapped))
	}

	var prevCum uint32
	nameOff, crcOff, o32Off := 0, 0, 0
	for k := 0; k < 256; k++ {
		cum := idx.Fanout[k]
		buckets := cum - prevCum
		prevCum = cum
		if buckets == 0 {
			continue
		}
		idx.FanoutMapping[k] = len(idx.Names)

		n0 := namesBase + nameOff
		n1 := n0 + int(buckets)*objectIDLength
		idx.Names = append(idx.Names, mapped[n0:n1:n1])
		nameOff += int(buckets) * objectIDLength

		c0 := crcBase + crcOff
		c1 := c0 + int(buckets)*4
		idx.CRC32 = append(idx.CRC32, mapped[c0:c1:c1])
		crcOff += int(buckets) * 4

		o0 := off32Base + o32Off
		o1 := o0 + int(buckets)*4
		idx.Offset32 = append(idx.Offset32, mapped[o0:o1:o1])
		o32Off += int(buckets) * 4
	}

	// Count Offset64 entries by scanning the high bit of each Offset32
	// entry — mirrors decoder.go's readOffsets exactly (an Offset32 entry
	// with its top bit set is an index into the Offset64 block, not a
	// literal 31-bit offset).
	var o64cnt int
	for _, block := range idx.Offset32 {
		for p := 0; p < len(block); p += 4 {
			if block[p]&(byte(1)<<7) > 0 {
				o64cnt++
			}
		}
	}

	off64Base := off32End
	checksumsBase := off64Base
	if o64cnt > 0 {
		o64Len := o64cnt * 8
		o64End := off64Base + o64Len
		if o64End > len(mapped) {
			return nil, fmt.Errorf("gogitstore: idx file truncated in offset64 block (need %d bytes, have %d)", o64End, len(mapped))
		}
		idx.Offset64 = mapped[off64Base:o64End:o64End]
		checksumsBase = o64End
	}

	if checksumsBase+2*objectIDLength > len(mapped) {
		return nil, fmt.Errorf("gogitstore: idx file truncated in checksum trailer")
	}
	copy(idx.PackfileChecksum[:], mapped[checksumsBase:checksumsBase+objectIDLength])
	copy(idx.IdxChecksum[:], mapped[checksumsBase+objectIDLength:checksumsBase+2*objectIDLength])

	return idx, nil
}

// mmapIndexHandle owns one mmap'd .idx file's backing memory plus the
// MemoryIndex built as zero-copy subslices of it, and the generation/
// refcount bookkeeping design doc §5.3 identifies as the genuinely hard
// part: a handle must never be Unmap'd while anything might still read a
// slice into its mapping.
//
// ALL fields are guarded by the owning SharedObjectStore's mu — never
// accessed without it held. This deliberately reuses the SAME mutex
// SharedObjectStore already serializes every idxfile.Index call behind
// (see index.go's package doc) rather than inventing a second lock or an
// atomic-based scheme: the only access path that can read mapping bytes
// AFTER that mutex has been released is a drained Entries() iterator
// (index.go's pinnedEntryIter pins/unpins pins, below, for exactly that
// window — see its doc comment for why EntriesByOffset does not need the
// same treatment).
type mmapIndexHandle struct {
	mapping mmap.MMap
	file    *os.File
	idx     *idxfile.MemoryIndex

	// pins counts iterators currently permitted to read mapping after mu
	// has been released. Never negative; incremented/decremented only
	// while the owning store's mu is held.
	pins int

	// retiring is set once SharedObjectStore.refreshIndexes (store.go)
	// determines this pack no longer appears in ObjectPacks() — a repack
	// wrote a new pack that supersedes it. A retiring handle is removed
	// from SharedObjectStore.index immediately (new lookups simply won't
	// find it — "stale", not "corrupt", per design doc §5.3's POSIX
	// unlink-while-mapped analysis) but its mapping is only actually
	// released once pins reaches zero.
	retiring bool

	// unmapped guards against calling mapping.Unmap() more than once for
	// this handle (undefined per mmap-go's own doc: "Unmap should only be
	// called on the slice value that was originally returned from a call
	// to Map").
	unmapped bool
}

// maybeUnmapLocked releases h's mapping and closes its file IF h is both
// retiring and has no live pins; a no-op otherwise (including if already
// unmapped). Callers MUST hold the owning SharedObjectStore's mu. Safe to
// call speculatively — e.g. once when refreshIndexes first marks a handle
// retiring, and again every time a pin is released — since it only acts
// when both conditions actually hold.
func (h *mmapIndexHandle) maybeUnmapLocked() {
	if !h.retiring || h.pins != 0 || h.unmapped {
		return
	}
	h.unmapped = true
	if err := h.mapping.Unmap(); err != nil {
		log.Warn("gogitstore: munmap failed for retired index", "err", err)
	}
	if err := h.file.Close(); err != nil {
		log.Warn("gogitstore: close failed for retired index file", "err", err)
	}
}

// openMmapIndexHandle mmaps commonDirAbs's pack-<h>.idx file and builds a
// zero-copy MemoryIndex over it.
//
// This deliberately bypasses billy.Filesystem and goes straight to os.Open:
// mmap.Map requires a real *os.File, and every caller of this package
// resolves commonFs via osfs.New (see open.go's resolveGitFilesystems) — an
// OS-backed path is always available in practice. The path itself
// reconstructs go-git's own on-disk layout (storage/filesystem/dotgit/
// dotgit.go's objectsPath="objects", packPath="pack",
// objectPackPath=".../pack-<hash>.<ext>") rather than going through
// dotgit.DotGit.ObjectPackIdx, which only ever hands back a billy.File. If
// this assumption doesn't hold for some future non-OS-backed commonFs, the
// caller (SharedObjectStore.buildIndexEntryLocked) falls back to the
// copy-based decoder path for that one pack rather than failing the whole
// store.
func openMmapIndexHandle(commonDirAbs string, h plumbing.Hash) (*mmapIndexHandle, error) {
	path := filepath.Join(commonDirAbs, "objects", "pack", fmt.Sprintf("pack-%s.idx", h.String()))
	f, err := os.Open(path) // #nosec G304 -- h is a go-git plumbing.Hash (fixed-length hex string), commonDirAbs is the repo's own git dir; neither is user input
	if err != nil {
		return nil, err
	}

	m, err := mmap.Map(f, mmap.RDONLY, 0)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("mmap %s: %w", path, err)
	}

	idx, err := loadMemoryIndexMmap(m)
	if err != nil {
		_ = m.Unmap()
		_ = f.Close()
		return nil, fmt.Errorf("decode mmap'd %s: %w", path, err)
	}

	return &mmapIndexHandle{mapping: m, file: f, idx: idx}, nil
}
