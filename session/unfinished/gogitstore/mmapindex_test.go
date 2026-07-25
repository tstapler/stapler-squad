package gogitstore

// Tests for stage 2's mmap-backed .idx loader (mmapindex.go), per
// session/unfinished/design/pluggable-gitstore.md §5. Two concerns:
//
//  1. Correctness: loadMemoryIndexMmap must produce a MemoryIndex
//     byte-for-byte equivalent (not just "close enough") to decoder.go's
//     io.ReadFull-based Decode, for the same on-disk .idx file.
//  2. The one deliberately dangerous negative-control test proving the
//     generation/refcount protection this package builds around
//     Entries()/EntriesByOffset() is actually load-bearing — see
//     TestMmapIndexHandle_UnmapWhileSliceHeld_CausesCrash's doc comment.
//
// The main concurrent generation/refcount safety proof lives in
// mmap_stage2_test.go, alongside the staleness-detection and toggle tests.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/idxfile"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// TestMmapIndex_MatchesCopyBasedLoader_ByteForByte builds a real packed
// fixture, decodes its .idx once via decoder.go's stock io.ReadFull path and
// once via loadMemoryIndexMmap over a REAL mmap of the same file (not just a
// []byte read into memory — this exercises the actual mmap.Map call, not
// merely the parsing logic in isolation), and asserts full equivalence:
// every field a caller can observe (Fanout, FanoutMapping, Count,
// PackfileChecksum, IdxChecksum, every Entries() tuple, every FindOffset/
// FindHash/FindCRC32/Contains result) must match exactly — not a spot check.
func TestMmapIndex_MatchesCopyBasedLoader_ByteForByte(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	buildPackedFixture(t, dir, 90)

	_, commonFs, _, commonDirAbs, err := resolveGitFilesystems(dir)
	if err != nil {
		t.Fatalf("resolveGitFilesystems: %v", err)
	}
	store := newSharedObjectStore(commonDirAbs, commonFs, nil, 0, false)
	packs, err := store.dir.ObjectPacks()
	if err != nil {
		t.Fatalf("ObjectPacks: %v", err)
	}
	if len(packs) != 1 {
		t.Fatalf("fixture produced %d packs, want exactly 1 (buildPackedFixture forces git gc --aggressive)", len(packs))
	}
	packHash := packs[0]

	// ---- copy-based (control) ----
	f, err := store.dir.ObjectPackIdx(packHash)
	if err != nil {
		t.Fatalf("ObjectPackIdx: %v", err)
	}
	miCopy := idxfile.NewMemoryIndex()
	if err := idxfile.NewDecoder(f).Decode(miCopy); err != nil {
		t.Fatalf("copy-based Decode: %v", err)
	}
	_ = f.Close()

	// ---- mmap-based (subject under test) ----
	handle, err := openMmapIndexHandle(commonDirAbs, packHash)
	if err != nil {
		t.Fatalf("openMmapIndexHandle: %v", err)
	}
	defer func() { _ = handle.mapping.Unmap(); _ = handle.file.Close() }()
	miMmap := handle.idx

	// --- Fanout / FanoutMapping ---
	if miCopy.Fanout != miMmap.Fanout {
		t.Error("Fanout tables differ")
	}
	if miCopy.FanoutMapping != miMmap.FanoutMapping {
		t.Error("FanoutMapping tables differ")
	}
	if miCopy.Version != miMmap.Version {
		t.Errorf("Version differs: copy=%d mmap=%d", miCopy.Version, miMmap.Version)
	}
	if miCopy.PackfileChecksum != miMmap.PackfileChecksum {
		t.Error("PackfileChecksum differs")
	}
	if miCopy.IdxChecksum != miMmap.IdxChecksum {
		t.Error("IdxChecksum differs")
	}

	// --- Count ---
	countCopy, err := miCopy.Count()
	if err != nil {
		t.Fatalf("copy Count: %v", err)
	}
	countMmap, err := miMmap.Count()
	if err != nil {
		t.Fatalf("mmap Count: %v", err)
	}
	if countCopy != countMmap {
		t.Fatalf("Count differs: copy=%d mmap=%d", countCopy, countMmap)
	}
	if countCopy == 0 {
		t.Fatal("fixture produced zero index entries — test is not exercising anything")
	}

	// --- every entry, in iteration order ---
	itCopy, err := miCopy.Entries()
	if err != nil {
		t.Fatalf("copy Entries: %v", err)
	}
	defer func() { _ = itCopy.Close() }()
	itMmap, err := miMmap.Entries()
	if err != nil {
		t.Fatalf("mmap Entries: %v", err)
	}
	defer func() { _ = itMmap.Close() }()

	var n int
	for {
		eCopy, errCopy := itCopy.Next()
		eMmap, errMmap := itMmap.Next()
		if errors.Is(errCopy, io.EOF) != errors.Is(errMmap, io.EOF) {
			t.Fatalf("entry %d: EOF mismatch (copy err=%v, mmap err=%v)", n, errCopy, errMmap)
		}
		if errors.Is(errCopy, io.EOF) {
			break
		}
		if errCopy != nil {
			t.Fatalf("copy Next() at entry %d: %v", n, errCopy)
		}
		if errMmap != nil {
			t.Fatalf("mmap Next() at entry %d: %v", n, errMmap)
		}
		if eCopy.Hash != eMmap.Hash {
			t.Errorf("entry %d: Hash differs: copy=%s mmap=%s", n, eCopy.Hash, eMmap.Hash)
		}
		if eCopy.Offset != eMmap.Offset {
			t.Errorf("entry %d (%s): Offset differs: copy=%d mmap=%d", n, eCopy.Hash, eCopy.Offset, eMmap.Offset)
		}
		if eCopy.CRC32 != eMmap.CRC32 {
			t.Errorf("entry %d (%s): CRC32 differs: copy=%d mmap=%d", n, eCopy.Hash, eCopy.CRC32, eMmap.CRC32)
		}
		n++
	}
	if int64(n) != countCopy {
		t.Errorf("iterated %d entries, want %d (Count())", n, countCopy)
	}
	t.Logf("compared %d entries byte-for-byte between copy-based and mmap-based loaders", n)

	// --- per-hash lookups: FindOffset / FindHash / FindCRC32 / Contains,
	// re-decoding fresh copy-based indexes for FindHash's reverse lookup
	// since MemoryIndex.FindHash lazily mutates internal state (the exact
	// property lockedIndex exists to serialize — see index.go) and reusing
	// miCopy/miMmap across both directions would conflate "loader
	// correctness" with "lazy-mutation behavior," which is already covered
	// by TestConcurrentReadsAcrossWorktrees_NoDataRace. ---
	itAll, err := miCopy.Entries()
	if err != nil {
		t.Fatalf("Entries for lookup table: %v", err)
	}
	type wantEntry struct {
		offset int64
		crc    uint32
	}
	want := make(map[string]wantEntry, countCopy)
	for {
		e, err := itAll.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		want[e.Hash.String()] = wantEntry{offset: int64(e.Offset), crc: e.CRC32}
	}
	_ = itAll.Close()

	for hashStr, w := range want {
		h := hashFromString(t, hashStr)

		gotOffset, err := miMmap.FindOffset(h)
		if err != nil {
			t.Errorf("mmap FindOffset(%s): %v", hashStr, err)
			continue
		}
		if gotOffset != w.offset {
			t.Errorf("mmap FindOffset(%s) = %d, want %d", hashStr, gotOffset, w.offset)
		}

		gotCRC, err := miMmap.FindCRC32(h)
		if err != nil {
			t.Errorf("mmap FindCRC32(%s): %v", hashStr, err)
		} else if gotCRC != w.crc {
			t.Errorf("mmap FindCRC32(%s) = %d, want %d", hashStr, gotCRC, w.crc)
		}

		gotHash, err := miMmap.FindHash(w.offset)
		if err != nil {
			t.Errorf("mmap FindHash(%d): %v", w.offset, err)
		} else if gotHash.String() != hashStr {
			t.Errorf("mmap FindHash(%d) = %s, want %s", w.offset, gotHash, hashStr)
		}

		ok, err := miMmap.Contains(h)
		if err != nil || !ok {
			t.Errorf("mmap Contains(%s) = (%v, %v), want (true, nil)", hashStr, ok, err)
		}
	}
}

