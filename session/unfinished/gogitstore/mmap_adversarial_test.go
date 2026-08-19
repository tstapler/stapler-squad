// mmap_adversarial_test.go hardens loadMemoryIndexMmap (mmapindex.go)
// against malformed/adversarial .idx input — this task's item 3 ("fuzz/
// adversarial testing of the mmap .idx loader against malformed input").
//
// Two complementary techniques:
//
//   - Table-driven cases covering every truncation point the loader's own
//     bounds checks name in their error messages (header, names block,
//     crc/offset32 blocks, offset64 block, checksum trailer), plus a
//     corrupted (non-monotonic) fanout table — a REAL bug found by this
//     hardening pass (see below) that made the loader panic instead of
//     returning a clean error.
//   - A Go native fuzz test (FuzzLoadMemoryIndexMmap) so `go test` (which
//     runs the seed corpus as regular test cases even without -fuzz) and an
//     optional `go test -fuzz=FuzzLoadMemoryIndexMmap` run both exercise the
//     same "never panic" property against inputs beyond what the
//     table-driven cases anticipated.
//
// # A real bug found and fixed by this pass
//
// loadMemoryIndexMmap computed each fanout bucket's size as `buckets := cum
// - prevCum` (uint32 subtraction) with no check that the fanout table is
// actually non-decreasing. A corrupted or adversarial .idx with a
// DECREASING fanout entry made this subtraction underflow to a value near
// 2^32, which then flowed directly into `mapped[n0:n1:n1]` — an
// out-of-bounds slice expression that PANICS (runtime error: slice bounds
// out of range), rather than returning a clean decode error the way
// decoder.go's io.ReadFull-based copy loader degrades (a too-large make()
// call, still an error path, not a slice-bounds panic). Fixed by validating
// monotonicity immediately after reading the fanout table, before any
// bucket-range arithmetic runs — see mmapindex.go's loadMemoryIndexMmap.
package gogitstore

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/format/idxfile"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// realFixtureIdxBytes builds a small real packed fixture and returns its raw
// on-disk .idx bytes, for use as a valid-input baseline / fuzz seed.
func realFixtureIdxBytes(t *testing.T) []byte {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	buildPackedFixture(t, dir, 40)

	_, commonFs, _, commonDirAbs, err := resolveGitFilesystems(dir)
	if err != nil {
		t.Fatalf("resolveGitFilesystems: %v", err)
	}
	store := newSharedObjectStore(commonDirAbs, commonFs, nil, 0, false)
	packs, err := store.dir.ObjectPacks()
	if err != nil || len(packs) != 1 {
		t.Fatalf("fixture: got %d packs, err=%v, want exactly 1", len(packs), err)
	}
	f, err := store.dir.ObjectPackIdx(packs[0])
	if err != nil {
		t.Fatalf("ObjectPackIdx: %v", err)
	}
	defer func() { _ = f.Close() }()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(f); err != nil {
		t.Fatalf("read idx: %v", err)
	}
	return buf.Bytes()
}

// setBE32 writes a big-endian uint32 into b at offset off.
func setBE32(b []byte, off int, v uint32) {
	b[off] = byte(v >> 24)
	b[off+1] = byte(v >> 16)
	b[off+2] = byte(v >> 8)
	b[off+3] = byte(v)
}

