package auditkit

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/fsops/memfs"
)

func TestAuditSink_ShouldCountQueuedDegradedOnlyWhenALineWaitsBehindAnother_WhenEnqueued(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	mfs.SyncStall = make(chan struct{})
	mfs.SyncEntered = make(chan struct{}, 1)
	var mu sync.Mutex
	queued := 0
	s, _, _ := newTestSink(t, mfs, WithAuditDegradedCounter(func(m string) {
		mu.Lock()
		defer mu.Unlock()
		if m == AuditModeQueued {
			queued++
		}
	}))
	count := func() int { mu.Lock(); defer mu.Unlock(); return queued }

	s.Enqueue(AuditLine{Kind: "flag_change", Seq: 1})
	<-mfs.SyncEntered // the drain goroutine holds line 1 inside the stalled fsync
	s.Enqueue(AuditLine{Kind: "flag_change", Seq: 2})
	assert.Equal(t, 0, count(), "one waiting line is not a backlog")
	s.Enqueue(AuditLine{Kind: "flag_change", Seq: 3})
	assert.Equal(t, 1, count(), "a second waiting line is")
	close(mfs.SyncStall)
}

func TestAuditSink_ShouldTakeBootSeqOneFromAZeroCounter_WhenCounterFileHoldsZero(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	mfs.Put("/cfg/audit/"+auditBootSeqName, []byte(" 0\n"))
	s, _, lg := newTestSink(t, mfs)

	require.NoError(t, s.Append(AuditLine{Kind: "flag_change"}))

	assert.Equal(t, "1", readLines(t, mfs)[0].BootSeq)
	assert.Zero(t, lg.count("WARN"), "zero is a valid counter")
	next, _ := mfs.Get("/cfg/audit/" + auditBootSeqName)
	assert.Equal(t, "1", string(next))
	assert.Empty(t, mfs.Names("/cfg/audit/"+auditBootSeqName+".tmp"), "the temp file is renamed away")
}

func TestAuditSink_ShouldReplaceAStaleBootSeqTempFile_WhenAPreviousStartCrashedMidWrite(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	mfs.Put("/cfg/audit/"+auditBootSeqName+".tmp", []byte("stale"))
	s, _, _ := newTestSink(t, mfs)

	require.NoError(t, s.Append(AuditLine{Kind: "flag_change"}))

	assert.Equal(t, "1", readLines(t, mfs)[0].BootSeq)
}

func TestAuditSink_ShouldLeaveNoBootSeqTempFile_WhenRenameFails(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	mfs.FailRename = errDiskFull
	s, _, _ := newTestSink(t, mfs)

	err := s.Append(AuditLine{Kind: "flag_change"})

	require.ErrorIs(t, err, ErrAuditFailed)
	assert.Empty(t, mfs.Names("/cfg/audit/"+auditBootSeqName), "neither the counter nor its temp file is left behind")
}

func TestAuditSink_ShouldFsyncTheCounterAndEveryLineBeforeReturning_WhenAppending(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	mfs.SyncStall = make(chan struct{})
	close(mfs.SyncStall)
	mfs.SyncEntered = make(chan struct{}, 8)
	s, _, _ := newTestSink(t, mfs)

	require.NoError(t, s.Append(AuditLine{Kind: "flag_change"}))
	assert.Len(t, mfs.SyncEntered, 2, "boot_seq temp file fsync plus the line fsync")
	require.NoError(t, s.Append(AuditLine{Kind: "flag_change"}))
	assert.Len(t, mfs.SyncEntered, 3, "one more fsync per line")
}

func TestAuditSink_ShouldWriteFallbackForEveryOverflowLine_WhenQueueIsFull(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	mfs.SyncStall = make(chan struct{})
	mfs.SyncEntered = make(chan struct{}, 1)
	s, _, lg := newTestSink(t, mfs)

	s.Enqueue(AuditLine{Kind: AuditKindFlagChg, Flag: "f0"})
	<-mfs.SyncEntered // one line is in flight, the queue itself is empty
	for i := 0; i < AuditQueueSize+3; i++ {
		s.Enqueue(AuditLine{Kind: AuditKindFlagChg, Flag: "f"})
	}

	assert.Equal(t, 3, lg.count("WARN flag_change_audit_degraded cause=queue full"))
	close(mfs.SyncStall)
}

func TestAuditSink_ShouldNotRotateAgain_WhenTheFirstAppendAfterRotationFits(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	s, _, _ := newTestSink(t, mfs)
	mfs.Put(testAuditFile, bytes.Repeat([]byte("L"), auditRotateBytes-10))

	require.NoError(t, s.Append(AuditLine{Kind: "flag_change", Flag: "a"}))
	require.NoError(t, s.Append(AuditLine{Kind: "flag_change", Flag: "b"}))

	rotated, _ := mfs.Get(testAuditFile + ".1")
	assert.Len(t, rotated, auditRotateBytes-10, "the full file rotates once and stays .1")
	assert.Len(t, readLines(t, mfs), 2, "both lines land in the new live file")
}

func TestAuditSink_ShouldStampUTC_WhenTheClockReturnsALocalZone(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	zone := time.FixedZone("x", 5*3600)
	local := time.Date(2026, 1, 2, 8, 4, 5, 0, zone)
	s := NewAuditSink(cfgDir, WithAuditFS(mfs), WithAuditClock(func() time.Time { return local }))
	defer s.Close()

	require.NoError(t, s.Append(AuditLine{Kind: "flag_change"}))

	got := readLines(t, mfs)[0]
	assert.Equal(t, "2026-01-02T03:04:05Z", got.TS)
	assert.Equal(t, "2026-01-02T03:04:05Z", got.BootTS)
}
