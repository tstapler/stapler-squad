package session

import (
	"errors"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// session_tagging_integration_test.go implements plan.md's Story 7.2.1: one test exercising
// the full pipeline seam-to-seam (sync fixpoint -> retraction -> LLM-poller fallback ->
// Unclassified-drop) end to end, since every other test in this package proves one seam in
// isolation. Uses a real *classifier.TaggingEngine, a real *Instance driven through its actor
// setters, and a fake headless.PoolClient — no real subprocess (deterministic-fast-tests).

// neverMatchingRule returns a TaggingRule whose condition can never hold for any Instance
// built by minimalInstance, used purely so a given OutputTag appears in
// currentVocabulary(engine)'s derived vocabulary without the rule ever firing itself.
func neverMatchingRule(id, outputTag string) classifier.TaggingRule {
	return classifier.TaggingRule{
		RuleMeta:       classifier.RuleMeta{ID: id, Name: outputTag, Priority: 1, Enabled: true, Source: "seed"},
		ProgramPattern: regexp.MustCompile(`^this-program-never-exists$`),
		OutputTag:      outputTag,
	}
}

// TestSessionTaggingPipeline_should_ProduceFinalLLMTagWithNoUnclassified_When_SyncTagRetractedThenPollerSucceedsFirstTry
// implements Story 7.2.1's Given-When-Then: a session on a branch matching the seed
// branch-pattern rule gets "Bugfix" applied synchronously with rule provenance; renaming off
// that branch retracts it; a single poller tick against a fake PoolClient returning
// ["Refactor"] then produces a final tag state of exactly ["Refactor"] with
// RuleTagProvenance["Refactor"] == "llm", and "Unclassified" never appears at any point.
func TestSessionTaggingPipeline_should_ProduceFinalLLMTagWithNoUnclassified_When_SyncTagRetractedThenPollerSucceedsFirstTry(t *testing.T) {
	t.Parallel()

	inst := minimalInstance(t)
	inst.Program = "claude"
	engine := classifier.NewTaggingEngine()
	// bugfixSeedRule (instance_actor_setters_test.go) mirrors SeedTaggingRules()'s seed-bugfix
	// entry. neverMatchingRule seeds "Refactor" into the vocabulary without ever firing as a
	// sync rule, so the later LLM step's ["Refactor"] response is in-vocabulary.
	engine.ReplaceRules([]classifier.TaggingRule{bugfixSeedRule(), neverMatchingRule("seed-refactor", "Refactor")})
	inst.SetTaggingEngine(engine)

	// Given: creating/mutating onto a branch matching the seed rule applies "Bugfix"
	// synchronously with rule provenance, zero manual tagging action.
	inst.SetGitHubResolution(GitHubResolution{Path: inst.Path, Branch: "bugfix/pr-poller"})
	require.Contains(t, inst.GetTags(), "Bugfix")
	require.Equal(t, "seed-bugfix", inst.RuleTagProvenance["Bugfix"])
	require.NotContains(t, inst.GetTags(), UnclassifiedTag)

	// When: the session is renamed off that branch, the sync fixpoint retracts "Bugfix".
	inst.SetGitHubResolution(GitHubResolution{Path: inst.Path, Branch: "main"})
	require.NotContains(t, inst.GetTags(), "Bugfix", "rule-owned tag must be retracted once its branch condition no longer holds")
	require.NotContains(t, inst.GetTags(), UnclassifiedTag)

	// And: one poller tick runs against a fake PoolClient that succeeds on the very first
	// attempt with an in-vocabulary tag (batch envelope keyed by the instance title).
	fake := &fakeTagPoolClient{response: `{"results":[{"name":` + strconv.Quote(t.Name()) + `,"tags":["Refactor"]}]}`}
	poller := NewSessionTagClassificationPoller(fake, engine)
	poller.SetInstances([]*Instance{inst})
	poller.pollOnce()

	// Then: the final tag state is exactly ["Refactor"] with LLM provenance, and
	// "Unclassified" never appears (the LLM call succeeded on the first attempt).
	tags := inst.GetTags()
	assert.Equal(t, []string{"Refactor"}, tags)
	assert.Equal(t, "llm", inst.RuleTagProvenance["Refactor"])
	assert.NotContains(t, tags, UnclassifiedTag)
	assert.Equal(t, 1, fake.callCount(), "the LLM must be called exactly once")
}

// TestSessionTaggingPipeline_should_LeaveTagsEmptyThenApplyRealTag_When_FirstPollFailsAndSecondSucceeds
// covers a failed first tick (no tags applied) followed by a successful retry.
func TestSessionTaggingPipeline_should_LeaveTagsEmptyThenApplyRealTag_When_FirstPollFailsAndSecondSucceeds(t *testing.T) {
	t.Parallel()

	inst := minimalInstance(t)
	inst.Program = "claude"
	inst.Branch = "feature/x"
	inst.snapshot.Store(buildSnapshot(inst))

	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{neverMatchingRule("seed-feature", "Feature")})

	fake := &fakeTagPoolClient{err: errors.New("fake pool client error")}
	poller := NewSessionTagClassificationPoller(fake, engine)
	// Cooldown disabled: the second tick must re-classify immediately after the branch
	// change (this test proves Unclassified-drop coexistence, not the cooldown gate).
	poller.config.MinReclassifyInterval = 0
	poller.SetInstances([]*Instance{inst})

	// First tick: the LLM call fails, so no tags are applied.
	poller.pollOnce()
	tags := inst.GetTags()
	require.Empty(t, tags)
	require.Equal(t, 1, fake.callCount())

	// Change the session's classification-relevant content (branch rename) so the content
	// hash changes and the second tick actually re-invokes the LLM rather than hitting cache.
	renameAndResnapshot(inst, "", "feature/y")
	fake.mu.Lock()
	fake.err = nil
	fake.response = `{"results":[{"name":` + strconv.Quote(t.Name()) + `,"tags":["Feature"]}]}`
	fake.mu.Unlock()

	poller.pollOnce()

	tags = inst.GetTags()
	assert.Equal(t, []string{"Feature"}, tags, "a later successful poll must apply the real tag")
	assert.Equal(t, "llm", inst.RuleTagProvenance["Feature"])
	assert.Equal(t, 2, fake.callCount())
}
