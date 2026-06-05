package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"
)

// RulesService handles auto-approval rule management and analytics RPCs.
type RulesService struct {
	rulesStore     *RulesStore
	analyticsStore *AnalyticsStore
	classifier     *classifier.RuleBasedClassifier
	promptBuilder  RulePromptBuilder // nil = AI generation unavailable
	aiClient       AIClient          // nil = AI generation unavailable
}

// NewRulesService creates a RulesService.
// promptBuilder and aiClient may be nil; nil means AI rule generation is unavailable.
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
		ToolCategory:   r.ToolCategory,
		CommandPattern: r.CommandPattern,
		FilePattern:    r.FilePattern,
		Decision:       autoDecisionToString(r.Decision),
		RiskLevel:      r.RiskLevel,
		Reason:         r.Reason,
		Alternative:    r.Alternative,
		Priority:       int(r.Priority),
		Enabled:        r.Enabled,
		Source:         "user",

		Programs:              r.Programs,
		Subcommands:           r.Subcommands,
		BlockedSubcommands:    r.BlockedSubcommands,
		RequiredFlags:         r.RequiredFlags,
		ForbiddenFlags:        r.ForbiddenFlags,
		RequiredFlagPrefixes:  r.RequiredFlagPrefixes,
		PythonModes:           r.PythonModes,
		SafePythonImportsOnly: r.SafePythonImportsOnly,
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

// GetProgramAnalytics returns drill-down analytics for a single program.
// Implements AC-7 (subcommand breakdown, examples, trend).
func (rs *RulesService) GetProgramAnalytics(
	ctx context.Context,
	req *connect.Request[sessionv1.GetProgramAnalyticsRequest],
) (*connect.Response[sessionv1.GetProgramAnalyticsResponse], error) {
	program := strings.TrimSpace(req.Msg.Program)
	if program == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("program is required"))
	}

	days := 7
	if req.Msg.WindowDays != nil {
		days = int(*req.Msg.WindowDays)
		if days <= 0 || days > 90 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("window_days must be 1–90"))
		}
	}
	since := time.Now().AddDate(0, 0, -days)

	// AC-4: subcommand breakdown (SQL GROUP BY)
	breakdownRows, err := rs.analyticsStore.GetSubcommandBreakdown(ctx, program, since)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("subcommand breakdown: %w", err))
	}

	// AC-5: recent examples (up to 20, all subcommands)
	examples, err := rs.analyticsStore.ListRecentCommands(ctx, program, "", since, 20)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("recent examples: %w", err))
	}

	// AC-6: trend (all rows for program in window → Go-level daily bucketing)
	entries, err := rs.analyticsStore.LoadProgramWindow(ctx, program, since)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("program window: %w", err))
	}
	dailyBuckets := ComputeDailyBuckets(entries)

	// Derive category from entries (use first non-empty value)
	category := ""
	for _, e := range entries {
		if e.CommandCategory != "" {
			category = e.CommandCategory
			break
		}
	}

	// Collect known subcommands for rule coverage check (AC-10 / has_rule_coverage).
	knownSubcmds := make([]string, 0, len(breakdownRows))
	for _, row := range breakdownRows {
		knownSubcmds = append(knownSubcmds, row.Subcommand)
	}
	coveredSubcmds := rs.coveredSubcommands(program, knownSubcmds)

	// Aggregate per-subcommand breakdown into proto messages
	aggr := make(map[string]map[string]int32) // subcommand → decision → count
	for _, row := range breakdownRows {
		sub := row.Subcommand // may be ""
		if aggr[sub] == nil {
			aggr[sub] = make(map[string]int32)
		}
		aggr[sub][row.Decision] += int32(row.Count) //#nosec G115 — count fits int32
	}

	subProtos := make([]*sessionv1.SubcommandBreakdownProto, 0, len(aggr))
	for sub, decMap := range aggr {
		p := &sessionv1.SubcommandBreakdownProto{
			Subcommand:        sub,
			AutoAllow:         decMap["auto_allow"],
			AutoDeny:          decMap["auto_deny"],
			Escalate:          decMap["escalate"],
			ManualAllow:       decMap["manual_allow"],
			ManualDeny:        decMap["manual_deny"],
			HasRuleCoverage:   coveredSubcmds[sub],
			SuggestedRuleHint: sub,
		}
		p.Total = p.AutoAllow + p.AutoDeny + p.Escalate + p.ManualAllow + p.ManualDeny
		subProtos = append(subProtos, p)
	}
	// Sort by total descending
	sort.Slice(subProtos, func(i, j int) bool {
		return subProtos[i].Total > subProtos[j].Total
	})

	// Build daily trend proto
	trendProtos := make([]*sessionv1.DailyBucketProto, 0, len(dailyBuckets))
	for _, b := range dailyBuckets {
		trendProtos = append(trendProtos, &sessionv1.DailyBucketProto{
			Date:        b.Date,
			AutoAllow:   int32(b.AutoAllow),
			AutoDeny:    int32(b.AutoDeny),
			Escalate:    int32(b.Escalate),
			ManualAllow: int32(b.ManualAllow),
			ManualDeny:  int32(b.ManualDeny),
			Total:       int32(b.Total),
		})
	}

	return connect.NewResponse(&sessionv1.GetProgramAnalyticsResponse{
		Program:        program,
		Category:       category,
		Subcommands:    subProtos,
		RecentExamples: examples,
		Trend:          trendProtos,
	}), nil
}

