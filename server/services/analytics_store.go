package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
)

// AnalyticsEntry records a single classification decision.
type AnalyticsEntry struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	SessionID      string    `json:"session_id"`
	ToolName       string    `json:"tool_name"`
	CommandPreview string    `json:"command_preview"` // first 200 chars
	Cwd            string    `json:"cwd"`
	// Decision: "auto_allow" | "auto_deny" | "escalate" | "manual_allow" | "manual_deny"
	Decision    string `json:"decision"`
	RiskLevel   string `json:"risk_level"`
	RuleID      string `json:"rule_id,omitempty"`
	RuleName    string `json:"rule_name,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Alternative string `json:"alternative,omitempty"`
	DurationMs  int64  `json:"duration_ms"`
	ApprovalID  string `json:"approval_id,omitempty"`

	// AST-derived command categorization (Bash tool only).
	// CommandProgram is the primary executable being called (e.g., "git", "npm").
	CommandProgram string `json:"command_program,omitempty"`
	// CommandCategory groups CommandProgram into a high-level category (e.g., "vcs", "node").
	CommandCategory string `json:"command_category,omitempty"`
	// CommandSubcategory is the first positional subcommand (e.g., "commit" for "git commit").
	CommandSubcategory string `json:"command_subcommand,omitempty"`
	// PythonImports lists top-level module names imported in inline Python (-c) invocations.
	PythonImports []string `json:"python_imports,omitempty"`
}

// ToolStat is a tool name with a count.
type ToolStat struct {
	ToolName string `json:"tool_name"`
	Count    int    `json:"count"`
}

// CommandStat is a command preview with a count.
type CommandStat struct {
	Preview  string `json:"preview"`
	ToolName string `json:"tool_name"`
	Count    int    `json:"count"`
}

// RuleStat is a rule with its trigger count.
type RuleStat struct {
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Count    int    `json:"count"`
}

// ProgramStat is a command program with its category and usage count.
type ProgramStat struct {
	Program  string `json:"program"`
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// ImportStat is a Python module import with its usage count.
type ImportStat struct {
	Module string `json:"module"`
	Count  int    `json:"count"`
}

// SubcommandStat is a (program, subcommand) pair with its usage count.
// Subcommand may contain a space for two-level CLIs (e.g., "pr create" for gh).
type SubcommandStat struct {
	Program    string `json:"program"`
	Subcommand string `json:"subcommand"`
	Category   string `json:"category"`
	Count      int    `json:"count"`
}

// AnalyticsSummary aggregates decisions over a time window.
type AnalyticsSummary struct {
	TotalDecisions    int            `json:"total_decisions"`
	DecisionCounts    map[string]int `json:"decision_counts"`
	TopTools          []ToolStat     `json:"top_tools"`
	TopDeniedCommands []CommandStat  `json:"top_denied_commands"`
	TopTriggeredRules []RuleStat     `json:"top_triggered_rules"`
	// TopCommandPrograms lists the most frequently invoked programs via the Bash tool.
	TopCommandPrograms []ProgramStat `json:"top_command_programs"`
	// TopPythonImports lists the most frequently imported Python modules from inline (-c) invocations.
	TopPythonImports []ImportStat `json:"top_python_imports"`
	AutoApproveRate  float64      `json:"auto_approve_rate"`
	ManualReviewRate float64      `json:"manual_review_rate"`
	WindowStart      time.Time    `json:"window_start"`
	WindowEnd        time.Time    `json:"window_end"`

	// Coverage gap: decisions that escaped all rules (escalated with no rule_id).
	// These are prime candidates for new rules to reduce manual review.
	CoverageGapCount     int           `json:"coverage_gap_count"`
	CoverageGapRate      float64       `json:"coverage_gap_rate"` // percentage 0–100
	TopUncoveredTools    []ToolStat    `json:"top_uncovered_tools"`
	TopUncoveredPrograms []ProgramStat `json:"top_uncovered_programs"`

	// CommandSubcommandStats is the complete (program, subcommand) distribution — not
	// truncated to top-N. Use this for drill-down analysis such as "which gh subcommands
	// does Claude use most?" or "what sed patterns need rules?".
	CommandSubcommandStats []SubcommandStat `json:"command_subcommand_stats"`
}

// AnalyticsStore writes AnalyticsEntry records asynchronously to a JSONL file
// and provides in-memory query aggregations.
type AnalyticsStore struct {
	filePath string
	ch       chan AnalyticsEntry
	dropped  int64 // atomic counter for dropped entries
}

const analyticsBufferSize = 1000

// NewAnalyticsStore creates an AnalyticsStore backed by the given JSONL file.
// Call Start() to begin the background flush goroutine.
func NewAnalyticsStore(filePath string) *AnalyticsStore {
	return &AnalyticsStore{
		filePath: filePath,
		ch:       make(chan AnalyticsEntry, analyticsBufferSize),
	}
}

// Start launches the background goroutine that flushes entries to disk.
// It stops when ctx is canceled.
func (s *AnalyticsStore) Start(ctx interface{ Done() <-chan struct{} }) {
	go s.flush(ctx)
}

// Record enqueues an analytics entry for async write. Non-blocking.
// If the buffer is full, the entry is dropped and the dropped counter incremented.
func (s *AnalyticsStore) Record(entry AnalyticsEntry) {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	select {
	case s.ch <- entry:
	default:
		atomic.AddInt64(&s.dropped, 1)
		log.WarningLog.Printf("[AnalyticsStore] Buffer full; dropped entry for %s/%s", entry.SessionID, entry.ToolName)
	}
}

// RecordFromResult builds and records an AnalyticsEntry from classification output.
func (s *AnalyticsStore) RecordFromResult(payload PermissionRequestPayload, result ClassificationResult, sessionID, approvalID string, durationMs int64) {
	cmd, _ := payload.ToolInput["command"].(string)
	filePath, _ := payload.ToolInput["file_path"].(string)
	preview := cmd
	if preview == "" {
		preview = filePath
	}
	if len(preview) > 200 {
		preview = preview[:200]
	}

	entry := AnalyticsEntry{
		SessionID:      sessionID,
		ToolName:       payload.ToolName,
		CommandPreview: preview,
		Cwd:            payload.Cwd,
		Decision:       decisionString(result.Decision),
		RiskLevel:      riskLevelString(result.RiskLevel),
		RuleID:         result.RuleID,
		RuleName:       result.RuleName,
		Reason:         result.Reason,
		Alternative:    result.Alternative,
		DurationMs:     durationMs,
		ApprovalID:     approvalID,
	}

	// For Bash tool calls, extract which programs are being invoked.
	if payload.ToolName == "Bash" && cmd != "" {
		info := ParseBashCommand(cmd)
		entry.CommandProgram = info.Program
		entry.CommandCategory = info.Category
		entry.CommandSubcategory = info.Subcommand
		// For Python interpreter calls, also extract inline imports.
		if pythonPrograms[info.Program] {
			pyInfo := ParsePythonCommand(cmd)
			entry.PythonImports = pyInfo.Imports
		}
	}

	s.Record(entry)
}

// RecordManualDecision records a manual approve/deny decision for an approval.
func (s *AnalyticsStore) RecordManualDecision(approvalID, sessionID, toolName, cwd, decision string) {
	s.Record(AnalyticsEntry{
		SessionID:  sessionID,
		ToolName:   toolName,
		Cwd:        cwd,
		Decision:   decision, // "manual_allow" or "manual_deny"
		ApprovalID: approvalID,
	})
}

// DroppedCount returns the number of entries dropped due to buffer overflow.
func (s *AnalyticsStore) DroppedCount() int64 {
	return atomic.LoadInt64(&s.dropped)
}

// LoadWindow reads JSONL entries from disk with timestamps >= since.
// Malformed lines are skipped with a warning.
func (s *AnalyticsStore) LoadWindow(since time.Time) ([]AnalyticsEntry, error) {
	f, err := os.Open(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", s.filePath, err)
	}
	defer f.Close()

	var entries []AnalyticsEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB per line
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e AnalyticsEntry
		if err := json.Unmarshal(line, &e); err != nil {
			log.WarningLog.Printf("[AnalyticsStore] Skipping malformed line: %v", err)
			continue
		}
		if !e.Timestamp.Before(since) {
			entries = append(entries, e)
		}
	}
	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("scan %s: %w", s.filePath, err)
	}
	return entries, nil
}

// ReclassifyGaps re-runs the current classifier against entries that were
// previously escalated with no matching rule (coverage gaps). Any entry that
// the current rules would now auto-allow or auto-deny has its Decision and
// RuleID updated in the returned copy — the underlying JSONL file is unchanged.
//
// Call this before ComputeSummary when you want coverage-gap metrics to reflect
// the CURRENT rule set rather than the rules that were active when the entry was
// recorded. This prevents historical gaps from artificially inflating the gap
// rate after new rules are added.
func ReclassifyGaps(entries []AnalyticsEntry, c Classifier) []AnalyticsEntry {
	result := make([]AnalyticsEntry, len(entries))
	copy(result, entries)
	for i, e := range result {
		if e.Decision != "escalate" || e.RuleID != "" {
			continue
		}
		// Build a minimal payload from the stored preview and tool name.
		payload := PermissionRequestPayload{
			ToolName: e.ToolName,
			ToolInput: map[string]interface{}{
				"command":   e.CommandPreview,
				"file_path": e.CommandPreview,
			},
		}
		r := c.Classify(payload, ClassificationContext{})
		if r.Decision == AutoAllow || r.Decision == AutoDeny {
			result[i].Decision = decisionString(r.Decision)
			result[i].RuleID = r.RuleID
			result[i].RuleName = r.RuleName
		}
	}
	return result
}

// ComputeSummary aggregates a slice of entries into an AnalyticsSummary.
// Pure function -- no I/O.
func ComputeSummary(entries []AnalyticsEntry) AnalyticsSummary {
	summary := AnalyticsSummary{
		DecisionCounts: make(map[string]int),
	}
	if len(entries) == 0 {
		return summary
	}

	summary.WindowStart = entries[0].Timestamp
	summary.WindowEnd = entries[0].Timestamp

	toolCounts := make(map[string]int)
	deniedCmds := make(map[string]CommandStat)
	ruleCounts := make(map[string]RuleStat)
	// program → {category, count} for Bash invocations
	programStats := make(map[string]ProgramStat)
	// module → count for Python inline imports
	importCounts := make(map[string]int)
	// coverage gap: tools/programs that escaped all rules (escalated with no rule_id)
	uncoveredToolCounts := make(map[string]int)
	uncoveredProgramStats := make(map[string]ProgramStat)
	// full (program, subcommand) distribution — keyed by "program\x00subcommand"
	subcommandStats := make(map[string]SubcommandStat)

	for _, e := range entries {
		summary.TotalDecisions++
		summary.DecisionCounts[e.Decision]++
		toolCounts[e.ToolName]++

		if e.Timestamp.Before(summary.WindowStart) {
			summary.WindowStart = e.Timestamp
		}
		if e.Timestamp.After(summary.WindowEnd) {
			summary.WindowEnd = e.Timestamp
		}

		if e.Decision == "auto_deny" || e.Decision == "manual_deny" {
			key := e.ToolName + ":" + e.CommandPreview
			stat := deniedCmds[key]
			stat.Preview = e.CommandPreview
			stat.ToolName = e.ToolName
			stat.Count++
			deniedCmds[key] = stat
		}

		if e.RuleID != "" {
			stat := ruleCounts[e.RuleID]
			stat.RuleID = e.RuleID
			stat.RuleName = e.RuleName
			stat.Count++
			ruleCounts[e.RuleID] = stat
		}

		// Aggregate command program stats (Bash tool only).
		if e.CommandProgram != "" {
			stat := programStats[e.CommandProgram]
			stat.Program = e.CommandProgram
			stat.Category = e.CommandCategory
			stat.Count++
			programStats[e.CommandProgram] = stat
		}

		// Aggregate Python import stats.
		for _, mod := range e.PythonImports {
			importCounts[mod]++
		}

		// Aggregate (program, subcommand) pairs — full distribution, no top-N cap.
		if e.CommandProgram != "" && e.CommandSubcategory != "" {
			key := e.CommandProgram + "\x00" + e.CommandSubcategory
			stat := subcommandStats[key]
			stat.Program = e.CommandProgram
			stat.Subcommand = e.CommandSubcategory
			stat.Category = e.CommandCategory
			stat.Count++
			subcommandStats[key] = stat
		}

		// Coverage gap: escalated with no matching rule (fell through all rules).
		if e.Decision == "escalate" && e.RuleID == "" {
			summary.CoverageGapCount++
			uncoveredToolCounts[e.ToolName]++
			if e.CommandProgram != "" {
				stat := uncoveredProgramStats[e.CommandProgram]
				stat.Program = e.CommandProgram
				stat.Category = e.CommandCategory
				stat.Count++
				uncoveredProgramStats[e.CommandProgram] = stat
			}
		}
	}

	// Build sorted top lists (top 10).
	summary.TopTools = topNTools(toolCounts, 10)
	summary.TopDeniedCommands = topNCommands(deniedCmds, 10)
	summary.TopTriggeredRules = topNRules(ruleCounts, 10)
	summary.TopCommandPrograms = topNPrograms(programStats, 10)
	summary.TopPythonImports = topNImports(importCounts, 10)
	summary.TopUncoveredTools = topNTools(uncoveredToolCounts, 10)
	summary.TopUncoveredPrograms = topNPrograms(uncoveredProgramStats, 10)

	// Build full (program, subcommand) distribution sorted by count desc.
	subStats := make([]SubcommandStat, 0, len(subcommandStats))
	for _, s := range subcommandStats {
		subStats = append(subStats, s)
	}
	sort.Slice(subStats, func(i, j int) bool {
		if subStats[i].Count != subStats[j].Count {
			return subStats[i].Count > subStats[j].Count
		}
		return subStats[i].Program+subStats[i].Subcommand < subStats[j].Program+subStats[j].Subcommand
	})
	summary.CommandSubcommandStats = subStats

	total := float64(summary.TotalDecisions)
	if total > 0 {
		summary.AutoApproveRate = float64(summary.DecisionCounts["auto_allow"]) / total
		manual := float64(summary.DecisionCounts["escalate"] + summary.DecisionCounts["manual_allow"] + summary.DecisionCounts["manual_deny"])
		summary.ManualReviewRate = manual / total
		summary.CoverageGapRate = float64(summary.CoverageGapCount) / total * 100
	}

	return summary
}

// DailyBucket aggregates classification decisions for a single calendar day.
type DailyBucket struct {
	Date        string `json:"date"` // "2006-01-02" in local time
	AutoAllow   int    `json:"auto_allow"`
	AutoDeny    int    `json:"auto_deny"`
	Escalate    int    `json:"escalate"`
	ManualAllow int    `json:"manual_allow"`
	ManualDeny  int    `json:"manual_deny"`
	Total       int    `json:"total"`
}

// AutoApproveRate returns the fraction of decisions that were auto-allowed.
func (b DailyBucket) AutoApproveRate() float64 {
	if b.Total == 0 {
		return 0
	}
	return float64(b.AutoAllow) / float64(b.Total)
}

// ComputeDailyBuckets groups entries by calendar day (local time) sorted ascending.
// Pure function — no I/O.
func ComputeDailyBuckets(entries []AnalyticsEntry) []DailyBucket {
	byDate := make(map[string]*DailyBucket)
	for _, e := range entries {
		date := e.Timestamp.Local().Format("2006-01-02")
		b := byDate[date]
		if b == nil {
			b = &DailyBucket{Date: date}
			byDate[date] = b
		}
		b.Total++
		switch e.Decision {
		case "auto_allow":
			b.AutoAllow++
		case "auto_deny":
			b.AutoDeny++
		case "escalate":
			b.Escalate++
		case "manual_allow":
			b.ManualAllow++
		case "manual_deny":
			b.ManualDeny++
		}
	}

	buckets := make([]DailyBucket, 0, len(byDate))
	for _, b := range byDate {
		buckets = append(buckets, *b)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Date < buckets[j].Date
	})
	return buckets
}

// flush drains the channel and appends to the JSONL file.
func (s *AnalyticsStore) flush(ctx interface{ Done() <-chan struct{} }) {
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0o755); err != nil {
		log.WarningLog.Printf("[AnalyticsStore] Cannot create analytics dir: %v", err)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var pending []AnalyticsEntry
	fsyncEvery := 10

	flushPending := func() {
		if len(pending) == 0 {
			return
		}
		f, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			log.WarningLog.Printf("[AnalyticsStore] Cannot open analytics file: %v", err)
			return
		}
		defer f.Close()
		for _, e := range pending {
			b, err := json.Marshal(e)
			if err != nil {
				log.WarningLog.Printf("[AnalyticsStore] Cannot marshal entry: %v", err)
				continue
			}
			if _, err := fmt.Fprintf(f, "%s\n", b); err != nil {
				log.WarningLog.Printf("[AnalyticsStore] Write error: %v", err)
			}
		}
		if len(pending) >= fsyncEvery {
			_ = f.Sync()
		}
		pending = pending[:0]
	}

	for {
		select {
		case <-ctx.Done():
			// Drain remaining
			for {
				select {
				case e := <-s.ch:
					pending = append(pending, e)
				default:
					flushPending()
					return
				}
			}
		case e := <-s.ch:
			pending = append(pending, e)
			if len(pending) >= fsyncEvery {
				flushPending()
			}
		case <-ticker.C:
			flushPending()
		}
	}
}

func decisionString(d ClassificationDecision) string {
	switch d {
	case AutoAllow:
		return "auto_allow"
	case AutoDeny:
		return "auto_deny"
	default:
		return "escalate"
	}
}

func riskLevelString(r RiskLevel) string {
	switch r {
	case RiskLow:
		return "low"
	case RiskMedium:
		return "medium"
	case RiskHigh:
		return "high"
	case RiskCritical:
		return "critical"
	default:
		return "medium"
	}
}

func topNTools(counts map[string]int, n int) []ToolStat {
	stats := make([]ToolStat, 0, len(counts))
	for name, count := range counts {
		stats = append(stats, ToolStat{ToolName: name, Count: count})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Count > stats[j].Count })
	if len(stats) > n {
		stats = stats[:n]
	}
	return stats
}

func topNCommands(cmds map[string]CommandStat, n int) []CommandStat {
	stats := make([]CommandStat, 0, len(cmds))
	for _, s := range cmds {
		stats = append(stats, s)
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Count > stats[j].Count })
	if len(stats) > n {
		stats = stats[:n]
	}
	return stats
}

func topNRules(rules map[string]RuleStat, n int) []RuleStat {
	stats := make([]RuleStat, 0, len(rules))
	for _, s := range rules {
		stats = append(stats, s)
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Count > stats[j].Count })
	if len(stats) > n {
		stats = stats[:n]
	}
	return stats
}

func topNPrograms(programs map[string]ProgramStat, n int) []ProgramStat {
	stats := make([]ProgramStat, 0, len(programs))
	for _, s := range programs {
		stats = append(stats, s)
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Count > stats[j].Count })
	if len(stats) > n {
		stats = stats[:n]
	}
	return stats
}

func topNImports(counts map[string]int, n int) []ImportStat {
	stats := make([]ImportStat, 0, len(counts))
	for mod, count := range counts {
		stats = append(stats, ImportStat{Module: mod, Count: count})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Count > stats[j].Count })
	if len(stats) > n {
		stats = stats[:n]
	}
	return stats
}
