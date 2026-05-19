package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// DefaultRulePromptBuilder implements RulePromptBuilder.
// It is a pure function — no I/O or external calls.
type DefaultRulePromptBuilder struct{}

// BuildSystemPrompt returns the system prompt for rule suggestion.
// Includes a JSON schema of SuggestedRuleProto fields, existing rules as JSON,
// 5 seed examples, pattern priority instructions, and a priority tier legend.
func (b *DefaultRulePromptBuilder) BuildSystemPrompt(ctx RulePromptContext) string {
	var sb strings.Builder

	sb.WriteString(`You are an expert security analyst helping configure auto-approval rules for an AI agent orchestration system called stapler-squad.

Your task is to propose new auto-approval rules that reduce unnecessary manual review while maintaining security.

## JSON schema for rule objects

Each rule in the returned JSON array must conform to this schema:

{
  "name": "string (required, human-readable)",
  "tool_name": "string (optional, exact match: Bash, Edit, Write, Read, etc.)",
  "tool_pattern": "string (optional, RE2 regex matching tool name)",
  "command_pattern": "string (optional, RE2 regex matching command text)",
  "file_pattern": "string (optional, RE2 regex matching file path)",
  "decision": "string (required, one of: auto_allow, auto_deny, escalate)",
  "risk_level": "string (required, one of: low, medium, high, critical)",
  "reason": "string (required, explains why this decision is safe)",
  "alternative": "string (optional, safer alternative if decision is deny/escalate)",
  "priority": "integer (1-999, default 100)",
  "confidence": "float (0.0-1.0, your certainty in this pattern)",
  "explanation": "string (why you chose these specific field values)",
  "source_commands": ["array of up to 20 representative commands"]
}

## Priority tiers

- 1000: AutoDeny (critical, must fire before any allow)
- 500: Escalate-before-allow (targeted escalations overriding allows at 100)
- 100: AutoAllow (standard development operations, default tier)
- 50: Escalate catch-all (operations with no allow rule)

Use priority 100 for standard auto-allow rules unless there is a strong reason to deviate.

## Pattern priority instructions

1. Prefer tool_name (exact match) over tool_pattern (regex) for simple tool matching.
2. Use command_pattern only when you need to match specific command text; do not use it to match everything.
3. Keep patterns specific enough to avoid unintended matches but general enough to cover the gap cluster.
4. Always use RE2-compatible regex syntax (no lookaheads, no backreferences).
5. Compile all patterns mentally — if you are unsure, keep them simple.

## Existing rules (do not duplicate these)

`)

	// Serialize existing rules as JSON.
	if len(ctx.ExistingRules) > 0 {
		existingJSON, err := json.MarshalIndent(ctx.ExistingRules, "", "  ")
		if err == nil {
			sb.Write(existingJSON)
		} else {
			sb.WriteString("[]")
		}
	} else {
		sb.WriteString("[]")
	}

	sb.WriteString("\n\n## Seed examples (style reference — 5 canonical rules)\n\n")

	// Pick up to 5 representative seed examples.
	seeds := pickSeedExamples()
	seedJSON, err := json.MarshalIndent(seeds, "", "  ")
	if err == nil {
		sb.Write(seedJSON)
	} else {
		sb.WriteString("[]")
	}

	sb.WriteString(`

## Response format

Return ONLY a JSON array of rule objects — no prose, no markdown fences, no explanation outside the JSON.
The array may contain 1–5 items. Example structure:

[
  { "name": "...", "tool_name": "Bash", "command_pattern": "...", "decision": "auto_allow", ... },
  { "name": "...", ... }
]
`)

	return sb.String()
}

