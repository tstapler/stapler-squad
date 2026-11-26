// Package analyticsstore persists permission-classification analytics and dismissed waste
// findings. session.EntRepository embeds *Analytics so existing callers keep their method set.
package analyticsstore

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/classificationanalytics"
	"github.com/tstapler/stapler-squad/session/ent/dismissedfinding"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
)

// Analytics owns the classification_analytics and dismissed_findings tables.
type Analytics struct {
	client *ent.Client
}

// New returns Analytics over client, which must already hold the migrated schema.
func New(client *ent.Client) *Analytics {
	return &Analytics{client: client}
}

// DismissFinding upserts a DismissedFinding row keyed by data.FindingID.
// Idempotent — dismissing an already-dismissed finding_id just refreshes
// session_id/conversation_id/finding_type (dismissed_at is immutable, so a
// re-dismiss never resets it).
func (a *Analytics) DismissFinding(ctx context.Context, data domain.DismissedFindingData) error {
	dismissedAt := data.DismissedAt
	if dismissedAt.IsZero() {
		dismissedAt = time.Now()
	}
	return a.client.DismissedFinding.Create().
		SetFindingID(data.FindingID).
		SetSessionID(data.SessionID).
		SetConversationID(data.ConversationID).
		SetFindingType(data.FindingType).
		SetDismissedAt(dismissedAt).
		OnConflictColumns(dismissedfinding.FieldFindingID).
		UpdateNewValues().
		Exec(ctx)
}

// ListDismissedFindingIDs returns the set of currently-dismissed finding_id
// values, for GetInsightsSummary to filter against.
func (a *Analytics) ListDismissedFindingIDs(ctx context.Context) (map[string]bool, error) {
	//nolint:entfullscan dismissed findings are a small table (bounded by the
	// findingsCap-capped panel's realistic dismissal volume); the whole set is
	// needed to filter every request's freshly computed findings, same
	// rationale as AllRules above.
	rows, err := a.client.DismissedFinding.Query().
		Select(dismissedfinding.FieldFindingID).
		All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(rows))
	for _, row := range rows {
		ids[row.FindingID] = true
	}
	return ids, nil
}

func (a *Analytics) RecordAnalytics(ctx context.Context, data domain.AnalyticsData) error {
	return a.analyticsCreate(data).Exec(ctx)
}

// RecordAnalyticsBatch persists the batch in one SQLite statement/transaction.
// Exact duplicate analytics IDs are idempotent while every other validation or
// persistence error rejects the whole statement.
func (a *Analytics) RecordAnalyticsBatch(ctx context.Context, batch []domain.AnalyticsData) error {
	if len(batch) == 0 {
		return nil
	}
	builders := make([]*ent.ClassificationAnalyticsCreate, 0, len(batch))
	for _, data := range batch {
		builders = append(builders, a.analyticsCreate(data))
	}
	return a.client.ClassificationAnalytics.CreateBulk(builders...).
		OnConflictColumns(classificationanalytics.FieldAnalyticsID).
		DoNothing().
		Exec(ctx)
}

func (a *Analytics) analyticsCreate(data domain.AnalyticsData) *ent.ClassificationAnalyticsCreate {
	return a.client.ClassificationAnalytics.Create().
		SetAnalyticsID(data.ID).
		SetSessionID(data.SessionID).
		SetToolName(data.ToolName).
		SetCommandPreview(data.CommandPreview).
		SetCwd(data.Cwd).
		SetDecision(data.Decision).
		SetRiskLevel(data.RiskLevel).
		SetRuleID(data.RuleID).
		SetRuleName(data.RuleName).
		SetReason(data.Reason).
		SetAlternative(data.Alternative).
		SetDurationMs(data.DurationMs).
		SetApprovalID(data.ApprovalID).
		SetCommandProgram(data.CommandProgram).
		SetCommandCategory(data.CommandCategory).
		SetCommandSubcategory(data.CommandSubcategory).
		SetPythonImports(data.PythonImports).
		SetSource(data.Source).
		SetCreatedAt(data.CreatedAt)
}

func (a *Analytics) ListAnalytics(ctx context.Context, limit int) ([]domain.AnalyticsData, error) {
	query := a.client.ClassificationAnalytics.Query().
		Order(ent.Desc(classificationanalytics.FieldCreatedAt))

	if limit > 0 {
		query = query.Limit(limit)
	}

	//nolint:entfullscan Order()+optional caller-supplied Limit(); no Where by design (returns most-recent-N analytics rows), acceptable for an analytics/debug endpoint.
	entries, err := query.All(ctx)
	if err != nil {
		return nil, err
	}

	return convertAnalyticsEntries(entries), nil
}

