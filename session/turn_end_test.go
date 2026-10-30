package session

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lockCheckingFake struct {
	inst  *Instance
	mu    sync.Mutex
	calls int
	// lockFree records whether i.mu could be write-locked inside the callback.
	lockFree bool
}

func (f *lockCheckingFake) observe() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lockFree = f.inst.mu.TryLock()
	if f.lockFree {
		f.inst.mu.Unlock()
	}
}

func (f *lockCheckingFake) PersistTurnEnded(string, TurnEnded) error { f.observe(); return nil }

type fakeRecorder struct {
	noopSessionEventRecorder
	lockCheckingFake
}

func (f *fakeRecorder) RecordTurnEnded(string, TurnEnded) { f.observe() }

func TestRecordTurnEnd_Should_SetSnapshotFactAndNotPanic_When_InstanceIsBareLiteral(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "bare", Status: Running}

	inst.RecordTurnEnd("finished", TurnEndSourceHook)

	assert.Equal(t, "finished", inst.Snapshot().TurnEnded.Excerpt)
	assert.Equal(t, TurnEndSourceHook, inst.Snapshot().TurnEnded.Source)
}

func TestRecordTurnEnd_Should_SetSnapshotFactAndNotPanic_When_InstanceFromInstanceData(t *testing.T) {
	t.Parallel()
	inst, err := FromInstanceDataDeferred(InstanceData{Title: "restored", Path: t.TempDir(), Status: Paused, Program: "claude"})
	require.NoError(t, err)

	inst.RecordTurnEnd("finished", TurnEndSourceScreen)

	assert.Equal(t, "finished", inst.Snapshot().TurnEnded.Excerpt)
}

func TestRecordTurnEnd_Should_CallSinkAndRecorderOnceAfterReleasingLock_When_CollaboratorsSet(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "wired", Status: Running}
	sink := &lockCheckingFake{inst: inst}
	rec := &fakeRecorder{lockCheckingFake: lockCheckingFake{inst: inst}}
	inst.SetTurnEndSink(sink)
	inst.SetEventRecorder(rec)

	inst.RecordTurnEnd("done", TurnEndSourceHook)

	assert.Equal(t, 1, sink.calls)
	assert.True(t, sink.lockFree, "sink ran under i.mu")
	assert.Equal(t, 1, rec.calls)
	assert.True(t, rec.lockFree, "recorder ran under i.mu")
}

func TestRecordTurnEnd_Should_SkipSinkAndRecorder_When_ActorStopped(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "stopped", Status: Running}
	sink := &lockCheckingFake{inst: inst}
	rec := &fakeRecorder{lockCheckingFake: lockCheckingFake{inst: inst}}
	inst.SetTurnEndSink(sink)
	inst.SetEventRecorder(rec)
	NewLiveInstance(inst).Stop()

	inst.RecordTurnEnd("late", TurnEndSourceHook)

	assert.Equal(t, 0, sink.calls)
	assert.Equal(t, 0, rec.calls)
}

func TestSetTurnEndSink_Should_KeepNoopDefault_When_TypedNilPassed(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "typednil", Status: Running}
	var sink *lockCheckingFake
	inst.SetTurnEndSink(sink)

	inst.RecordTurnEnd("done", TurnEndSourceHook)

	assert.Equal(t, "done", inst.Snapshot().TurnEnded.Excerpt)
}