// coveredSubcommands returns a map of subcommand → true for all known subcommands
// of the given program that are covered by at least one existing rule.
// knownSubcmds is the list of subcommand strings observed in the analytics window;
// it is used to test regex-based CommandPatterns against synthetic "<program> <subcommand>" strings.
func (rs *RulesService) coveredSubcommands(program string, knownSubcmds []string) map[string]bool {
	specs := rs.allRuleSpecs()
	covered := make(map[string]bool)
	for _, spec := range specs {
		if !spec.Enabled {
			continue
		}
		// Check for Bash tool match (exact or category)
		isBashTool := strings.EqualFold(spec.ToolName, "Bash")
		isBashCat := strings.EqualFold(spec.ToolCategory, "bash")
		if !isBashTool && !isBashCat && spec.ToolName != "" {
			continue
		}
		// Criteria-based matching: check Programs + Subcommands directly.
		if len(spec.Programs) > 0 {
			programMatched := false
			for _, p := range spec.Programs {
				if strings.EqualFold(p, program) {
					programMatched = true
					break
				}
			}
			if programMatched {
				if len(spec.Subcommands) == 0 {
					// No subcommand restriction — covers all subcommands.
					covered[""] = true
					for _, sub := range knownSubcmds {
						covered[sub] = true
					}
				} else {
					for _, sub := range spec.Subcommands {
						covered[sub] = true
					}
				}
			}
			continue
		}

		if spec.CommandPattern == "" {
			// A rule with no CommandPattern matches all commands — every subcommand covered.
			covered[""] = true
			continue
		}
		// Compile the pattern once; skip invalid patterns rather than panicking.
		re, err := regexp.Compile(spec.CommandPattern)
		if err != nil {
			continue
		}
		// Test each known subcommand against a synthetic "<program> <subcommand>" string.
		// This correctly handles regex-style patterns (e.g. \bgit\b.*\bpush\b) that the
		// previous strings.Fields tokenizer could not parse.
		for _, sub := range knownSubcmds {
			synthetic := program
			if sub != "" {
				synthetic = program + " " + sub
			}
			if re.MatchString(synthetic) {
				covered[sub] = true
			}
		}
		// Also check whether the pattern matches the bare program name (covers all subcommands).
		if re.MatchString(program) {
			covered[""] = true
		}
	}
	return covered
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
		ToolCategory:   spec.ToolCategory,
		CommandPattern: spec.CommandPattern,
		FilePattern:    spec.FilePattern,
		Decision:       stringToAutoDecision(spec.Decision),
		RiskLevel:      spec.RiskLevel,
		Reason:         spec.Reason,
		Alternative:    spec.Alternative,
		Priority:       int32(spec.Priority),
		Enabled:        spec.Enabled,
		Source:         spec.Source,

		Programs:              spec.Programs,
		Subcommands:           spec.Subcommands,
		BlockedSubcommands:    spec.BlockedSubcommands,
		RequiredFlags:         spec.RequiredFlags,
		ForbiddenFlags:        spec.ForbiddenFlags,
		RequiredFlagPrefixes:  spec.RequiredFlagPrefixes,
		PythonModes:           spec.PythonModes,
		SafePythonImportsOnly: spec.SafePythonImportsOnly,
	}
	if !spec.CreatedAt.IsZero() {
		p.CreatedAt = timestamppb.New(spec.CreatedAt)
	}
	return p
}