// BuildUserPrompt returns the user prompt, formatted by source type.
// Before including any CommandPreview, it applies a second-pass secret scan
// and replaces positives with [REDACTED] (defense-in-depth, per FLAG-1).
func (b *DefaultRulePromptBuilder) BuildUserPrompt(ctx RulePromptContext) string {
	var sb strings.Builder

	switch {
	case ctx.CommandSample != "":
		// COMMAND_SAMPLE source: focus on a single raw command.
		cmd := ctx.CommandSample
		if hit := ScanForSecrets(cmd); hit.Found {
			cmd = redactedPrompt
		}
		sb.WriteString("Analyze this specific command and propose 1 rule to auto-approve or auto-deny it appropriately:\n\n")
		sb.WriteString("Command: ")
		sb.WriteString(cmd)
		sb.WriteString("\n\n")
		sb.WriteString("Return a JSON array with exactly 1 rule object. Do not duplicate any existing rule pattern.")

	default:
		// ANALYTICS_GAPS (default): format top gap clusters.
		gaps := ctx.AnalyticsGaps
		// Sort by count descending.
		sort.Slice(gaps, func(i, j int) bool { return gaps[i].Count > gaps[j].Count })

		// Apply filters.
		if ctx.ToolNameFilter != "" || ctx.ProgramFilter != "" {
			var filtered []AnalyticsGap
			for _, g := range gaps {
				if ctx.ToolNameFilter != "" && g.ToolName != ctx.ToolNameFilter {
					continue
				}
				if ctx.ProgramFilter != "" && g.Program != ctx.ProgramFilter {
					continue
				}
				filtered = append(filtered, g)
			}
			gaps = filtered
		}

		// Cap to top 5 gap clusters.
		if len(gaps) > 5 {
			gaps = gaps[:5]
		}

		if len(gaps) == 0 {
			sb.WriteString("There are no uncovered command gaps in the analytics window. No rules are needed.\n\n")
			sb.WriteString("Return an empty JSON array: []")
			return sb.String()
		}

		fmt.Fprintf(&sb, "The following %d gap cluster(s) represent commands that escaped all auto-approval rules in the last %d day(s). ", len(gaps), ctx.WindowDays)
		sb.WriteString("Each cluster groups similar commands by tool and program. Propose rules to cover these gaps.\n\n")

		for i, gap := range gaps {
			fmt.Fprintf(&sb, "### Gap %d: Tool=%q, Program=%q, Count=%d\n", i+1, gap.ToolName, gap.Program, gap.Count)
			if len(gap.RepresentativeCmds) > 0 {
				sb.WriteString("Representative commands:\n")
				for _, cmd := range gap.RepresentativeCmds {
					// Second-pass secret redaction (defense-in-depth).
					if hit := ScanForSecrets(cmd); hit.Found {
						cmd = redactedPrompt
					}
					// Wrap in XML delimiters to prevent prompt injection from command content.
					fmt.Fprintf(&sb, "  - <command>%s</command>\n", cmd)
				}
			}
			sb.WriteString("\n")
		}

		sb.WriteString("Propose up to 5 rules. Return a JSON array. Do not duplicate any existing rule pattern.")
	}

	return sb.String()
}

// pickSeedExamples selects 5 representative seed rules as style examples.
func pickSeedExamples() []map[string]interface{} {
	allSeeds := classifier.SeedRules()
	// Pick one from each decision type as examples, plus a few more.
	// We target: 1 auto_deny, 1 escalate, 3 auto_allow (diverse).
	var deny, escalate, allow []classifier.Rule
	for _, r := range allSeeds {
		switch r.Decision {
		case classifier.AutoDeny:
			deny = append(deny, r)
		case classifier.Escalate:
			escalate = append(escalate, r)
		case classifier.AutoAllow:
			allow = append(allow, r)
		}
	}

	var selected []classifier.Rule
	if len(deny) > 0 {
		selected = append(selected, deny[0])
	}
	if len(escalate) > 0 {
		selected = append(selected, escalate[0])
	}
	for i := 0; i < 3 && i < len(allow); i++ {
		selected = append(selected, allow[i])
	}
	// Cap at 5.
	if len(selected) > 5 {
		selected = selected[:5]
	}

	result := make([]map[string]interface{}, 0, len(selected))
	for _, r := range selected {
		m := map[string]interface{}{
			"name":       r.Name,
			"decision":   decisionString(r.Decision),
			"risk_level": riskLevelString(r.RiskLevel),
			"priority":   r.Priority,
			"reason":     r.Reason,
			"source":     r.Source,
		}
		if r.ToolName != "" {
			m["tool_name"] = r.ToolName
		}
		if r.ToolPattern != nil {
			m["tool_pattern"] = r.ToolPattern.String()
		}
		if r.CommandPattern != nil {
			m["command_pattern"] = r.CommandPattern.String()
		}
		if r.FilePattern != nil {
			m["file_pattern"] = r.FilePattern.String()
		}
		result = append(result, m)
	}
	return result
}
