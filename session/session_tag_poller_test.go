package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session/headless"
)

// fakeTagPoolClient is a minimal headless.PoolClient test double for
// SessionTagClassificationPoller tests — records every call so tests can assert whether
// GenerateSessionTags was invoked, and what vocabulary it observed, without a real subprocess
// (deterministic-fast-tests).
type fakeTagPoolClient struct {
	mu sync.Mutex

	response string
	err      error

	calls          int
	lastUserPrompt string
}

func (f *fakeTagPoolClient) CallBlocking(_ context.Context, _ headless.FeatureKey, _, userPrompt string, _ headless.CallOptions, sink headless.CostSink) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastUserPrompt = userPrompt
	sink(0)
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

func (f *fakeTagPoolClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeTagPoolClient) userPrompt() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastUserPrompt
}

// tagPollerFixture bundles a poller under test with the live engine backing it, so tests that
// need to mutate the rule set mid-run (vocabulary recomputation) can reach both without a
// multi-value discard at every other call site.
type tagPollerFixture struct {
	poller *SessionTagClassificationPoller
	engine *classifier.TaggingEngine
}

// newTagPollerFixture builds a poller wired to a fake PoolClient and a fresh TaggingEngine
// seeded with the given OutputTags — the vocabulary the poller derives is exactly those tags
// plus UnclassifiedTag.
func newTagPollerFixture(fake *fakeTagPoolClient, outputTags ...string) tagPollerFixture {
	engine := &classifier.TaggingEngine{}
	rules := make([]classifier.TaggingRule, len(outputTags))
	for i, tag := range outputTags {
		rules[i] = classifier.TaggingRule{
			RuleMeta:  classifier.RuleMeta{ID: "rule-" + tag, Name: tag, Priority: 1, Enabled: true, Source: "seed"},
			OutputTag: tag,
		}
	}
	engine.AddRules(rules)
	return tagPollerFixture{poller: NewSessionTagClassificationPoller(fake, engine), engine: engine}
}

// primedInstance builds a bare Instance with a snapshot immediately published so
// SessionTagClassificationPoller.pollOnce sees the given field values on its first tick.
func primedInstance(title string, branch string) *Instance {
	inst := &Instance{Title: title, Branch: branch}
	inst.snapshot.Store(buildSnapshot(inst))
	return inst
}

// renameAndResnapshot mutates title/branch directly and republishes the snapshot (test-only,
// same-package access), mirroring instance_snapshot_test.go's own convention for forcing a
// snapshot rebuild without going through the actor mailbox.
func renameAndResnapshot(inst *Instance, title, branch string) {
	inst.mu.Lock()
	if title != "" {
		inst.Title = title
	}
	if branch != "" {
		inst.Branch = branch
	}
	inst.snapshot.Store(buildSnapshot(inst))
	inst.mu.Unlock()
}

// TestSessionTagPoller_should_NotRaceOrPanic_When_StartStopCalledRepeatedly is a regression test
// for a data race between pollLoop's read of p.ctx and Stop()'s write to it (p.ctx/p.cancel are
// reset to nil under p.mu to make the poller restartable). Both Start and pollLoop now pass the
// goroutine's context down as a local value rather than re-reading the mutable p.ctx field, so
// this stays race- and panic-free under -race regardless of Start/Stop timing.
func TestSessionTagPoller_should_NotRaceOrPanic_When_StartStopCalledRepeatedly(t *testing.T) {
	t.Parallel()
	fake := &fakeTagPoolClient{response: `{"tags":["Feature"]}`}
	fx := newTagPollerFixture(fake, "Feature")

	for range 100 {
		fx.poller.Start(context.Background())
		fx.poller.Stop()
	}
}

func TestSessionTagPoller_should_SkipLLMCall_When_ContentHashUnchanged(t *testing.T) {
	t.Parallel()
	fake := &fakeTagPoolClient{response: `{"tags":["Feature"]}`}
	fx := newTagPollerFixture(fake, "Feature")
	inst := primedInstance("sess-1", "feature/x")
	fx.poller.SetInstances([]*Instance{inst})

	fx.poller.pollOnce()
	if got := fake.callCount(); got != 1 {
		t.Fatalf("first tick: CallBlocking called %d times, want 1", got)
	}

	fx.poller.pollOnce()
	if got := fake.callCount(); got != 1 {
		t.Fatalf("second tick with unchanged hash: CallBlocking called %d times, want still 1 (skipped)", got)
	}
}

