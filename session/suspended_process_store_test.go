package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// newTestSuspendedProcessStore creates a SuspendedProcessStore rooted at a
// fresh temp dir, isolated from both the developer's real
// ~/.stapler-squad/config and any other test in this package (matches the
// STAPLER_SQUAD_TEST_DIR isolation pattern used throughout session/*_test.go).
func newTestSuspendedProcessStore(t *testing.T) *SuspendedProcessStore {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	store, err := NewSuspendedProcessStore()
	require.NoError(t, err)
	return store
}

func TestSuspendedProcessStore_List_ReturnsEmpty_When_NoFileExists(t *testing.T) {
	store := newTestSuspendedProcessStore(t)

	records, err := store.List()
	require.NoError(t, err)
	if len(records) != 0 {
		t.Fatalf("expected no records for a fresh store, got %d", len(records))
	}
}

func TestSuspendedProcessStore_Get_ReturnsNotFound_When_InstanceIDAbsent(t *testing.T) {
	store := newTestSuspendedProcessStore(t)

	_, found, err := store.Get("does-not-exist")
	require.NoError(t, err)
	if found {
		t.Fatal("expected found=false for an instance ID that was never added")
	}
}

func TestSuspendedProcessStore_Add_PersistsRecord_When_RoundTripped(t *testing.T) {
	store := newTestSuspendedProcessStore(t)

	record := SuspendedProcessRecord{
		PID:          4242,
		CreateTimeMs: 1000,
		InstanceID:   "instance-a",
		Candidate:    ExternalSessionCandidate{Path: "/tmp/foo", TmuxSession: "sess-a"},
	}
	require.NoError(t, store.Add(record))

	got, found, err := store.Get("instance-a")
	require.NoError(t, err)
	if !found {
		t.Fatal("expected the added record to be found")
	}
	if got != record {
		t.Fatalf("expected round-tripped record %+v, got %+v", record, got)
	}

	// A second store instance pointed at the same config dir must observe
	// the same persisted state -- this is what "durable" means for this
	// store (survives a fresh process/store construction, not just an
	// in-memory cache).
	reopened, err := NewSuspendedProcessStore()
	require.NoError(t, err)
	list, err := reopened.List()
	require.NoError(t, err)
	if len(list) != 1 || list[0] != record {
		t.Fatalf("expected reopened store to see exactly the persisted record, got %+v", list)
	}
}

func TestSuspendedProcessStore_Add_UpsertsExistingRecord_When_InstanceIDAlreadyPresent(t *testing.T) {
	store := newTestSuspendedProcessStore(t)

	first := SuspendedProcessRecord{PID: 111, InstanceID: "instance-a", CreateTimeMs: 1}
	second := SuspendedProcessRecord{PID: 222, InstanceID: "instance-a", CreateTimeMs: 2}

	require.NoError(t, store.Add(first))
	require.NoError(t, store.Add(second))

	list, err := store.List()
	require.NoError(t, err)
	if len(list) != 1 {
		t.Fatalf("expected Add to upsert (dedup by InstanceID) rather than duplicate, got %d records: %+v", len(list), list)
	}
	if list[0] != second {
		t.Fatalf("expected the second Add to win, got %+v", list[0])
	}
}

func TestSuspendedProcessStore_Add_KeepsDistinctRecords_When_InstanceIDsDiffer(t *testing.T) {
	store := newTestSuspendedProcessStore(t)

	a := SuspendedProcessRecord{PID: 111, InstanceID: "instance-a"}
	b := SuspendedProcessRecord{PID: 222, InstanceID: "instance-b"}

	require.NoError(t, store.Add(a))
	require.NoError(t, store.Add(b))

	list, err := store.List()
	require.NoError(t, err)
	if len(list) != 2 {
		t.Fatalf("expected both distinct-InstanceID records to be kept, got %d: %+v", len(list), list)
	}
}

func TestSuspendedProcessStore_Remove_DeletesRecord_When_InstanceIDPresent(t *testing.T) {
	store := newTestSuspendedProcessStore(t)

	require.NoError(t, store.Add(SuspendedProcessRecord{PID: 111, InstanceID: "instance-a"}))
	require.NoError(t, store.Add(SuspendedProcessRecord{PID: 222, InstanceID: "instance-b"}))

	require.NoError(t, store.Remove("instance-a"))

	_, found, err := store.Get("instance-a")
	require.NoError(t, err)
	if found {
		t.Fatal("expected instance-a to be removed")
	}

	_, found, err = store.Get("instance-b")
	require.NoError(t, err)
	if !found {
		t.Fatal("expected instance-b to remain untouched by removing instance-a")
	}
}

func TestSuspendedProcessStore_Remove_IsNotAnError_When_InstanceIDAbsent(t *testing.T) {
	store := newTestSuspendedProcessStore(t)

	// Remove on a record that was never added (or already removed) must be
	// a no-op, not an error -- callers invoke this on every resume/kill
	// path, some of which race with reconciliation.
	require.NoError(t, store.Remove("never-added"))
}

func TestSuspendedProcessStore_List_ReturnsAllRecords_When_MultipleAdded(t *testing.T) {
	store := newTestSuspendedProcessStore(t)

	want := []SuspendedProcessRecord{
		{PID: 1, InstanceID: "a"},
		{PID: 2, InstanceID: "b"},
		{PID: 3, InstanceID: "c"},
	}
	for _, r := range want {
		require.NoError(t, store.Add(r))
	}

	got, err := store.List()
	require.NoError(t, err)
	if len(got) != len(want) {
		t.Fatalf("expected %d records, got %d: %+v", len(want), len(got), got)
	}
}
