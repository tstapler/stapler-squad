package instance

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/internal/fixturetest"

	"github.com/tstapler/stapler-squad/session/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func extractFixtureInstance(f *claimFixture, repo string, mutate func(*Instance)) *Instance {
	inst := &Instance{
		Title:           "extract",
		Path:            repo,
		SessionType:     domain.SessionTypeDirectory,
		CreatedAt:       f.now.Add(-time.Minute),
		historyDetector: f.detector,
	}
	if mutate != nil {
		mutate(inst)
	}
	return inst
}

func extractedUUID(inst *Instance) string {
	if inst.claudeSession == nil {
		return ""
	}
	return inst.claudeSession.ConversationUUID
}

func TestTryExtract_OwnedNewestFallsThroughToNextCandidate(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/extract")
	f.write(t, claimOther, "{}", 2*time.Second)
	f.write(t, claimSibling, "{}", time.Second) // newest, owned by a live sibling

	inst := extractFixtureInstance(f, "/repo/extract", func(i *Instance) {
		i.SetConversationOwnershipGuard(func(uuid, _ string) (string, bool) { return "sibling", uuid == claimSibling })
	})
	inst.TryExtractConversationUUID()

	assert.Equal(t, claimOther, extractedUUID(inst))
}

func TestTryExtract_AllOwned_StaysUnlinked(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/extract")
	f.write(t, claimSibling, "{}", time.Second)

	inst := extractFixtureInstance(f, "/repo/extract", func(i *Instance) {
		i.SetConversationOwnershipGuard(func(string, string) (string, bool) { return "sibling", true })
	})
	inst.TryExtractConversationUUID()

	assert.Empty(t, extractedUUID(inst))
}

func TestTryExtract_SkipsConversationPredatingSession(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/extract")
	created := f.now.Add(-time.Minute)
	f.write(t, claimOwnStart, stampedEntry(created.Add(10*time.Second)), 5*time.Second)
	f.write(t, claimOldStart, stampedEntry(created.Add(-time.Hour)), time.Second) // newest mtime

	inst := extractFixtureInstance(f, "/repo/extract", nil)
	inst.TryExtractConversationUUID()

	assert.Equal(t, claimOwnStart, extractedUUID(inst))
}

func TestTryExtract_PredatingConversationAcceptedWhenEverHadHistory(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/extract")
	f.write(t, claimOldStart, stampedEntry(f.now.Add(-time.Hour)), time.Second)

	inst := extractFixtureInstance(f, "/repo/extract", func(i *Instance) { i.EverHadConversationHistory = true })
	inst.TryExtractConversationUUID()

	assert.Equal(t, claimOldStart, extractedUUID(inst))
}

func TestTryExtract_ClearedAt_SkipsStaleAndTakesNewer(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/extract")
	clearedAt := f.now.Add(-time.Minute)
	f.write(t, claimOldStart, "{}", time.Hour) // before the clear
	f.write(t, claimOther, "{}", time.Second)  // after the clear

	inst := extractFixtureInstance(f, "/repo/extract", func(i *Instance) {
		i.EverHadConversationHistory = true
		i.conversationClearedAt = clearedAt
	})
	inst.TryExtractConversationUUID()

	require.Equal(t, claimOther, extractedUUID(inst))
}

const (
	claimSibling  = "cccccccc-0000-0000-0000-000000000000"
	claimOther    = "dddddddd-0000-0000-0000-000000000000"
	claimOldStart = "eeeeeeee-0000-0000-0000-000000000000"
	claimOwnStart = "ffffffff-0000-0000-0000-000000000000"
)

type claimFixture struct {
	detector *HistoryFileDetector
	dir      string
	now      time.Time
}

func newClaimFixture(t *testing.T, repoPath string) *claimFixture {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", ClaudeProjectDirName(repoPath))
	require.NoError(t, os.MkdirAll(dir, 0755))
	detector := NewHistoryFileDetectorWithHomeDir(&fixturetest.MockProcessInspector{Files: []string{}}, home)
	return &claimFixture{detector: detector, dir: dir, now: time.Now()}
}

// write creates <uuid>.jsonl with the given content and mtime = now - age.
func (f *claimFixture) write(t *testing.T, uuid, content string, age time.Duration) string {
	t.Helper()
	p := filepath.Join(f.dir, uuid+".jsonl")
	require.NoError(t, os.WriteFile(p, []byte(content), 0644))
	mt := f.now.Add(-age)
	require.NoError(t, os.Chtimes(p, mt, mt))
	return p
}

func stampedEntry(ts time.Time) string {
	return `{"type":"queue-operation","timestamp":"` + ts.UTC().Format(time.RFC3339Nano) + `"}` + "\n"
}
