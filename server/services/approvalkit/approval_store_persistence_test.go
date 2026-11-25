package approvalkit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPending(id, sessionID, toolName string) *PendingApproval {
	return &PendingApproval{ID: id, SessionID: sessionID, ToolName: toolName}
}

// TestApprovalStore_LoadFromDisk_PreservesEscalationReason is the AC2 "four-struct chain"
// regression guard (plan.md Story 5.1.1's 4th AC, validation.md AC2 "persist round-trip
// through disk" row): a pending_approvals.json fixture written with
// escalation_reason/escalation_category populated for one entry must survive a simulated
// restart (a fresh ApprovalStore constructed against that file) with both fields intact on
// the resulting ApprovalMetadata, and Orphaned == true. If any single struct/copy-loop in
// the PendingApproval -> PersistedApproval -> disk -> PendingApproval -> ApprovalMetadata
// chain were missed, this test fails with an empty string, not a compile error.
func TestApprovalStore_LoadFromDisk_PreservesEscalationReason(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pending_approvals.json")

	fixture := []PersistedApproval{
		{
			ID:                 "appr-restart-1",
			SessionID:          "session-restart",
			ToolName:           "Bash",
			ToolInput:          map[string]interface{}{"command": "git branch -d feature/foo"},
			Cwd:                "/tmp",
			CreatedAt:          time.Now().Add(-1 * time.Minute),
			ExpiresAt:          time.Now().Add(3 * time.Minute),
			EscalationReason:   "Branch deletion modifies repository structure and should be reviewed.",
			EscalationCategory: "explicit-rule",
			Orphaned:           false, // pre-restart value; loadFromDisk must force this to true regardless
		},
	}
	data, err := json.MarshalIndent(fixture, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))

	// Simulate a server restart: a fresh ApprovalStore constructed against the same file.
	store := NewApprovalStore(path)

	metas := store.GetApprovalMetadataBySession("session-restart")
	require.Len(t, metas, 1)
	assert.Equal(t, "Branch deletion modifies repository structure and should be reviewed.", metas[0].EscalationReason)
	assert.Equal(t, "explicit-rule", metas[0].EscalationCategory)
	assert.True(t, metas[0].Orphaned, "approvals loaded from disk after a restart must be marked Orphaned")
}

// TestApprovalStore_should_PreserveRiskLevelAcrossPersistAndReload_When_ApprovalIsOrphaned
// covers plan.md Task 1.2.2(a): a PendingApproval created with a classified RiskLevel must
// survive a persist-then-reload (simulated server restart) round trip intact.
func TestApprovalStore_should_PreserveRiskLevelAcrossPersistAndReload_When_ApprovalIsOrphaned(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pending_approvals.json")

	store := NewApprovalStore(path)
	a := newPending("appr-risk-1", "session-risk", "Bash")
	a.RiskLevel = "high"
	require.NoError(t, store.Create(a))

	// Simulate a server restart: a fresh ApprovalStore constructed against the same file.
	reloaded := NewApprovalStore(path)
	metas := reloaded.GetApprovalMetadataBySession("session-risk")
	require.Len(t, metas, 1)
	assert.Equal(t, "high", metas[0].RiskLevel)
	assert.True(t, metas[0].Orphaned)
}

// TestApprovalStore_should_LoadEmptyRiskLevel_When_LegacyJSONHasNoRiskLevelKey covers
// pre-mortem.md Failure #1's persistence half: a pending_approvals.json written by a
// pre-feature binary (no "risk_level" key at all) must deserialize to RiskLevel == "" --
// the "not recorded" sentinel -- never fall back to "low".
func TestApprovalStore_should_LoadEmptyRiskLevel_When_LegacyJSONHasNoRiskLevelKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pending_approvals.json")

	// Deliberately hand-written JSON with no risk_level key, mirroring a pre-feature file.
	legacyJSON := `[{"id":"appr-legacy-1","session_id":"session-legacy","tool_name":"Bash","cwd":"/tmp","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-01-01T00:05:00Z","orphaned":false}]`
	require.NoError(t, os.WriteFile(path, []byte(legacyJSON), 0600))

	store := NewApprovalStore(path)
	metas := store.GetApprovalMetadataBySession("session-legacy")
	require.Len(t, metas, 1)
	assert.Equal(t, "", metas[0].RiskLevel, "legacy JSON with no risk_level key must load as not-recorded, not \"low\"")
}

// TestApprovalStore_Create_ConcurrentEscalations_NoDataRace is the pre-mortem P2
// concurrency regression guard (plan.md Task 5.1.1d). N goroutines call store.Create
// concurrently, each with a distinct EscalationReason/EscalationCategory pair, and all N
// must persist intact afterward -- no lost writes, no corrupted EscalationReason bleeding
// across entries. ApprovalStore's existing single mutex already serializes Create; this
// test's job is confirming the two new string fields don't introduce a copy/aliasing bug
// under concurrent load, not benchmarking throughput. Must be run with `go test -race`.
func TestApprovalStore_Create_ConcurrentEscalations_NoDataRace(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore("")
	const n = 20

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			a := &PendingApproval{
				ID:                 fmt.Sprintf("appr-concurrent-%d", i),
				SessionID:          fmt.Sprintf("session-%d", i),
				ToolName:           "Bash",
				EscalationReason:   fmt.Sprintf("reason-%d", i),
				EscalationCategory: fmt.Sprintf("category-%d", i),
			}
			if err := store.Create(a); err != nil {
				t.Errorf("Create(%d) returned error: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	all := store.ListAll()
	require.Len(t, all, n)

	byID := make(map[string]*PendingApproval, n)
	for _, a := range all {
		byID[a.ID] = a
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("appr-concurrent-%d", i)
		a, ok := byID[id]
		require.True(t, ok, "approval %s missing after concurrent Create", id)
		assert.Equal(t, fmt.Sprintf("reason-%d", i), a.EscalationReason, "EscalationReason must not bleed across concurrently-created entries")
		assert.Equal(t, fmt.Sprintf("category-%d", i), a.EscalationCategory, "EscalationCategory must not bleed across concurrently-created entries")
	}
}

// TestApprovalStore_HumanResolving_MarksAndClears pins the primitive the
// human-vs-reconciliation arbitration mechanism depends on: mark/clear
// round-trip, and ClearHumanResolving on a never-marked ID is a no-op.
func TestApprovalStore_HumanResolving_MarksAndClears(t *testing.T) {
	t.Parallel()
	store := NewApprovalStore("")

	assert.False(t, store.IsHumanResolving("appr-x"))

	store.MarkHumanResolving("appr-x")
	assert.True(t, store.IsHumanResolving("appr-x"))

	store.ClearHumanResolving("appr-x")
	assert.False(t, store.IsHumanResolving("appr-x"))

	// Clearing a never-marked ID is a no-op — must not panic.
	assert.NotPanics(t, func() { store.ClearHumanResolving("appr-never-marked") })
}
