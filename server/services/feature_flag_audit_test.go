package services

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/auditkit"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/fsops/memfs"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

const (
	auditTestFile = "/cfg/audit/" + auditkit.AuditFileName
	// flagTightening and flagLoosening are existing registry flags used as
	// stand-ins: one audited as a tightening flip, one with a loosening policy.
	flagTightening = "backlog:conversation-view"
	flagLoosening  = programCLIFlagProbeFlagName // default on; turning it off loosens
	flagHatch      = "backlog:sdd-default-pipeline"
)

type auditEnv struct {
	svc      *FeatureFlagService
	sink     *auditkit.AuditSink
	mfs      *memfs.MemFS
	clk      *gateTestClock
	timeout  chan time.Time
	mu       sync.Mutex
	degraded map[string]int
	warns    func(string) int
}

func newAuditEnv(t *testing.T) *auditEnv {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	e := &auditEnv{mfs: memfs.New(), clk: newGateTestClock(), timeout: make(chan time.Time, 1), degraded: map[string]int{}}
	lg, warns := newLogCapture()
	e.warns = warns
	e.sink = auditkit.NewAuditSink(func() (string, error) { return "/cfg", nil },
		auditkit.WithAuditFS(e.mfs), auditkit.WithAuditClock(e.clk.Now), auditkit.WithAuditLogger(lg),
		auditkit.WithAuditAfter(func(time.Duration) <-chan time.Time { return e.timeout }),
		auditkit.WithAuditDegradedCounter(func(m string) { e.mu.Lock(); e.degraded[m]++; e.mu.Unlock() }))
	e.svc = NewFeatureFlagService()
	e.svc.SetAudit(e.sink, map[string]featureflags.FlagAuditPolicy{
		flagTightening: {},
		flagLoosening:  {Loosening: func(enabled bool) bool { return !enabled }},
		flagHatch:      {Loosening: func(enabled bool) bool { return !enabled }, EscapeHatch: true},
	})
	t.Cleanup(e.sink.Close)
	return e
}

func (e *auditEnv) degradedCount(mode string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.degraded[mode]
}

func (e *auditEnv) flip(name string, enabled bool) error {
	_, err := e.svc.UpdateFeatureFlag(context.Background(),
		connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{Name: name, Enabled: enabled}))
	return err
}

func (e *auditEnv) lines(t *testing.T) []auditkit.AuditLine {
	t.Helper()
	raw, ok := e.mfs.Get(auditTestFile)
	if !ok || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var out []auditkit.AuditLine
	for _, l := range lines(raw) {
		var al auditkit.AuditLine
		require.NoError(t, json.Unmarshal(l, &al), string(l))
		out = append(out, al)
	}
	return out
}

func overrideOf(name string) (value, ok bool) {
	return config.LoadConfig().GetFeatureFlagOverride(name)
}

func TestTighteningFlip_ShouldPersistAndReturnWithoutWaitingOnTheSink_WhenFsyncIsStalled(t *testing.T) {
	e := newAuditEnv(t)
	e.mfs.SyncStall = make(chan struct{})
	e.mfs.SyncEntered = make(chan struct{}, 4)

	require.NoError(t, e.flip(flagTightening, true)) // returns although every fsync stalls
	v, ok := overrideOf(flagTightening)
	assert.True(t, ok && v, "the flip persisted")

	<-e.mfs.SyncEntered // the drain goroutine is now inside the stalled fsync
	close(e.mfs.SyncStall)
	e.sink.Close() // drains the queue

	got := e.lines(t)
	require.Len(t, got, 1)
	assert.Equal(t, auditkit.AuditPhaseResult, got[0].Phase)
	assert.Equal(t, auditkit.FlagOutcomeApplied, got[0].Outcome)
	require.NotNil(t, got[0].New)
	assert.True(t, *got[0].New)
}

func TestTighteningFlip_ShouldFallBackToMainLogRecordAndCount_WhenQueueIsFull(t *testing.T) {
	e := newAuditEnv(t)
	e.mfs.SyncStall = make(chan struct{})
	e.mfs.SyncEntered = make(chan struct{}, 1)

	e.sink.Enqueue(auditkit.AuditLine{Kind: "flag_change", Flag: "f0", Phase: auditkit.AuditPhaseResult})
	<-e.mfs.SyncEntered // one line is in flight, the queue itself is empty
	for i := 0; i < auditkit.AuditQueueSize+5; i++ {
		e.sink.Enqueue(auditkit.AuditLine{Kind: "flag_change", Flag: "f", Phase: auditkit.AuditPhaseResult, Seq: int64(i)})
	}
	assert.Equal(t, 5, e.warns("flag_change_audit_degraded"), "overflow lines go to the main log")
	assert.Equal(t, 5, e.degradedCount(auditkit.AuditModeFallbackLog))
	assert.Positive(t, e.degradedCount(auditkit.AuditModeQueued))
	close(e.mfs.SyncStall)
}

