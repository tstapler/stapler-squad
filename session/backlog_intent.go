package session

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BacklogIntentDraft is the LLM's structured read of a free-text omnibar
// message, meant for the user to review/edit before CreateBacklogItem is
// called — never created verbatim (see ParseBacklogItemIntent's proto doc).
type BacklogIntentDraft struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	// Confidence is 0..1, a hint for the review UI to flag the draft for
	// closer scrutiny — not a threshold that blocks creation.
	Confidence float64 `json:"confidence"`
}

// backlogIntentSystemPrompt is the stable system prompt for
// ParseBacklogItemIntent calls. Stable prompts enable prefix-caching across
// repeated calls, same rationale as headless.summarizeSystemPrompt.
const backlogIntentSystemPrompt = `You are a product analyst turning a loosely-described task into a well-formed backlog item. You do not have tool access and must not attempt to use one — output only the JSON described below.

Rules:
1. title: a short, clear, human-readable title (not kebab-case, not truncated raw text) summarizing the task.
2. description: a fleshed-out description of the task — expand on the input, but do not invent unrelated scope or fabricate specifics (file paths, function names, numbers) that are not implied by the input.
3. acceptance_criteria: 2-5 concrete, testable criteria the task should satisfy. If the input is too vague to derive any, return an empty list rather than inventing generic filler.
4. confidence: your 0.0-1.0 confidence that this structuring faithfully represents the input's intent. Use a low value when the input is ambiguous or very sparse.
5. Output ONLY a single JSON object, no text before or after it, matching this schema:
{"title":"...","description":"...","acceptance_criteria":["...","..."],"confidence":0.9}`

// BuildBacklogIntentSystemPrompt returns the stable system prompt for
// ParseBacklogItemIntent calls.
func BuildBacklogIntentSystemPrompt() string { return backlogIntentSystemPrompt }

// BuildBacklogIntentUserPrompt constructs the user prompt for a single
// ParseBacklogItemIntent call from the omnibar's free-text message.
// repoPath is optional context that narrows phrasing to a known repo.
func BuildBacklogIntentUserPrompt(message, repoPath string) string {
	var sb strings.Builder
	sb.WriteString("Task description:\n")
	sb.WriteString(message)
	if repoPath != "" {
		fmt.Fprintf(&sb, "\n\nRepo: %s", repoPath)
	}
	return sb.String()
}

// ParseBacklogItemIntentDraft unmarshals an LLM JSON response into a
// BacklogIntentDraft, tolerating preamble/trailing text and stray unrelated
// braces — mirrors ParseHeadlessTriageResult's use of
// extractTopLevelJSONObjects, trying candidates from the end of the response
// backwards since the prompt instructs the model to emit the JSON object
// last.
func ParseBacklogItemIntentDraft(raw string) (BacklogIntentDraft, error) {
	candidates := extractTopLevelJSONObjects(raw)
	if len(candidates) == 0 {
		preview := raw
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return BacklogIntentDraft{}, fmt.Errorf("ParseBacklogItemIntentDraft: no JSON object found in output (raw: %q)", preview)
	}

	var lastErr error
	for i := len(candidates) - 1; i >= 0; i-- {
		var draft BacklogIntentDraft
		if err := json.Unmarshal([]byte(candidates[i]), &draft); err != nil {
			lastErr = err
			continue
		}
		if draft.Title == "" && draft.Description == "" {
			// Syntactically valid but not a draft object (e.g. a decoy/example
			// object quoted earlier in the response) — keep looking.
			lastErr = fmt.Errorf("candidate has neither title nor description")
			continue
		}
		return draft, nil
	}

	preview := raw
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	return BacklogIntentDraft{}, fmt.Errorf("ParseBacklogItemIntentDraft: JSON parse error: %w (raw: %q)", lastErr, preview)
}
