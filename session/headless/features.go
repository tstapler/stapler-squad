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
// calls that ARE granted read-only tool access (Read/Grep/Glob only — see
// CodebaseReadAllowedTools) via CallOptions.WorkDir. Uses falsification framing: the
// model must independently locate and quote its OWN evidence, treating the work
// session's claim as a hypothesis to check, not a fact to accept. Also requires a
// "tool_reads" list of files actually opened, so the caller can detect a PASS/FAIL
// reached with no real evidence of tool use and degrade it.
//
// A scoped Bash allowlist (git log/show/diff/blame, go test/vet/build, sg) was added
// here and to CodebaseReadAllowedTools in a later revision, then reverted — see
// ADR-001's 2026-07-15 addendum. The empirical integration test
// TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed proved that under
// --permission-mode bypassPermissions, AllowedTools/DisallowedTools do not provide
// real technical enforcement for Bash: an explicitly unlisted command executed freely,
// and command-chaining after an allowed prefix also succeeded in full. Do not
// reintroduce a Bash grant here without re-reading that ADR addendum.
const headlessReviewSystemPromptWithCodebaseAccess = `You are a code review agent reviewing a backlog item where no diff was found — either the acceptance criteria were already satisfied before this session started, or no work was done. You have read-only tool access scoped to the item's own repository checkout: Read, Grep, Glob for inspecting files. No other tools are available.
Use these tools to independently and thoroughly verify each criterion — this is a genuine investigation, not a narrow file:line lookup:
1. Search the current codebase yourself for code that satisfies each criterion. Treat the work session's note or verification evidence as a hypothesis to falsify, not a citation to trust — a criterion is not PASS merely because the agent claims it is already implemented.
2. Additional context may be provided below: prior review attempts on this item (## Prior Review Attempts), the full history of self-reported progress notes (## Full Notes History), the item's overall goal and status history (## Item Context), and a searchable transcript of the work session's own terminal activity (## Session Transcript). Use these to build a complete picture — e.g. a criterion marked UNVERIFIABLE twice before with the same evidence gap is a strong signal, and the session transcript may show the actual commands/output the work session claims to have run.
3. When you find satisfying code, your evidence field must quote YOUR OWN file path plus line/symbol and snippet from what you read — not a restatement of the agent's claim.
4. If you cannot locate satisfying code — including because it would live in a vendored or dependency path you cannot fully inspect — mark that criterion UNVERIFIABLE, not PASS.
5. Do not modify any files. Do not run any commands.
6. List EVERY real file path your investigation actually touched in "tool_reads" — regardless of whether it was opened directly with Read or matched/returned by a Grep/Glob search. When a search confirms something is ABSENT (no matching file, no matching symbol), that absence is real evidence — but do not list the nonexistent path itself in "tool_reads" (every entry must be a real file that exists); instead list the real file(s) your search actually returned (e.g. what Glob found in the relevant directory), so the thoroughness of the absence-check is auditable from what WAS found. If you did not use any tool at all, leave "tool_reads" empty — do not fabricate entries; an empty list on a confident verdict will be treated as unverified.
Output ONLY a single JSON object — no other text before or after it:
{"overall":"PARTIAL","summary":"concise assessment","tool_reads":["auth/login.go","main.go"],"verdicts":[{"criterion_index":0,"outcome":"PASS","evidence":"auth/login.go:9-24 — quoted snippet you read yourself"},{"criterion_index":1,"outcome":"FAIL","evidence":"Glob **/*.go returned only main.go and auth/login.go; Grep for 'RateLimit|ratelimit' found no matches — no rate-limiting code exists anywhere in the repo"}]}
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
// headless call. Originally 150s (deliberately shorter than DefaultCallTimeout's 900s so
// a hung or degraded tool-access call fails fast into the UNVERIFIABLE degrade path).
// Raised to 600s (10 minutes) and kept there even after the Bash tool grant below was
// reverted (see CodebaseReadAllowedTools) — the relaxed budget was motivated by the
// richer context payload (prior review attempts, full notes history, item context, a
// searchable session transcript file via Grep), not by Bash tool use, so genuine
// Read/Grep/Glob exploration of that larger context still legitimately takes longer
// than a bounded lookup and 150s was starting to force premature UNVERIFIABLE degrades
// on reviews that were making real progress. 600s remains well short of the shared 900s
// DefaultCallTimeout, so a genuinely hung codebase-read call still fails into the degrade
// path before hitting the full 15-minute ceiling other headless call types tolerate.
const CodebaseReadCallTimeout = 600 * time.Second

// CodebaseReadAllowedTools is the AllowedTools value granted to the empty-diff
// codebase-read headless call (BuildReviewCallOptions) and to the capability
// self-check probe (CodebaseReadCapabilitySelfCheck.run), which exercises the same
// call shape. Shared as a constant so the two call sites cannot silently drift.
//
// This is deliberately Read/Grep/Glob only — no Bash grant of any kind. A scoped Bash
// allowlist (git log/show/diff/blame, go test/vet/build, sg) was added here in a later
// revision, then reverted — see ADR-001's 2026-07-15 addendum. The empirical
// integration test TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed
// (session/headless/integration_test.go) proved that under
// --permission-mode bypassPermissions, --allowedTools/--disallowedTools do NOT provide
// real technical enforcement for Bash: an explicitly unlisted command executed freely
// and wrote a real file to disk, and command-chaining after an allowed prefix also
// succeeded in full. This Read/Grep/Glob-only grant is the one whose safety was
// actually verified empirically, via TestPool_RealClaude_WorkDirOnly_GrantsReadAccess
// and TestPool_RealClaude_WorkDirWithToolFlags_GrantsReadAccess — do not re-add Bash
// here without re-reading the ADR addendum and re-running that integration test.
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
