package services

import (
	"context"
	"fmt"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RulesService handles auto-approval rule management and analytics RPCs.
type RulesService struct {
	rulesStore     *RulesStore
	analyticsStore *AnalyticsStore
	classifier     *RuleBasedClassifier
}

// NewRulesService creates a RulesService.
func NewRulesService(rulesStore *RulesStore, analyticsStore *AnalyticsStore, classifier *RuleBasedClassifier) *RulesService {
	return &RulesService{
		rulesStore:     rulesStore,
		analyticsStore: analyticsStore,
		classifier:     classifier,
	}
}

// ListApprovalRules returns all rules: user + seed + claude-settings.
func (rs *RulesService) ListApprovalRules(
	ctx context.Context,
	req *connect.Request[sessionv1.ListApprovalRulesRequest],
) (*connect.Response[sessionv1.ListApprovalRulesResponse], error) {
	all := rs.allRuleSpecs()

	sourceFilter := ""
	if req.Msg.SourceFilter != nil {
		sourceFilter = *req.Msg.SourceFilter
	}

	var protos []*sessionv1.ApprovalRuleProto
	for _, spec := range all {
		if sourceFilter != "" && spec.Source != sourceFilter {
			continue
		}
		protos = append(protos, specToProto(spec))
	}
	return connect.NewResponse(&sessionv1.ListApprovalRulesResponse{Rules: protos}), nil
}

// UpsertApprovalRule creates or updates a user rule.
func (rs *RulesService) UpsertApprovalRule(
	ctx context.Context,
	req *connect.Request[sessionv1.UpsertApprovalRuleRequest],
) (*connect.Response[sessionv1.UpsertApprovalRuleResponse], error) {
	if req.Msg.Rule == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rule is required"))
	}
	r := req.Msg.Rule
	if r.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rule.id is required"))
	}

	existing := rs.rulesStore.All()
	isCreate := true
	for _, s := range existing {
		if s.ID == r.Id {
			isCreate = false
			break
		}
	}

	spec := RuleSpec{
		ID:             r.Id,
		Name:           r.Name,
		ToolName:       r.ToolName,
		ToolPattern:    r.ToolPattern,
		CommandPattern: r.CommandPattern,
		FilePattern:    r.FilePattern,
		Decision:       autoDecisionToString(r.Decision),
		RiskLevel:      r.RiskLevel,
		Reason:         r.Reason,
		Alternative:    r.Alternative,
		Priority:       int(r.Priority),
		Enabled:        r.Enabled,
		Source:         "user",
	}
	if r.CreatedAt != nil {
		spec.CreatedAt = r.CreatedAt.AsTime()
	}

	saved, err := rs.rulesStore.Upsert(spec)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Rebuild classifier rules.
	rs.rebuildClassifier()

	log.InfoLog.Printf("[RulesService] Upserted rule %s (create=%v)", saved.ID, isCreate)
	return connect.NewResponse(&sessionv1.UpsertApprovalRuleResponse{
		Rule:    specToProto(saved),
		Created: isCreate,
	}), nil
}

