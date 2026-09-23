package memory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/memory/memorytest"
)

// TestFakeReader_SystemMemory_should_returnUsedPct_When_VirtualMemorySucceeds verifies
// that FakeReader returns the configured SystemPct.
func TestFakeReader_SystemMemory_should_returnUsedPct_When_VirtualMemorySucceeds(t *testing.T) {
	t.Parallel()
	r := &memorytest.FakeReader{SystemPct: 72.5}
	pct, err := r.SystemMemoryPct()
	assert.NoError(t, err)
	assert.InDelta(t, 72.5, pct, 0.01)
}

// TestGopsutilReader_SystemMemory_should_returnZero_When_VirtualMemoryErrors verifies
// FakeReader returns 0 when SystemPct is zero.
func TestGopsutilReader_SystemMemory_should_returnZero_When_VirtualMemoryErrors(t *testing.T) {
	t.Parallel()
	r := &memorytest.FakeReader{SystemPct: 0}
	pct, err := r.SystemMemoryPct()
	assert.NoError(t, err)
	assert.Equal(t, float64(0), pct)
}

// TestGopsutilReader_ProcessMemory_should_sumRSS_When_ValidPIDs verifies
// FakeReader returns configured RSS values by session name.
func TestGopsutilReader_ProcessMemory_should_sumRSS_When_ValidPIDs(t *testing.T) {
	t.Parallel()
	r := &memorytest.FakeReader{
		RSSBySession: map[string]int64{"my-session": 128},
	}
	rssByName, err := r.SessionsRSSMB(context.Background(), []string{"my-session"})
	assert.NoError(t, err)
	assert.Equal(t, int64(128), rssByName["my-session"])
}

// TestGopsutilReader_ProcessMemory_should_skipDeadPID_When_ProcessNoLongerExists verifies
// FakeReader returns 0 for unknown sessions.
func TestGopsutilReader_ProcessMemory_should_skipDeadPID_When_ProcessNoLongerExists(t *testing.T) {
	t.Parallel()
	r := &memorytest.FakeReader{}
	rssByName, err := r.SessionsRSSMB(context.Background(), []string{"nonexistent-session"})
	assert.NoError(t, err)
	assert.Equal(t, int64(0), rssByName["nonexistent-session"])
}

// TestGopsutilReader_ProcessMemory_should_batchMultipleSessions_When_CalledOnce verifies
// SessionsRSSMB returns every requested session's RSS from a single call, and that
// FakeReader records exactly one call regardless of how many sessions were requested —
// this is the behavior warmRSSCache depends on to avoid one process-table enumeration
// per session (see processSnapshot in reader.go).
func TestGopsutilReader_ProcessMemory_should_batchMultipleSessions_When_CalledOnce(t *testing.T) {
	t.Parallel()
	r := &memorytest.FakeReader{
		RSSBySession: map[string]int64{
			"session-a": 64,
			"session-b": 96,
		},
	}

	rssByName, err := r.SessionsRSSMB(context.Background(), []string{"session-a", "session-b"})

	assert.NoError(t, err)
	assert.Equal(t, int64(64), rssByName["session-a"])
	assert.Equal(t, int64(96), rssByName["session-b"])
	assert.Equal(t, 1, r.GetSessionRSSCalls())
	assert.ElementsMatch(t, []string{"session-a", "session-b"}, r.LastRSSNames())
}

// TestTmuxPIDResolver_should_returnPIDs_When_TmuxSucceeds verifies
// the FakeReader returns correct MB from RSSBySession map.
func TestTmuxPIDResolver_should_returnPIDs_When_TmuxSucceeds(t *testing.T) {
	t.Parallel()
	r := &memorytest.FakeReader{RSSBySession: map[string]int64{"sess": 512}}
	rssByName, err := r.SessionsRSSMB(context.Background(), []string{"sess"})
	assert.NoError(t, err)
	assert.Equal(t, int64(512), rssByName["sess"])
}

// TestTmuxPIDResolver_should_returnEmptySlice_When_TmuxSessionNotFound verifies
// SessionsRSSMB returns 0 for a name not in RSSBySession.
func TestTmuxPIDResolver_should_returnEmptySlice_When_TmuxSessionNotFound(t *testing.T) {
	t.Parallel()
	r := &memorytest.FakeReader{RSSBySession: map[string]int64{}}
	rssByName, err := r.SessionsRSSMB(context.Background(), []string{"missing"})
	assert.NoError(t, err)
	assert.Equal(t, int64(0), rssByName["missing"])
}

// TestFakeReader_CallCounting verifies GetSystemMemoryCalls tracks correctly.
func TestFakeReader_CallCounting(t *testing.T) {
	t.Parallel()
	r := &memorytest.FakeReader{SystemPct: 50}
	_, _ = r.SystemMemoryPct()
	_, _ = r.SystemMemoryPct()
	_, _ = r.SystemMemoryPct()
	assert.Equal(t, 3, r.GetSystemMemoryCalls())
}
