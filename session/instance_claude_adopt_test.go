package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func extractFixtureInstance(f *claimFixture, repo string, mutate func(*Instance)) *Instance {
	inst := &Instance{
		Title:           "extract",
		Path:            repo,
		SessionType:     SessionTypeDirectory,
		CreatedAt:       f.now.Add(-time.Minute),
		historyDetector: f.linker.detector,
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
	inst.tryExtractConversationUUID()

	assert.Equal(t, claimOther, extractedUUID(inst))
}

func TestTryExtract_AllOwned_StaysUnlinked(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/extract")
	f.write(t, claimSibling, "{}", time.Second)

	inst := extractFixtureInstance(f, "/repo/extract", func(i *Instance) {
		i.SetConversationOwnershipGuard(func(string, string) (string, bool) { return "sibling", true })
	})
	inst.tryExtractConversationUUID()

	assert.Empty(t, extractedUUID(inst))
}

func TestTryExtract_SkipsConversationPredatingSession(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/extract")
	created := f.now.Add(-time.Minute)
	f.write(t, claimOwnStart, stampedEntry(created.Add(10*time.Second)), 5*time.Second)
	f.write(t, claimOldStart, stampedEntry(created.Add(-time.Hour)), time.Second) // newest mtime

	inst := extractFixtureInstance(f, "/repo/extract", nil)
	inst.tryExtractConversationUUID()

	assert.Equal(t, claimOwnStart, extractedUUID(inst))
}

func TestTryExtract_PredatingConversationAcceptedWhenEverHadHistory(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/extract")
	f.write(t, claimOldStart, stampedEntry(f.now.Add(-time.Hour)), time.Second)

	inst := extractFixtureInstance(f, "/repo/extract", func(i *Instance) { i.EverHadConversationHistory = true })
	inst.tryExtractConversationUUID()

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
	inst.tryExtractConversationUUID()

	require.Equal(t, claimOther, extractedUUID(inst))
}
