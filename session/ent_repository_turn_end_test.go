package session

import (
	"context"
	"math/rand"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var turnEndBase = time.Date(2026, 10, 24, 14, 2, 11, 123456789, time.UTC)

func turnAt(session string, offset time.Duration, kind TurnEndKind) TurnEnded {
	return FromHook(session, "Which approach, A or B?", turnEndBase.Add(offset), kind, TurnEndConfidenceHigh)
}

func openDiskStorage(t *testing.T, path string) *Storage {
	t.Helper()
	repo, err := NewEntRepository(WithDatabasePath(path))
	require.NoError(t, err)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage
}

func storedRow(t *testing.T, s *Storage, title string) InstanceData {
	t.Helper()
	rows, err := s.ListInstanceData()
	require.NoError(t, err)
	for _, r := range rows {
		if r.Title == title {
			return r
		}
	}
	t.Fatalf("session %q not stored", title)
	return InstanceData{}
}

func assertSameTurn(t *testing.T, want, got TurnEnded) {
	t.Helper()
	assert.Equal(t, want.Episode, got.Episode)
	assert.Equal(t, want.Kind, got.Kind)
	assert.Equal(t, want.Confidence, got.Confidence)
	assert.Equal(t, want.Excerpt, got.Excerpt)
	assert.Equal(t, want.Source, got.Source)
	assert.True(t, want.At.Equal(got.At), "At: want %v got %v", want.At, got.At)
}

func TestTurnEnd_RoundTrip_Should_RestoreIdenticalFactAndAck(t *testing.T) {
	t.Parallel()
	s := openDiskStorage(t, filepath.Join(t.TempDir(), "s.db"))
	defer s.Close()
	inst := newTestInstance("demo")
	turn := turnAt("demo", 0, TurnEndKindQuestion)
	inst.TurnEnded = turn
	inst.AckedEpisodeID = EpisodeID("ep-acked")
	require.NoError(t, s.AddInstance(inst))

	restored, err := FromInstanceDataDeferred(storedRow(t, s, "demo"))
	require.NoError(t, err)

	assertSameTurn(t, turn, restored.Snapshot().TurnEnded)
	assert.Equal(t, EpisodeID("ep-acked"), restored.Snapshot().AckedEpisodeID)
}

func TestTurnEnd_LegacyRow_Should_LoadZeroValue(t *testing.T) {
	t.Parallel()
	s := openDiskStorage(t, filepath.Join(t.TempDir(), "s.db"))
	defer s.Close()
	require.NoError(t, s.AddInstance(newTestInstance("legacy")))

	restored, err := FromInstanceDataDeferred(storedRow(t, s, "legacy"))
	require.NoError(t, err)

	assert.True(t, restored.Snapshot().TurnEnded.IsZero())
	assert.Empty(t, restored.Snapshot().AckedEpisodeID)
}

func TestAckedEpisode_ClearBranch_Should_PersistEmptyAfterNewTurn(t *testing.T) {
	t.Parallel()
	s := openDiskStorage(t, filepath.Join(t.TempDir(), "s.db"))
	defer s.Close()
	inst := newTestInstance("demo")
	inst.TurnEnded = turnAt("demo", 0, TurnEndKindQuestion)
	inst.AckedEpisodeID = inst.TurnEnded.Episode
	inst.LastAcknowledged = turnEndBase.Add(time.Second)
	require.NoError(t, s.AddInstance(inst))
	require.Equal(t, inst.TurnEnded.Episode, storedRow(t, s, "demo").AckedEpisodeID)

	// RecordTurnEnd with a new episode clears the in-memory ack; the full save must persist the clear.
	inst.RecordTurnEnd("Anything else?", TurnEndSourceHook)
	require.Empty(t, inst.Snapshot().AckedEpisodeID)
	require.NoError(t, s.UpdateInstance(inst))

	row := storedRow(t, s, "demo")
	assert.Empty(t, row.AckedEpisodeID)
	assert.Equal(t, inst.Snapshot().TurnEnded.Episode, row.TurnEnded.Episode)
}

func TestTurnEnd_NarrowWriter_Should_SurviveReopenWithoutFullSave(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s.db")
	s := openDiskStorage(t, path)
	inst := newTestInstance("demo")
	require.NoError(t, s.AddInstance(inst))
	inst.SetTurnEndSink(NewTurnEndSink(s))

	inst.RecordTurnEnd("Which approach, A or B?", TurnEndSourceHook)
	require.NoError(t, s.Close())

	s2 := openDiskStorage(t, path)
	defer s2.Close()
	assertSameTurn(t, inst.Snapshot().TurnEnded, storedRow(t, s2, "demo").TurnEnded)

	// A following zero-value write exercises the Clear* branch.
	require.NoError(t, s2.UpdateInstanceTurnEnded("demo", TurnEnded{}))
	assert.True(t, storedRow(t, s2, "demo").TurnEnded.IsZero())
}

func TestAckedEpisode_NarrowWriter_Should_SurviveReopenWithoutFullSave(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s.db")
	s := openDiskStorage(t, path)
	inst := newTestInstance("demo")
	inst.TurnEnded = turnAt("demo", 0, TurnEndKindQuestion)
	require.NoError(t, s.AddInstance(inst))

	ep := inst.Snapshot().TurnEnded.Episode
	ackedAt := inst.MarkAcknowledged(ep)
	require.NoError(t, s.UpdateInstanceAckedEpisode("demo", ep, ackedAt))
	require.NoError(t, s.Close())

	s2 := openDiskStorage(t, path)
	defer s2.Close()
	row := storedRow(t, s2, "demo")
	assert.Equal(t, ep, row.AckedEpisodeID)
	assert.True(t, ackedAt.Equal(row.LastAcknowledged))
	restored, err := FromInstanceDataDeferred(row)
	require.NoError(t, err)
	assert.Equal(t, ep, restored.Snapshot().AckedEpisodeID)
	assert.Equal(t, restored.Snapshot().TurnEnded.Episode, restored.Snapshot().AckedEpisodeID,
		"restored open question is already acked")
}

func TestAckedEpisode_MarkAcknowledged_Should_KeepMemorySnapshotAndReturnedTimeAligned(t *testing.T) {
	t.Parallel()
	inst := newTestInstance("demo")
	at := inst.MarkAcknowledged("E1")
	snap := inst.Snapshot()
	assert.Equal(t, EpisodeID("E1"), snap.AckedEpisodeID)
	assert.True(t, at.Equal(snap.LastAcknowledged))
}

func TestTurnEnd_RecordTurnEnd_Should_ClearAckOnlyForNewEpisode(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "demo", Status: Running}
	inst.RecordTurnEnd("first?", TurnEndSourceHook)
	ep := inst.Snapshot().TurnEnded.Episode
	inst.MarkAcknowledged(ep)
	assert.Equal(t, ep, inst.Snapshot().AckedEpisodeID)

	time.Sleep(2 * time.Millisecond) // distinct episode id needs a distinct At
	inst.RecordTurnEnd("second?", TurnEndSourceHook)
	assert.NotEqual(t, ep, inst.Snapshot().TurnEnded.Episode)
	assert.Empty(t, inst.Snapshot().AckedEpisodeID)
}

