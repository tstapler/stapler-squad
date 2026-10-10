package notifications

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Notification type values mirrored from types.proto (see store.go).
const (
	ptInfo         = int32(5)
	ptTaskComplete = int32(6)
	ptWarning      = notifTypeWarning
)

var pruneClock = time.Date(2026, 10, 9, 18, 30, 45, 0, time.UTC)

// seedPruneStore replaces the store's rows directly (Append would dedup and
// apply retention) and persists them so the backup copies real bytes.
func seedPruneStore(t *testing.T, recs ...*NotificationRecord) *NotificationHistoryStore {
	t.Helper()
	s := newTestStore(t)
	s.mu.Lock()
	s.records = recs
	err := s.saveToDisk()
	s.mu.Unlock()
	require.NoError(t, err)
	return s
}

func prow(id, session string, typ int32, read bool) *NotificationRecord {
	r := makeRecord(id, session, typ)
	r.IsRead = read
	return r
}

// classifyBySession maps a raw session id to a decision; unknown ids are undeterminable.
func classifyBySession(m map[string]PruneDecision) PruneClassifier {
	return func(r *NotificationRecord) PruneDecision { return m[r.SessionID] }
}

var hiddenD = PruneDecision{Hidden: true, Group: "h [review]", Form: "uuid"}
var visibleD = PruneDecision{Visible: true}

// The story's example store: hidden h1 (WARNING read, INFO, TASK_COMPLETE),
// hidden h2 (WARNING unread), deleted d1, visible v1 (WARNING).
func storyStore(t *testing.T) (*NotificationHistoryStore, PruneClassifier) {
	s := seedPruneStore(t,
		prow("h1-warn", "h1", ptWarning, true),
		prow("h1-info", "h1", ptInfo, false),
		prow("h1-done", "h1", ptTaskComplete, false),
		prow("h2-warn", "h2", ptWarning, false),
		prow("d1-warn", "d1", ptWarning, false),
		prow("v1-warn", "v1", ptWarning, false),
	)
	return s, classifyBySession(map[string]PruneDecision{"h1": hiddenD, "h2": hiddenD, "v1": visibleD})
}

func storeIDs(s *NotificationHistoryStore) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.records))
	for _, r := range s.records {
		ids = append(ids, r.ID)
	}
	return ids
}

func TestPrune_ShouldListIdsAndDeleteNothing_WhenApplyUnset(t *testing.T) {
	s, classify := storyStore(t)
	before, err := os.ReadFile(s.filePath)
	require.NoError(t, err)

	plan, err := s.PruneByPredicate(classify, PruneOptions{Now: pruneClock})
	require.NoError(t, err)

	assert.Equal(t, []string{"h1-warn", "h1-info", "h1-done"}, plan.IDs())
	assert.False(t, plan.Applied)
	assert.Empty(t, plan.BackupPath)
	assert.Equal(t, 1, plan.KeptUnreadActionable)
	assert.Equal(t, 1, plan.Undeterminable)
	assert.Equal(t, 1, plan.Visible)
	assert.Len(t, storeIDs(s), 6)
	after, err := os.ReadFile(s.filePath)
	require.NoError(t, err)
	assert.Equal(t, before, after, "a dry run writes nothing")
	assert.NoFileExists(t, s.filePath+".pre-prune-20261009T183045Z.bak")
}

func TestPrune_ShouldKeepUnreadActionable_WhenApplyWithoutIncludeUnreadActionable(t *testing.T) {
	s, classify := storyStore(t)

	plan, err := s.PruneByPredicate(classify, PruneOptions{Apply: true, Now: pruneClock})
	require.NoError(t, err)
	require.True(t, plan.Applied)
	assert.ElementsMatch(t, []string{"h2-warn", "d1-warn", "v1-warn"}, storeIDs(s))

	plan, err = s.PruneByPredicate(classify, PruneOptions{Apply: true, IncludeUnreadActionable: true, Now: pruneClock.Add(time.Second)})
	require.NoError(t, err)
	assert.Equal(t, []string{"h2-warn"}, plan.IDs())
	assert.Equal(t, PruneReasonUnreadActionable, plan.Remove[0].Reason)
	assert.ElementsMatch(t, []string{"d1-warn", "v1-warn"}, storeIDs(s))
}

