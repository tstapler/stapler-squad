package auditkit

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/fsops/memfs"
)

var boundaryLine = AuditLine{Kind: "flag_change", Flag: "boundary"}

// stampedLen is the byte length of one stamped, newline-terminated boundaryLine.
func stampedLen(t *testing.T) int {
	t.Helper()
	mfs := memfs.New()
	s, _, _ := newTestSink(t, mfs)
	require.NoError(t, s.Append(boundaryLine))
	raw, _ := mfs.Get(testAuditFile)
	return len(raw)
}

// A line that makes the file exactly 5 MiB stays in the live file;
// one byte more rotates. Pinned for the check at open (rotateLocked) and the
// check on a live handle (write).
func TestAuditSink_ShouldRotateOnlyWhenTheLineExceedsTheLimit_WhenFileIsPrefilled(t *testing.T) {
	t.Parallel()
	l := stampedLen(t)
	for _, tc := range []struct {
		name    string
		delta   int
		rotates bool
	}{{"limit-1", -1, false}, {"limit", 0, false}, {"limit+1", 1, true}} {
		t.Run("open/"+tc.name, func(t *testing.T) {
			t.Parallel()
			mfs := memfs.New()
			mfs.Put(testAuditFile, bytes.Repeat([]byte("p"), (5<<20)-l+tc.delta))
			s, _, _ := newTestSink(t, mfs)

			require.NoError(t, s.Append(boundaryLine))

			_, rotated := mfs.Get(testAuditFile + ".1")
			assert.Equal(t, tc.rotates, rotated)
		})
		t.Run("live/"+tc.name, func(t *testing.T) {
			t.Parallel()
			mfs := memfs.New()
			mfs.Put(testAuditFile, bytes.Repeat([]byte("p"), (5<<20)-2*l+tc.delta))
			s, _, _ := newTestSink(t, mfs)
			require.NoError(t, s.Append(boundaryLine)) // opens; the file is now limit-l+delta bytes
			_, rotated := mfs.Get(testAuditFile + ".1")
			require.False(t, rotated)

			require.NoError(t, s.Append(boundaryLine)) // the checked append

			_, rotated = mfs.Get(testAuditFile + ".1")
			assert.Equal(t, tc.rotates, rotated)
		})
	}
}