func TestTurnEnd_UpdateTurnEnded_Should_BeMonotonicAtSubSecondPrecision(t *testing.T) {
	t.Parallel()
	s := openDiskStorage(t, filepath.Join(t.TempDir(), "s.db"))
	defer s.Close()
	require.NoError(t, s.AddInstance(newTestInstance("demo")))
	newer := turnAt("demo", 500*time.Millisecond, TurnEndKindQuestion)
	older := turnAt("demo", 5*time.Millisecond, TurnEndKindFinished)
	oldest := turnAt("demo", -time.Second, TurnEndKindError)

	require.NoError(t, s.UpdateInstanceTurnEnded("demo", older))
	require.NoError(t, s.UpdateInstanceTurnEnded("demo", newer))
	require.NoError(t, s.UpdateInstanceTurnEnded("demo", older), "stale write is a successful no-op")
	require.NoError(t, s.UpdateInstanceTurnEnded("demo", oldest))

	assertSameTurn(t, newer, storedRow(t, s, "demo").TurnEnded)
	assert.Error(t, s.UpdateInstanceTurnEnded("missing", newer))
}

func TestAckedEpisode_UpdateAckedEpisode_Should_BeMonotonic(t *testing.T) {
	t.Parallel()
	s := openDiskStorage(t, filepath.Join(t.TempDir(), "s.db"))
	defer s.Close()
	require.NoError(t, s.AddInstance(newTestInstance("demo")))

	require.NoError(t, s.UpdateInstanceAckedEpisode("demo", "E2", turnEndBase.Add(time.Second)))
	require.NoError(t, s.UpdateInstanceAckedEpisode("demo", "E1", turnEndBase))

	row := storedRow(t, s, "demo")
	assert.Equal(t, EpisodeID("E2"), row.AckedEpisodeID)
	assert.True(t, turnEndBase.Add(time.Second).Equal(row.LastAcknowledged))
}

