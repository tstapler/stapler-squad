package rulestore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
)

func newRules(t testing.TB) *Rules {
	t.Helper()
	return New(entdbtest.NewClient(t))
}

func TestRules_should_OrderByPriorityAndRoundTripEveryField_When_ApprovalRulesUpserted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRules(t)
	full := domain.ApprovalRuleData{
		ID: "b", Name: "n", ToolName: "Bash", ToolPattern: "tp", ToolCategory: "tc", CommandPattern: "cp",
		FilePattern: "fp", Decision: 2, RiskLevel: 3, Reason: "why", Alternative: "alt", Priority: 20,
		Enabled: true, Source: "user", Programs: []string{"git"}, Subcommands: []string{"status"},
		BlockedSubcommands: []string{"push"}, RequiredFlags: []string{"-n"}, ForbiddenFlags: []string{"-f"},
		RequiredFlagPrefixes: []string{"--x"}, PythonModes: []string{"-c"}, SafePythonImportsOnly: true,
		RequireCIPassing: true, MinSessionIdleMinutes: 7,
	}
	require.NoError(t, r.UpsertRule(ctx, full))
	require.NoError(t, r.UpsertRule(ctx, domain.ApprovalRuleData{ID: "a", Name: "a", Priority: 10}))

	got, err := r.AllRules(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].ID, "lower priority sorts first")
	assert.Empty(t, got[0].Programs)
	assert.NotNil(t, got[0].Programs, "nil slices persist as empty, not null")
	got[1].CreatedAt, got[1].UpdatedAt = time.Time{}, time.Time{}
	assert.Equal(t, full, got[1])
}

func TestRules_should_ReplaceInPlaceAndDeleteByID_When_ApprovalRuleUpsertedTwice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRules(t)
	require.NoError(t, r.UpsertRule(ctx, domain.ApprovalRuleData{ID: "a", Name: "one", Priority: 1}))
	require.NoError(t, r.UpsertRule(ctx, domain.ApprovalRuleData{ID: "a", Name: "two", Priority: 2}))
	require.NoError(t, r.UpsertRule(ctx, domain.ApprovalRuleData{ID: "keep", Name: "keep", Priority: 3}))

	got, err := r.AllRules(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "two", got[0].Name)

	require.NoError(t, r.DeleteRule(ctx, "a"))
	got, err = r.AllRules(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "keep", got[0].ID)
}

func TestRules_should_OrderByPriorityAndUpsertAtomically_When_TaggingRulesWritten(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRules(t)
	full := domain.TaggingRuleData{
		RuleID: "b", Name: "n", NamePattern: "np", BranchPattern: "bp", PathPattern: "pp", ProgramPattern: "gp",
		RequiredTags: []string{"x"}, OutputTag: "out", Priority: 9, Enabled: true, Source: "user",
	}
	require.NoError(t, r.UpsertTaggingRule(ctx, full))
	require.NoError(t, r.UpsertTaggingRule(ctx, domain.TaggingRuleData{RuleID: "a", Name: "a", OutputTag: "o", Priority: 1}))
	full.Name = "renamed"
	require.NoError(t, r.UpsertTaggingRule(ctx, full))

	got, err := r.AllTaggingRules(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].RuleID)
	assert.NotNil(t, got[0].RequiredTags, "nil RequiredTags persist as empty, not null")
	got[1].CreatedAt, got[1].UpdatedAt = time.Time{}, time.Time{}
	assert.Equal(t, full, got[1])

	require.NoError(t, r.DeleteTaggingRule(ctx, "b"))
	got, err = r.AllTaggingRules(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].RuleID)
}

func TestRules_should_CountOnlyFiresAtOrAfterSincePerRule_When_Queried(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRules(t)
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, r.RecordTaggingRuleFire(ctx, "a", base.Add(-time.Minute)))
	require.NoError(t, r.RecordTaggingRuleFire(ctx, "a", base))
	require.NoError(t, r.RecordTaggingRuleFire(ctx, "a", base.Add(time.Minute)))
	require.NoError(t, r.RecordTaggingRuleFire(ctx, "b", base.Add(time.Hour)))

	got, err := r.GetTaggingRuleFireCounts(ctx, base)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"a": 2, "b": 1}, got)

	none, err := r.GetTaggingRuleFireCounts(ctx, base.Add(24*time.Hour))
	require.NoError(t, err)
	assert.Empty(t, none)
}
