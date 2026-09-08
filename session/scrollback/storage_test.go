package scrollback

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileScrollbackStorage_Write_RejectsSessionIDEscapingBasePath verifies
// getFilePath's containment check: a sessionID crafted to traverse outside
// basePath must cause Write to fail with an error rather than writing a
// scrollback file outside the intended directory. sessionID reaches here
// unsanitized from RPC/MCP fields (see getFilePath's doc comment), so a
// caller-supplied ".." segment is the realistic attack shape.
func TestFileScrollbackStorage_Write_RejectsSessionIDEscapingBasePath(t *testing.T) {
	basePath := t.TempDir()
	storage := NewFileScrollbackStorage(basePath, "none", 0)

	const maliciousSessionID = "../../etc/passwd"
	entries := []ScrollbackEntry{{Timestamp: time.Now(), Data: []byte("payload"), Sequence: 1}}

	err := storage.Write(maliciousSessionID, entries)
	require.Error(t, err, "Write must reject a sessionID that escapes basePath")

	// Confirm nothing was written outside basePath at the escaped location.
	escapedPath := filepath.Join(filepath.Dir(basePath), "etc", "passwd", "scrollback.jsonl")
	_, statErr := os.Stat(escapedPath)
	assert.True(t, os.IsNotExist(statErr), "no file should have been created at the escaped path %s", escapedPath)
}

// TestFileScrollbackStorage_Write_LegitimateSessionID_Succeeds is the control
// case: a normal sessionID must still be writable and readable under
// basePath.
func TestFileScrollbackStorage_Write_LegitimateSessionID_Succeeds(t *testing.T) {
	basePath := t.TempDir()
	storage := NewFileScrollbackStorage(basePath, "none", 0)

	const sessionID = "legit-session-123"
	entries := []ScrollbackEntry{{Timestamp: time.Now(), Data: []byte("hello world"), Sequence: 1}}

	err := storage.Write(sessionID, entries)
	require.NoError(t, err)

	wantPath := filepath.Join(basePath, sessionID, "scrollback.jsonl")
	_, statErr := os.Stat(wantPath)
	require.NoError(t, statErr, "scrollback file should exist at %s", wantPath)

	got, err := storage.Read(sessionID, 0, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "hello world", string(got[0].Data))
}

// TestFileScrollbackStorage_Read_RejectsSessionIDEscapingBasePath mirrors the
// Write case for the read path -- Read must also refuse to resolve a path
// outside basePath rather than silently returning no rows.
func TestFileScrollbackStorage_Read_RejectsSessionIDEscapingBasePath(t *testing.T) {
	basePath := t.TempDir()
	storage := NewFileScrollbackStorage(basePath, "none", 0)

	_, err := storage.Read("../../etc/passwd", 0, 10)
	require.Error(t, err, "Read must reject a sessionID that escapes basePath")
}