// staleSnapshotOf returns the stored row as an in-memory instance would have held it before a
// narrow write, i.e. what a full save in flight carries.
func staleSnapshotOf(t *testing.T, s *Storage, title string) *Instance {
	t.Helper()
	inst, err := FromInstanceDataDeferred(storedRow(t, s, title))
	require.NoError(t, err)
	return inst
}

func TestLostUpdate_StaleFullSaveAfterNarrowWrites_Should_KeepNarrowValues(t *testing.T) {
	t.Parallel()
	s := openDiskStorage(t, filepath.Join(t.TempDir(), "s.db"))
	defer s.Close()
	require.NoError(t, s.AddInstance(newTestInstance("demo")))
	stale := staleSnapshotOf(t, s, "demo")
	e2 := turnAt("demo", time.Second, TurnEndKindQuestion)
	ackAt := turnEndBase.Add(2 * time.Second)

	require.NoError(t, s.UpdateInstanceTurnEnded("demo", e2))
	require.NoError(t, s.UpdateInstanceAckedEpisode("demo", "E1", ackAt))
	require.NoError(t, s.UpdateInstance(stale))

	row := storedRow(t, s, "demo")
	assertSameTurn(t, e2, row.TurnEnded)
	assert.Equal(t, EpisodeID("E1"), row.AckedEpisodeID)
	assert.True(t, ackAt.Equal(row.LastAcknowledged))
}

// The full save's SELECT has run; the narrow writers fire inside the window before its UPDATE.
// They queue on the single writer connection and commit after the full save, so the narrow
// (newer) values must end up stored.
func TestLostUpdate_NarrowWritersForcedIntoSelectToUpdateWindow_Should_Win(t *testing.T) {
	t.Parallel()
	s := openDiskStorage(t, filepath.Join(t.TempDir(), "s.db"))
	defer s.Close()
	require.NoError(t, s.AddInstance(newTestInstance("demo")))
	stale := staleSnapshotOf(t, s, "demo")
	e2 := turnAt("demo", time.Second, TurnEndKindQuestion)
	ackAt := turnEndBase.Add(2 * time.Second)

	var wg sync.WaitGroup
	s.repo.afterUpdateSelect = func(context.Context) {
		wg.Add(2)
		go func() { defer wg.Done(); assert.NoError(t, s.UpdateInstanceTurnEnded("demo", e2)) }()
		go func() { defer wg.Done(); assert.NoError(t, s.UpdateInstanceAckedEpisode("demo", "E1", ackAt)) }()
	}
	require.NoError(t, s.UpdateInstance(stale))
	s.repo.afterUpdateSelect = nil
	wg.Wait()

	row := storedRow(t, s, "demo")
	assertSameTurn(t, e2, row.TurnEnded)
	assert.Equal(t, EpisodeID("E1"), row.AckedEpisodeID)
}

