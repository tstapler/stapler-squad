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

// extractTopLevelJSONObjects scans raw for every balanced, top-level `{...}` span,
// respecting string literals (so a `{` or `}` inside a quoted JSON string does not
// affect brace depth) and returned in the order they appear in raw.
//
// This is more robust than a naive first-`{`/last-`}` scan: if the model's response
// contains an unrelated brace in prose before the real JSON (e.g. an illustrative
// "{example}" snippet), first/last indexing spans across both and produces a
// concatenated, unparseable blob. Balanced scanning instead yields one candidate
// per brace-delimited span, letting the caller try each independently.
func extractTopLevelJSONObjects(raw string) []string {
	var candidates []string
	var inString, escaped bool
	depth := 0
	start := -1

	for i, r := range raw {
		if inString {
			switch {
			case escaped:
				escaped = false
			case r == '\\':
				escaped = true
			case r == '"':
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start != -1 {
					candidates = append(candidates, raw[start:i+1]) // +1: include the closing '}' itself
					start = -1
				}
			}
		}
	}
	return candidates
}

// ParseHeadlessTriageResult unmarshals an LLM JSON response into HeadlessTriageResult.
// Tolerates preamble text before the JSON block (e.g. "Here is the result:\n\n{...}")
// and stray unrelated braces earlier in the response (e.g. an illustrative snippet).
//
// The triage prompt instructs the model to emit the JSON object last, so candidates
// are tried from the end of the response backwards — the first candidate (i.e. the
// last brace-delimited span in raw) that unmarshals cleanly wins. This correctly
// skips over any earlier decoy object that happens to also be syntactically valid
// JSON but isn't the real result.
//
// Caps tasks at maxHeadlessTriageTasks.
func ParseHeadlessTriageResult(raw string) (HeadlessTriageResult, error) {
	candidates := extractTopLevelJSONObjects(raw)
	if len(candidates) == 0 {
		preview := raw
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return HeadlessTriageResult{}, fmt.Errorf("ParseHeadlessTriageResult: no JSON object found in output (raw: %q)", preview)
	}

	var lastErr error
	for i := len(candidates) - 1; i >= 0; i-- {
		var result HeadlessTriageResult
		if err := json.Unmarshal([]byte(candidates[i]), &result); err != nil {
			lastErr = err
			continue
		}
		if len(result.Tasks) > maxHeadlessTriageTasks {
			result.Tasks = result.Tasks[:maxHeadlessTriageTasks]
		}
		return result, nil
	}

	preview := raw
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	return HeadlessTriageResult{}, fmt.Errorf("ParseHeadlessTriageResult: JSON parse error: %w (raw: %q)", lastErr, preview)
}
