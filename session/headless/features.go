package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Feature key constants for well-known AI features.
const (
	FeatureKeyReview             FeatureKey = "review"
	FeatureKeySummarize          FeatureKey = "summarize"
	FeatureKeyAC                 FeatureKey = "acceptance-criteria"
	FeatureKeyPRDescription      FeatureKey = "pr-description"
	FeatureKeyCommitMessage      FeatureKey = "commit-message"
	FeatureKeyCustom             FeatureKey = "custom"
	FeatureKeyAutonomousFix      FeatureKey = "autonomous_fix"
	FeatureKeyAutonomousApproval FeatureKey = "autonomous_approval"
	FeatureKeyTriage             FeatureKey = "triage"
)

// AllowedFeatureKeys is the set of feature keys accepted by the MCP-exposed RunHeadlessCall path
// (server/services/headless_service.go). FeatureKeyTriage is intentionally excluded — triage calls
// go through BacklogService.TriggerTriage → Pool.CallBlockingWithOptions directly, bypassing the
// MCP gate. This prevents triage from being triggered via the public headless API.
var AllowedFeatureKeys = map[FeatureKey]bool{
	FeatureKeyReview:        true,
	FeatureKeySummarize:     true,
	FeatureKeyAC:            true,
	FeatureKeyPRDescription: true,
	FeatureKeyCommitMessage: true,
	FeatureKeyCustom:        true,
}