func TestSessionTagPoller_should_CallLLMAndUpdateCache_When_ContentHashChanged(t *testing.T) {
	t.Parallel()
	fake := &fakeTagPoolClient{response: `{"tags":["Feature"]}`}
	fx := newTagPollerFixture(fake, "Feature")
	inst := primedInstance("sess-1", "feature/x")
	fx.poller.SetInstances([]*Instance{inst})

	fx.poller.pollOnce()
	if got := fake.callCount(); got != 1 {
		t.Fatalf("first tick: CallBlocking called %d times, want 1", got)
	}

	// Simulate a rename that changes the content hash; re-register under the poller's instance
	// slice so the cache lookup (keyed by title) misses and re-triggers classification.
	renameAndResnapshot(inst, "sess-1-renamed", "")
	fx.poller.SetInstances([]*Instance{inst})

	fx.poller.pollOnce()
	if got := fake.callCount(); got != 2 {
		t.Fatalf("tick after rename: CallBlocking called %d times total, want 2 (one new call)", got)
	}
}

func TestSessionTagPoller_should_ApplyUnclassifiedAndUpdateCache_When_LLMCallFails(t *testing.T) {
	t.Parallel()
	fake := &fakeTagPoolClient{err: errors.New("fake pool client error")}
	fx := newTagPollerFixture(fake, "Feature")
	inst := primedInstance("sess-2", "feature/x")
	fx.poller.SetInstances([]*Instance{inst})

	fx.poller.pollOnce()

	tags := inst.GetTags()
	if len(tags) != 1 || tags[0] != UnclassifiedTag {
		t.Fatalf("expected tags=[%s] after failed call, got %v", UnclassifiedTag, tags)
	}
	if inst.RuleTagProvenance[UnclassifiedTag] != llmSentinelRuleID {
		t.Errorf("RuleTagProvenance[%s] = %q, want %q", UnclassifiedTag, inst.RuleTagProvenance[UnclassifiedTag], llmSentinelRuleID)
	}

	// Second tick with no session change must not re-call the LLM (cache updated on failure too).
	fx.poller.pollOnce()
	if got := fake.callCount(); got != 1 {
		t.Fatalf("second tick after failure with no session change: CallBlocking called %d times total, want still 1 (skipped)", got)
	}
}

// vocabularyFromPrompt extracts the "Vocabulary (choose only from these): ..." line's
// comma-separated values from a GenerateSessionTags user prompt, for asserting exactly which
// vocabulary the fake PoolClient observed — proving the claim at the argument level, not just
// that the call succeeded (Story 4.3.1's fourth acceptance criterion).
func vocabularyFromPrompt(prompt string) []string {
	const marker = "Vocabulary (choose only from these): "
	idx := strings.Index(prompt, marker)
	if idx < 0 {
		return nil
	}
	rest := prompt[idx+len(marker):]
	if nl := strings.Index(rest, "\n"); nl >= 0 {
		rest = rest[:nl]
	}
	parts := strings.Split(rest, ",")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

func TestSessionTagPoller_should_RecomputeVocabularyFromLiveEngine_When_RuleAddedBetweenTicks(t *testing.T) {
	t.Parallel()
	fake := &fakeTagPoolClient{response: `{"tags":["Hotfix"]}`}
	fx := newTagPollerFixture(fake, "Bugfix", "Feature")
	inst := primedInstance("sess-3", "feature/x")
	fx.poller.SetInstances([]*Instance{inst})

	fx.poller.pollOnce()
	firstVocab := vocabularyFromPrompt(fake.userPrompt())
	if !slicesContainString(firstVocab, "Bugfix") {
		t.Fatalf("first tick vocabulary missing seeded rule tags: %v", firstVocab)
	}
	if slicesContainString(firstVocab, "Hotfix") {
		t.Fatalf("first tick vocabulary must not yet contain the not-yet-added Hotfix tag: %v", firstVocab)
	}

	// Simulate a CRUD-added rule (TaggingRulesService.UpsertTaggingRule → engine.ReplaceRules)
	// landing mid-run, and force a hash change so the second tick actually re-classifies.
	fx.engine.AddRules([]classifier.TaggingRule{{
		RuleMeta:  classifier.RuleMeta{ID: "rule-hotfix", Name: "Hotfix", Priority: 1, Enabled: true, Source: "user"},
		OutputTag: "Hotfix",
	}})
	renameAndResnapshot(inst, "", "hotfix/y")

	fx.poller.pollOnce()
	secondVocab := vocabularyFromPrompt(fake.userPrompt())
	if !slicesContainString(secondVocab, "Hotfix") {
		t.Fatalf("second tick vocabulary must contain CRUD-added Hotfix tag with no poller restart: %v", secondVocab)
	}
}

func slicesContainString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