func ruleToSpec(r classifier.Rule) RuleSpec {
	spec := RuleSpec{
		ID:           r.ID,
		Name:         r.Name,
		ToolName:     r.ToolName,
		ToolCategory: r.ToolCategory,
		Decision:     decisionString(r.Decision),
		RiskLevel:    riskLevelString(r.RiskLevel),
		Reason:       r.Reason,
		Alternative:  r.Alternative,
		Priority:     r.Priority,
		Enabled:      r.Enabled,
		Source:       r.Source,
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
	// Expose structured CommandCriteria so seed/claude-settings rules are readable in the UI.
	if r.Criteria != nil {
		spec.Programs = r.Criteria.Programs
		spec.Subcommands = r.Criteria.Subcommands
		spec.BlockedSubcommands = r.Criteria.BlockedSubcommands
		spec.RequiredFlags = r.Criteria.RequiredFlags
		spec.ForbiddenFlags = r.Criteria.ForbiddenFlags
		spec.RequiredFlagPrefixes = r.Criteria.RequiredFlagPrefixes
		spec.PythonModes = r.Criteria.PythonModes
		spec.SafePythonImportsOnly = r.Criteria.SafePythonImportsOnly
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

// ── AI Rule Generation ────────────────────────────────────────────────────────

// GenerateSuggestedRule asks an AI to propose new auto-approval rules.
// It is read-only — it never calls rulesStore.Upsert.
// +api: rules:generate-suggested
func (rs *RulesService) GenerateSuggestedRule(
	ctx context.Context,
	req *connect.Request[sessionv1.GenerateSuggestedRuleRequest],
) (*connect.Response[sessionv1.GenerateSuggestedRuleResponse], error) {
	// Guard: both interfaces must be configured.
	if rs.promptBuilder == nil || rs.aiClient == nil {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("AI rule generation requires ANTHROPIC_API_KEY to be set"))
	}
	if req.Msg.Source == sessionv1.SuggestionSource_SUGGESTION_SOURCE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("source is required"))
	}
	days := 7
	if req.Msg.WindowDays != nil && *req.Msg.WindowDays > 0 && *req.Msg.WindowDays <= 90 {
		days = int(*req.Msg.WindowDays)
	}

	promptCtx := rs.buildPromptContext(req.Msg, days)
	systemPrompt := rs.promptBuilder.BuildSystemPrompt(promptCtx)
	userPrompt := rs.promptBuilder.BuildUserPrompt(promptCtx)

	rawJSON, err := rs.aiClient.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("AI client error: %w", err))
	}

	suggestions, err := rs.parseSuggestions(rawJSON)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("AI returned invalid response: %w", err))
	}

	for _, s := range suggestions {
		rs.attachConflictInfo(s)
	}

	return connect.NewResponse(&sessionv1.GenerateSuggestedRuleResponse{
		Suggestions: suggestions,
	}), nil
}