// DeleteApprovalRule removes a user rule by ID.
func (rs *RulesService) DeleteApprovalRule(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteApprovalRuleRequest],
) (*connect.Response[sessionv1.DeleteApprovalRuleResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	if err := rs.rulesStore.Delete(req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	rs.rebuildClassifier()
	log.InfoLog.Printf("[RulesService] Deleted rule %s", req.Msg.Id)
	return connect.NewResponse(&sessionv1.DeleteApprovalRuleResponse{
		Success: true,
		Message: fmt.Sprintf("Rule %s deleted", req.Msg.Id),
	}), nil
}

// GetApprovalAnalytics returns aggregated analytics for the requested time window.
func (rs *RulesService) GetApprovalAnalytics(
	ctx context.Context,
	req *connect.Request[sessionv1.GetApprovalAnalyticsRequest],
) (*connect.Response[sessionv1.GetApprovalAnalyticsResponse], error) {
	days := 7
	if req.Msg.WindowDays != nil {
		d := int(*req.Msg.WindowDays)
		if d > 0 && d <= 90 {
			days = d
		}
	}

	since := time.Now().AddDate(0, 0, -days)
	entries, err := rs.analyticsStore.LoadWindow(since)
	if err != nil {
		log.WarningLog.Printf("[RulesService] Analytics load error: %v", err)
		// Return empty summary rather than erroring.
	}

	// Re-classify old coverage gaps against the current rules so the dashboard
	// shows what is STILL uncovered today, not what was missed by older rule sets.
	entries = ReclassifyGaps(entries, rs.classifier)

	summary := ComputeSummary(entries)
	buckets := ComputeDailyBuckets(entries)

	protoResp := &sessionv1.GetApprovalAnalyticsResponse{
		Summary:      summaryToProto(summary),
		DailyBuckets: make([]*sessionv1.DailyBucketProto, 0, len(buckets)),
	}
	for _, b := range buckets {
		protoResp.DailyBuckets = append(protoResp.DailyBuckets, &sessionv1.DailyBucketProto{
			Date:        b.Date,
			AutoAllow:   int32(b.AutoAllow),
			AutoDeny:    int32(b.AutoDeny),
			Escalate:    int32(b.Escalate),
			ManualAllow: int32(b.ManualAllow),
			ManualDeny:  int32(b.ManualDeny),
			Total:       int32(b.Total),
		})
	}
	return connect.NewResponse(protoResp), nil
}

// allRuleSpecs returns user rules + seed rules as specs (for listing).
func (rs *RulesService) allRuleSpecs() []RuleSpec {
	var all []RuleSpec

	// User rules from store.
	all = append(all, rs.rulesStore.All()...)

	// Seed rules as specs.
	for _, r := range SeedRules() {
		all = append(all, ruleToSpec(r))
	}

	// Classifier rules that are claude-settings sourced.
	for _, r := range rs.classifier.Rules() {
		if r.Source == "claude-settings" {
			all = append(all, ruleToSpec(r))
		}
	}

	return all
}

// rebuildClassifier reloads user rules from the store and hot-swaps them in the classifier.
func (rs *RulesService) rebuildClassifier() {
	userRules := rs.rulesStore.ToRules()
	// Keep seed rules and claude-settings rules; replace user rules.
	existing := rs.classifier.Rules()
	var nonUser []Rule
	for _, r := range existing {
		if r.Source != "user" {
			nonUser = append(nonUser, r)
		}
	}
	rs.classifier.ReplaceRules(append(nonUser, userRules...))
}

// -- Mapping helpers ----------------------------------------------------------

func specToProto(spec RuleSpec) *sessionv1.ApprovalRuleProto {
	p := &sessionv1.ApprovalRuleProto{
		Id:             spec.ID,
		Name:           spec.Name,
		ToolName:       spec.ToolName,
		ToolPattern:    spec.ToolPattern,
		CommandPattern: spec.CommandPattern,
		FilePattern:    spec.FilePattern,
		Decision:       stringToAutoDecision(spec.Decision),
		RiskLevel:      spec.RiskLevel,
		Reason:         spec.Reason,
		Alternative:    spec.Alternative,
		Priority:       int32(spec.Priority),
		Enabled:        spec.Enabled,
		Source:         spec.Source,
	}
	if !spec.CreatedAt.IsZero() {
		p.CreatedAt = timestamppb.New(spec.CreatedAt)
	}
	return p
}

func ruleToSpec(r Rule) RuleSpec {
	spec := RuleSpec{
		ID:          r.ID,
		Name:        r.Name,
		ToolName:    r.ToolName,
		Decision:    decisionString(r.Decision),
		RiskLevel:   riskLevelString(r.RiskLevel),
		Reason:      r.Reason,
		Alternative: r.Alternative,
		Priority:    r.Priority,
		Enabled:     r.Enabled,
		Source:      r.Source,
	}
	if r.ToolPattern != nil {
		spec.ToolPattern = r.ToolPattern.String()
	}
	if r.CommandPattern != nil {
		spec.CommandPattern = r.CommandPattern.String()
	}
	if r.FilePattern != nil {
		spec.FilePattern = r.FilePattern.String()
	}
	return spec
}

func summaryToProto(s AnalyticsSummary) *sessionv1.AnalyticsSummaryProto {
	p := &sessionv1.AnalyticsSummaryProto{
		TotalDecisions:   int32(s.TotalDecisions),
		DecisionCounts:   make(map[string]int32, len(s.DecisionCounts)),
		AutoApproveRate:  s.AutoApproveRate,
		ManualReviewRate: s.ManualReviewRate,
	}
	for k, v := range s.DecisionCounts {
		p.DecisionCounts[k] = int32(v)
	}
	for _, t := range s.TopTools {
		p.TopTools = append(p.TopTools, &sessionv1.ToolStatProto{ToolName: t.ToolName, Count: int32(t.Count)})
	}
	for _, c := range s.TopDeniedCommands {
		p.TopDeniedCommands = append(p.TopDeniedCommands, &sessionv1.CommandStatProto{Preview: c.Preview, ToolName: c.ToolName, Count: int32(c.Count)})
	}
	for _, r := range s.TopTriggeredRules {
		p.TopTriggeredRules = append(p.TopTriggeredRules, &sessionv1.RuleStatProto{RuleId: r.RuleID, RuleName: r.RuleName, Count: int32(r.Count)})
	}
	for _, prog := range s.TopCommandPrograms {
		p.TopCommandPrograms = append(p.TopCommandPrograms, &sessionv1.ProgramStatProto{
			ProgramName: prog.Program,
			Category:    prog.Category,
			Count:       int32(prog.Count),
		})
	}
	for _, imp := range s.TopPythonImports {
		p.TopPythonImports = append(p.TopPythonImports, &sessionv1.ImportStatProto{
			Module: imp.Module,
			Count:  int32(imp.Count),
		})
	}
	p.CoverageGapCount = int32(s.CoverageGapCount)
	p.CoverageGapRate = s.CoverageGapRate
	for _, t := range s.TopUncoveredTools {
		p.TopUncoveredTools = append(p.TopUncoveredTools, &sessionv1.ToolStatProto{
			ToolName: t.ToolName,
			Count:    int32(t.Count),
		})
	}
	for _, prog := range s.TopUncoveredPrograms {
		p.TopUncoveredPrograms = append(p.TopUncoveredPrograms, &sessionv1.ProgramStatProto{
			ProgramName: prog.Program,
			Category:    prog.Category,
			Count:       int32(prog.Count),
		})
	}
	for _, s := range s.CommandSubcommandStats {
		p.CommandSubcommandStats = append(p.CommandSubcommandStats, &sessionv1.SubcommandStatProto{
			ProgramName: s.Program,
			Subcommand:  s.Subcommand,
			Category:    s.Category,
			Count:       int32(s.Count),
		})
	}
	if !s.WindowStart.IsZero() {
		p.WindowStart = timestamppb.New(s.WindowStart)
	}
	if !s.WindowEnd.IsZero() {
		p.WindowEnd = timestamppb.New(s.WindowEnd)
	}
	return p
}

func autoDecisionToString(d sessionv1.AutoDecision) string {
	switch d {
	case sessionv1.AutoDecision_AUTO_DECISION_ALLOW:
		return "auto_allow"
	case sessionv1.AutoDecision_AUTO_DECISION_DENY:
		return "auto_deny"
	default:
		return "escalate"
	}
}

func stringToAutoDecision(s string) sessionv1.AutoDecision {
	switch s {
	case "auto_allow":
		return sessionv1.AutoDecision_AUTO_DECISION_ALLOW
	case "auto_deny":
		return sessionv1.AutoDecision_AUTO_DECISION_DENY
	default:
		return sessionv1.AutoDecision_AUTO_DECISION_ESCALATE
	}
}
