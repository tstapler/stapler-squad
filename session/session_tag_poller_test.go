package session

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	ssqlog "github.com/tstapler/stapler-squad/log"
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
	sink(0, true)
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

func TestSessionTagPoller_should_SkipLLMCall_When_AlreadyClassified(t *testing.T) {
	t.Parallel()
	fake := &fakeTagPoolClient{response: `{"results":[{"name":"sess-1","tags":["Feature"]}]}`}
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
	fake := &fakeTagPoolClient{response: `{"results":[{"name":"sess-1","tags":["Feature"]},{"name":"sess-1-renamed","tags":["Feature"]}]}`}
	fx := newTagPollerFixture(fake, "Feature")
	inst := primedInstance("sess-1", "feature/x")
	fx.poller.SetInstances([]*Instance{inst})

	fx.poller.pollOnce()
	if got := fake.callCount(); got != 1 {
		t.Fatalf("first tick: CallBlocking called %d times, want 1", got)
	}

	// Simulate a rename: the new title has no cache entry, so classify-once does not cover
	// it and the next tick classifies it (renames are deliberate, not flaps).
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

// TestSessionTagPoller_should_LogFailedUnclassifiedOutcome_When_GenerateSessionTagsDegrades is
// the regression test for the bug fixed alongside this test (commit 55cd5ac24 added this
// structured logging specifically for classification-outcome visibility, but classifyOne was
// keying the "failed_unclassified" label off GenerateSessionTags' err return, which that
// function's doc comment says is never non-nil — every call, successful or not, logged
// "applied"). GenerateSessionTags' new degraded return value now carries this signal instead,
// derived from a real internal failure (here: a hard CallBlocking error) rather than a
// never-populated error.
func TestSessionTagPoller_should_LogFailedUnclassifiedOutcome_When_GenerateSessionTagsDegrades(t *testing.T) {
	var buf bytes.Buffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { ssqlog.SetSlogDefaultForTest(prev) })

	fake := &fakeTagPoolClient{err: errors.New("fake pool client error")}
	fx := newTagPollerFixture(fake, "Feature")
	inst := primedInstance("sess-degraded", "feature/x")
	fx.poller.SetInstances([]*Instance{inst})

	fx.poller.pollOnce()

	logOutput := buf.String()
	assert.Contains(t, logOutput, `outcome=failed_unclassified`)
	assert.NotContains(t, logOutput, `outcome=applied`)
}

// TestSessionTagPoller_should_LogAppliedOutcome_When_ModelLegitimatelyReturnsUnclassified is the
// companion case: the model itself decided no vocabulary tag fit and returned Unclassified as a
// genuine, in-vocabulary classification (not a CallBlocking/JSON/filtering failure) — this must
// still log "applied", not "failed_unclassified".
func TestSessionTagPoller_should_LogAppliedOutcome_When_ModelLegitimatelyReturnsUnclassified(t *testing.T) {
	var buf bytes.Buffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { ssqlog.SetSlogDefaultForTest(prev) })

	fake := &fakeTagPoolClient{response: `{"results":[{"name":"sess-genuine-unclassified","tags":["Unclassified"]}]}`}
	fx := newTagPollerFixture(fake, "Feature")
	inst := primedInstance("sess-genuine-unclassified", "feature/x")
	fx.poller.SetInstances([]*Instance{inst})

	fx.poller.pollOnce()

	logOutput := buf.String()
	assert.Contains(t, logOutput, `outcome=applied`)
	assert.NotContains(t, logOutput, `outcome=failed_unclassified`)
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
	fake := &fakeTagPoolClient{response: `{"results":[{"name":"sess-3","tags":["Hotfix"]},{"name":"sess-4","tags":["Hotfix"]}]}`}
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
	// landing mid-run. Classify-once covers sess-3, so the second tick classifies a NEW
	// session — proving the recomputed vocabulary reaches classifications with no restart.
	fx.engine.AddRules([]classifier.TaggingRule{{
		RuleMeta:  classifier.RuleMeta{ID: "rule-hotfix", Name: "Hotfix", Priority: 1, Enabled: true, Source: "user"},
		OutputTag: "Hotfix",
	}})
	fx.poller.SetInstances([]*Instance{inst, primedInstance("sess-4", "hotfix/y")})

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

