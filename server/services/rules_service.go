package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RulesService handles auto-approval rule management and analytics RPCs.
type RulesService struct {
	rulesStore     *RulesStore
	analyticsStore *AnalyticsStore
	classifier     *classifier.RuleBasedClassifier
	promptBuilder  RulePromptBuilder
	aiClient       AIClient
}

// NewRulesService creates a RulesService.
// promptBuilder and aiClient may be nil; GenerateSuggestedRule returns CodeUnimplemented if aiClient is nil.
func NewRulesService(rulesStore *RulesStore, analyticsStore *AnalyticsStore, classifier *classifier.RuleBasedClassifier, promptBuilder RulePromptBuilder, aiClient AIClient) *RulesService {
	return &RulesService{
		rulesStore:     rulesStore,
		analyticsStore: analyticsStore,
		classifier:     classifier,
		promptBuilder:  promptBuilder,
		aiClient:       aiClient,
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

	log.Info("[RulesService] upserted rule", "id", saved.ID, "create", isCreate)
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
	log.Info("[RulesService] deleted rule", "id", req.Msg.Id)
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
		log.Warn("[RulesService] analytics load error", "err", err)
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

// GenerateSuggestedRule asks an AI agent to propose new auto-approval rules.
// It builds a prompt from existing rules and analytics gaps, calls the AI backend,
// parses the JSON response, and returns up to 5 SuggestedRuleProto values.
func (rs *RulesService) GenerateSuggestedRule(
	ctx context.Context,
	req *connect.Request[sessionv1.GenerateSuggestedRuleRequest],
) (*connect.Response[sessionv1.GenerateSuggestedRuleResponse], error) {
	if req.Msg.Source == sessionv1.SuggestionSource_SUGGESTION_SOURCE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("source is required"))
	}
	if rs.aiClient == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("GenerateSuggestedRule requires ANTHROPIC_API_KEY to be configured"))
	}

	days := 7
	if req.Msg.WindowDays != nil && *req.Msg.WindowDays > 0 {
		days = int(*req.Msg.WindowDays)
	}

	promptCtx := rs.buildPromptContext(req.Msg, days)

	builder := rs.promptBuilder
	if builder == nil {
		builder = &DefaultRulePromptBuilder{}
	}

	systemPrompt := builder.BuildSystemPrompt(promptCtx)
	userPrompt := builder.BuildUserPrompt(promptCtx)

	rawJSON, err := rs.aiClient.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AI backend error: %w", err))
	}

	suggestions, err := rs.parseSuggestions(rawJSON)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse AI response: %w", err))
	}

	for _, s := range suggestions {
		rs.attachConflictInfo(s)
	}

	return connect.NewResponse(&sessionv1.GenerateSuggestedRuleResponse{
		Suggestions: suggestions,
	}), nil
}

// buildPromptContext assembles the RulePromptContext from current rules and analytics data.
func (rs *RulesService) buildPromptContext(req *sessionv1.GenerateSuggestedRuleRequest, days int) RulePromptContext {
	promptCtx := RulePromptContext{
		ExistingRules:  rs.allRuleSpecs(),
		WindowDays:     days,
		CommandSample:  req.CommandSample,
		ToolNameFilter: req.ToolNameFilter,
		ProgramFilter:  req.ProgramNameFilter,
	}

	// Build analytics gaps from the analytics store.
	since := time.Now().AddDate(0, 0, -days)
	entries, _ := rs.analyticsStore.LoadWindow(since)
	entries = ReclassifyGaps(entries, rs.classifier)

	// Group gap entries (escalated with no rule match) by (ToolName, Program).
	type gapKey struct {
		ToolName string
		Program  string
	}
	gapCounts := map[gapKey][]string{}
	for _, e := range entries {
		if e.Decision != "escalate" || e.RuleID != "" {
			continue
		}
		k := gapKey{ToolName: e.ToolName, Program: e.CommandProgram}
		cmds := gapCounts[k]
		if len(cmds) < 5 {
			cmds = append(cmds, e.CommandPreview)
		}
		gapCounts[k] = cmds
	}

	for k, cmds := range gapCounts {
		promptCtx.AnalyticsGaps = append(promptCtx.AnalyticsGaps, AnalyticsGap{
			ToolName:           k.ToolName,
			Program:            k.Program,
			Count:              len(cmds),
			RepresentativeCmds: cmds,
		})
	}

	return promptCtx
}

// aiSuggestionJSON is the raw JSON shape returned by the AI (snake_case field names).
type aiSuggestionJSON struct {
	Name           string   `json:"name"`
	ToolName       string   `json:"tool_name"`
	ToolPattern    string   `json:"tool_pattern"`
	CommandPattern string   `json:"command_pattern"`
	FilePattern    string   `json:"file_pattern"`
	Decision       string   `json:"decision"`
	RiskLevel      string   `json:"risk_level"`
	Reason         string   `json:"reason"`
	Alternative    string   `json:"alternative"`
	Priority       int32    `json:"priority"`
	Confidence     float32  `json:"confidence"`
	Explanation    string   `json:"explanation"`
	SourceCommands []string `json:"source_commands"`
}

