// Package rulestore persists the admin-editable approval and tagging rules and the
// tagging-rule fire log. session.EntRepository embeds *Rules so existing callers keep their
// method set.
package rulestore

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/approvalrule"
	"github.com/tstapler/stapler-squad/session/ent/taggingrule"
	"github.com/tstapler/stapler-squad/session/ent/taggingrulefire"
)

// Rules owns the approval_rules, tagging_rules and tagging_rule_fires tables.
type Rules struct {
	client *ent.Client
}

// New returns Rules over client, which must already hold the migrated schema.
func New(client *ent.Client) *Rules {
	return &Rules{client: client}
}

func (r *Rules) AllRules(ctx context.Context) ([]domain.ApprovalRuleData, error) {
	//nolint:entfullscan approval rules are a small, admin-configured table; the whole set is needed to evaluate rule precedence.
	rules, err := r.client.ApprovalRule.Query().
		Order(ent.Asc(approvalrule.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]domain.ApprovalRuleData, len(rules))
	for i, rule := range rules {
		result[i] = domain.ApprovalRuleData{
			ID:             rule.RuleID,
			Name:           rule.Name,
			ToolName:       rule.ToolName,
			ToolPattern:    rule.ToolPattern,
			ToolCategory:   rule.ToolCategory,
			CommandPattern: rule.CommandPattern,
			FilePattern:    rule.FilePattern,
			Decision:       rule.Decision,
			RiskLevel:      rule.RiskLevel,
			Reason:         rule.Reason,
			Alternative:    rule.Alternative,
			Priority:       rule.Priority,
			Enabled:        rule.Enabled,
			Source:         rule.Source,
			CreatedAt:      rule.CreatedAt,
			UpdatedAt:      rule.UpdatedAt,

			Programs:              rule.Programs,
			Subcommands:           rule.Subcommands,
			BlockedSubcommands:    rule.BlockedSubcommands,
			RequiredFlags:         rule.RequiredFlags,
			ForbiddenFlags:        rule.ForbiddenFlags,
			RequiredFlagPrefixes:  rule.RequiredFlagPrefixes,
			PythonModes:           rule.PythonModes,
			SafePythonImportsOnly: rule.SafePythonImportsOnly,
			RequireCIPassing:      rule.RequireCiPassing,
			MinSessionIdleMinutes: rule.MinSessionIdleMinutes,
		}
	}
	return result, nil
}

func (r *Rules) UpsertRule(ctx context.Context, data domain.ApprovalRuleData) error {
	programs := data.Programs
	if programs == nil {
		programs = []string{}
	}
	subcommands := data.Subcommands
	if subcommands == nil {
		subcommands = []string{}
	}
	blockedSubcommands := data.BlockedSubcommands
	if blockedSubcommands == nil {
		blockedSubcommands = []string{}
	}
	requiredFlags := data.RequiredFlags
	if requiredFlags == nil {
		requiredFlags = []string{}
	}
	forbiddenFlags := data.ForbiddenFlags
	if forbiddenFlags == nil {
		forbiddenFlags = []string{}
	}
	requiredFlagPrefixes := data.RequiredFlagPrefixes
	if requiredFlagPrefixes == nil {
		requiredFlagPrefixes = []string{}
	}
	pythonModes := data.PythonModes
	if pythonModes == nil {
		pythonModes = []string{}
	}
	return r.client.ApprovalRule.Create().
		SetRuleID(data.ID).
		SetName(data.Name).
		SetToolName(data.ToolName).
		SetToolPattern(data.ToolPattern).
		SetToolCategory(data.ToolCategory).
		SetCommandPattern(data.CommandPattern).
		SetFilePattern(data.FilePattern).
		SetDecision(data.Decision).
		SetRiskLevel(data.RiskLevel).
		SetReason(data.Reason).
		SetAlternative(data.Alternative).
		SetPriority(data.Priority).
		SetEnabled(data.Enabled).
		SetSource(data.Source).
		SetPrograms(programs).
		SetSubcommands(subcommands).
		SetBlockedSubcommands(blockedSubcommands).
		SetRequiredFlags(requiredFlags).
		SetForbiddenFlags(forbiddenFlags).
		SetRequiredFlagPrefixes(requiredFlagPrefixes).
		SetPythonModes(pythonModes).
		SetSafePythonImportsOnly(data.SafePythonImportsOnly).
		SetRequireCiPassing(data.RequireCIPassing).
		SetMinSessionIdleMinutes(data.MinSessionIdleMinutes).
		OnConflictColumns(approvalrule.FieldRuleID).
		UpdateNewValues().
		Exec(ctx)
}

