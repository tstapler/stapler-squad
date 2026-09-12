package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session"
)

// newTestTaggingRulesService builds a TaggingRulesService wired to a fresh, empty
// TaggingRulesStore and a TaggingEngine with zero rules (the seed rules ReplaceRules(nil)
// clears), backed by an in-memory Ent repository for use by this file's tests.
func newTestTaggingRulesService(t *testing.T, analyticsStore *AnalyticsStore) (*TaggingRulesService, *classifier.TaggingEngine) {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	rulesStore, err := NewTaggingRulesStore(storage)
	require.NoError(t, err)

	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules(nil)

	return NewTaggingRulesService(rulesStore, engine, analyticsStore), engine
}

// TestTaggingRulesService_UpsertTaggingRule_should_RebuildEngineImmediately_When_RuleAdded
// covers Story 2.3.2's acceptance criterion: no restart or explicit reload call needed.
func TestTaggingRulesService_UpsertTaggingRule_should_RebuildEngineImmediately_When_RuleAdded(t *testing.T) {
	svc, engine := newTestTaggingRulesService(t, nil)

	_, err := svc.UpsertTaggingRule(context.Background(), TaggingRuleSpec{
		ID:            "hotfix-rule",
		Name:          "Hotfix branch",
		BranchPattern: "^hotfix/",
		OutputTag:     "Hotfix",
		Enabled:       true,
		Source:        "user",
	})
	require.NoError(t, err)

	added, _, _ := engine.ApplyToFixpoint(classifier.SessionTaggingContext{Branch: "hotfix/x"})
	require.Len(t, added, 1)
	assert.Equal(t, "Hotfix", added[0].Tag)
}

// TestTaggingRulesService_UpsertTaggingRule_should_LeaveEngineUnchanged_When_UpsertFails
// covers the failed-upsert isolation case from validation.md's Test Mapping: a rejected
// (bad-regex) upsert must not rebuild/corrupt the live TaggingEngine.
func TestTaggingRulesService_UpsertTaggingRule_should_LeaveEngineUnchanged_When_UpsertFails(t *testing.T) {
	svc, engine := newTestTaggingRulesService(t, nil)

	before := engine.Rules()

	_, err := svc.UpsertTaggingRule(context.Background(), TaggingRuleSpec{
		ID:            "bad-rule",
		Name:          "Bad rule",
		BranchPattern: "^(unterminated[",
		OutputTag:     "X",
	})
	require.Error(t, err)

	assert.Equal(t, before, engine.Rules(), "engine rules must be unchanged after a rejected upsert")
}

// TestTaggingRulesService_ListTaggingRules_should_DefaultFireCountToZero_When_AnalyticsStoreNil
// covers Story 2.3.3's third acceptance criterion: a nil analyticsStore must never panic and
// must default every rule's fire count to 0.
func TestTaggingRulesService_ListTaggingRules_should_DefaultFireCountToZero_When_AnalyticsStoreNil(t *testing.T) {
	svc, _ := newTestTaggingRulesService(t, nil)

	_, err := svc.UpsertTaggingRule(context.Background(), TaggingRuleSpec{
		ID:            "seed-bugfix",
		Name:          "Bugfix branch",
		BranchPattern: "^(bugfix|fix)/",
		OutputTag:     "Bugfix",
		Enabled:       true,
		Source:        "user",
	})
	require.NoError(t, err)

	rules, err := svc.ListTaggingRules(context.Background())
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, 0, rules[0].FireCount7d)
}

// TestTaggingRulesService_ListTaggingRules_should_SurfaceFireCount_When_AnalyticsStoreRecordsFires
// covers Story 2.3.3's third acceptance criterion's positive case: a real analyticsStore's
// recorded fires surface on the matching rule by RuleID.
func TestTaggingRulesService_ListTaggingRules_should_SurfaceFireCount_When_AnalyticsStoreRecordsFires(t *testing.T) {
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	rulesStore, err := NewTaggingRulesStore(storage)
	require.NoError(t, err)

	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules(nil)

	analyticsStore := NewAnalyticsStore(storage)
	svc := NewTaggingRulesService(rulesStore, engine, analyticsStore)

	_, err = svc.UpsertTaggingRule(context.Background(), TaggingRuleSpec{
		ID:            "seed-bugfix",
		Name:          "Bugfix branch",
		BranchPattern: "^(bugfix|fix)/",
		OutputTag:     "Bugfix",
		Enabled:       true,
		Source:        "user",
	})
	require.NoError(t, err)

	require.NoError(t, storage.RecordTaggingRuleFire(context.Background(), "seed-bugfix", time.Now()))
	require.NoError(t, storage.RecordTaggingRuleFire(context.Background(), "seed-bugfix", time.Now()))

	rules, err := svc.ListTaggingRules(context.Background())
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, 2, rules[0].FireCount7d)
}