// hashFromString parses a hex hash string (as produced by plumbing.Hash's
// String method during the Entries() walk above) back into a
// plumbing.Hash. plumbing.NewHash returns the zero hash on malformed input
// rather than erroring; the strings here always come from a real Entries()
// walk, so a zero-hash result would itself be a test bug worth catching.
func hashFromString(t *testing.T, s string) plumbing.Hash {
	t.Helper()
	h := plumbing.NewHash(s)
	if h.IsZero() {
		t.Fatalf("hashFromString(%q) produced the zero hash — malformed input", s)
	}
	return h
}

// TestMmapIndexHandle_UnmapWhileSliceHeld_CausesCrash is a DELIBERATE
// negative control, run in a subprocess because it is EXPECTED to either
// crash the process (SIGSEGV/SIGBUS) or silently return corrupted data —
// both are real, observed manifestations of the same underlying UB (see
// below), and the test treats either as proof.
//
// It bypasses lockedIndex/mmapIndexHandle's pin protection entirely —
// grabbing a raw slice into an mmap'd index's Names block, recording its
// correct checksum while the mapping is still valid, then calling
// mapping.Unmap() directly while that slice is still held, and reading it
// again — to prove, not just assert, that the failure mode this package's
// refcounting exists to prevent (design doc §5.3's "genuinely hard part") is
// real and reachable if that refcounting is ever removed or bypassed. This
// directly mirrors the design doc §4.2 methodology: temporarily remove the
// protection, show the exact failure it prevents, put the protection back.
//
// Empirically (see this task's report), reading unmapped memory on this
// environment does NOT always SIGSEGV — occasionally the freed virtual
// address range gets silently reused by the Go runtime's own allocator for
// a fresh, zeroed heap arena before the read happens, so the process exits
// cleanly but the read returns all-zero bytes instead of the real (checksum-
// verified nonzero) data. That is itself the exact failure class this
// package's pin mechanism exists to prevent — SILENT CORRUPTION, not just a
// crash — so this test accepts either outcome as proof and only fails if
// the subprocess exits cleanly AND the data read back still matches the
// pre-unmap checksum (which would mean the UB, against all odds, didn't
// manifest at all this run).
func TestMmapIndexHandle_UnmapWhileSliceHeld_CausesCrash(t *testing.T) {
	if os.Getenv("GOGITSTORE_MMAP_CRASH_HELPER") == "1" {
		runMmapCrashHelper(t)
		return
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}

	cmd := safeexec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestMmapIndexHandle_UnmapWhileSliceHeld_CausesCrash$", "-test.v")
	cmd.Env = append(os.Environ(), "GOGITSTORE_MMAP_CRASH_HELPER=1")
	out, err := cmd.CombinedOutput()

	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("expected *exec.ExitError from the subprocess, got %T: %v\noutput:\n%s", err, err, out)
		}
		t.Logf("subprocess crashed reading unmapped memory (this IS the point of the test) — err=%v\noutput:\n%s", err, out)
		return
	}

	// The subprocess exited cleanly — this only proves the pin protection
	// is unnecessary if the data it read back was ALSO correct. Parse the
	// two checksums it printed and compare.
	var expected, actual int
	for _, line := range strings.Split(string(out), "\n") {
		if _, serr := fmt.Sscanf(line, "EXPECTED_SUM %d", &expected); serr == nil {
			continue
		}
		_, _ = fmt.Sscanf(line, "ACTUAL_SUM %d", &actual)
	}
	t.Logf("subprocess exited cleanly; pre-unmap checksum=%d post-unmap checksum=%d\noutput:\n%s", expected, actual, out)
	if expected == actual {
		t.Fatalf("subprocess exited cleanly AND read back the exact pre-unmap checksum — the UB this test exists to demonstrate did not manifest at all this run (neither a crash nor silent corruption). output:\n%s", out)
	}
	t.Logf("confirmed silent corruption instead of a crash: reading unmapped memory returned different bytes than before Unmap() — this is the same class of bug the pin mechanism prevents, manifesting as wrong data rather than a fault")
}