// TestSessionTagPoller_should_IssueOneCall_When_MultipleSessionsChanged verifies the batching
// contract at the poller level: three simultaneously-changed sessions cost exactly one LLM call,
// and every session still gets its own tags applied.
func TestSessionTagPoller_should_IssueOneCall_When_MultipleSessionsChanged(t *testing.T) {
	t.Parallel()
	fake := &fakeTagPoolClient{response: `{"results":[
		{"name":"sess-a","tags":["Feature"]},
		{"name":"sess-b","tags":["Bugfix"]},
		{"name":"sess-c","tags":["Unclassified"]}]}`}
	fx := newTagPollerFixture(fake, "Feature", "Bugfix")
	fx.poller.SetInstances([]*Instance{
		primedInstance("sess-a", "feature/x"),
		primedInstance("sess-b", "bugfix/y"),
		primedInstance("sess-c", "main"),
	})

	fx.poller.pollOnce()

	if got := fake.callCount(); got != 1 {
		t.Fatalf("one tick classifying 3 changed sessions: CallBlocking called %d times, want 1 batch call", got)
	}
	for _, inst := range fx.poller.Instances() {
		if len(inst.GetTags()) == 0 {
			t.Errorf("session %q has no tags after batch classification", inst.Snapshot().Title)
		}
	}
}

// TestSessionTagPoller_should_DeferSurplusSessions_When_ChangedExceedBatchSize verifies the
// backpressure: with MaxBatchSessions=1 and two changed sessions, the first tick classifies
// exactly one session in one call, and the second session waits for the next tick instead of
// triggering a second call in the same tick.
func TestSessionTagPoller_should_DeferSurplusSessions_When_ChangedExceedBatchSize(t *testing.T) {
	t.Parallel()
	fake := &fakeTagPoolClient{response: `{"results":[
		{"name":"sess-a","tags":["Feature"]},
		{"name":"sess-b","tags":["Feature"]}]}`}
	fx := newTagPollerFixture(fake, "Feature")
	fx.poller.config.MaxBatchSessions = 1
	fx.poller.config.MinReclassifyInterval = 0
	instA := primedInstance("sess-a", "feature/x")
	instB := primedInstance("sess-b", "feature/y")
	fx.poller.SetInstances([]*Instance{instA, instB})

	fx.poller.pollOnce()
	if got := fake.callCount(); got != 1 {
		t.Fatalf("first tick: CallBlocking called %d times, want 1 (surplus deferred)", got)
	}
	classified := len(instA.GetTags()) > 0
	deferred := len(instB.GetTags()) > 0
	if classified == deferred {
		t.Fatalf("first tick must classify exactly one of the two sessions (a=%v b=%v)", classified, deferred)
	}

	fx.poller.pollOnce()
	if got := fake.callCount(); got != 2 {
		t.Fatalf("second tick: CallBlocking called %d times total, want 2 (deferred session classified)", got)
	}
	if len(instA.GetTags()) == 0 || len(instB.GetTags()) == 0 {
		t.Fatalf("both sessions must be tagged after two ticks (a=%v b=%v)", instA.GetTags(), instB.GetTags())
	}
}

// TestSessionTagPoller_should_NeverReclassify_When_AlreadyApplied verifies classify-once: a
// branch change landing after a successful classification issues no LLM call, no matter how
// much time passes — the session is left alone until the user explicitly re-runs it.
func TestSessionTagPoller_should_NeverReclassify_When_AlreadyApplied(t *testing.T) {
	t.Parallel()
	fake := &fakeTagPoolClient{response: `{"results":[{"name":"sess-1","tags":["Feature"]}]}`}
	fx := newTagPollerFixture(fake, "Feature")
	inst := primedInstance("sess-1", "feature/x")
	fx.poller.SetInstances([]*Instance{inst})

	fx.poller.pollOnce()
	if got := fake.callCount(); got != 1 {
		t.Fatalf("first tick: CallBlocking called %d times, want 1", got)
	}

	renameAndResnapshot(inst, "", "feature/y")
	fx.poller.pollOnce()
	if got := fake.callCount(); got != 1 {
		t.Fatalf("re-tick after branch change on applied session: CallBlocking called %d times, want still 1 (left alone)", got)
	}
}

// TestSessionTagPoller_should_RetryDegraded_When_BackoffExpired verifies the degraded-retry
// backoff: a failed classification is NOT retried on the immediate next tick (no retry storm
// against a down proxy), but becomes retryable once MinReclassifyInterval elapses.
func TestSessionTagPoller_should_RetryDegraded_When_BackoffExpired(t *testing.T) {
	t.Parallel()
	fake := &fakeTagPoolClient{response: `{"results":[{"name":"sess-1","tags":["Feature"]}]}`}
	fx := newTagPollerFixture(fake, "Feature")
	inst := primedInstance("sess-1", "feature/x")
	fx.poller.SetInstances([]*Instance{inst})

	// Seed a degraded entry as if the last attempt failed just now.
	fx.poller.cache.Store("sess-1", cachedTagResult{tags: []string{UnclassifiedTag}, classifiedAt: time.Now(), applied: false})
	fx.poller.pollOnce()
	if got := fake.callCount(); got != 0 {
		t.Fatalf("tick inside degraded backoff: CallBlocking called %d times, want 0", got)
	}

	if cached, ok := fx.poller.cache.Load("sess-1"); ok {
		cached.classifiedAt = time.Now().Add(-time.Hour)
		fx.poller.cache.Store("sess-1", cached)
	} else {
		t.Fatal("expected a cache entry for sess-1")
	}
	fx.poller.pollOnce()
	if got := fake.callCount(); got != 1 {
		t.Fatalf("tick after backoff expiry: CallBlocking called %d times, want 1", got)
	}
	if tags := inst.GetTags(); len(tags) != 1 || tags[0] != "Feature" {
		t.Fatalf("expected tags=[Feature] after successful retry, got %v", tags)
	}
}