func (r *Rules) DeleteRule(ctx context.Context, id string) error {
	_, err := r.client.ApprovalRule.Delete().
		Where(approvalrule.RuleID(id)).
		Exec(ctx)
	return err
}

func (r *Rules) AllTaggingRules(ctx context.Context) ([]domain.TaggingRuleData, error) {
	//nolint:entfullscan tagging rules are a small, admin-configured table; the whole set is needed to evaluate rule precedence.
	rules, err := r.client.TaggingRule.Query().
		Order(ent.Asc(taggingrule.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]domain.TaggingRuleData, len(rules))
	for i, rule := range rules {
		result[i] = domain.TaggingRuleData{
			RuleID:         rule.RuleID,
			Name:           rule.Name,
			NamePattern:    rule.NamePattern,
			BranchPattern:  rule.BranchPattern,
			PathPattern:    rule.PathPattern,
			ProgramPattern: rule.ProgramPattern,
			RequiredTags:   rule.RequiredTags,
			OutputTag:      rule.OutputTag,
			Priority:       rule.Priority,
			Enabled:        rule.Enabled,
			Source:         rule.Source,
			CreatedAt:      rule.CreatedAt,
			UpdatedAt:      rule.UpdatedAt,
		}
	}
	return result, nil
}

func (r *Rules) UpsertTaggingRule(ctx context.Context, data domain.TaggingRuleData) error {
	requiredTags := data.RequiredTags
	if requiredTags == nil {
		requiredTags = []string{}
	}
	return r.client.TaggingRule.Create().
		SetRuleID(data.RuleID).
		SetName(data.Name).
		SetNamePattern(data.NamePattern).
		SetBranchPattern(data.BranchPattern).
		SetPathPattern(data.PathPattern).
		SetProgramPattern(data.ProgramPattern).
		SetRequiredTags(requiredTags).
		SetOutputTag(data.OutputTag).
		SetPriority(data.Priority).
		SetEnabled(data.Enabled).
		SetSource(data.Source).
		OnConflictColumns(taggingrule.FieldRuleID).
		UpdateNewValues().
		Exec(ctx)
}

func (r *Rules) DeleteTaggingRule(ctx context.Context, id string) error {
	_, err := r.client.TaggingRule.Delete().
		Where(taggingrule.RuleID(id)).
		Exec(ctx)
	return err
}

// RecordTaggingRuleFire inserts a fire record for ruleID at firedAt.
func (r *Rules) RecordTaggingRuleFire(ctx context.Context, ruleID string, firedAt time.Time) error {
	return r.client.TaggingRuleFire.Create().
		SetRuleID(ruleID).
		SetFiredAt(firedAt).
		Exec(ctx)
}

// GetTaggingRuleFireCounts returns a rule_id -> count map of fires recorded at or after since.
func (r *Rules) GetTaggingRuleFireCounts(ctx context.Context, since time.Time) (map[string]int, error) {
	type fireCountRow struct {
		RuleID string `json:"rule_id"`
		Count  int    `json:"count"`
	}
	var rows []fireCountRow
	err := r.client.TaggingRuleFire.Query().
		Where(taggingrulefire.FiredAtGTE(since)).
		GroupBy(taggingrulefire.FieldRuleID).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("get tagging rule fire counts since %s: %w", since.Format(time.RFC3339), err)
	}

	result := make(map[string]int, len(rows))
	for _, row := range rows {
		result[row.RuleID] = row.Count
	}
	return result, nil
}