func TestPrune_ShouldNeverDeleteUnreadPendingDecisionRow_WhenRandomizedStoresOf200Rows(t *testing.T) {
	rng := rand.New(rand.NewSource(20261009))
	types := []int32{ptInfo, ptTaskComplete, ptWarning, notifTypeError, notifTypeFailure, notifTypeApprovalNeeded}
	for store := 0; store < 200; store++ {
		recs := make([]*NotificationRecord, 0, 200)
		decisions := map[string]PruneDecision{}
		for i := 0; i < 200; i++ {
			sess := fmt.Sprintf("s%d", i)
			r := prow(fmt.Sprintf("r%d", i), sess, types[rng.Intn(len(types))], rng.Intn(2) == 0)
			if rng.Intn(5) == 0 {
				r.Metadata["auto_remediating"] = "true"
			}
			recs = append(recs, r)
			switch rng.Intn(3) {
			case 0:
				decisions[sess] = hiddenD
			case 1:
				decisions[sess] = visibleD
			}
		}
		for _, include := range []bool{false, true} {
			for _, apply := range []bool{false, true} {
				s := seedPruneStore(t, cloneRecords(recs)...)
				plan, err := s.PruneByPredicate(classifyBySession(decisions), PruneOptions{Apply: apply, IncludeUnreadActionable: include, Now: pruneClock})
				require.NoError(t, err)
				for _, row := range plan.Remove {
					r := findByID(t, recs, row.ID)
					if IsPendingDecision(r.NotificationType, r.Metadata, r.IsRead) {
						require.True(t, include, "store %d: unread pending row %s planned without opt-in", store, row.ID)
					}
					require.Equal(t, hiddenD, decisions[r.SessionID], "only hidden rows are ever removed")
				}
				if apply {
					for _, id := range plan.IDs() {
						require.NotContains(t, storeIDs(s), id)
					}
				}
			}
		}
	}
}

func cloneRecords(in []*NotificationRecord) []*NotificationRecord {
	out := make([]*NotificationRecord, len(in))
	for i, r := range in {
		cp := *r
		cp.Metadata = map[string]string{}
		for k, v := range r.Metadata {
			cp.Metadata[k] = v
		}
		out[i] = &cp
	}
	return out
}