// TestSessionTagPoller_should_HotSwapModels_When_SetModelConfigCalled verifies the settings-UI
// path: SetModelConfig changes the hierarchy the next batch call uses, with no restart.
func TestSessionTagPoller_should_HotSwapModels_When_SetModelConfigCalled(t *testing.T) {
	t.Parallel()
	fake := &fakeTagPoolClient{response: `{"results":[]}`}
	fx := newTagPollerFixture(fake, "Feature")

	if got := fx.poller.modelHierarchy(); len(got) != 1 || got[0] != "haiku" {
		t.Fatalf("default hierarchy = %v, want [haiku]", got)
	}
	fx.poller.SetModelConfig("sonnet", []string{"proxy-free"})
	if got := fx.poller.modelHierarchy(); len(got) != 2 || got[0] != "sonnet" || got[1] != "proxy-free" {
		t.Fatalf("hierarchy after SetModelConfig = %v, want [sonnet proxy-free]", got)
	}
	fx.poller.SetModelConfig("", nil)
	if got := fx.poller.modelHierarchy(); len(got) != 1 || got[0] != "haiku" {
		t.Fatalf("hierarchy after blank reset = %v, want [haiku]", got)
	}
}

// TestSessionTagPoller_should_ClassifySynchronously_When_ClassifyNowCalled verifies the manual
// path: ClassifyNow bypasses the classify-once gate and applies fresh tags immediately,
// returning an error only for unknown titles.
func TestSessionTagPoller_should_ClassifySynchronously_When_ClassifyNowCalled(t *testing.T) {
	t.Parallel()
	fake := &fakeTagPoolClient{response: `{"results":[{"name":"sess-1","tags":["Feature"]}]}`}
	fx := newTagPollerFixture(fake, "Feature")
	inst := primedInstance("sess-1", "feature/x")
	fx.poller.SetInstances([]*Instance{inst})

	fx.poller.pollOnce()
	renameAndResnapshot(inst, "", "feature/y")
	fx.poller.pollOnce()
	if got := fake.callCount(); got != 1 {
		t.Fatalf("automatic re-tick must not reclassify (classify-once): calls=%d, want 1", got)
	}

	if err := fx.poller.ClassifyNow("sess-1"); err != nil {
		t.Fatalf("ClassifyNow(sess-1) returned error: %v", err)
	}
	if got := fake.callCount(); got != 2 {
		t.Fatalf("ClassifyNow: CallBlocking called %d times total, want 2", got)
	}

	if err := fx.poller.ClassifyNow("no-such-session"); err == nil {
		t.Fatal("ClassifyNow(unknown) must return an error")
	}
}

// TestSessionTagPoller_should_NotLogClassificationLines_When_NothingChanged verifies the log
// hygiene fix: a tick where every session is a cache hit must not emit per-session
// "LLM classification" INFO lines (the wall that prompted this change) — only classified
// sessions log that message.
func TestSessionTagPoller_should_NotLogClassificationLines_When_NothingChanged(t *testing.T) {
	var buf bytes.Buffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { ssqlog.SetSlogDefaultForTest(prev) })

	fake := &fakeTagPoolClient{response: `{"results":[{"name":"sess-1","tags":["Feature"]}]}`}
	fx := newTagPollerFixture(fake, "Feature")
	fx.poller.SetInstances([]*Instance{primedInstance("sess-1", "feature/x")})

	fx.poller.pollOnce() // classifies: one INFO classification line expected
	if got := fake.callCount(); got != 1 {
		t.Fatalf("first tick: CallBlocking called %d times, want 1", got)
	}

	buf.Reset()
	fx.poller.pollOnce() // all cache hits: zero LLM spend, zero classification lines
	if got := fake.callCount(); got != 1 {
		t.Fatalf("second tick: CallBlocking called %d times, want still 1", got)
	}
	assert.NotContains(t, buf.String(), "LLM classification",
		"cache-hit tick must not log per-session classification lines")
}