// buildPromptContext assembles a RulePromptContext from the request and analytics store.
func (rs *RulesService) buildPromptContext(req *sessionv1.GenerateSuggestedRuleRequest, days int) RulePromptContext {
	ctx := RulePromptContext{
		ExistingRules:  rs.allRuleSpecs(),
		WindowDays:     days,
		CommandSample:  req.CommandSample,
		ToolNameFilter: req.ToolNameFilter,
		ProgramFilter:  req.ProgramNameFilter,
	}

	// Load analytics window and build gap clusters.
	since := time.Now().AddDate(0, 0, -days)
	entries, err := rs.analyticsStore.LoadWindow(since)
	if err != nil {
		log.Warn("[RulesService] buildPromptContext: analytics load error", "err", err)
	}

	// Re-classify gaps against current rules so we see what is STILL uncovered.
	entries = ReclassifyGaps(entries, rs.classifier)

	// Group escalated, rule-less entries by (ToolName, Program).
	type gapKey struct{ tool, program string }
	gapMap := make(map[gapKey]*AnalyticsGap)
	for _, e := range entries {
		if e.Decision != "escalate" {
			continue
		}
		if e.RuleID != "" {
			continue // has a matching rule, not a gap
		}
		k := gapKey{tool: e.ToolName, program: e.CommandProgram}
		g, ok := gapMap[k]
		if !ok {
			g = &AnalyticsGap{ToolName: e.ToolName, Program: e.CommandProgram}
			gapMap[k] = g
		}
		g.Count++
		if len(g.RepresentativeCmds) < 5 && e.CommandPreview != "" {
			g.RepresentativeCmds = append(g.RepresentativeCmds, e.CommandPreview)
		}
	}

	gaps := make([]AnalyticsGap, 0, len(gapMap))
	for _, g := range gapMap {
		gaps = append(gaps, *g)
	}
	// Sort by count descending, cap at 10.
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].Count > gaps[j].Count })
	if len(gaps) > 10 {
		gaps = gaps[:10]
	}
	ctx.AnalyticsGaps = gaps

	return ctx
}

