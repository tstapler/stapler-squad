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
// go through BacklogService.TriggerTriage → Pool.CallBlocking directly, bypassing the
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
const reviewSystemPrompt = `You are a code review agent. Your ONLY task is to evaluate the diff against the acceptance criteria and call submit_review_verdict. Do not write any code. Do not modify any files.
Some acceptance criteria describe things that cannot be observed in a diff at all — a test suite passing, a build succeeding, a manually-verified UI behavior. For these, consult the "## Verification Evidence" section if present: it is evidence self-reported by the work session, not derived from the diff. Treat it as you would a claim from a colleague — specific, checkable claims (an exact command and its result, a specific UI element and what was observed) are credible evidence and may resolve a criterion as PASS. Vague or generic claims ("I tested it", "verified manually", "works as expected") with no specifics are not evidence — do not let them upgrade a verdict. If there is no Verification Evidence section and a criterion is not visible in the diff, mark it UNVERIFIABLE as usual.`

// headlessReviewSystemPrompt is used for headless review calls that have no tool access.
// Instructs the model to output JSON instead of invoking a tool. Used on the diff != ""
// path (BuildReviewCallOptions) — the diff == "" path uses
// headlessReviewSystemPromptWithCodebaseAccess instead.
const headlessReviewSystemPrompt = `You are a code review agent. Evaluate the diff against the acceptance criteria. Output ONLY a single JSON object — no other text before or after it:
{"overall":"PASS","summary":"concise assessment","verdicts":[{"criterion_index":0,"outcome":"PASS","evidence":"direct quote from diff"}]}
Valid outcome values: PASS, FAIL, PARTIAL, UNVERIFIABLE. Set overall to PASS only when every criterion passes.
Some acceptance criteria describe things that cannot be observed in a diff at all — a test suite passing, a build succeeding, a manually-verified UI behavior. For these, consult the "## Verification Evidence" section if present: it is evidence self-reported by the work session, not derived from the diff. Treat it as you would a claim from a colleague — specific, checkable claims (an exact command and its result, a specific UI element and what was observed) are credible evidence and may resolve a criterion as PASS, with the evidence field quoting the specific claim. Vague or generic claims ("I tested it", "verified manually", "works as expected") with no specifics are not evidence — do not let them upgrade a verdict. If there is no Verification Evidence section and a criterion is not visible in the diff, mark it UNVERIFIABLE as usual.
A criterion's self-reported Note (e.g. "already implemented, no diff needed") is informational context only, not evidence. It is never sufficient by itself for that criterion's PASS — you must still find the criterion's satisfying change reflected in the diff itself, or mark it FAIL/UNVERIFIABLE.`

// headlessReviewSystemPromptWithCodebaseAccess is used for empty-diff headless review
// calls that ARE granted read-only tool access (Read/Grep/Glob) via CallOptions.WorkDir.
// Uses falsification framing: the model must independently locate and quote its OWN
// evidence, treating the work session's claim as a hypothesis to check, not a fact to
// accept. Also requires a "tool_reads" list of files actually opened, so the caller can
// detect a PASS/FAIL reached with no real evidence of tool use and degrade it.
const headlessReviewSystemPromptWithCodebaseAccess = `You are a code review agent reviewing a backlog item where no diff was found — either the acceptance criteria were already satisfied before this session started, or no work was done. You have read-only tool access (Read, Grep, Glob) scoped to the item's own repository checkout. Use it to independently verify each criterion:
1. Search the current codebase yourself for code that satisfies each criterion. Treat the work session's note or verification evidence as a hypothesis to falsify, not a citation to trust — a criterion is not PASS merely because the agent claims it is already implemented.
2. When you find satisfying code, your evidence field must quote YOUR OWN file path plus line/symbol and snippet from what you read — not a restatement of the agent's claim.
3. If you cannot locate satisfying code — including because it would live in a vendored or dependency path you cannot fully inspect — mark that criterion UNVERIFIABLE, not PASS.
4. Do not modify any files. Do not run destructive or write commands.
5. List EVERY file path you actually opened with Read/Grep/Glob during this review in "tool_reads", even if it didn't contain relevant code. If you did not use any tool, leave "tool_reads" empty — do not fabricate entries; an empty list on a confident verdict will be treated as unverified.
Output ONLY a single JSON object — no other text before or after it:
{"overall":"PASS","summary":"concise assessment","tool_reads":["path/to/file.go","path/to/other.go"],"verdicts":[{"criterion_index":0,"outcome":"PASS","evidence":"file:line — quoted snippet you read yourself"}]}
Valid outcome values: PASS, FAIL, PARTIAL, UNVERIFIABLE. Set overall to PASS only when every criterion passes.`

// ReviewSystemPrompt returns the stable system prompt for review gate calls.
// Exported so session/backlog_lifecycle.go can use it without embedding the prompt inline.
func ReviewSystemPrompt() string { return reviewSystemPrompt }

// HeadlessReviewSystemPrompt returns the system prompt for headless (no-tool) review calls.
// Requests JSON output so the caller can parse the verdict without tool execution.
func HeadlessReviewSystemPrompt() string { return headlessReviewSystemPrompt }

// HeadlessReviewSystemPromptWithCodebaseAccess returns the system prompt used for
// empty-diff headless review calls granted codebase read access.
func HeadlessReviewSystemPromptWithCodebaseAccess() string {
	return headlessReviewSystemPromptWithCodebaseAccess
}

// CodebaseReadCallTimeout is the context timeout used for the empty-diff codebase-read
// headless call — deliberately shorter than DefaultCallTimeout (900s) so a hung or
// degraded tool-access call fails fast into the UNVERIFIABLE degrade path instead of
// blocking the review gate for up to 15 minutes.
const CodebaseReadCallTimeout = 150 * time.Second

// CodebaseReadAllowedTools is the AllowedTools value granted to the empty-diff
// codebase-read headless call (BuildReviewCallOptions) and to the capability
// self-check probe (CodebaseReadCapabilitySelfCheck.run), which exercises the same
// call shape. Shared as a constant so the two call sites cannot silently drift.
const CodebaseReadAllowedTools = "Read,Grep,Glob"

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
	raw, _, err := pool.CallBlocking(ctx, FeatureKeySummarize, summarizeSystemPrompt, userPrompt, CallOptions{})
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
	raw, _, err := pool.CallBlocking(ctx, FeatureKeyAC, acSystemPrompt, userPrompt, CallOptions{})
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
	raw, _, err := pool.CallBlocking(ctx, FeatureKeyPRDescription, prDescriptionSystemPrompt, userPrompt, CallOptions{})
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
	raw, _, err := pool.CallBlocking(ctx, FeatureKeyCommitMessage, commitMessageSystemPrompt, diff, CallOptions{})
	if err != nil {
		return "", fmt.Errorf("SuggestCommitMessage: %w", err)
	}
	return raw, nil
}