func TestLostUpdate_NewerFullSaveThenOlderNarrowWrites_Should_KeepFullSaveValues(t *testing.T) {
	t.Parallel()
	s := openDiskStorage(t, filepath.Join(t.TempDir(), "s.db"))
	defer s.Close()
	require.NoError(t, s.AddInstance(newTestInstance("demo")))
	inst := staleSnapshotOf(t, s, "demo")
	e2 := turnAt("demo", time.Second, TurnEndKindQuestion)
	inst.TurnEnded = e2
	inst.AckedEpisodeID = "E2"
	inst.LastAcknowledged = turnEndBase.Add(3 * time.Second)

	require.NoError(t, s.UpdateInstance(inst))
	require.NoError(t, s.UpdateInstanceTurnEnded("demo", turnAt("demo", 0, TurnEndKindFinished)))
	require.NoError(t, s.UpdateInstanceAckedEpisode("demo", "E1", turnEndBase.Add(time.Second)))

	row := storedRow(t, s, "demo")
	assertSameTurn(t, e2, row.TurnEnded)
	assert.Equal(t, EpisodeID("E2"), row.AckedEpisodeID)
}

func TestLostUpdate_StaleEmptyAckWithSameTurn_Should_NotClearNarrowAck(t *testing.T) {
	t.Parallel()
	s := openDiskStorage(t, filepath.Join(t.TempDir(), "s.db"))
	defer s.Close()
	inst := newTestInstance("demo")
	inst.TurnEnded = turnAt("demo", 0, TurnEndKindQuestion)
	require.NoError(t, s.AddInstance(inst))
	stale := staleSnapshotOf(t, s, "demo")
	ep := inst.TurnEnded.Episode

	require.NoError(t, s.UpdateInstanceAckedEpisode("demo", ep, turnEndBase.Add(time.Second)))
	require.NoError(t, s.UpdateInstance(stale))

	assert.Equal(t, ep, storedRow(t, s, "demo").AckedEpisodeID)
}

func TestLostUpdate_StaleFullSaveRacingNarrowWriters_Should_ConvergeToNewestValues(t *testing.T) {
	t.Parallel()
	seed := time.Now().UnixNano()
	t.Logf("seed=%d", seed)
	rng := rand.New(rand.NewSource(seed))
	s := openDiskStorage(t, filepath.Join(t.TempDir(), "s.db"))
	defer s.Close()
	require.NoError(t, s.AddInstance(newTestInstance("demo")))

	const n = 24
	var ops []func()
	var maxTurn TurnEnded
	var maxAck time.Time
	var maxAckEp EpisodeID
	for k := 0; k < n; k++ {
		k := k
		turn := turnAt("demo", time.Duration(k)*time.Millisecond, TurnEndKindQuestion)
		ackAt := turnEndBase.Add(time.Duration(k) * time.Millisecond)
		ep := EpisodeID("E" + string(rune('A'+k)))
		if turn.At.After(maxTurn.At) {
			maxTurn = turn
		}
		if ackAt.After(maxAck) {
			maxAck, maxAckEp = ackAt, ep
		}
		switch k % 3 {
		case 0:
			ops = append(ops, func() { assert.NoError(t, s.UpdateInstanceTurnEnded("demo", turn)) })
		case 1:
			ops = append(ops, func() { assert.NoError(t, s.UpdateInstanceAckedEpisode("demo", ep, ackAt)) })
		default:
			full := staleSnapshotOf(t, s, "demo")
			full.TurnEnded, full.AckedEpisodeID, full.LastAcknowledged = turn, ep, ackAt
			ops = append(ops, func() { assert.NoError(t, s.UpdateInstance(full)) })
		}
	}
	rng.Shuffle(len(ops), func(a, b int) { ops[a], ops[b] = ops[b], ops[a] })
	var wg sync.WaitGroup
	for _, op := range ops {
		wg.Add(1)
		go func(op func()) { defer wg.Done(); op() }(op)
	}
	wg.Wait()

	row := storedRow(t, s, "demo")
	assertSameTurn(t, maxTurn, row.TurnEnded)
	assert.Equal(t, maxAckEp, row.AckedEpisodeID)
	assert.True(t, maxAck.Equal(row.LastAcknowledged))
}