// rawSuggestion is the intermediate struct used to unmarshal the AI JSON response.
type rawSuggestion struct {
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

// parseSuggestions parses a JSON array of rule suggestions from the AI response.
// Validates each entry (regex compile, confidence clamp, priority clamp, caps array at 5).
func (rs *RulesService) parseSuggestions(rawJSON string) ([]*sessionv1.SuggestedRuleProto, error) {
	var raws []rawSuggestion
	if err := json.Unmarshal([]byte(rawJSON), &raws); err != nil {
		// Try stripping common markdown fences.
		cleaned := rawJSON
		for _, fence := range []string{"```json\n", "```\n", "```"} {
			if len(cleaned) > len(fence) && cleaned[:len(fence)] == fence {
				cleaned = cleaned[len(fence):]
			}
			if len(cleaned) > len(fence) && cleaned[len(cleaned)-len(fence):] == fence {
				cleaned = cleaned[:len(cleaned)-len(fence)]
			}
		}
		if err2 := json.Unmarshal([]byte(cleaned), &raws); err2 != nil {
			return nil, fmt.Errorf("parse JSON array: %w (original: %v)", err2, err)
		}
	}

	// Cap at 5 suggestions.
	if len(raws) > 5 {
		raws = raws[:5]
	}

	result := make([]*sessionv1.SuggestedRuleProto, 0, len(raws))
	for i, raw := range raws {
		s, err := rs.validateSuggestion(raw, i)
		if err != nil {
			log.Warn("[RulesService] parseSuggestions: dropping invalid suggestion", "index", i, "err", err)
			continue
		}
		result = append(result, s)
	}
	return result, nil
}

// overbreadCommandPatterns are commandPattern values that explicitly match every
// command and are therefore unsafe to auto-allow. An empty commandPattern is NOT
// included because it is valid when scoped by tool_name or tool_pattern.
var overbreadCommandPatterns = map[string]bool{
	".*": true,
	".+": true,
}

// validateSuggestion converts a rawSuggestion to a SuggestedRuleProto, applying all validation.
func (rs *RulesService) validateSuggestion(raw rawSuggestion, idx int) (*sessionv1.SuggestedRuleProto, error) {
	// Guard: reject overbroad auto_allow/allow suggestions whose commandPattern explicitly
	// matches everything (e.g., ".*" or ".+"). An empty commandPattern is allowed only
	// when scoped by a non-empty tool_name or tool_pattern.
	rawDecision := raw.Decision
	if rawDecision == "auto_allow" || rawDecision == "allow" {
		if overbreadCommandPatterns[raw.CommandPattern] {
			return nil, fmt.Errorf("suggestion[%d]: dangerously overbroad commandPattern %q for %q decision — must be more specific", idx, raw.CommandPattern, rawDecision)
		}
		if raw.CommandPattern == "" && raw.ToolName == "" && raw.ToolPattern == "" {
			return nil, fmt.Errorf("suggestion[%d]: %q suggestion has no commandPattern, tool_name, or tool_pattern — too broad to auto-allow", idx, rawDecision)
		}
	}

	// Validate regex patterns.
	if raw.CommandPattern != "" {
		if _, err := regexp.Compile(raw.CommandPattern); err != nil {
			return nil, fmt.Errorf("suggestion[%d]: invalid commandPattern %q: %w", idx, raw.CommandPattern, err)
		}
	}
	if raw.ToolPattern != "" {
		if _, err := regexp.Compile(raw.ToolPattern); err != nil {
			return nil, fmt.Errorf("suggestion[%d]: invalid toolPattern %q: %w", idx, raw.ToolPattern, err)
		}
	}
	if raw.FilePattern != "" {
		if _, err := regexp.Compile(raw.FilePattern); err != nil {
			return nil, fmt.Errorf("suggestion[%d]: invalid filePattern %q: %w", idx, raw.FilePattern, err)
		}
	}

	// Clamp confidence to [0.0, 1.0].
	conf := raw.Confidence
	if conf < 0 {
		conf = 0
	}
	if conf > 1.0 {
		conf = 1.0
	}

	// Validate/default priority.
	priority := raw.Priority
	if priority < 1 || priority > 999 {
		priority = 100
	}

	// Validate decision enum.
	decision := stringToAutoDecision(raw.Decision)

	// Cap source_commands at 20.
	srcCmds := raw.SourceCommands
	if len(srcCmds) > 20 {
		srcCmds = srcCmds[:20]
	}

	return &sessionv1.SuggestedRuleProto{
		Name:           raw.Name,
		ToolName:       raw.ToolName,
		ToolPattern:    raw.ToolPattern,
		CommandPattern: raw.CommandPattern,
		FilePattern:    raw.FilePattern,
		Decision:       decision,
		RiskLevel:      raw.RiskLevel,
		Reason:         raw.Reason,
		Alternative:    raw.Alternative,
		Priority:       priority,
		Confidence:     conf,
		Explanation:    raw.Explanation,
		SourceCommands: srcCmds,
	}, nil
}

// attachConflictInfo populates ShadowedByRuleIds and ShadowsRuleIds on a suggestion.
// Uses a conservative heuristic: if an existing rule shares the same ToolName and both
// have non-empty CommandPattern, they may overlap. Exact regex intersection is not computed.
func (rs *RulesService) attachConflictInfo(s *sessionv1.SuggestedRuleProto) {
	allSpecs := rs.allRuleSpecs()
	// Sort by priority descending.
	sort.Slice(allSpecs, func(i, j int) bool { return allSpecs[i].Priority > allSpecs[j].Priority })

	const maxConflicts = 10

	for _, spec := range allSpecs {
		if len(s.ShadowedByRuleIds)+len(s.ShadowsRuleIds) >= maxConflicts*2 {
			break
		}
		// Conservative overlap heuristic: same ToolName (when both non-empty) AND both have CommandPattern.
		toolOverlap := (s.ToolName != "" && spec.ToolName != "" && s.ToolName == spec.ToolName)
		cmdOverlap := s.CommandPattern != "" && spec.CommandPattern != ""
		if !toolOverlap || !cmdOverlap {
			continue
		}

		if spec.Priority > int(s.Priority) {
			// Existing rule has higher priority — it fires first, shadowing the suggestion.
			if len(s.ShadowedByRuleIds) < maxConflicts {
				s.ShadowedByRuleIds = append(s.ShadowedByRuleIds, spec.ID)
			}
		} else if spec.Priority < int(s.Priority) {
			// Suggestion has higher priority — it fires first, shadowing the existing rule.
			if len(s.ShadowsRuleIds) < maxConflicts {
				s.ShadowsRuleIds = append(s.ShadowsRuleIds, spec.ID)
			}
		}
	}
}

// ── YAML import/export constants and structs ──────────────────────────────────

const (
	maxYAMLBytes = 512 * 1024
	maxRuleCount = 500
)

// yamlRulesFile is the top-level YAML structure for import.
type yamlRulesFile struct {
	Rules []yamlRuleEntry `yaml:"rules"`
}

// yamlRuleEntry is the per-rule import struct (strict: KnownFields enabled).
type yamlRuleEntry struct {
	Name           string   `yaml:"name"`
	Tool           string   `yaml:"tool"`
	ToolPattern    string   `yaml:"tool_pattern"`
	Programs       []string `yaml:"programs"`
	Subcommands    []string `yaml:"subcommands"`
	CommandPattern string   `yaml:"command_pattern"`
	FilePattern    string   `yaml:"file_pattern"`
	Decision       string   `yaml:"decision"`
	Reason         string   `yaml:"reason"`
	Alternative    string   `yaml:"alternative"`
	Priority       int      `yaml:"priority"`
	Enabled        *bool    `yaml:"enabled"`
}

// yamlRuleExport is the per-rule export struct (omitempty for optional fields).
type yamlRuleExport struct {
	Name           string   `yaml:"name"`
	Tool           string   `yaml:"tool,omitempty"`
	ToolPattern    string   `yaml:"tool_pattern,omitempty"`
	Programs       []string `yaml:"programs,omitempty"`
	Subcommands    []string `yaml:"subcommands,omitempty"`
	CommandPattern string   `yaml:"command_pattern,omitempty"`
	FilePattern    string   `yaml:"file_pattern,omitempty"`
	Decision       string   `yaml:"decision"`
	Reason         string   `yaml:"reason,omitempty"`
	Alternative    string   `yaml:"alternative,omitempty"`
	// Priority 0 is skipped by omitempty; this is a known limitation (see validation.md Concern 5).
	Priority int32 `yaml:"priority,omitempty"`
	Enabled  *bool `yaml:"enabled,omitempty"`
}

// yamlRulesExportFile is the top-level YAML export structure.
type yamlRulesExportFile struct {
	Rules []yamlRuleExport `yaml:"rules"`
}

// ── ValidateRules ─────────────────────────────────────────────────────────────

// ValidateRules parses and validates a YAML rules file without persisting anything.
// Returns per-rule results; does not short-circuit on first error.
// +api: rules:validate
func (rs *RulesService) ValidateRules(
	ctx context.Context,
	req *connect.Request[sessionv1.ValidateRulesRequest],
) (*connect.Response[sessionv1.ValidateRulesResponse], error) {
	yamlContent := req.Msg.YamlContent

	// Security: size guard before any parse.
	if len(yamlContent) > maxYAMLBytes {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("YAML payload too large: %d bytes (max %d)", len(yamlContent), maxYAMLBytes))
	}

	// Parse with KnownFields(true) to reject unrecognized keys (catches typos).
	var file yamlRulesFile
	dec := yaml.NewDecoder(strings.NewReader(yamlContent))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("YAML parse error: %w", err))
	}

	// Rule count guard (post-parse, catches alias expansion attacks).
	if len(file.Rules) > maxRuleCount {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("too many rules: %d (max %d)", len(file.Rules), maxRuleCount))
	}

	results := make([]*sessionv1.ParsedRuleResult, 0, len(file.Rules))
	validCount, errorCount := int32(0), int32(0)
	for _, entry := range file.Rules {
		rule, errs := validateYAMLEntry(entry)
		pr := &sessionv1.ParsedRuleResult{
			OriginalName: entry.Name,
			Valid:        len(errs) == 0,
			Errors:       errs,
		}
		if len(errs) == 0 {
			pr.Rule = rule
			validCount++
		} else {
			errorCount++
		}
		results = append(results, pr)
	}

	return connect.NewResponse(&sessionv1.ValidateRulesResponse{
		Results:    results,
		ValidCount: validCount,
		ErrorCount: errorCount,
	}), nil
}