// convertAnalyticsEntry maps a single ent row to the domain model.
func convertAnalyticsEntry(e *ent.ClassificationAnalytics) domain.AnalyticsData {
	return domain.AnalyticsData{
		ID:                 e.AnalyticsID,
		SessionID:          e.SessionID,
		ToolName:           e.ToolName,
		CommandPreview:     e.CommandPreview,
		Cwd:                e.Cwd,
		Decision:           e.Decision,
		RiskLevel:          e.RiskLevel,
		RuleID:             e.RuleID,
		RuleName:           e.RuleName,
		Reason:             e.Reason,
		Alternative:        e.Alternative,
		DurationMs:         e.DurationMs,
		ApprovalID:         e.ApprovalID,
		CommandProgram:     e.CommandProgram,
		CommandCategory:    e.CommandCategory,
		CommandSubcategory: e.CommandSubcategory,
		PythonImports:      e.PythonImports,
		Source:             e.Source,
		CreatedAt:          e.CreatedAt,
	}
}

// convertAnalyticsEntries maps a slice of ent rows to the domain model.
func convertAnalyticsEntries(es []*ent.ClassificationAnalytics) []domain.AnalyticsData {
	out := make([]domain.AnalyticsData, len(es))
	for i, e := range es {
		out[i] = convertAnalyticsEntry(e)
	}
	return out
}

func (a *Analytics) ListAnalyticsSince(ctx context.Context, since time.Time, limit int) ([]domain.AnalyticsData, error) {
	query := a.client.ClassificationAnalytics.Query().
		Where(classificationanalytics.CreatedAtGTE(since)).
		Order(ent.Desc(classificationanalytics.FieldCreatedAt))
	if limit > 0 {
		query = query.Limit(limit)
	}
	entries, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list analytics since %s: %w", since.Format(time.RFC3339), err)
	}
	return convertAnalyticsEntries(entries), nil
}

func (a *Analytics) ListAnalyticsByProgramSince(ctx context.Context, program string, since time.Time, limit int) ([]domain.AnalyticsData, error) {
	query := a.client.ClassificationAnalytics.Query().
		Where(
			classificationanalytics.CommandProgramEQ(program),
			classificationanalytics.CreatedAtGTE(since),
		).
		Order(ent.Desc(classificationanalytics.FieldCreatedAt))
	if limit > 0 {
		query = query.Limit(limit)
	}
	entries, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list analytics by program %q since %s: %w", program, since.Format(time.RFC3339), err)
	}
	return convertAnalyticsEntries(entries), nil
}

func (a *Analytics) GetSubcommandBreakdown(ctx context.Context, program string, since time.Time) ([]domain.SubcommandDecisionCount, error) {
	type breakdownRow struct {
		CommandSubcategory string `json:"command_subcategory"`
		Decision           string `json:"decision"`
		Count              int    `json:"count"`
	}
	var rows []breakdownRow
	// Exclude rows where command_subcategory IS NULL to avoid sql.ScanSlice
	// errors when scanning nullable columns into a plain string field.
	err := a.client.ClassificationAnalytics.Query().
		Where(
			classificationanalytics.CommandProgramEQ(program),
			classificationanalytics.CreatedAtGTE(since),
			classificationanalytics.CommandSubcategoryNotNil(),
		).
		GroupBy(
			classificationanalytics.FieldCommandSubcategory,
			classificationanalytics.FieldDecision,
		).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("subcommand breakdown for %q: %w", program, err)
	}
	result := make([]domain.SubcommandDecisionCount, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.SubcommandDecisionCount{
			Subcommand: row.CommandSubcategory,
			Decision:   row.Decision,
			Count:      row.Count,
		})
	}
	return result, nil
}

func (a *Analytics) ListRecentCommandsByProgram(ctx context.Context, program, subcommand string, since time.Time, n int) ([]string, error) {
	predicates := []predicate.ClassificationAnalytics{
		classificationanalytics.CommandProgramEQ(program),
		classificationanalytics.CreatedAtGTE(since),
		classificationanalytics.CommandPreviewNotNil(),
	}
	if subcommand != "" {
		predicates = append(predicates, classificationanalytics.CommandSubcategoryEQ(subcommand))
	}
	entries, err := a.client.ClassificationAnalytics.Query().
		Where(predicates...).
		Order(ent.Desc(classificationanalytics.FieldCreatedAt)).
		Limit(n).
		Select(classificationanalytics.FieldCommandPreview).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("recent commands for %q/%q: %w", program, subcommand, err)
	}
	previews := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.CommandPreview != "" {
			previews = append(previews, e.CommandPreview)
		}
	}
	return previews, nil
}

func (a *Analytics) GetSubcommandTrend(ctx context.Context, program, subcommand string, since time.Time) ([]domain.AnalyticsData, error) {
	predicates := []predicate.ClassificationAnalytics{
		classificationanalytics.CommandProgramEQ(program),
		classificationanalytics.CreatedAtGTE(since),
	}
	if subcommand != "" {
		predicates = append(predicates, classificationanalytics.CommandSubcategoryEQ(subcommand))
	}
	entries, err := a.client.ClassificationAnalytics.Query().
		Where(predicates...).
		Order(ent.Asc(classificationanalytics.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("subcommand trend for %q/%q: %w", program, subcommand, err)
	}
	return convertAnalyticsEntries(entries), nil
}