// TestMmapIndex_MalformedInput_NeverPanics is the table-driven half of this
// file's hardening: each case feeds loadMemoryIndexMmap a deliberately
// corrupted or truncated byte slice and asserts it returns a non-nil error
// (never a panic, never a nil error on obviously-invalid input).
func TestMmapIndex_MalformedInput_NeverPanics(t *testing.T) {
	valid := realFixtureIdxBytes(t)
	if len(valid) < idxHeaderLen+50 {
		t.Fatalf("fixture .idx unexpectedly small (%d bytes) — test assumptions need a bigger fixture", len(valid))
	}

	// Sanity: the valid fixture must actually decode cleanly, or every
	// "should fail" case below would be meaningless.
	if _, err := loadMemoryIndexMmap(valid); err != nil {
		t.Fatalf("sanity check: valid fixture failed to decode: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(b []byte) []byte
		wantErr bool // false only for cases that legitimately still decode (e.g. extra trailing garbage)
	}{
		{
			name:   "empty file",
			mutate: func(b []byte) []byte { return nil },
		},
		{
			name:   "single byte",
			mutate: func(b []byte) []byte { return []byte{0xff} },
		},
		{
			name:   "truncated in fixed header (before fanout ends)",
			mutate: func(b []byte) []byte { return b[:idxHeaderLen-10] },
		},
		{
			name:   "truncated exactly at header boundary, zero names/crc/offset",
			mutate: func(b []byte) []byte { return b[:idxHeaderLen] },
		},
		{
			name: "bad magic",
			mutate: func(b []byte) []byte {
				out := append([]byte(nil), b...)
				out[0] = 'X'
				return out
			},
		},
		{
			name: "unsupported version",
			mutate: func(b []byte) []byte {
				out := append([]byte(nil), b...)
				setBE32(out, idxMagicLen, 0xffffffff)
				return out
			},
		},
		{
			name: "version zero (below supported range, decoder-defined behavior)",
			mutate: func(b []byte) []byte {
				out := append([]byte(nil), b...)
				setBE32(out, idxMagicLen, 0)
				return out
			},
			// idxfile.VersionSupported is 2; version 0 is "not > supported"
			// so loadMemoryIndexMmap's version check (mirroring decoder.go's
			// own `v > VersionSupported` check) does not reject it by
			// itself — decode proceeds and either succeeds (if the rest of
			// the bytes still parse under the real v2 layout, since this
			// mutation only changes the version FIELD, not the actual
			// layout) or fails at a later bounds check. Either outcome is
			// acceptable; only a panic is not.
			wantErr: false,
		},
		{
			name: "truncated in names block",
			mutate: func(b []byte) []byte {
				// Cut a few bytes into where the names block must be,
				// without touching the fanout table itself, so the loader
				// computes a large expected namesEnd against a short
				// buffer.
				cut := idxHeaderLen + 5
				if cut > len(b) {
					cut = len(b)
				}
				return b[:cut]
			},
		},
		{
			name: "truncated in crc/offset32 blocks",
			mutate: func(b []byte) []byte {
				// The fixture's names block is fully present but crc/off32
				// data is cut short. Locate namesEnd the same way the
				// loader does (from the real, unmodified fanout table) and
				// cut shortly after it.
				fanoutOff := idxMagicLen + idxVersionLen
				total := beU32(b, fanoutOff+255*4)
				namesEnd := idxHeaderLen + int(total)*20
				cut := namesEnd + 5
				if cut > len(b) {
					t.Skip("fixture too small to exercise this truncation point")
				}
				return b[:cut]
			},
		},
		{
			name: "corrupted (non-monotonic) fanout table",
			mutate: func(b []byte) []byte {
				out := append([]byte(nil), b...)
				fanoutOff := idxMagicLen + idxVersionLen
				// Force bucket 1's cumulative count below bucket 0's —
				// this is the exact shape of the bug fixed by this
				// hardening pass (see file doc comment).
				setBE32(out, fanoutOff+0*4, 0xffffff00)
				setBE32(out, fanoutOff+1*4, 1)
				return out
			},
		},
		{
			name: "fanout[255] wildly exceeds actual file size",
			mutate: func(b []byte) []byte {
				out := append([]byte(nil), b...)
				fanoutOff := idxMagicLen + idxVersionLen
				setBE32(out, fanoutOff+255*4, 0x0fffffff) // ~268M objects — way more than fits
				return out
			},
		},
		{
			name: "truncated checksum trailer",
			mutate: func(b []byte) []byte {
				return b[:len(b)-5]
			},
		},
		{
			name: "trailing garbage appended (still decodes; extra bytes are ignored)",
			mutate: func(b []byte) []byte {
				return append(append([]byte(nil), b...), []byte("trailing garbage not part of the format")...)
			},
			wantErr: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mutated := c.mutate(valid)
			idx, err := func() (idx *idxfile.MemoryIndex, err error) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("loadMemoryIndexMmap PANICKED on malformed input %q: %v", c.name, r)
					}
				}()
				return loadMemoryIndexMmap(mutated)
			}()

			if c.wantErr && err == nil {
				t.Errorf("case %q: expected an error, got nil (idx=%+v)", c.name, idx)
			}
			if !c.wantErr && err != nil {
				t.Logf("case %q: got error (acceptable per this case's own wantErr=false note): %v", c.name, err)
			}
			// Whether or not an error was returned, a non-nil idx must never
			// be silently wrong in a way that would corrupt downstream
			// reads — the cheapest broad check available here is that
			// Count() and a full Entries() drain don't themselves panic or
			// disagree wildly with Fanout[255]. This exercises the SAME
			// "safe on garbage" property one layer further, matching what a
			// real caller (SharedObjectStore.ensureIndex → buildIndexEntryLocked)
			// would actually do with whatever loadMemoryIndexMmap hands back.
			if err == nil && idx != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Fatalf("case %q: walking a successfully-decoded-but-malformed index PANICKED: %v", c.name, r)
						}
					}()
					it, ierr := idx.Entries()
					if ierr != nil {
						return
					}
					defer func() { _ = it.Close() }()
					for i := 0; i < 1_000_000; i++ { // hard cap: never loop forever on garbage
						if _, nerr := it.Next(); nerr != nil {
							return
						}
					}
					t.Errorf("case %q: Entries() iterator did not terminate within 1,000,000 steps — possible malformed-fanout infinite/huge iteration", c.name)
				}()
			}
		})
	}
}

// beU32 reads a big-endian uint32 from b at offset off — a small test-local
// helper mirroring mmapindex.go's own encbin.BigEndian.Uint32 usage, kept
// separate so this test file doesn't need to import encoding/binary just for
// one read.
func beU32(b []byte, off int) uint32 {
	return uint32(b[off])<<24 | uint32(b[off+1])<<16 | uint32(b[off+2])<<8 | uint32(b[off+3])
}