// TestFileScrollbackStorage_Truncate_AbortsOnCompressorCloseFailure covers
// the fix where a failed compressor/temp-file Close() during Truncate now
// aborts before the os.Rename that would otherwise silently replace the
// original file with truncated/corrupt data. It forces the zstd encoder's
// Close() specifically (not an earlier Write()) to fail by making tempPath a
// FIFO whose only reader closes immediately: the klauspost/compress zstd
// encoder buffers all Write() calls client-side and only performs its first
// real write to the underlying writer on Close() (verified empirically --
// unlike compress/gzip, whose Writer flushes on every Write call, which
// would fail at the write-entry step instead of the Close step this test
// targets), so every Encode() call during Truncate succeeds and only the
// final zstdWriter.Close() hits the closed pipe and fails.
func TestFileScrollbackStorage_Truncate_AbortsOnCompressorCloseFailure(t *testing.T) {
	basePath := t.TempDir()
	storage := NewFileScrollbackStorage(basePath, "zstd", 3)

	const sessionID = "truncate-close-failure"
	var entries []ScrollbackEntry
	for i := 0; i < 50; i++ {
		entries = append(entries, ScrollbackEntry{
			Timestamp: time.Now(),
			Data:      []byte("some scrollback line of text, padded to make the file non-trivially sized"),
			Sequence:  uint64(i),
		})
	}
	require.NoError(t, storage.Write(sessionID, entries))

	filePath, err := storage.getFilePath(sessionID)
	require.NoError(t, err)
	originalContent, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.NotEmpty(t, originalContent)

	// Open the FIFO for reading (which unblocks Truncate's os.OpenFile on the
	// write side) and close the read end immediately without consuming any
	// data, so the write side's eventual write fails with EPIPE. Truncate
	// runs synchronously on this goroutine, matching how it's actually
	// called in production -- empirically, running the writer on a spawned
	// goroutine instead (with the reader on the caller's goroutine) made the
	// write far more likely to slip into the kernel's pipe buffer before the
	// close was processed, rather than less.
	//
	// keepBytes is derived from the real compressed file size rather than a
	// hardcoded magic number, and deliberately kept strictly between two
	// failure zones observed empirically against this fixture:
	//   - too large (>= the compressed file size) and Truncate's own
	//     `stat.Size() <= keepBytes` early-return fires before ever opening
	//     tempPath, so the FIFO reader below blocks forever with no writer
	//     ever showing up -- a hang, not a fast failure, until Go's test
	//     timeout kills the whole package's test run.
	//   - too small (small enough that keepCount computes to 0) and the
	//     entry-encode loop below never calls Encode(), so the zstd encoder
	//     never buffers anything and its Close() may write nothing at all to
	//     the pipe -- no write, no EPIPE, and the test flakes (observed
	//     ~1-in-5 failures with a hardcoded keepBytes=10 against this
	//     50-entry fixture).
	// Halving the real compressed size keeps comfortably clear of both ends:
	// well below the file size (forces truncation) while still large enough
	// to keep several real entries (Close() has actual buffered data to
	// flush).
	keepBytes := int64(len(originalContent)) / 2
	require.Greater(t, keepBytes, int64(0), "fixture must compress to more than 2 bytes for this test to be meaningful")

	// Whether a write to an already-closed FIFO read end actually surfaces
	// as EPIPE depends on OS-level scheduling of the reader's close() versus
	// the writer's write() syscall -- empirically, even with the reader
	// closing as its very first instruction after the FIFO's blocking
	// open() calls rendezvous with Truncate's, the write still occasionally
	// (~1 in 10 runs observed on macOS) lands in the kernel's pipe buffer
	// before the close is processed and succeeds anyway. That's an
	// inherent property of the OS-level race this test depends on to
	// exercise Close()-failure handling, not a defect in the setup below,
	// so retry a bounded number of times rather than treating one spurious
	// "the write raced ahead and succeeded" outcome as a real failure --
	// same rationale as retrying a network-timing-dependent test.
	const maxAttempts = 8
	tempPath := filePath + ".tmp"
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		require.NoError(t, syscall.Mkfifo(tempPath, 0600), "must be able to create a FIFO at the temp path")

		readerDone := make(chan struct{})
		go func() {
			defer close(readerDone)
			f, ferr := os.OpenFile(tempPath, os.O_RDONLY, 0) // #nosec G304 -- test-controlled fixed path under t.TempDir()
			if ferr != nil {
				return
			}
			_ = f.Close()
		}()
		err = storage.Truncate(sessionID, keepBytes)

		// A bounded wait here, not an unconditional <-readerDone: if
		// Truncate took the stat.Size() <= keepBytes early-return path
		// above (i.e. keepBytes was miscalculated to be >= the real
		// compressed size), it returns immediately without ever opening
		// tempPath, and the reader goroutine's blocking open(O_RDONLY) --
		// which only unblocks once a writer opens the other end -- would
		// otherwise hang forever with no writer ever showing up. Failing
		// fast here with a clear message beats hanging until Go's
		// whole-package test timeout (10 minutes) kills the run with a
		// much less legible stack dump.
		select {
		case <-readerDone:
		case <-time.After(5 * time.Second):
			t.Fatal("FIFO reader never unblocked -- Truncate likely took its stat.Size() <= keepBytes early-return path without ever opening tempPath (keepBytes may be too large relative to the fixture's actual compressed size)")
		}

		if err != nil {
			break // the race landed the way this test needs; proceed to the assertions below.
		}
		if attempt == maxAttempts {
			t.Fatalf("write never lost the race to the reader's close() across %d attempts -- Truncate succeeded every time instead of hitting the zstd writer's Close() failure this test exercises", maxAttempts)
		}
		// Truncate succeeded (the write raced ahead of the close): reset
		// the fixture's on-disk state and retry. A successful Truncate
		// renames tempPath -- which is a FIFO, not a regular file -- onto
		// filePath, so filePath itself is now a FIFO with no reader.
		// storage.Write opens filePath with O_WRONLY (no O_TRUNC), which
		// would block forever against that orphaned FIFO; remove it first
		// so Write's O_CREATE produces a fresh regular file instead.
		require.NoError(t, os.Remove(filePath))
		require.NoError(t, storage.Write(sessionID, entries))
	}

	require.Error(t, err, "Truncate must return an error when the zstd writer's Close() fails")
	assert.Contains(t, err.Error(), "failed to close zstd writer")

	// (a) original file left untouched -- no corrupt/truncated data was
	// renamed over it.
	afterContent, readErr := os.ReadFile(filePath)
	require.NoError(t, readErr)
	assert.Equal(t, originalContent, afterContent, "original scrollback file must be unchanged after a failed Truncate")

	// (b) the temp file (FIFO) does not leak -- the deferred os.Remove(tempPath)
	// must still run on this error path.
	_, statErr := os.Stat(tempPath)
	assert.True(t, os.IsNotExist(statErr), "temp path should have been removed even though Truncate failed")
}