// AllowedFeatureKeyList returns a sorted comma-separated list of allowed feature keys
// for use in error messages. Generated from AllowedFeatureKeys to stay in sync.
func AllowedFeatureKeyList() string {
	keys := make([]string, 0, len(AllowedFeatureKeys))
	for k := range AllowedFeatureKeys {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// DefaultCallTimeout is the default headless call timeout applied when timeout_seconds is 0.
const DefaultCallTimeout = 900 * time.Second

// MaxCallTimeout caps timeout_seconds.
const MaxCallTimeout = 1800 * time.Second

// MaxDiffSizeReview is the maximum number of bytes included in a review prompt diff.
const MaxDiffSizeReview = 40_000

// maxDiffSizePR is the max byte size for diffs passed to DraftPRDescription.
const maxDiffSizePR = 40_000

// maxDiffSizeCommit is the max byte size for diffs passed to SuggestCommitMessage.
const maxDiffSizeCommit = 20_000

// summarizeSystemPrompt is the stable system prompt for SummarizeBacklogItem.
// Stable prompts enable prefix-caching across repeated calls.
const summarizeSystemPrompt = `You are a backlog analyst. Produce a one-paragraph summary and suggest up to 3 tags. Output JSON: {"summary":"...","tags":[...]}`

// acSystemPrompt is the stable system prompt for GenerateAcceptanceCriteria.
const acSystemPrompt = `You are a product analyst. Output exactly 3-5 acceptance criteria as a JSON array of strings. Each criterion must be testable and specific.`

// prDescriptionSystemPrompt is the stable system prompt for DraftPRDescription.
const prDescriptionSystemPrompt = `You are a technical writer. Draft a pull request description using Conventional Commit conventions. Format: ## Summary, ## Changes, ## Test plan.`

// commitMessageSystemPrompt is the stable system prompt for SuggestCommitMessage.
const commitMessageSystemPrompt = `You are a commit message expert. Output a single Conventional Commit message (type(scope): description). No extra text.`

// reviewSystemPrompt is the stable role/instruction portion of the review prompt.
// This is separated from the per-call data payload (item, diff) to enable prefix-caching.
const reviewSystemPrompt = `You are a code review agent. Your ONLY task is to evaluate the diff against the acceptance criteria and call submit_review_verdict. Do not write any code. Do not modify any files.`

// headlessReviewSystemPrompt is used for headless review calls that have no tool access.
// Instructs the model to output JSON instead of invoking a tool.
const headlessReviewSystemPrompt = `You are a code review agent. Evaluate the diff against the acceptance criteria. Output ONLY a single JSON object — no other text before or after it:
{"overall":"PASS","summary":"concise assessment","verdicts":[{"criterion_index":0,"outcome":"PASS","evidence":"direct quote from diff"}]}
Valid outcome values: PASS, FAIL, PARTIAL, UNVERIFIABLE. Set overall to PASS only when every criterion passes.`

// ReviewSystemPrompt returns the stable system prompt for review gate calls.
// Exported so session/backlog_lifecycle.go can use it without embedding the prompt inline.
func ReviewSystemPrompt() string { return reviewSystemPrompt }

// HeadlessReviewSystemPrompt returns the system prompt for headless (no-tool) review calls.
// Requests JSON output so the caller can parse the verdict without tool execution.
func HeadlessReviewSystemPrompt() string { return headlessReviewSystemPrompt }

// headlessTriageSystemPrompt instructs the model to perform pre-implementation
// triage and output JSON. No submit_triage_result call; result is parsed directly.
const headlessTriageSystemPrompt = `You are a senior software architect performing pre-implementation triage. You have full filesystem write access to the artifact directory specified in the user prompt. Work systematically.

Rules:
1. Write all planning files to the artifact directory specified in the user prompt.
2. Do NOT modify any source code.
3. After writing all files, output ONLY a single JSON object — no text before or after it — matching this schema:
{"summary":"2-3 sentence executive summary","suggestions":[{"text":"...","rationale":"..."}],"tasks":[{"text":"one-line task","estimate":"2h","category":"backend"}]}
Valid categories: backend, frontend, test, infra, docs. Maximum 12 tasks.`

// HeadlessTriageSystemPrompt returns the stable system prompt for headless triage calls.
// Requests JSON output so the caller can parse the result without MCP tool execution.
func HeadlessTriageSystemPrompt() string { return headlessTriageSystemPrompt }

// SummarizeBacklogItem calls the LLM to summarize a backlog item.
// Returns the summary text from the JSON response.
func SummarizeBacklogItem(ctx context.Context, pool *Pool, title, description string) (string, error) {
	userPrompt := fmt.Sprintf("Title: %s\n\nDescription: %s", title, description)
	raw, err := pool.CallBlocking(ctx, FeatureKeySummarize, summarizeSystemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("SummarizeBacklogItem: %w", err)
	}

	var resp struct {
		Summary string   `json:"summary"`
		Tags    []string `json:"tags"`
	}
	if jsonErr := json.Unmarshal([]byte(raw), &resp); jsonErr != nil {
		// Return raw text as fallback when JSON parsing fails.
		return raw, nil
	}
	return resp.Summary, nil
}

// GenerateAcceptanceCriteria calls the LLM to generate acceptance criteria.
// Returns a slice of criterion strings.
func GenerateAcceptanceCriteria(ctx context.Context, pool *Pool, title, description string) ([]string, error) {
	userPrompt := fmt.Sprintf("Title: %s\n\nDescription: %s", title, description)
	raw, err := pool.CallBlocking(ctx, FeatureKeyAC, acSystemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("GenerateAcceptanceCriteria: %w", err)
	}

	var criteria []string
	if jsonErr := json.Unmarshal([]byte(raw), &criteria); jsonErr != nil {
		return nil, fmt.Errorf("GenerateAcceptanceCriteria: JSON parse error: %w", jsonErr)
	}
	if len(criteria) == 0 {
		return nil, fmt.Errorf("GenerateAcceptanceCriteria: empty criteria list")
	}
	return criteria, nil
}

// DraftPRDescription calls the LLM to draft a pull request description.
// Diffs longer than maxDiffSizePR bytes are truncated before sending.
func DraftPRDescription(ctx context.Context, pool *Pool, diff, branchName string) (string, error) {
	if len(diff) > maxDiffSizePR {
		diff = diff[:maxDiffSizePR]
	}
	userPrompt := fmt.Sprintf("Branch: %s\n\nDiff:\n%s", branchName, diff)
	raw, err := pool.CallBlocking(ctx, FeatureKeyPRDescription, prDescriptionSystemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("DraftPRDescription: %w", err)
	}
	return raw, nil
}

// SuggestCommitMessage calls the LLM to generate a Conventional Commit message.
// Diffs longer than maxDiffSizeCommit bytes are truncated before sending.
func SuggestCommitMessage(ctx context.Context, pool *Pool, diff string) (string, error) {
	if len(diff) > maxDiffSizeCommit {
		diff = diff[:maxDiffSizeCommit]
	}
	raw, err := pool.CallBlocking(ctx, FeatureKeyCommitMessage, commitMessageSystemPrompt, diff)
	if err != nil {
		return "", fmt.Errorf("SuggestCommitMessage: %w", err)
	}
	return raw, nil
}
