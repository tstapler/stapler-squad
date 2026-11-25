package approvalkit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func recvDecision(t *testing.T, a *PendingApproval) ApprovalDecision {
	t.Helper()
	select {
	case d := <-a.Decisions():
		return d
	case <-time.After(time.Second):
		t.Fatal("no decision delivered")
		return ApprovalDecision{}
	}
}

func assertNoDecision(t *testing.T, a *PendingApproval) {
	t.Helper()
	select {
	case d := <-a.Decisions():
		t.Fatalf("unexpected decision %+v", d)
	default:
	}
}

func idsOf(as []*PendingApproval) []string {
	out := make([]string, 0, len(as))
	for _, a := range as {
		out = append(out, a.ID)
	}
	return out
}

func TestCreate_should_RejectEmptyAndDuplicateIDs(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	require.Error(t, s.Create(newPending("", "s", "Bash")))
	require.NoError(t, s.Create(newPending("a1", "s", "Bash")))
	err := s.Create(newPending("a1", "s", "Bash"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.Len(t, s.ListAll(), 1)
	assert.Len(t, s.GetBySession("s"), 1, "a rejected duplicate must not be indexed twice")
}

func TestCreate_should_InitDecisionChannelAndIndexBySession(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	a := newPending("a1", "s1", "Bash")
	assert.Nil(t, a.Decisions(), "a fresh approval has no channel until Create")
	require.NoError(t, s.Create(a))
	assert.NotNil(t, a.Decisions())

	got, ok := s.Get("a1")
	require.True(t, ok)
	assert.Same(t, a, got)
	_, ok = s.Get("missing")
	assert.False(t, ok)

	require.NoError(t, s.Create(newPending("a2", "s1", "Bash")))
	require.NoError(t, s.Create(newPending("a3", "s2", "Bash")))
	assert.Len(t, s.GetBySession("s1"), 2)
	assert.Len(t, s.GetBySession("s2"), 1)
	assert.Empty(t, s.GetBySession("none"))
	assert.Len(t, s.ListAll(), 3)
}

func TestGetBySession_should_SkipIndexedIDsNoLongerPending(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	require.NoError(t, s.Create(newPending("a1", "s", "Bash")))
	require.NoError(t, s.Create(newPending("a2", "s", "Bash")))
	delete(s.pending, "a1") // stale index entry
	got := s.GetBySession("s")
	require.Len(t, got, 1)
	assert.Equal(t, "a2", got[0].ID)
	assert.Len(t, s.GetApprovalMetadataBySession("s"), 1)
}

func TestGetApprovalMetadataBySession_should_CopyEveryField(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	a := &PendingApproval{
		ID: "a1", SessionID: "s", ToolName: "Bash", ToolInput: map[string]interface{}{"command": "ls"},
		Cwd: "/w", Orphaned: false, EscalationReason: "why", EscalationCategory: "cat", RiskLevel: "high",
	}
	require.NoError(t, s.Create(a))
	metas := s.GetApprovalMetadataBySession("s")
	require.Len(t, metas, 1)
	m := metas[0]
	assert.Equal(t, "a1", m.ApprovalID)
	assert.Equal(t, "Bash", m.ToolName)
	assert.Equal(t, map[string]interface{}{"command": "ls"}, m.ToolInput)
	assert.Equal(t, "/w", m.Cwd)
	assert.False(t, m.Orphaned)
	assert.Equal(t, "why", m.EscalationReason)
	assert.Equal(t, "cat", m.EscalationCategory)
	assert.Equal(t, "high", m.RiskLevel)
	assert.Empty(t, s.GetApprovalMetadataBySession("none"))
}

func TestResolve_should_DeliverDecisionAndRemove(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	a := newPending("a1", "s", "Bash")
	require.NoError(t, s.Create(a))
	require.NoError(t, s.Resolve("a1", ApprovalDecision{Behavior: "deny", Message: "no"}))
	assert.Equal(t, ApprovalDecision{Behavior: "deny", Message: "no"}, recvDecision(t, a))
	_, ok := s.Get("a1")
	assert.False(t, ok)
	assert.Empty(t, s.GetBySession("s"))

	err := s.Resolve("a1", ApprovalDecision{Behavior: "allow"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found or already resolved")
}

func TestResolve_should_RemoveOrphanedWithoutDelivering(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	a := newPending("a1", "s", "Bash")
	a.Orphaned = true
	require.NoError(t, s.Create(a))
	require.NoError(t, s.Resolve("a1", ApprovalDecision{Behavior: "allow"}))
	assertNoDecision(t, a)
	_, ok := s.Get("a1")
	assert.False(t, ok)
}

func TestResolve_should_RemoveWhenDecisionChannelNil(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	a := newPending("a1", "s", "Bash")
	s.pending["a1"] = a
	s.bySession["s"] = []string{"a1"}
	require.NoError(t, s.Resolve("a1", ApprovalDecision{Behavior: "allow"}))
	_, ok := s.Get("a1")
	assert.False(t, ok)
}

func TestResolve_should_ErrorWhenChannelAlreadyFull(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	a := newPending("a1", "s", "Bash")
	require.NoError(t, s.Create(a))
	a.decisionCh <- ApprovalDecision{Behavior: "allow"}
	err := s.Resolve("a1", ApprovalDecision{Behavior: "deny"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel full")
	_, ok := s.Get("a1")
	assert.False(t, ok, "the record is removed even when the send loses")
}

func TestRemove_should_DropRecordWithoutDecisionAndIgnoreUnknown(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	a := newPending("a1", "s", "Bash")
	require.NoError(t, s.Create(a))
	require.NoError(t, s.Create(newPending("a2", "s", "Bash")))
	s.Remove("nope")
	s.Remove("a1")
	assertNoDecision(t, a)
	_, ok := s.Get("a1")
	assert.False(t, ok)
	got := s.GetBySession("s")
	require.Len(t, got, 1)
	assert.Equal(t, "a2", got[0].ID, "removal must keep sibling approvals of the same session indexed")
}

func TestCancelSession_should_DenyLiveSkipOrphanedAndReturnIDs(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	live := newPending("live", "s", "Bash")
	orphan := newPending("orphan", "s", "Bash")
	orphan.Orphaned = true
	other := newPending("other", "t", "Bash")
	for _, a := range []*PendingApproval{live, orphan, other} {
		require.NoError(t, s.Create(a))
	}
	assert.Empty(t, s.CancelSession("unknown"))

	ids := s.CancelSession("s")
	assert.ElementsMatch(t, []string{"live", "orphan"}, ids)
	assert.Equal(t, ApprovalDecision{Behavior: "deny", Message: "Session restarted"}, recvDecision(t, live))
	assertNoDecision(t, orphan)
	assertNoDecision(t, other)
	assert.Empty(t, s.GetBySession("s"))
	_, ok := s.Get("other")
	assert.True(t, ok, "other sessions are untouched")
}

func TestCleanupExpired_should_DenyExpiredLiveAndKeepFresh(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	expired := newPending("expired", "s", "Bash")
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	fresh := newPending("fresh", "s", "Bash")
	fresh.ExpiresAt = time.Now().Add(time.Hour)
	require.NoError(t, s.Create(expired))
	require.NoError(t, s.Create(fresh))

	assert.Equal(t, []string{"expired"}, s.CleanupExpired())
	d := recvDecision(t, expired)
	assert.Equal(t, "deny", d.Behavior)
	assert.Equal(t, "Approval timed out. Please respond in the terminal.", d.Message)
	_, ok := s.Get("fresh")
	assert.True(t, ok)
	assertNoDecision(t, fresh)
	assert.Empty(t, s.CleanupExpired())
}

func TestCleanupExpired_should_RemoveOrphanedOnlyPastAgeThreshold(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	old := newPending("old", "s", "Bash")
	old.Orphaned = true
	old.CreatedAt = time.Now().Add(-orphanedCleanupThreshold - time.Minute)
	young := newPending("young", "s", "Bash")
	young.Orphaned = true
	young.CreatedAt = time.Now().Add(-time.Minute)
	young.ExpiresAt = time.Now().Add(-time.Hour) // expired, but orphans age out by CreatedAt only
	for _, a := range []*PendingApproval{old, young} {
		require.NoError(t, s.Create(a))
	}
	assert.Equal(t, []string{"old"}, s.CleanupExpired())
	_, ok := s.Get("young")
	assert.True(t, ok)
	assertNoDecision(t, old)
	assert.Equal(t, []string{"young"}, idsOf(s.GetBySession("s")))
}

func TestCleanupExpired_should_ToleratePendingWithoutDecisionChannel(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	a := newPending("a1", "s", "Bash")
	a.ExpiresAt = time.Now().Add(-time.Minute)
	s.pending["a1"] = a
	s.bySession["s"] = []string{"a1"}
	assert.NotPanics(t, func() { assert.Equal(t, []string{"a1"}, s.CleanupExpired()) })
}

func TestHumanResolving_should_BeIndependentPerID(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	s.MarkHumanResolving("a")
	assert.True(t, s.IsHumanResolving("a"))
	assert.False(t, s.IsHumanResolving("b"))
}

func TestPersistence_should_WriteAtomicallyWithOwnerOnlyPermissions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "dir", "pending_approvals.json")
	s := NewApprovalStore(path)
	assert.Equal(t, path, s.GetFilePath())
	require.NoError(t, s.Create(newPending("a1", "s", "Bash")))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	_, err = os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(err), "temp file must be renamed away")

	s.Remove("a1")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, "[]", string(data))
}

func TestPersistence_should_NotWriteWhenFilePathEmpty(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	require.NoError(t, s.Create(newPending("a1", "s", "Bash")))
	assert.Equal(t, "", s.GetFilePath())
	assert.NoError(t, s.loadFromDisk())
}

func TestLoad_should_StartEmptyOnMissingEmptyOrCorruptFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	assert.Empty(t, NewApprovalStore(filepath.Join(dir, "missing.json")).ListAll())

	empty := filepath.Join(dir, "empty.json")
	require.NoError(t, os.WriteFile(empty, nil, 0600))
	assert.Empty(t, NewApprovalStore(empty).ListAll())

	corrupt := filepath.Join(dir, "corrupt.json")
	require.NoError(t, os.WriteFile(corrupt, []byte("{not json"), 0600))
	assert.Empty(t, NewApprovalStore(corrupt).ListAll())
}

func TestLoad_should_StartEmptyWhenPathIsUnreadable(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore(t.TempDir()) // a directory: ReadFile fails with a non-NotExist error
	assert.Empty(t, s.ListAll())
	assert.Error(t, s.loadFromDisk())
}

func TestLoad_should_SkipBlankIDsMarkOrphanedAndIndexBySession(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p.json")
	require.NoError(t, os.WriteFile(path, []byte(`[
	 {"id":"","session_id":"s","tool_name":"Bash"},
	 {"id":"a1","session_id":"s","claude_session_id":"c","tool_name":"Bash","permission_mode":"plan","cwd":"/w","tool_input":{"k":"v"},"escalation_reason":"r","escalation_category":"c","risk_level":"low","orphaned":false},
	 {"id":"a2","session_id":"s","tool_name":"Edit"}
	]`), 0600))
	s := NewApprovalStore(path)
	got := s.GetBySession("s")
	require.Len(t, got, 2)
	a, ok := s.Get("a1")
	require.True(t, ok)
	assert.True(t, a.Orphaned)
	assert.Nil(t, a.Decisions())
	assert.Equal(t, "c", a.ClaudeSessionID)
	assert.Equal(t, "plan", a.PermissionMode)
	assert.Equal(t, "/w", a.Cwd)
	assert.Equal(t, map[string]interface{}{"k": "v"}, a.ToolInput)
	assert.Equal(t, "r", a.EscalationReason)
	assert.Equal(t, "c", a.EscalationCategory)
	assert.Equal(t, "low", a.RiskLevel)
}

func TestPersistence_should_RoundTripEveryField(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p.json")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s := NewApprovalStore(path)
	require.NoError(t, s.Create(&PendingApproval{
		ID: "a1", SessionID: "s", ClaudeSessionID: "c", ToolName: "Bash",
		ToolInput: map[string]interface{}{"k": "v"}, Cwd: "/w", PermissionMode: "plan",
		CreatedAt: now, ExpiresAt: now.Add(time.Minute),
		EscalationReason: "r", EscalationCategory: "cat", RiskLevel: "high",
	}))
	got, ok := NewApprovalStore(path).Get("a1")
	require.True(t, ok)
	assert.Equal(t, "c", got.ClaudeSessionID)
	assert.Equal(t, "plan", got.PermissionMode)
	assert.Equal(t, "/w", got.Cwd)
	assert.True(t, now.Equal(got.CreatedAt))
	assert.True(t, now.Add(time.Minute).Equal(got.ExpiresAt))
	assert.Equal(t, "high", got.RiskLevel)
	assert.Equal(t, "r", got.EscalationReason)
	assert.Equal(t, "cat", got.EscalationCategory)
}

func TestPersistence_should_NotPanicWhenDirectoryCannotBeCreated(t *testing.T) {
	t.Parallel()
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0600))
	s := NewApprovalStore(filepath.Join(blocker, "sub", "p.json"))
	assert.NotPanics(t, func() { require.NoError(t, s.Create(newPending("a1", "s", "Bash"))) })
	_, ok := s.Get("a1")
	assert.True(t, ok, "a persistence failure must not lose the in-memory approval")
}