func TestLooseningFlip_ShouldReturnInternalAfterBoundWithNothingPersistedAndNotDelayATighteningFlip_WhenFsyncIsStalled(t *testing.T) {
	e := newAuditEnv(t)
	e.mfs.SyncStall = make(chan struct{})
	e.mfs.SyncEntered = make(chan struct{}, 4)

	errCh := make(chan error, 1)
	go func() { errCh <- e.flip(flagLoosening, false) }()
	<-e.mfs.SyncEntered // the loosening append is parked in fsync, before updateMu

	// A tightening flip is neither delayed by it nor blocked on updateMu.
	require.NoError(t, e.flip(flagTightening, true))

	e.timeout <- e.clk.Now() // the 2s bound fires
	err := <-errCh
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	_, ok := overrideOf(flagLoosening)
	assert.False(t, ok, "nothing persisted for the refused loosening flip")
	close(e.mfs.SyncStall)
}

func TestLooseningFlip_ShouldFailWithInternalAndZeroPersisted_WhenDiskIsFullButHatchFlipSucceedsWithFallback(t *testing.T) {
	e := newAuditEnv(t)
	e.mfs.SetFailWrite(errDiskFull)

	err := e.flip(flagLoosening, false)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), errDiskFull.Error(), "internal audit error text stays server-side")
	_, ok := overrideOf(flagLoosening)
	assert.False(t, ok)

	require.NoError(t, e.flip(flagHatch, false), "an escape-hatch flip persists with only the fallback record")
	v, ok := overrideOf(flagHatch)
	assert.True(t, ok && !v)
	assert.GreaterOrEqual(t, e.degradedCount(auditkit.AuditModeFallbackLog), 1)
}

func TestFlagChangeAudit_ShouldWriteLinkedRequestedAndResultLinesWithTruePreviousValue_WhenLooseningFlipOfDefaultOnFlagApplies(t *testing.T) {
	e := newAuditEnv(t)
	require.NoError(t, e.flip(flagLoosening, false))
	e.sink.Close()

	got := e.lines(t)
	require.Len(t, got, 2)
	req, res := got[0], got[1]
	assert.Equal(t, auditkit.AuditPhaseRequest, req.Phase)
	assert.Nil(t, req.Previous, "a requested line is an intent: no previous value")
	assert.Equal(t, req.ChangeID, res.ChangeID)
	assert.Equal(t, auditkit.AuditPhaseResult, res.Phase)
	assert.Equal(t, auditkit.FlagOutcomeApplied, res.Outcome)
	require.NotNil(t, res.Previous)
	assert.True(t, *res.Previous, "no persisted key: the previous value is the registered default (true), not false")
	assert.Equal(t, int64(1), res.Seq)
	assert.Equal(t, "global", res.Scope)
}

func TestFlagChangeAudit_ShouldRollBackByDeletingTheKeyAndRecordAbortedRolledBack_WhenControllerFailsOnDefaultOnFlag(t *testing.T) {
	e := newAuditEnv(t)
	e.svc.SetFeatureController(flagLoosening, &fakeFeatureController{enabled: true, failDisable: assert.AnError})

	err := e.flip(flagLoosening, false)
	require.Error(t, err)
	_, ok := overrideOf(flagLoosening)
	assert.False(t, ok, "rollback deletes the key; an explicit false would leave a safety flag off")
	assert.True(t, config.LoadConfig().GetFeatureFlagWithDefault(flagLoosening, true))
	e.sink.Close()

	got := e.lines(t)
	require.Len(t, got, 2)
	res := got[1]
	assert.Equal(t, auditkit.FlagOutcomeRolledBack, res.Outcome)
	require.NotNil(t, res.Previous)
	assert.True(t, *res.Previous)
	for _, l := range got {
		assert.NotEqual(t, auditkit.FlagOutcomeApplied, l.Outcome, "a failed flip leaves no applied line")
	}
}

func TestFlagChangeAudit_ShouldGiveEachFlipItsOwnChangeIDAndIncreasingSeq_WhenFlipsRace(t *testing.T) {
	e := newAuditEnv(t)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(on bool) {
			defer wg.Done()
			_ = e.flip(flagTightening, on)
		}(i%2 == 0)
	}
	wg.Wait()
	e.sink.Close()

	got := e.lines(t)
	require.Len(t, got, 6)
	ids := map[string]bool{}
	seqs := map[int64]bool{}
	for _, l := range got {
		ids[l.ChangeID] = true
		seqs[l.Seq] = true
	}
	assert.Len(t, ids, 6)
	assert.Len(t, seqs, 6)
	for s := int64(1); s <= 6; s++ {
		assert.True(t, seqs[s], "seq %d", s)
	}
}

func TestUpdateFeatureFlag_ShouldRollBackADefaultOnFlagByDeletion_WhenNoAuditIsConfigured(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	svc.SetFeatureController(flagLoosening, &fakeFeatureController{enabled: true, failDisable: assert.AnError})
	_, err := svc.UpdateFeatureFlag(context.Background(),
		connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{Name: flagLoosening, Enabled: false}))
	require.Error(t, err)
	_, ok := config.LoadConfig().GetFeatureFlagOverride(flagLoosening)
	assert.False(t, ok)
}
