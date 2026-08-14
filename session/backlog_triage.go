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
	Title   string `json:"title"`
	Summary string `json:"summary"`
	// Priority is the LLM's assessed urgency/impact (1=P1 critical ... 5=P5
	// trivial), applied to the item once triage completes — see
	// applyTriageResultToUpdate (server/services/backlog_service_triage.go).
	// Zero (omitted by the model) means "no assessment" and leaves the item's
	// existing priority untouched, same convention as AcceptanceCriteria below.
	Priority int `json:"priority,omitempty"`
	// ItemCategory is the LLM's classification of what kind of work this is —
	// one of session.BacklogCategory's values (bugfix/feature/chore/refactor).
	// Named distinctly from TriageTask.Category (engineering area: backend/
	// frontend/test/infra/docs) to avoid the two colliding in the same JSON
	// object the model produces. Empty or invalid leaves the item's existing
	// category untouched.
	ItemCategory       string             `json:"item_category,omitempty"`
	Suggestions        []TriageSuggestion `json:"suggestions"`
	Tasks              []TriageTask       `json:"tasks,omitempty"`
	AcceptanceCriteria []AcCriterion      `json:"acceptance_criteria,omitempty"`
	// Iteration and Feedback are not part of the LLM's JSON output — the caller
	// sets them after parsing, from server-tracked state, before persisting.
	Iteration int    `json:"iteration,omitempty"`
	Feedback  string `json:"feedback,omitempty"`
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
{"title":"fix-short-kebab-name","summary":"2-3 sentence summary","acceptance_criteria":[{"index":0,"text":"Clear, testable criterion","status":"pending"}],"suggestions":[{"text":"...","rationale":"..."}],"tasks":[{"text":"task description","estimate":"2h","category":"backend"}]}
- title: short kebab-case session name (3-5 words, imperative verb first, e.g. "fix-session-rename" or "add-pr-status-badge")
- summary: 2-3 sentence executive summary
- acceptance_criteria: full list of testable acceptance criteria (replace any existing ones). Each has index (0-based), text (one clear testable statement), status ("pending"). Merge with existing criteria: keep unchanged ones, add new ones, update clarified ones.
- suggestions: additional open questions or improvement ideas beyond the ACs (questions use rationale="question")
- tasks: implementation task breakdown from plan.md (max 12)
- Do NOT call submit_triage_result. Do NOT write any source code.
`, researchDir, researchDir, researchDir, researchDir, artifactAbsPath, artifactAbsPath)

	return sb.String()
}

// BuildHeadlessRetriagePrompt constructs a JSON-output prompt that refines a prior
// triage result using free-text user feedback. artifactAbsPath is the same
// directory used by the original triage run — research/*.md, plan.md, and
// validation.md already exist there and are treated as valid context unless the
// feedback indicates otherwise.
func BuildHeadlessRetriagePrompt(item *BacklogItemData, artifactAbsPath string, prior HeadlessTriageResult, feedback string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Backlog Item: %s\n\n", item.Title)
	fmt.Fprintf(&sb, "item_id: %s\n\n", item.ID)
	if item.Description != "" {
		fmt.Fprintf(&sb, "## Description\n%s\n\n", item.Description)
	}

	sb.WriteString("## Prior triage result (iteration ")
	fmt.Fprintf(&sb, "%d)\n", prior.Iteration)
	fmt.Fprintf(&sb, "Summary: %s\n", prior.Summary)
	if len(prior.Suggestions) > 0 {
		sb.WriteString("Suggestions:\n")
		for _, s := range prior.Suggestions {
			fmt.Fprintf(&sb, "- %s\n", s.Text)
		}
	}
	if len(prior.Tasks) > 0 {
		sb.WriteString("Tasks:\n")
		for _, t := range prior.Tasks {
			fmt.Fprintf(&sb, "- [%s, %s] %s\n", t.Category, t.Estimate, t.Text)
		}
	}

	researchDir := artifactAbsPath + "/research"
	fmt.Fprintf(&sb, `
## User feedback
%s

## Task
Revise the triage for this backlog item using the feedback above. The
existing artifacts at %s/plan.md, %s/validation.md, and %s/*.md remain valid
context — read and revise them in place; do not start from scratch.

If the feedback indicates the prior research (stack.md, features.md,
architecture.md, pitfalls.md) was incomplete or wrong, re-run only the
affected research subagent(s) and rewrite those files before revising the
plan. Otherwise revise plan.md and validation.md directly using the existing
research as-is.

After writing all files, output ONLY a JSON object (no other text before or
after):
{"title":"fix-short-kebab-name","summary":"2-3 sentence summary","suggestions":[{"text":"...","rationale":"..."}],"tasks":[{"text":"task description","estimate":"2h","category":"backend"}]}
- title: short kebab-case session name (3-5 words, imperative verb first)
- summary: 2-3 sentence executive summary of the REVISED plan
- suggestions: AC gaps, open questions, improvement ideas (questions use rationale="question")
- tasks: revised implementation task breakdown (max 12)
- Do NOT call submit_triage_result. Do NOT write any source code.
`, feedback, artifactAbsPath, artifactAbsPath, researchDir)

	return sb.String()
}

// BuildHeadlessChatRetriagePrompt wraps BuildHeadlessRetriagePrompt with an
// instruction to ask at most one clarifying question per turn — used for
// chat-originated refinement (CreateBacklogItemFromChat's existing_item_id
// path), where a tightened one-question-at-a-time round trip is expected
// instead of a batch dump of questions.
func BuildHeadlessChatRetriagePrompt(item *BacklogItemData, artifactAbsPath string, prior HeadlessTriageResult, feedback string) string {
	base := BuildHeadlessRetriagePrompt(item, artifactAbsPath, prior, feedback)
	return base + `

## Chat mode
This is a live back-and-forth chat conversation, not a batch review. If you
have any open questions, include AT MOST ONE in suggestions (rationale=
"question") — never more than one question in a single response. Save any
other questions for a later turn.
`
}

// extractTopLevelJSONObjects returns every complete JSON object found in raw, in
// the order they appear, by attempting a real JSON decode starting at each `{`.
//
// This is more robust than a naive first-`{`/last-`}` scan (which spans across an
// unrelated brace in prose and the real JSON, producing an unparseable concatenated
// blob) and more robust than a hand-rolled brace-depth counter (which permanently
// "gets stuck" if an earlier `{` in prose is never closed — depth never returns to
// zero, silently swallowing every well-formed object that follows). Delegating to
// json.Decoder makes each attempt independent: a malformed or unmatched brace at
// one position simply fails to decode there and has no effect on whether a later,
// well-formed object is found. A `{` embedded inside an already-successfully-parsed
// object's string value is never re-examined, since the scan skips forward past
// the full span the decoder consumed.
func extractTopLevelJSONObjects(raw string) []string {
	var candidates []string
	for i := 0; i < len(raw); i++ {
		if raw[i] != '{' {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(raw[i:]))
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			continue // not a valid JSON value starting here; keep scanning byte by byte
		}
		candidates = append(candidates, string(v))
		i += int(dec.InputOffset()) - 1 // -1 compensates for the loop's own i++
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
