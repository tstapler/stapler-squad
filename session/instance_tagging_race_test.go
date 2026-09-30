package session

// instance_tagging_race_test.go holds the two -race regression tests plan.md's Story 3.3.1
// and Story 3.3.2 both call for explicitly: proof that the fixpoint hook's field
// reads/writes never race a concurrent reader, and that ReclassifyTagsAfterCreate's fresh
// i.mu.Lock() (not setupFirstTimeWorktree's startMu) is what protects Tags/RuleTagProvenance
// on the new-session-creation path. Run with `go test -race`.

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// TestReclassifyTagsLocked_should_NotRace_When_SetterAndGetTagsRunConcurrently mirrors
// TestCreateSession_GitHubURLResolution_NotBoundByRequestContext's shape (server/services/
// session_service_create_test.go): one goroutine repeatedly mutating a tag-relevant field via
// an actor setter, another repeatedly reading Tags via GetTags()/Snapshot(), run under
// `go test -race`. Proves the whole fixpoint pass for one mutation runs inside a single held
// i.mu.Lock() critical section, with no intermediate read of i.Tags outside the lock.
func TestReclassifyTagsLocked_should_NotRace_When_SetterAndGetTagsRunConcurrently(t *testing.T) {
	inst := minimalInstance(t)
	inst.Path = "/tmp/race-test"
	inst.SetTaggingEngine(classifier.NewTaggingEngine())

	const iterations = 200
	branches := []string{"bugfix/x", "main", "feature/y", "hotfix/z"}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			inst.SetGitHubResolution(GitHubResolution{Path: inst.Path, Branch: branches[i%len(branches)]})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = inst.GetTags()
			_ = inst.Snapshot()
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent setter/reader goroutines did not finish in time")
	}
}

// TestReclassifyTagsAfterCreate_should_AcquireOwnLock_When_CalledConcurrentlyWithGetTags proves
// ReclassifyTagsAfterCreate acquires i.mu itself rather than assuming setupFirstTimeWorktree's
// startMu covers Tags/RuleTagProvenance (Story 3.3.2's lock-boundary correctness): a real
// Start() (which runs setupFirstTimeWorktree() then ReclassifyTagsAfterCreate()) races against
// a separate goroutine hammering GetTags()/Snapshot() throughout. `-race` must report clean.
func TestReclassifyTagsAfterCreate_should_AcquireOwnLock_When_CalledConcurrentlyWithGetTags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	base := t.TempDir()
	projectDir := base + "/lock-order-race-project"

	title := fmt.Sprintf("test-tag-lock-order-race-%d", time.Now().UnixNano())
	inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            title,
		Path:             projectDir,
		Program:          "sleep 300",
		SessionType:      SessionTypeNewProject,
		Branch:           "bugfix/lock-order-race",
		TmuxPrefix:       fmt.Sprintf("test_taglockrace_%d_", time.Now().UnixNano()),
		TmuxServerSocket: coldRestoreSocket(t),
	})
	require.NoError(t, err)
	defer func() { _ = cleanup() }()

	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{bugfixSeedRule()})
	inst.SetTaggingEngine(engine)

	stopReading := make(chan struct{})
	var readerWG sync.WaitGroup
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		for {
			select {
			case <-stopReading:
				return
			default:
				_ = inst.GetTags()
				_ = inst.Snapshot()
			}
		}
	}()

	err = inst.Start(true)
	close(stopReading)
	readerWG.Wait()

	require.NoError(t, err)
	assert.Contains(t, inst.GetTags(), "Bugfix")
}