func findByID(t *testing.T, recs []*NotificationRecord, id string) *NotificationRecord {
	t.Helper()
	for _, r := range recs {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("record %s not found", id)
	return nil
}

func TestPrune_ShouldKeepRowsOfUnresolvedAndNotASession_WhenAnyMode(t *testing.T) {
	for _, mode := range []PruneOptions{
		{}, {Apply: true}, {IncludeUnreadActionable: true}, {Apply: true, IncludeUnreadActionable: true},
	} {
		s := seedPruneStore(t,
			prow("u1", "unresolved-session", ptInfo, true),
			prow("sys", "system", ptWarning, false),
			prow("h", "h1", ptInfo, true),
		)
		mode.Now = pruneClock
		plan, err := s.PruneByPredicate(classifyBySession(map[string]PruneDecision{"h1": hiddenD}), mode)
		require.NoError(t, err)
		assert.Equal(t, []string{"h"}, plan.IDs())
		assert.Equal(t, 2, plan.Undeterminable)
		assert.Contains(t, storeIDs(s), "u1")
		assert.Contains(t, storeIDs(s), "sys")
	}
}

func TestPrune_ShouldWriteTimestampedBackupAndNoOpOnRerun_WhenAppliedTwice(t *testing.T) {
	s, classify := storyStore(t)
	original, err := os.ReadFile(s.filePath)
	require.NoError(t, err)

	first, err := s.PruneByPredicate(classify, PruneOptions{Apply: true, Now: pruneClock})
	require.NoError(t, err)
	wantFirst := s.filePath + ".pre-prune-20261009T183045Z.bak"
	assert.Equal(t, wantFirst, first.BackupPath)
	backup, err := os.ReadFile(wantFirst)
	require.NoError(t, err)
	assert.Equal(t, original, backup)

	// Re-run: nothing left to remove, no new backup, first backup intact.
	second, err := s.PruneByPredicate(classify, PruneOptions{Apply: true, Now: pruneClock.Add(time.Hour)})
	require.NoError(t, err)
	assert.Empty(t, second.IDs())
	assert.False(t, second.Applied)
	assert.Empty(t, second.BackupPath)
	assert.NoFileExists(t, s.filePath+".pre-prune-20261009T193045Z.bak")

	// A later apply that removes rows gets its own backup and leaves the first alone.
	s.mu.Lock()
	s.records = append(s.records, prow("h1-late", "h1", ptInfo, true))
	require.NoError(t, s.saveToDisk())
	s.mu.Unlock()
	third, err := s.PruneByPredicate(classify, PruneOptions{Apply: true, Now: pruneClock.Add(2 * time.Hour)})
	require.NoError(t, err)
	assert.Equal(t, s.filePath+".pre-prune-20261009T203045Z.bak", third.BackupPath)
	again, err := os.ReadFile(wantFirst)
	require.NoError(t, err)
	assert.Equal(t, original, again)
}

func TestPrune_ShouldNotOverwriteAnExistingBackup_WhenTheTimestampCollides(t *testing.T) {
	s, classify := storyStore(t)
	path := s.filePath + ".pre-prune-20261009T183045Z.bak"
	require.NoError(t, os.WriteFile(path, []byte("earlier backup"), 0o600))

	_, err := s.PruneByPredicate(classify, PruneOptions{Apply: true, Now: pruneClock})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPruneBackupExists))
	assert.Len(t, storeIDs(s), 6, "no delete without a backup")
	b, _ := os.ReadFile(path)
	assert.Equal(t, "earlier backup", string(b))
}

func TestPrune_ShouldDeleteNothingAndWriteNoBackup_WhenBeforeApplyFails(t *testing.T) {
	s, classify := storyStore(t)
	boom := errors.New("audit down")

	plan, err := s.PruneByPredicate(classify, PruneOptions{Apply: true, Now: pruneClock, BeforeApply: func(PrunePlan) error { return boom }})
	require.ErrorIs(t, err, boom)
	assert.False(t, plan.Applied)
	assert.Len(t, storeIDs(s), 6)
	matches, _ := filepath.Glob(s.filePath + ".pre-prune-*")
	assert.Empty(t, matches)
}

func TestPruneApply_ShouldPassTheFinalPlanToBeforeApply_BeforeAnyRowIsDeleted(t *testing.T) {
	s, classify := storyStore(t)
	var seen PrunePlan
	var rowsAtCall int
	_, err := s.PruneByPredicate(classify, PruneOptions{Apply: true, Now: pruneClock, BeforeApply: func(p PrunePlan) error {
		seen = p
		rowsAtCall = len(s.records) // under the store lock; not re-entering the store
		return nil
	}})
	require.NoError(t, err)
	assert.Equal(t, 6, rowsAtCall)
	assert.Equal(t, []string{"h1-warn", "h1-info", "h1-done"}, seen.IDs())
	assert.Empty(t, seen.BackupPath)
}