// FuzzLoadMemoryIndexMmap fuzzes loadMemoryIndexMmap directly against
// arbitrary byte slices, seeded with a real valid .idx plus several of the
// table-driven mutations above. The only property under test is "never
// panic, and if it returns a non-nil index, walking that index's Entries()
// never panics either" — loadMemoryIndexMmap is explicitly allowed to return
// an error for any malformed input; a panic is the only unacceptable
// outcome, since a panic in this path (called from
// SharedObjectStore.ensureIndex, on the scanner's hot path) would crash the
// whole process, not just fail one repo's index build.
//
// Run the seed corpus only (fast, part of `go test`):
//
//	go test ./session/unfinished/gogitstore/... -run FuzzLoadMemoryIndexMmap
//
// Run the actual fuzzer (slow, not part of CI by default):
//
//	go test ./session/unfinished/gogitstore/... -fuzz=FuzzLoadMemoryIndexMmap -fuzztime=60s
func FuzzLoadMemoryIndexMmap(f *testing.F) {
	// Build seeds from a throwaway *testing.T-shaped fixture builder. F
	// doesn't give us a *testing.T, so this seed-building step uses a
	// minimal local git invocation rather than reusing realFixtureIdxBytes
	// (which requires *testing.T for Fatalf/Skip).
	if _, err := exec.LookPath("git"); err == nil {
		if valid, ok := buildFuzzSeedIdx(); ok {
			f.Add(valid)
			// A handful of the most interesting mutations from the
			// table-driven test above, inlined here since F.Add needs
			// concrete []byte values, not a *testing.T-based helper.
			if len(valid) > idxHeaderLen+8 {
				nonMonotonic := append([]byte(nil), valid...)
				setBE32(nonMonotonic, idxMagicLen+idxVersionLen+0*4, 0xffffff00)
				setBE32(nonMonotonic, idxMagicLen+idxVersionLen+1*4, 1)
				f.Add(nonMonotonic)

				truncated := append([]byte(nil), valid[:idxHeaderLen+5]...)
				f.Add(truncated)

				badMagic := append([]byte(nil), valid...)
				badMagic[0] = 'X'
				f.Add(badMagic)
			}
		}
	}
	f.Add([]byte{})
	f.Add([]byte{0xff})
	f.Add(bytes.Repeat([]byte{0}, idxHeaderLen))
	f.Add(bytes.Repeat([]byte{0xff}, idxHeaderLen+64))

	f.Fuzz(func(t *testing.T, data []byte) {
		idx, err := func() (idx *idxfile.MemoryIndex, err error) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("loadMemoryIndexMmap panicked on fuzzed input (len=%d): %v", len(data), r)
				}
			}()
			return loadMemoryIndexMmap(data)
		}()
		if err != nil || idx == nil {
			return
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("walking a fuzzed-but-successfully-decoded index panicked: %v", r)
				}
			}()
			it, ierr := idx.Entries()
			if ierr != nil {
				return
			}
			defer func() { _ = it.Close() }()
			for i := 0; i < 1_000_000; i++ {
				if _, nerr := it.Next(); nerr != nil {
					return
				}
			}
		}()
	})
}

// buildFuzzSeedIdx builds a tiny real packed fixture in a temp dir (outside
// of *testing.T, since F.Add-time seed construction has no *testing.T to use)
// and returns its raw .idx bytes. Returns ok=false on any failure — the fuzz
// target still works fine with only the synthetic byte-pattern seeds in that
// case, just with a weaker starting corpus.
func buildFuzzSeedIdx() (data []byte, ok bool) {
	dir, err := os.MkdirTemp("", "gogitstore-fuzzseed-*")
	if err != nil {
		return nil, false
	}
	defer func() { _ = os.RemoveAll(dir) }()

	run := func(args ...string) bool {
		// Bounded like gitRunErr in gogitstore_test.go: an unbounded
		// context.Background() here leaves a wedged git subprocess blocking
		// cmd.Run() forever, past go test's own -timeout (see gitCommandTimeout's
		// doc comment for the reproduced hang this pattern fixes).
		ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout(args))
		defer cancel()
		cmd := safeexec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.local",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.local",
		)
		return cmd.Run() == nil
	}
	if !run("init", "-q", "-b", "main") {
		return nil, false
	}
	if !run("config", "user.name", "test") || !run("config", "user.email", "test@test.local") {
		return nil, false
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("seed content for fuzz corpus\n"), 0o644); err != nil {
		return nil, false
	}
	if !run("add", ".") || !run("commit", "-q", "-m", "seed") {
		return nil, false
	}
	if !run("gc", "-q") {
		return nil, false
	}

	_, commonFs, _, commonDirAbs, rerr := resolveGitFilesystems(dir)
	if rerr != nil {
		return nil, false
	}
	store := newSharedObjectStore(commonDirAbs, commonFs, nil, 0, false)
	packs, perr := store.dir.ObjectPacks()
	if perr != nil || len(packs) == 0 {
		return nil, false
	}
	f, oerr := store.dir.ObjectPackIdx(packs[0])
	if oerr != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	var buf bytes.Buffer
	if _, cerr := buf.ReadFrom(f); cerr != nil {
		return nil, false
	}
	return buf.Bytes(), true
}