// validateYAMLEntry validates a single YAML rule entry and converts it to proto.
// Returns all validation errors found (not just the first).
func validateYAMLEntry(e yamlRuleEntry) (*sessionv1.ApprovalRuleProto, []string) {
	var errs []string

	if strings.TrimSpace(e.Name) == "" {
		errs = append(errs, "name is required")
	}

	// Mutually exclusive tool fields.
	if e.Tool != "" && e.ToolPattern != "" {
		errs = append(errs, "tool and tool_pattern are mutually exclusive; use one or the other")
	}

	// Decision mapping -- only allow, deny, escalate accepted.
	decisionMap := map[string]sessionv1.AutoDecision{
		"allow":    sessionv1.AutoDecision_AUTO_DECISION_ALLOW,
		"deny":     sessionv1.AutoDecision_AUTO_DECISION_DENY,
		"escalate": sessionv1.AutoDecision_AUTO_DECISION_ESCALATE,
	}
	decision, ok := decisionMap[e.Decision]
	if !ok {
		errs = append(errs, fmt.Sprintf("invalid decision %q: must be allow, deny, or escalate", e.Decision))
	}
	_ = ok

	// Overbroad allow: decision=allow with no match criteria at all.
	if e.Decision == "allow" && e.Tool == "" && e.ToolPattern == "" &&
		e.CommandPattern == "" && e.FilePattern == "" &&
		len(e.Programs) == 0 && len(e.Subcommands) == 0 {
		errs = append(errs, "overbroad allow rule: at least one match criterion (tool, tool_pattern, command_pattern, file_pattern, programs, or subcommands) is required for decision=allow")
	}

	// Regex validation -- collect all invalid patterns, do not stop at first error.
	if e.ToolPattern != "" {
		if _, err := regexp.Compile(e.ToolPattern); err != nil {
			errs = append(errs, fmt.Sprintf("tool_pattern is not a valid regex: %v", err))
		}
	}
	if e.CommandPattern != "" {
		if _, err := regexp.Compile(e.CommandPattern); err != nil {
			errs = append(errs, fmt.Sprintf("command_pattern is not a valid regex: %v", err))
		}
	}
	if e.FilePattern != "" {
		if _, err := regexp.Compile(e.FilePattern); err != nil {
			errs = append(errs, fmt.Sprintf("file_pattern is not a valid regex: %v", err))
		}
	}

	if len(errs) > 0 {
		return nil, errs
	}

	priority := int32(e.Priority)
	if priority == 0 {
		priority = 10
	}
	enabled := true
	if e.Enabled != nil {
		enabled = *e.Enabled
	}

	return &sessionv1.ApprovalRuleProto{
		Name:           e.Name,
		ToolName:       e.Tool,
		ToolPattern:    e.ToolPattern,
		Programs:       e.Programs,
		Subcommands:    e.Subcommands,
		CommandPattern: e.CommandPattern,
		FilePattern:    e.FilePattern,
		Decision:       decision,
		Reason:         e.Reason,
		Alternative:    e.Alternative,
		Priority:       priority,
		Enabled:        enabled,
		Source:         "user",
	}, nil
}

