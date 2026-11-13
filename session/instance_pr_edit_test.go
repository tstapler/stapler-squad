package session

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// fakePRSetEditor is a persister that also implements the user-edit calls.
type fakePRSetEditor struct {
	fakePRPersister
	emu      sync.Mutex
	events   []string
	mirrors  []PRMirror
	editErr  error
	tombErr  error
	pinCalls []bool
}

func (f *fakePRSetEditor) record(e string) {
	f.emu.Lock()
	f.events = append(f.events, e)
	f.emu.Unlock()
}

func (f *fakePRSetEditor) DetachSessionPR(_ context.Context, _ string, ref domain.PRRef) error {
	f.record("detach " + ref.String())
	return f.editErr
}

func (f *fakePRSetEditor) SetSessionPRPinned(_ context.Context, _ string, ref domain.PRRef, pinned bool) error {
	f.record("pin " + ref.String())
	f.pinCalls = append(f.pinCalls, pinned)
	return f.editErr
}

func (f *fakePRSetEditor) ClearSessionPRTombstone(_ context.Context, _ string, ref domain.PRRef) error {
	f.record("untombstone " + ref.String())
	return f.tombErr
}

func (f *fakePRSetEditor) PersistPRMirror(_ string, url string, number int, terminal bool) error {
	f.emu.Lock()
	f.mirrors = append(f.mirrors, PRMirror{URL: url, Number: number, Terminal: terminal})
	f.emu.Unlock()
	return nil
}

func editorInstance(t *testing.T) (*Instance, *fakePRSetEditor) {
	t.Helper()
	inst := newPRTestInstance(t)
	ed := &fakePRSetEditor{}
	inst.SetPRPersister(ed)
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBranchDiscovery, domain.PRInitial{State: domain.PRStateOpen}))
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 13), domain.AttachSourceManual, domain.PRInitial{State: domain.PRStateOpen}))
	return inst, ed
}

func TestDetachPRPersisted_ShouldWriteRepositoryThenMemoryThenMirror(t *testing.T) {
	t.Parallel()
	inst, ed := editorInstance(t)
	r12 := mustPRRef(t, "", "acme", "app", 12)

	found, err := inst.DetachPRPersisted(context.Background(), r12)

	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []int{13}, attachNumbers(t, inst))
	assert.Equal(t, []string{"detach " + r12.String()}, ed.events)
	require.NotEmpty(t, ed.mirrors)
	assert.Equal(t, 13, ed.mirrors[len(ed.mirrors)-1].Number, "the stored mirror follows the remaining active PR")
}

func TestDetachPRPersisted_ShouldLeaveMemoryAlone_WhenRepositoryFails(t *testing.T) {
	t.Parallel()
	inst, ed := editorInstance(t)
	ed.editErr = errors.New("db down")

	found, err := inst.DetachPRPersisted(context.Background(), mustPRRef(t, "", "acme", "app", 12))

	require.Error(t, err)
	assert.False(t, found)
	assert.Equal(t, []int{12, 13}, attachNumbers(t, inst), "a failed user detach must not half-apply")
}

func TestDetachPRPersisted_ShouldReportNotFound_WhenPRNotAttached(t *testing.T) {
	t.Parallel()
	inst, ed := editorInstance(t)
	found, err := inst.DetachPRPersisted(context.Background(), mustPRRef(t, "", "acme", "app", 99))
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, ed.events, "nothing to detach, nothing written")
}

func TestPinPRPersisted_ShouldPinThenUnpin(t *testing.T) {
	t.Parallel()
	inst, ed := editorInstance(t)
	r12 := mustPRRef(t, "", "acme", "app", 12)
	pinsBefore, unpinsBefore := PRPinCount("pin"), PRPinCount("unpin")

	require.NoError(t, inst.PinPRPersisted(context.Background(), r12, true))
	active := domain.ActivePR(inst.Snapshot().GitHub.PRs, "")
	require.NotNil(t, active)
	assert.Equal(t, 12, active.Ref.Number(), "the pinned PR wins over the newer #13")

	require.NoError(t, inst.PinPRPersisted(context.Background(), r12, false))
	active = domain.ActivePR(inst.Snapshot().GitHub.PRs, "")
	assert.Equal(t, 13, active.Ref.Number())
	assert.Equal(t, []bool{true, false}, ed.pinCalls)
	assert.GreaterOrEqual(t, PRPinCount("pin"), pinsBefore+1, "ssq_pr_pin_total{action=pin}")
	assert.GreaterOrEqual(t, PRPinCount("unpin"), unpinsBefore+1, "ssq_pr_pin_total{action=unpin}")
}

func TestPinPRPersisted_ShouldNotPinInMemory_WhenRepositoryFails(t *testing.T) {
	t.Parallel()
	inst, ed := editorInstance(t)
	ed.editErr = errors.New("db down")
	require.Error(t, inst.PinPRPersisted(context.Background(), mustPRRef(t, "", "acme", "app", 12), true))
	assert.Equal(t, 13, domain.ActivePR(inst.Snapshot().GitHub.PRs, "").Ref.Number())
}

func TestAttachPRManual_ShouldClearTombstoneBeforeAttaching(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	ed := &fakePRSetEditor{}
	inst.SetPRPersister(ed)
	r30 := mustPRRef(t, "", "acme", "app", 30)

	require.NoError(t, inst.AttachPRManual(context.Background(), r30, domain.PRInitial{State: domain.PRStateOpen}))

	require.NotEmpty(t, ed.events)
	assert.Equal(t, "untombstone "+r30.String(), ed.events[0])
	assert.Equal(t, []int{30}, attachNumbers(t, inst))
	assert.Equal(t, domain.AttachSourceManual, inst.Snapshot().GitHub.PRs[0].AttachSource)
}

func TestPRSetEdits_ShouldFail_WhenPersisterCannotEdit(t *testing.T) {
	t.Parallel()
	inst := newPRTestInstance(t)
	inst.SetPRPersister(&fakePRPersister{})
	_, err := inst.DetachPRPersisted(context.Background(), mustPRRef(t, "", "acme", "app", 12))
	assert.ErrorIs(t, err, ErrPRPersisterUnavailable)
}