func TestPersistence_should_NotPanicWhenRenameFails(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p.json")
	require.NoError(t, os.Mkdir(path, 0750)) // rename onto a directory fails
	s := &ApprovalStore{pending: map[string]*PendingApproval{}, bySession: map[string][]string{}, humanResolving: map[string]struct{}{}, filePath: path}
	assert.NotPanics(t, func() { require.NoError(t, s.Create(newPending("a1", "s", "Bash"))) })
	_, err := os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(err), "temp file is cleaned up after a failed rename")
}

func TestPersistence_should_NotPanicWhenTempFileCannotBeWritten(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p.json")
	require.NoError(t, os.Mkdir(path+".tmp", 0750)) // WriteFile onto a directory fails
	s := NewApprovalStore(path)
	assert.NotPanics(t, func() { require.NoError(t, s.Create(newPending("a1", "s", "Bash"))) })
}

func TestCancelSession_should_DropSessionIndexEntry(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	require.NoError(t, s.Create(newPending("a1", "s", "Bash")))
	s.CancelSession("s")
	assert.NotContains(t, s.bySession, "s")
}

func TestCleanupExpired_should_KeepOrphanedJustInsideThreshold(t *testing.T) {
	t.Parallel()
	s := NewApprovalStore("")
	a := newPending("a1", "s", "Bash")
	a.Orphaned = true
	a.CreatedAt = time.Now().Add(-orphanedCleanupThreshold + 30*time.Minute)
	require.NoError(t, s.Create(a))
	assert.Empty(t, s.CleanupExpired())
	assert.Equal(t, 4*time.Hour, orphanedCleanupThreshold)
}