// ── ExportRules ───────────────────────────────────────────────────────────────

// ExportRules serializes user-authored rules to YAML for download.
// Only source="user" rules are exported; seed and claude-settings rules are excluded.
// +api: rules:export
func (rs *RulesService) ExportRules(
	ctx context.Context,
	req *connect.Request[sessionv1.ExportRulesRequest],
) (*connect.Response[sessionv1.ExportRulesResponse], error) {
	allSpecs := rs.rulesStore.All()

	filterIDs := make(map[string]bool, len(req.Msg.RuleIds))
	for _, id := range req.Msg.RuleIds {
		filterIDs[id] = true
	}

	var entries []yamlRuleExport
	for _, spec := range allSpecs {
		if spec.Source != "user" {
			continue
		}
		if len(filterIDs) > 0 && !filterIDs[spec.ID] {
			continue
		}
		// Map internal decision string (e.g. "auto_allow") to user-friendly YAML value.
		decision := autoDecisionToYAML(stringToAutoDecision(spec.Decision))
		entry := yamlRuleExport{
			Name:           spec.Name,
			Tool:           spec.ToolName,
			ToolPattern:    spec.ToolPattern,
			Programs:       spec.Programs,
			Subcommands:    spec.Subcommands,
			CommandPattern: spec.CommandPattern,
			FilePattern:    spec.FilePattern,
			Decision:       decision,
			Reason:         spec.Reason,
			Alternative:    spec.Alternative,
			Priority:       int32(spec.Priority), //#nosec G115 -- priority fits int32
		}
		// Omit enabled field when true (default), include only when false.
		if !spec.Enabled {
			f := false
			entry.Enabled = &f
		}
		entries = append(entries, entry)
	}

	if entries == nil {
		entries = []yamlRuleExport{}
	}

	out, err := yaml.Marshal(yamlRulesExportFile{Rules: entries})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("YAML marshal failed: %w", err))
	}

	return connect.NewResponse(&sessionv1.ExportRulesResponse{
		YamlContent: string(out),
	}), nil
}

