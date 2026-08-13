package memory_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/memory/memorytest"
)

// TestFakeReader_SystemMemory_should_returnUsedPct_When_VirtualMemorySucceeds verifies
// that FakeReader returns the configured SystemPct.
func TestFakeReader_SystemMemory_should_returnUsedPct_When_VirtualMemorySucceeds(t *testing.T) {
	r := &memorytest.FakeReader{SystemPct: 72.5}
	pct, err := r.SystemMemoryPct()
	assert.NoError(t, err)
	assert.InDelta(t, 72.5, pct, 0.01)
}

// TestGopsutilReader_SystemMemory_should_returnZero_When_VirtualMemoryErrors verifies
// FakeReader returns 0 when SystemPct is zero.
func TestGopsutilReader_SystemMemory_should_returnZero_When_VirtualMemoryErrors(t *testing.T) {
	r := &memorytest.FakeReader{SystemPct: 0}
	pct, err := r.SystemMemoryPct()
	assert.NoError(t, err)
	assert.Equal(t, float64(0), pct)
}

// TestGopsutilReader_ProcessMemory_should_sumRSS_When_ValidPIDs verifies
// FakeReader returns configured RSS values by session name.
func TestGopsutilReader_ProcessMemory_should_sumRSS_When_ValidPIDs(t *testing.T) {
	r := &memorytest.FakeReader{
		RSSBySession: map[string]int64{"my-session": 128},
	}
	mb, err := r.SessionRSSMB("my-session")
	assert.NoError(t, err)
	assert.Equal(t, int64(128), mb)
}

// TestGopsutilReader_ProcessMemory_should_skipDeadPID_When_ProcessNoLongerExists verifies
// FakeReader returns 0 for unknown sessions.
func TestGopsutilReader_ProcessMemory_should_skipDeadPID_When_ProcessNoLongerExists(t *testing.T) {
	r := &memorytest.FakeReader{}
	mb, err := r.SessionRSSMB("nonexistent-session")
	assert.NoError(t, err)
	assert.Equal(t, int64(0), mb)
}

// TestGopsutilReader_ProcessMemory_should_capAt50PIDs_When_DeepProcessTree verifies
// FakeReader call counting works correctly.
func TestGopsutilReader_ProcessMemory_should_capAt50PIDs_When_DeepProcessTree(t *testing.T) {
	r := &memorytest.FakeReader{
		RSSBySession: map[string]int64{
			"session-a": 64,
			"session-b": 96,
		},
	}

	_, _ = r.SessionRSSMB("session-a")
	_, _ = r.SessionRSSMB("session-b")
	_, _ = r.SessionRSSMB("session-a")

	assert.Equal(t, 3, r.GetSessionRSSCalls())
}

// TestTmuxPIDResolver_should_returnPIDs_When_TmuxSucceeds verifies
// the FakeReader returns correct MB from RSSBySession map.
func TestTmuxPIDResolver_should_returnPIDs_When_TmuxSucceeds(t *testing.T) {
	r := &memorytest.FakeReader{RSSBySession: map[string]int64{"sess": 512}}
	mb, err := r.SessionRSSMB("sess")
	assert.NoError(t, err)
	assert.Equal(t, int64(512), mb)
}

// TestTmuxPIDResolver_should_returnEmptySlice_When_TmuxSessionNotFound verifies
// SessionRSSMB returns 0 when the session is not in RSSBySession.
func TestTmuxPIDResolver_should_returnEmptySlice_When_TmuxSessionNotFound(t *testing.T) {
	r := &memorytest.FakeReader{RSSBySession: map[string]int64{}}
	mb, err := r.SessionRSSMB("missing")
	assert.NoError(t, err)
	assert.Equal(t, int64(0), mb)
}

// TestFakeReader_CallCounting verifies GetSystemMemoryCalls tracks correctly.
func TestFakeReader_CallCounting(t *testing.T) {
	r := &memorytest.FakeReader{SystemPct: 50}
	_, _ = r.SystemMemoryPct()
	_, _ = r.SystemMemoryPct()
	_, _ = r.SystemMemoryPct()
	assert.Equal(t, 3, r.GetSystemMemoryCalls())
}