func TestPrune_ShouldKeep84DeletedSessionRows_WhenRunOn319RowSampleFixture(t *testing.T) {
	var recs []*NotificationRecord
	decisions := map[string]PruneDecision{}
	wantRemoved := 0
	for i := 0; i < 319; i++ {
		sess := fmt.Sprintf("sess-%d", i)
		r := prow(fmt.Sprintf("n%d", i), sess, ptInfo, true)
		recs = append(recs, r)
		switch {
		case i < 84: // sessions deleted before the feature shipped: no index entry
		case i%2 == 0:
			decisions[sess] = hiddenD
			wantRemoved++
		default:
			decisions[sess] = visibleD
		}
	}
	s := seedPruneStore(t, recs...)

	plan, err := s.PruneByPredicate(classifyBySession(decisions), PruneOptions{Apply: true, IncludeUnreadActionable: true, Now: pruneClock})
	require.NoError(t, err)
	assert.Equal(t, 84, plan.Undeterminable)
	assert.Len(t, plan.Remove, wantRemoved)
	assert.Len(t, storeIDs(s), 319-wantRemoved)
	for i := 0; i < 84; i++ {
		assert.Contains(t, storeIDs(s), fmt.Sprintf("n%d", i))
	}
}

func TestPrune_ShouldNotLoseConcurrentAppends_WhenApplyRacesAppend(t *testing.T) {
	recs := make([]*NotificationRecord, 0, 50)
	decisions := map[string]PruneDecision{}
	for i := 0; i < 50; i++ {
		sess := fmt.Sprintf("old-%d", i)
		recs = append(recs, prow(fmt.Sprintf("old%d", i), sess, ptInfo, true))
		decisions[sess] = hiddenD
	}
	s := seedPruneStore(t, recs...)

	var wg sync.WaitGroup
	const appended = 30
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < appended; i++ {
			r := makeRecord(fmt.Sprintf("new%d", i), fmt.Sprintf("new-%d", i), ptInfo)
			r.SessionScoped = false
			if err := s.Append(r); err != nil {
				t.Errorf("append: %v", err)
			}
		}
	}()
	plan, err := s.PruneByPredicate(classifyBySession(decisions), PruneOptions{Apply: true, Now: pruneClock})
	wg.Wait()
	require.NoError(t, err)
	assert.Len(t, plan.Remove, 50)

	ids := storeIDs(s)
	for i := 0; i < appended; i++ {
		assert.Contains(t, ids, fmt.Sprintf("new%d", i), "an append racing the prune must survive")
	}
}

// migration_should_be_reversible: the documented rollback is copying the
// timestamped backup back.
func TestPruneApply_ShouldRestoreStoreByteIdentical_WhenBackupCopiedBack(t *testing.T) {
	s, classify := storyStore(t)
	original, err := os.ReadFile(s.filePath)
	require.NoError(t, err)

	plan, err := s.PruneByPredicate(classify, PruneOptions{Apply: true, IncludeUnreadActionable: true, Now: pruneClock})
	require.NoError(t, err)
	pruned, err := os.ReadFile(s.filePath)
	require.NoError(t, err)
	require.NotEqual(t, original, pruned)

	backup, err := os.ReadFile(plan.BackupPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(s.filePath, backup, 0o600))
	restored, err := os.ReadFile(s.filePath)
	require.NoError(t, err)
	assert.Equal(t, original, restored)

	reopened, err := NewNotificationHistoryStore(s.filePath)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"h1-warn", "h1-info", "h1-done", "h2-warn", "d1-warn", "v1-warn"}, storeIDs(reopened))
}

func TestPrune_ShouldReportGroupFormAndAliasFlagPerPlannedRow(t *testing.T) {
	s := seedPruneStore(t,
		prow("a", "uuid-1", ptInfo, true),
		prow("b", "Title One", ptTaskComplete, true),
	)
	plan, err := s.PruneByPredicate(classifyBySession(map[string]PruneDecision{
		"uuid-1":    {Hidden: true, Group: "Title One [review]", Form: "uuid"},
		"Title One": {Hidden: true, Group: "Title One [review]", Form: "alias", ByAlias: true},
	}), PruneOptions{Now: pruneClock})
	require.NoError(t, err)
	require.Len(t, plan.Remove, 2)
	assert.Equal(t, PrunePlanRow{ID: "a", Reason: PruneReasonRoutine, Group: "Title One [review]", Form: "uuid"}, plan.Remove[0])
	assert.True(t, plan.Remove[1].ByAlias)
	assert.Equal(t, map[string]int{PruneReasonRoutine: 2}, plan.ReasonCounts())
}