func TestCleanupExpired_should_PersistWhenOnlyOrphansWereRemoved(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p.json")
	s := NewApprovalStore(path)
	a := newPending("a1", "s", "Bash")
	a.Orphaned = true
	a.CreatedAt = time.Now().Add(-orphanedCleanupThreshold - time.Minute)
	require.NoError(t, s.Create(a))
	require.Equal(t, []string{"a1"}, s.CleanupExpired())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, "[]", string(data))
}

func TestPersistence_should_CreateParentDirectoryWithoutWorldAccess(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	s := NewApprovalStore(filepath.Join(dir, "p.json"))
	require.NoError(t, s.Create(newPending("a1", "s", "Bash")))
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&0o007, "approval state dir must not be world-accessible")
	assert.Zero(t, info.Mode().Perm()&0o020, "approval state dir must not be group-writable")
}

func TestLoadFromDisk_should_NotReturnErrorForCorruptJSON(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p.json")
	require.NoError(t, os.WriteFile(path, []byte("[{"), 0600))
	s := &ApprovalStore{pending: map[string]*PendingApproval{}, bySession: map[string][]string{}, humanResolving: map[string]struct{}{}, filePath: path}
	assert.NoError(t, s.loadFromDisk())
	assert.Empty(t, s.pending)
}