// parseSuggestions decodes the AI JSON response into []*SuggestedRuleProto.
// Items with an invalid CommandPattern are silently dropped.
// Confidence is clamped to [0,1]. Priority 0 defaults to 100.
// The result is capped at 5 items.
func (rs *RulesService) parseSuggestions(rawJSON string) ([]*sessionv1.SuggestedRuleProto, error) {
	var items []aiSuggestionJSON
	if err := json.Unmarshal([]byte(rawJSON), &items); err != nil {
		return nil, fmt.Errorf("unmarshal AI JSON: %w", err)
	}

	var out []*sessionv1.SuggestedRuleProto
	for _, item := range items {
		if len(out) >= 5 {
			break
		}

		// Validate regex patterns — drop item if invalid.
		if item.CommandPattern != "" {
			if _, err := regexp.Compile(item.CommandPattern); err != nil {
				log.Warn("[parseSuggestions] dropping item with invalid commandPattern", "pattern", item.CommandPattern, "err", err)
				continue
			}
		}
		if item.ToolPattern != "" {
			if _, err := regexp.Compile(item.ToolPattern); err != nil {
				log.Warn("[parseSuggestions] dropping item with invalid toolPattern", "pattern", item.ToolPattern, "err", err)
				continue
			}
		}
		if item.FilePattern != "" {
			if _, err := regexp.Compile(item.FilePattern); err != nil {
				log.Warn("[parseSuggestions] dropping item with invalid filePattern", "pattern", item.FilePattern, "err", err)
				continue
			}
		}

		// Clamp confidence.
		conf := item.Confidence
		if conf < 0 {
			conf = 0
		} else if conf > 1.0 {
			conf = 1.0
		}

		// Default priority.
		prio := item.Priority
		if prio == 0 {
			prio = 100
		}

		out = append(out, &sessionv1.SuggestedRuleProto{
			Name:           item.Name,
			ToolName:       item.ToolName,
			ToolPattern:    item.ToolPattern,
			CommandPattern: item.CommandPattern,
			FilePattern:    item.FilePattern,
			Decision:       stringToAutoDecision(item.Decision),
			RiskLevel:      item.RiskLevel,
			Reason:         item.Reason,
			Alternative:    item.Alternative,
			Priority:       prio,
			Confidence:     conf,
			Explanation:    item.Explanation,
			SourceCommands: item.SourceCommands,
		})
	}

	return out, nil
}

// attachConflictInfo populates ShadowedByRuleIds and ShadowsRuleIds on a suggestion.
// A rule "shadows" the suggestion if it has a higher priority and overlapping tool/command.
// The suggestion "shadows" a rule if it has a higher priority and overlapping tool/command.
func (rs *RulesService) attachConflictInfo(s *sessionv1.SuggestedRuleProto) {
	all := rs.allRuleSpecs()
	for _, r := range all {
		if !rulesOverlap(s, r) {
			continue
		}
		rPrio := r.Priority
		sPrio := int(s.Priority)
		if rPrio > sPrio {
			// Existing rule fires first — suggestion is shadowed.
			s.ShadowedByRuleIds = append(s.ShadowedByRuleIds, r.ID)
		} else if sPrio > rPrio {
			// Suggestion fires first — existing rule is shadowed.
			s.ShadowsRuleIds = append(s.ShadowsRuleIds, r.ID)
		}
	}
}

// rulesOverlap returns true if an existing RuleSpec may match the same commands as a suggestion.
// A heuristic check: same ToolName (or both unset) and same CommandPattern (or both unset).
func rulesOverlap(s *sessionv1.SuggestedRuleProto, r RuleSpec) bool {
	// Tool name must agree (either both unset or one matches the other).
	if s.ToolName != "" && r.ToolName != "" && s.ToolName != r.ToolName {
		return false
	}
	// Command pattern must agree.
	if s.CommandPattern != "" && r.CommandPattern != "" && s.CommandPattern != r.CommandPattern {
		return false
	}
	return true
}

// allRuleSpecs returns user rules + seed rules as specs (for listing).
func (rs *RulesService) allRuleSpecs() []RuleSpec {
	var all []RuleSpec

	// User rules from store.
	all = append(all, rs.rulesStore.All()...)

	// Seed rules as specs.
	for _, r := range classifier.SeedRules() {
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
	var nonUser []classifier.Rule
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

func ruleToSpec(r classifier.Rule) RuleSpec {
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