// autoDecisionToYAML maps a proto AutoDecision enum to a user-friendly YAML string.
func autoDecisionToYAML(d sessionv1.AutoDecision) string {
	switch d {
	case sessionv1.AutoDecision_AUTO_DECISION_ALLOW:
		return "allow"
	case sessionv1.AutoDecision_AUTO_DECISION_DENY:
		return "deny"
	default:
		return "escalate"
	}
}

// ── BulkUpsertRules ───────────────────────────────────────────────────────────

// BulkUpsertRules creates or updates multiple rules in one call.
// Rebuilds the in-memory classifier exactly once at the end (not per-rule).
// Security: client-supplied id and source fields are always discarded.
// +api: rules:bulk-upsert
func (rs *RulesService) BulkUpsertRules(
	ctx context.Context,
	req *connect.Request[sessionv1.BulkUpsertRulesRequest],
) (*connect.Response[sessionv1.BulkUpsertRulesResponse], error) {
	if len(req.Msg.Rules) == 0 {
		return connect.NewResponse(&sessionv1.BulkUpsertRulesResponse{}), nil
	}
	if len(req.Msg.Rules) > maxRuleCount {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("too many rules: %d exceeds limit of %d", len(req.Msg.Rules), maxRuleCount))
	}

	var validationErrors []string
	for _, proto := range req.Msg.Rules {
		entry := yamlRuleEntry{
			Name:           proto.Name,
			Tool:           proto.ToolName,
			ToolPattern:    proto.ToolPattern,
			CommandPattern: proto.CommandPattern,
			FilePattern:    proto.FilePattern,
			Programs:       proto.Programs,
			Subcommands:    proto.Subcommands,
			Decision:       autoDecisionToYAML(proto.Decision),
			Reason:         proto.Reason,
			Alternative:    proto.Alternative,
		}
		_, errs := validateYAMLEntry(entry)
		for _, e := range errs {
			validationErrors = append(validationErrors, fmt.Sprintf("rule %q: %s", proto.Name, e))
		}
	}
	if len(validationErrors) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("validation errors: %s", strings.Join(validationErrors, "; ")))
	}

	specs := make([]RuleSpec, 0, len(req.Msg.Rules))
	for _, proto := range req.Msg.Rules {
		specs = append(specs, ruleProtoToSpec(proto))
	}

	res := rs.rulesStore.BulkUpsert(ctx, specs, req.Msg.OverwriteDuplicates)
	// Rebuild classifier exactly once after all rules are stored.
	rs.rebuildClassifier()

	return connect.NewResponse(&sessionv1.BulkUpsertRulesResponse{
		Created: int32(res.Created), //#nosec G115 -- count fits int32
		Updated: int32(res.Updated), //#nosec G115 -- count fits int32
		Skipped: int32(res.Skipped), //#nosec G115 -- count fits int32
		Errors:  res.Errors,
	}), nil
}

// ruleProtoToSpec converts an ApprovalRuleProto to a RuleSpec for storage.
// Security: always discards client-supplied id and source to prevent injection.
func ruleProtoToSpec(p *sessionv1.ApprovalRuleProto) RuleSpec {
	if p == nil {
		return RuleSpec{}
	}
	return RuleSpec{
		// ID intentionally left empty -- BulkUpsert assigns "user-<uuid>" server-side.
		Name:                  p.Name,
		ToolName:              p.ToolName,
		ToolPattern:           p.ToolPattern,
		CommandPattern:        p.CommandPattern,
		FilePattern:           p.FilePattern,
		Programs:              p.Programs,
		Subcommands:           p.Subcommands,
		BlockedSubcommands:    p.BlockedSubcommands,
		RequiredFlags:         p.RequiredFlags,
		ForbiddenFlags:        p.ForbiddenFlags,
		RequiredFlagPrefixes:  p.RequiredFlagPrefixes,
		PythonModes:           p.PythonModes,
		SafePythonImportsOnly: p.SafePythonImportsOnly,
		Decision:              autoDecisionToString(p.Decision),
		RiskLevel:             p.RiskLevel,
		Reason:                p.Reason,
		Alternative:           p.Alternative,
		Priority:              int(p.Priority),
		Enabled:               p.Enabled,
		Source:                "user", // always override -- never accept client-supplied source
		CreatedAt:             time.Now(),
	}
}