// runMmapCrashHelper is not itself a meaningful test assertion — it is only
// ever invoked as a SEPARATE PROCESS by the test above, specifically because
// it is expected to crash or corrupt data. See that test's doc comment.
func runMmapCrashHelper(t *testing.T) {
	dir, err := os.MkdirTemp("", "gogitstore-crash-*")
	if err != nil {
		t.Fatal(err)
	}
	buildPackedFixture(t, dir, 60)

	_, commonFs, _, commonDirAbs, err := resolveGitFilesystems(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := newSharedObjectStore(commonDirAbs, commonFs, nil, 0, false)
	packs, err := store.dir.ObjectPacks()
	if err != nil || len(packs) == 0 {
		t.Fatalf("no packs in fixture: %v", err)
	}

	handle, err := openMmapIndexHandle(commonDirAbs, packs[0])
	if err != nil {
		t.Fatal(err)
	}

	// Grab a raw slice into the mapping — exactly what lockedIndex's
	// Entries()-pinning mechanism exists to protect against becoming
	// dangling before it is fully read.
	var raw []byte
	for _, n := range handle.idx.Names {
		if len(n) > 0 {
			raw = n
			break
		}
	}
	if raw == nil {
		t.Fatal("fixture produced no non-empty Names bucket")
	}

	// Record the CORRECT checksum while the mapping is still valid — this
	// read is entirely legitimate (nothing has been unmapped yet).
	expectedSum := 0
	for _, b := range raw {
		expectedSum += int(b)
	}
	fmt.Println("EXPECTED_SUM", expectedSum)

	// Deliberately bypass the pin/refcount protection: unmap while `raw` is
	// still held and about to be read again. This is exactly the bug class
	// design doc §5.3 flags as the hard part; the real package API
	// (lockedIndex / pinnedEntryIter, plus index.go's unmappedLocked guard)
	// makes this unreachable through normal use.
	if err := handle.mapping.Unmap(); err != nil {
		t.Fatal(err)
	}
	_ = handle.file.Close()

	// Encourage the OS/Go runtime to actually reuse the freed address range
	// for something else rather than leaving it merely logically invalid —
	// this is what turns the UB into an observable outcome (a crash OR
	// silently-wrong data) instead of a no-op.
	junk := make([][]byte, 400)
	for i := range junk {
		junk[i] = make([]byte, 1<<20) // 400 x 1MiB
		for j := range junk[i] {
			junk[i][j] = byte(i)
		}
	}
	runtime.KeepAlive(junk)

	// This read is undefined behavior by design — it is what this whole
	// helper exists to trigger. It may SIGSEGV (most common on a fresh
	// unmap), or it may silently return different (e.g. zeroed, if the
	// freed VMA got reused for a fresh heap arena) bytes than expectedSum
	// above. Either way, the parent process's comparison against
	// EXPECTED_SUM is what actually proves the point.
	actualSum := 0
	for _, b := range raw {
		actualSum += int(b)
	}
	fmt.Println("ACTUAL_SUM", actualSum)
}
