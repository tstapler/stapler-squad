package session

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TriageSuggestion is a canonical suggestion entry shared by the headless triage
// path and the submit_triage_result MCP tool.
type TriageSuggestion struct {
	Text      string `json:"text"`
	Rationale string `json:"rationale"`
}

// TriageTask is a canonical implementation task shared by the headless triage
// path and the submit_triage_result MCP tool.
type TriageTask struct {
	Text     string `json:"text"`
	Estimate string `json:"estimate"`
	Category string `json:"category"`
}

// HeadlessTriageResult is the parsed output from a headless triage LLM call.
type HeadlessTriageResult struct {
	Summary     string             `json:"summary"`
	Suggestions []TriageSuggestion `json:"suggestions"`
	Tasks       []TriageTask       `json:"tasks,omitempty"`
}

// maxHeadlessTriageTasks caps the task list to keep the checklist scannable.
const maxHeadlessTriageTasks = 12

// BuildHeadlessTriagePrompt constructs the JSON-output triage prompt for a backlog item.
// artifactAbsPath is the absolute path where the LLM should write planning files.
func BuildHeadlessTriagePrompt(item *BacklogItemData, artifactAbsPath string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Backlog Item: %s\n\n", item.Title)
	fmt.Fprintf(&sb, "item_id: %s\n\n", item.ID)
	if item.Description != "" {
		fmt.Fprintf(&sb, "## Description\n%s\n\n", item.Description)
	}
	if item.AcceptanceCriteria != "" {
		criteria, _ := ParseAcCriteria(item.AcceptanceCriteria)
		if len(criteria) > 0 {
			sb.WriteString("## Acceptance Criteria\n")
			for _, c := range criteria {
				fmt.Fprintf(&sb, "%d. %s\n", c.Index, c.Text)
			}
			sb.WriteString("\n")
		}
	}

	researchDir := artifactAbsPath + "/research"
	fmt.Fprintf(&sb, `## Task

Perform pre-implementation triage for this backlog item. Work in parallel:

### Step 1 — Research (run 4 subagents in parallel)
Each subagent writes one file:
- %s/stack.md        — Technology choices, versions, compatibility
- %s/features.md     — Similar existing features, patterns to reuse
- %s/architecture.md — Proposed architecture, component boundaries
- %s/pitfalls.md     — Known risks, gotchas, failure modes

### Step 2 — Synthesis (after research completes)
Write %s/plan.md containing:
- Executive summary (2-3 sentences)
- Implementation approach
- Task breakdown with time estimates
- Dependencies and blockers

### Step 3 — Validation
Write %s/validation.md containing:
- Test plan mapping each acceptance criterion to a specific test
- Edge cases and error scenarios

### Step 4 — Output
After all files are written, output ONLY a JSON object (no other text before or after):
{"summary":"2-3 sentence summary","suggestions":[{"text":"...","rationale":"..."}],"tasks":[{"text":"task description","estimate":"2h","category":"backend"}]}
- suggestions: AC gaps, open questions, improvement ideas (questions use rationale="question")
- tasks: implementation task breakdown from plan.md (max 12)
- Do NOT call submit_triage_result. Do NOT write any source code.
`, researchDir, researchDir, researchDir, researchDir, artifactAbsPath, artifactAbsPath)

	return sb.String()
}

// ParseHeadlessTriageResult unmarshals an LLM JSON response into HeadlessTriageResult.
// Strips markdown code fences before parsing. Caps tasks at maxHeadlessTriageTasks.
func ParseHeadlessTriageResult(raw string) (HeadlessTriageResult, error) {
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```") {
		lines := strings.SplitN(text, "\n", 2)
		if len(lines) == 2 {
			text = lines[1]
		}
		if idx := strings.LastIndex(text, "```"); idx >= 0 {
			text = strings.TrimSpace(text[:idx])
		}
	}

	var result HeadlessTriageResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		preview := raw
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return HeadlessTriageResult{}, fmt.Errorf("ParseHeadlessTriageResult: JSON parse error: %w (raw: %q)", err, preview)
	}
	if len(result.Tasks) > maxHeadlessTriageTasks {
		result.Tasks = result.Tasks[:maxHeadlessTriageTasks]
	}
	return result, nil
}
